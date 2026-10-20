//go:build !noagent

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

const (
	decisionItemBytes = 256 << 10
	decisionPageBytes = 1 << 20
)

type decisionReadsConfig struct {
	enabled bool
	origins []string
}

// ConfigureDecisionReads configures the default-on HTTP-only observer. Call
// before constructing the HTTP listener, independently of LLM/AGUI setup.
func (s *Server) ConfigureDecisionReads(cfg DecisionsConfig, origins []string) error {
	norm, err := normalizeOrigins(origins)
	if err != nil {
		return fmt.Errorf("agent.origin_allowlist: %w", err)
	}
	s.agentMu.Lock()
	s.decisionsExt = &decisionReadsConfig{enabled: cfg.Enabled == nil || *cfg.Enabled, origins: norm}
	s.agentMu.Unlock()
	if cfg.Enabled != nil && !*cfg.Enabled {
		s.gate.broker.setHistoryEnabled(false)
	}
	return nil
}

func init() { RegisterRouteProvider(mountDecisionReads) }

func (s *Server) decisionReadsConfig() *decisionReadsConfig {
	s.agentMu.RLock()
	defer s.agentMu.RUnlock()
	cfg, _ := s.decisionsExt.(*decisionReadsConfig)
	return cfg
}

func mountDecisionReads(s *Server, m RouteMounter) error {
	cfg := s.decisionReadsConfig()
	if cfg != nil && !cfg.enabled {
		return nil
	}
	var origins []string
	if cfg != nil {
		origins = cfg.origins
	}
	s.gate.broker.setHistoryEnabled(true)
	runtime := s.decisionRuntime()
	if listener, ok := m.(*HTTPServer); ok {
		listener.onServe(runtime.start)
	}
	cors := CORSPolicy{Methods: []string{http.MethodGet, http.MethodOptions}, Headers: []string{"authorization", "content-type"}, Expose: []string{"Retry-After"}}
	listener, _ := m.(*HTTPServer)
	return m.Mount(RouteDecisions, &decisionReadsHandler{srv: s, runtime: runtime, listener: listener}, MountOptions{
		Kinds: []string{KindUI, KindHumanMCP, KindA2A, KindUAR}, AllowedOrigins: origins, CORS: &cors, NoStore: true,
	})
}

type decisionAuthority struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

type decisionActions struct {
	Approve      bool                  `json:"approve"`
	Deny         bool                  `json:"deny"`
	Surface      string                `json:"authorized_surface,omitempty"`
	Continuation *decisionContinuation `json:"continuation,omitempty"`
}

type decisionContinuation struct {
	Method      string `json:"method"`
	Route       string `json:"route"`
	ThreadID    string `json:"thread_id"`
	InterruptID string `json:"interrupt_id"`
}

type decisionObservation struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"`
	SourceKind   string            `json:"source_kind"`
	State        string            `json:"approval_state"`
	Summary      Summary           `json:"summary"`
	Authority    decisionAuthority `json:"authority"`
	CreatedAt    time.Time         `json:"created_at"`
	ExpiresAt    *time.Time        `json:"expires_at,omitempty"`
	ResolvedAt   *time.Time        `json:"resolved_at,omitempty"`
	EffectDigest string            `json:"effect_digest,omitempty"`
	Destination  *Destination      `json:"destination,omitempty"`
	ResolvedBy   string            `json:"resolved_by,omitempty"`
	Surface      *ResolvingSurface `json:"resolved_surface,omitempty"`
	NativePrompt bool              `json:"requires_native_prompt"`
	Execution    *ExecutionRecord  `json:"execution,omitempty"`
	ActivityID   string            `json:"activity_id,omitempty"`
	Actions      decisionActions   `json:"actions"`
}

type decisionRetention struct {
	Scope         string `json:"scope"`
	MaxResolved   int    `json:"max_resolved"`
	MaxAgeSeconds int    `json:"max_age_seconds"`
	Resets        bool   `json:"resets_on_restart"`
}

type decisionReadMeta struct {
	Epoch     string            `json:"server_epoch"`
	Revision  string            `json:"revision"`
	StartedAt time.Time         `json:"history_started_at"`
	Retention decisionRetention `json:"retention"`
}

type decisionPage struct {
	HasMore bool   `json:"has_more"`
	Cursor  string `json:"next_cursor,omitempty"`
}

type decisionListResponse struct {
	decisionReadMeta
	Items []json.RawMessage `json:"items"`
	Page  decisionPage      `json:"page"`
}

type decisionItemResponse struct {
	decisionReadMeta
	Item json.RawMessage `json:"item"`
}

func decisionMeta(s decisionSnapshot) decisionReadMeta {
	return decisionReadMeta{Epoch: s.Epoch, Revision: s.Revision, StartedAt: s.HistoryStartedAt,
		Retention: decisionRetention{Scope: "server", MaxResolved: decisionHistoryLimit, MaxAgeSeconds: int(decisionHistoryTTL.Seconds()), Resets: true}}
}

var (
	decisionIDPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	decisionNodeIDPattern = regexp.MustCompile(`^cf_[A-Za-z0-9_-]{16,64}$`)
)

func projectDecision(p Pending) ([]byte, error) {
	return projectDecisionActions(p, defaultDecisionActions(p))
}

func defaultDecisionActions(p Pending) decisionActions {
	actions := decisionActions{}
	if p.State == StatePending || p.State == StateApproved {
		switch {
		case p.Source == DecisionSourceUAR:
			actions.Surface = "backend_host"
		case p.NativePrompt:
			actions.Surface = "native_system_prompt"
		case p.Mechanism == MechanismElicitation:
			actions.Surface = "originating_mcp_host"
		}
	}
	return actions
}

func projectDecisionActions(p Pending, actions decisionActions) ([]byte, error) {
	if !decisionNodeIDPattern.MatchString(p.ID) || p.CreatedAt.IsZero() || len(p.Summary.Tool) == 0 || utf8.RuneCountInString(p.Summary.Tool) > 128 ||
		!slices.Contains([]string{StatePending, StateApproved, StateDenied, StateExpired, StateExecuted}, p.State) ||
		!slices.Contains([]string{KindUI, KindHumanMCP, KindA2A, KindUAR}, p.Principal.Kind) ||
		!slices.Contains([]string{"write", "destructive"}, p.Summary.Class) {
		return nil, errors.New("unrepresentable decision")
	}
	if p.Source == DecisionSourceKubo && p.SourceKind != SourceKindNodeConfirmation ||
		p.Source == DecisionSourceUAR && (p.SourceKind != SourceKindBackendApproval || !strings.HasPrefix(p.ID, "cf_uar_") || !strings.HasPrefix(p.Summary.Tool, "uar:") || p.Execution != nil) ||
		!slices.Contains([]string{DecisionSourceKubo, DecisionSourceUAR}, p.Source) {
		return nil, errors.New("unrepresentable decision source")
	}
	if p.EffectDigest != "" && !validEffectDigest(p.EffectDigest) {
		return nil, errors.New("unrepresentable effect binding")
	}
	if utf8.RuneCountInString(p.ActivityID) > 128 {
		return nil, errors.New("unrepresentable activity correlation")
	}
	summary := p.Summary
	summary.Text = clipRunes(strings.ToValidUTF8(summary.Text, ""), maxSummaryRunes)
	if utf8.RuneCountInString(summary.Text) < summaryMinRunes {
		summary.Text = "Prepared operation"
	}
	count := len(summary.Targets)
	summary.Targets = slices.Clone(summary.Targets[:min(count, 16)])
	if summary.Targets == nil {
		summary.Targets = []Target{}
	}
	if count > 16 {
		note := fmt.Sprintf(" Showing 16 of %d targets.", count)
		summary.Text = clipRunes(summary.Text, maxSummaryRunes-utf8.RuneCountInString(note)) + note
	}
	for _, t := range summary.Targets {
		if utf8.RuneCountInString(t.Value) > maxTargetRunes || !slices.Contains([]string{"cid", "content_path", "mfs_path", "key", "ipns_name", "peer", "repo", "index", "content"}, t.Kind) {
			return nil, errors.New("unrepresentable target")
		}
	}
	if p.Destination != nil && (!slices.Contains([]string{"node", "pinning_service", "ipns_name"}, p.Destination.Kind) || utf8.RuneCountInString(p.Destination.Name) == 0 || utf8.RuneCountInString(p.Destination.Name) > 256) {
		return nil, errors.New("unrepresentable destination")
	}
	if !validDecisionOutcome(p) {
		return nil, errors.New("unrepresentable execution binding")
	}
	o := decisionObservation{ID: p.ID, Source: p.Source, SourceKind: p.SourceKind, State: p.State, Summary: summary,
		Authority: decisionAuthority{Kind: p.Principal.Kind}, CreatedAt: p.CreatedAt.UTC(), EffectDigest: p.EffectDigest,
		Destination: cloneDestination(p.Destination), ResolvedBy: p.ResolvedBy, Surface: cloneSurface(p.ResolvedSurface),
		NativePrompt: p.NativePrompt, Execution: cloneExecution(p.Execution), Actions: actions}
	o.ActivityID = p.ActivityID
	if utf8.RuneCountInString(p.Principal.ID) <= 128 {
		o.Authority.Name = p.Principal.ID
	}
	if !p.ExpiresAt.IsZero() {
		t := p.ExpiresAt.UTC()
		o.ExpiresAt = &t
	}
	if !p.ResolvedAt.IsZero() {
		t := p.ResolvedAt.UTC()
		o.ResolvedAt = &t
	}
	data, err := json.Marshal(o)
	if err != nil || len(data) > decisionItemBytes {
		return nil, errors.New("unrepresentable decision size")
	}
	return data, nil
}

type decisionReadsHandler struct {
	srv      *Server
	runtime  *decisionStreamRuntime
	listener *HTTPServer
}

func (h *decisionReadsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if cfg := h.srv.decisionReadsConfig(); cfg != nil && !cfg.enabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodOptions {
		w.Header().Set("Allow", "GET, OPTIONS")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	info := auth.TokenInfoFromContext(r.Context())
	if info == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	id, ok := info.Extra[identityExtraKey].(Identity)
	authenticated, _ := info.Extra[authenticatedExtraKey].(bool)
	if !authenticated || !ok || id.ID == "" || !slices.Contains([]string{KindUI, KindHumanMCP, KindA2A, KindUAR}, id.Kind) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Body != nil {
		var one [1]byte
		if n, err := io.ReadFull(r.Body, one[:]); n != 0 || !errors.Is(err, io.EOF) {
			decisionReadError(w, http.StatusBadRequest, "invalid_argument", "Decision reads do not accept a body.", nil)
			return
		}
	}
	if r.URL.Path == RouteDecisions+"/events" {
		if r.URL.RawQuery != "" {
			decisionReadError(w, 400, "invalid_argument", "Invalid decision query.", nil)
			return
		}
		h.events(w, r, id, info)
		return
	}
	owner := id.BindingKey()
	if r.URL.Path == RouteDecisions {
		h.list(w, r, id)
		return
	}
	key, found := strings.CutPrefix(r.URL.Path, RouteDecisions+"/")
	if !found || !decisionIDPattern.MatchString(key) || r.URL.RawPath != "" {
		decisionReadError(w, http.StatusBadRequest, "invalid_argument", "Invalid decision identifier.", nil)
		return
	}
	if r.URL.RawQuery != "" {
		decisionReadError(w, http.StatusBadRequest, "invalid_argument", "Decision detail does not accept query parameters.", nil)
		return
	}
	snap := h.srv.decisionSnapshotFor(owner)
	resumeID, canResume := h.resumeIdentity(r, id)
	for _, p := range snap.Items {
		if p.ID != key {
			continue
		}
		data, err := projectDecisionActions(p, h.srv.decisionActions(p, resumeID, canResume))
		if err != nil {
			decisionReadError(w, 500, "internal", "Decision detail is unavailable.", nil)
			return
		}
		writeDecisionJSON(w, http.StatusOK, decisionItemResponse{decisionReadMeta: decisionMeta(snap), Item: data})
		return
	}
	decisionReadError(w, http.StatusNotFound, "not_found", "Decision is unavailable.", nil)
}

func readDecisionQuery(raw string) (int, string, error) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return 0, "", err
	}
	limit := 50
	cursor := ""
	for key, values := range query {
		if len(values) != 1 {
			return 0, "", errors.New("repeated query")
		}
		switch key {
		case "limit":
			if values[0] == "" || strings.Trim(values[0], "0123456789") != "" {
				return 0, "", errors.New("invalid limit")
			}
			limit, err = strconv.Atoi(values[0])
			if err != nil || limit < 1 || limit > 100 {
				return 0, "", errors.New("invalid limit")
			}
		case "cursor":
			cursor = values[0]
			if len(cursor) == 0 || len(cursor) > 2048 || !opaqueCursorPattern.MatchString(cursor) {
				return 0, "", errors.New("invalid cursor")
			}
		default:
			return 0, "", errors.New("unknown query")
		}
	}
	return limit, cursor, nil
}

func (h *decisionReadsHandler) list(w http.ResponseWriter, r *http.Request, id Identity) {
	limit, cursor, err := readDecisionQuery(r.URL.RawQuery)
	if err != nil {
		decisionReadError(w, 400, "invalid_argument", "Invalid decision query.", nil)
		return
	}
	owner := id.BindingKey()
	snap := h.srv.decisionSnapshotFor(owner)
	resumeID, canResume := h.resumeIdentity(r, id)
	start := 0
	if cursor != "" {
		key, stale, err := h.srv.gate.broker.readDecisionCursor(cursor, owner, snap, limit)
		if err != nil {
			decisionReadError(w, 400, "invalid_argument", "Invalid decision cursor.", nil)
			return
		}
		if stale {
			decisionReadError(w, 409, "cursor_refresh_required", "Read a fresh decision snapshot.", &snap)
			return
		}
		start = slices.IndexFunc(snap.Items, func(p Pending) bool { return p.ID == key.ID && p.CreatedAt.Equal(key.CreatedAt) }) + 1
		if start == 0 {
			decisionReadError(w, 409, "cursor_refresh_required", "Read a fresh decision snapshot.", &snap)
			return
		}
	}
	body := decisionListResponse{decisionReadMeta: decisionMeta(snap), Items: []json.RawMessage{}}
	// Reserve enough room for metadata and a maximum-sized cursor. Every item
	// is encoded once and never partially truncated to satisfy a page bound.
	bytes := 4096
	next := start
	for next < len(snap.Items) && len(body.Items) < limit {
		p := snap.Items[next]
		data, err := projectDecisionActions(p, h.srv.decisionActions(p, resumeID, canResume))
		if err != nil {
			decisionReadError(w, 500, "internal", "Decision snapshot is unavailable.", nil)
			return
		}
		if bytes+len(data)+1 > decisionPageBytes {
			break
		}
		body.Items = append(body.Items, data)
		bytes += len(data) + 1
		next++
	}
	body.Page.HasMore = next < len(snap.Items)
	if body.Page.HasMore {
		if len(body.Items) == 0 {
			decisionReadError(w, 500, "internal", "Decision snapshot is unavailable.", nil)
			return
		}
		body.Page.Cursor = h.srv.gate.broker.makeDecisionCursor(owner, snap, snap.Items[next-1], limit)
	}
	writeDecisionJSON(w, 200, body)
}

func decisionReadError(w http.ResponseWriter, status int, code, message string, snap *decisionSnapshot) {
	e := map[string]any{"code": code, "message": message, "retryable": code == "cursor_refresh_required"}
	if snap != nil {
		e["refresh"] = map[string]string{"server_epoch": snap.Epoch, "revision": snap.Revision}
	}
	writeDecisionJSON(w, status, map[string]any{"error": e})
}

func writeDecisionJSON(w http.ResponseWriter, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil || len(data) > decisionPageBytes {
		status = http.StatusInternalServerError
		data = []byte(`{"error":{"code":"internal","message":"Decision response is unavailable.","retryable":false}}`)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data) // delivery failure cannot change operation evidence
}
