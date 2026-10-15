//go:build !noagent

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// AGUISettings configure the AG-UI endpoint (POST /agui on the --http
// listener). The endpoint is off unless Server.EnableAGUI is called; once
// enabled it is mounted with or without an agent backend, and with none it
// answers every run request 503 backend_not_configured.
type AGUISettings struct {
	// OriginAllowlist is agent.origin_allowlist: the page origins that may call
	// /agui from a browser. Empty (the default) rejects every request that
	// carries an Origin header.
	OriginAllowlist []string
	// A2UI is agent.a2ui: emit server-built A2UI surface messages as CUSTOM
	// events. Nil means the default, on; false emits none, and the run stream
	// is otherwise unchanged (the client's ToolCallCard fallback renders).
	A2UI *bool
}

// aguiConfig is AGUISettings after validation.
type aguiConfig struct {
	origins []string
	a2ui    bool
}

// EnableAGUI turns the AG-UI endpoint on. It does not need the agent loop:
// with no LLM backend the mounted route answers 503 backend_not_configured
// until one is configured, so a chat UI can show its empty state instead of a
// dead link. Call it before NewHTTPServer; the endpoint is mounted by
// NewHTTPServer and is therefore never served over stdio.
func (s *Server) EnableAGUI(set AGUISettings) error {
	origins, err := normalizeOrigins(set.OriginAllowlist)
	if err != nil {
		return fmt.Errorf("agent.origin_allowlist: %w", err)
	}
	s.aguiExt = &aguiConfig{origins: origins, a2ui: set.A2UI == nil || *set.A2UI}
	return nil
}

func init() { RegisterRouteProvider(mountAGUI) }

// aguiCORS is the contract's CORS answer for /agui (agent.origin_allowlist).
var aguiCORS = CORSPolicy{
	Methods: []string{http.MethodPost, http.MethodOptions},
	Headers: []string{"authorization", "content-type"},
	// Retry-After tells the browser chat when to retry a 503.
	Expose: []string{"Retry-After"},
}

// mountAGUI mounts /agui when EnableAGUI was called, with or without an agent
// backend: with none the handler answers 503 backend_not_configured. A server
// without EnableAGUI mounts nothing, so the path is a plain 404.
func mountAGUI(s *Server, m RouteMounter) error {
	cfg, _ := s.aguiExt.(*aguiConfig)
	if cfg == nil {
		return nil
	}
	s.gate.addResolver(aguiResolver{})
	// The handler keeps no agent: each request resolves the current one, so a
	// backend swap needs no remount. With agent.a2ui off no emitter is built
	// and the stream carries no A2UI CUSTOM events.
	var a2ui *a2uiEmitter
	if cfg.a2ui {
		var err error
		a2ui, err = newA2UIEmitter(s.cat)
		if err != nil {
			return err
		}
	}
	h := &aguiHandler{srv: s, log: s.log, watches: map[string]*aguiWatch{}, a2ui: a2ui}
	cors := aguiCORS
	// Path-scoped authentication: the route's default principal kind is ui.
	if err := m.Mount(RouteAGUI, h, MountOptions{AllowedOrigins: cfg.origins, CORS: &cors}); err != nil {
		return err
	}
	if a2ui != nil {
		// Catalog discovery mounts with the same protections as /agui and
		// shares the agent.a2ui off switch: with a2ui off the path is a plain
		// 404 and the run stream carries no advertisement.
		catalogCORS := aguiCatalogCORS
		if err := m.Mount(RouteAGUICatalog, aguiCatalogHandler{}, MountOptions{AllowedOrigins: cfg.origins, CORS: &catalogCORS}); err != nil {
			return err
		}
	}
	return nil
}

// aguiResolver is the chat's place in the confirmation gate. It asks nobody: the
// chat island answers by resuming the interrupt, and the endpoint then records
// the decision with ResolveConfirmation. The gate asks it, and lists it, only
// for the chat principal's calls made by the agent loop, so an MCP session of a
// ui principal does not wait for a chat that is not there.
type aguiResolver struct{}

func (aguiResolver) Name() string { return ResolverAGUIInterrupt }

func (aguiResolver) agentOnly() {}

func (aguiResolver) Request(context.Context, Pending, ResolveFunc) error { return nil }

// aguiHandler serves POST /agui.
type aguiHandler struct {
	srv *Server
	log *slog.Logger
	// a2ui builds the A2UI surface messages each stream emits per tool result.
	a2ui *a2uiEmitter

	mu      sync.Mutex
	watches map[string]*aguiWatch
}

// aguiWatch is an open {wait: true} observation of one thread.
type aguiWatch struct {
	interruptID string
	cancel      context.CancelFunc
}

// ServeHTTP implements http.Handler. The route's protection (Host, Origin,
// bearer, principal kind, body limit) already ran.
func (h *aguiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != RouteAGUI {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeAGUIError(w, http.StatusMethodNotAllowed, CodeInvalidArgument, "POST a RunAgentInput", false)
		return
	}
	info := auth.TokenInfoFromContext(r.Context())
	var id Identity
	if info != nil {
		id, _ = info.Extra[identityExtraKey].(Identity)
	}
	if id.ID == "" || id.Kind != KindUI {
		writeAGUIError(w, http.StatusForbidden, CodePermissionDenied, "this endpoint serves the chat principal only", false)
		return
	}
	if ub := h.srv.UARBackend(); ub != nil {
		// The backend is a UAR agent: kubo proxies the run and translates its
		// interrupts (ADR-0010).
		ub.serve(h, w, r, id)
		return
	}
	if h.srv.Agent() == nil {
		// Mounted without a backend: the chat's empty state. Nothing is dialed.
		writeAGUIError(w, http.StatusServiceUnavailable, CodeBackendNotConfigured, "no model backend is configured", false)
		return
	}
	in, err := readAGUIInput(r.Body)
	if err != nil {
		h.reject(w, err)
		return
	}
	st := newAGUIStream(w, in, h.a2ui, id.EgressAck)
	if in.Resume != nil {
		h.resume(r.Context(), st, id, in)
		return
	}
	h.message(r.Context(), st, id, in)
}

// message starts a run from the thread's last user message.
func (h *aguiHandler) message(ctx context.Context, st *aguiStream, id Identity, in *aguiInput) {
	text, err := in.lastUserText()
	if err != nil {
		h.reject(st.w, err)
		return
	}
	_, err = h.srv.Agent().Run(ctx, id, RunInput{SessionID: in.ThreadID, Message: text}, st.handle)
	h.finish(st, err)
}

// finish reports how a Run call ended: before the stream began it is an HTTP
// status, after it a RUN_ERROR.
func (h *aguiHandler) finish(st *aguiStream, err error) {
	if err == nil {
		return
	}
	if st.started {
		st.runError(err)
		return
	}
	h.reject(st.w, err)
}

// reject writes the HTTP error for a failure before the stream began.
func (h *aguiHandler) reject(w http.ResponseWriter, err error) {
	status, code, retry := aguiHTTPStatus(err)
	msg := aguiPublicMessage(err, status)
	if status == http.StatusInternalServerError {
		// The type only: an error text could carry conversation content.
		h.log.Warn("agui request failed before the stream began", "error_type", fmt.Sprintf("%T", err))
	}
	if retry {
		after := 5
		var rl *RateLimitedError
		if errors.As(err, &rl) {
			after = rl.RetryAfterSeconds()
		}
		w.Header().Set("Retry-After", strconv.Itoa(after))
	}
	var te *ToolError
	if errors.As(err, &te) {
		writeAGUIToolError(w, status, te)
		return
	}
	if errors.Is(err, ErrBackendChanged) {
		// The shared failed_precondition code, with the stable reason a client
		// matches on (L5): the interrupt's backend moved or was re-keyed.
		writeAGUIToolError(w, status, &ToolError{Code: code, Message: msg, Details: map[string]any{"reason": ReasonBackendChanged}})
		return
	}
	writeAGUIError(w, status, code, msg, retry)
}
