//go:build !noagent

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// AGUISettings configure the AG-UI endpoint (POST /agui on the --http
// listener). The endpoint is off unless Server.EnableAGUI is called, and then
// only when an agent backend is configured; without it the path answers 404.
type AGUISettings struct {
	// OriginAllowlist is agent.origin_allowlist: the page origins that may call
	// /agui from a browser. Empty (the default) rejects every request that
	// carries an Origin header.
	OriginAllowlist []string
}

// aguiConfig is AGUISettings after validation.
type aguiConfig struct {
	origins []string
}

// EnableAGUI turns the AG-UI endpoint on. It needs the agent loop (an LLM
// backend): without one it fails rather than mount an endpoint that could not
// answer. Call it after EnableAgent and before NewHTTPServer; the endpoint is
// mounted by NewHTTPServer and is therefore never served over stdio.
func (s *Server) EnableAGUI(set AGUISettings) error {
	if s.Agent() == nil {
		return errors.New("the AG-UI endpoint needs an LLM backend (--llm-base-url and --llm-model)")
	}
	origins, err := normalizeOrigins(set.OriginAllowlist)
	if err != nil {
		return fmt.Errorf("agent.origin_allowlist: %w", err)
	}
	s.aguiExt = &aguiConfig{origins: origins}
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

// mountAGUI mounts /agui when EnableAGUI was called. A server without it
// mounts nothing, so the path is a plain 404.
func mountAGUI(s *Server, m RouteMounter) error {
	cfg, _ := s.aguiExt.(*aguiConfig)
	agent := s.Agent()
	if cfg == nil || agent == nil {
		return nil
	}
	s.gate.addResolver(aguiResolver{})
	h := &aguiHandler{srv: s, agent: agent, log: s.log, watches: map[string]*aguiWatch{}}
	cors := aguiCORS
	// Path-scoped authentication: the route's default principal kind is ui.
	return m.Mount(RouteAGUI, h, MountOptions{AllowedOrigins: cfg.origins, CORS: &cors})
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
	srv   *Server
	agent *Agent
	log   *slog.Logger

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
	in, err := readAGUIInput(r.Body)
	if err != nil {
		h.reject(w, err)
		return
	}
	st := newAGUIStream(w, in)
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
	_, err = h.agent.Run(ctx, id, RunInput{SessionID: in.ThreadID, Message: text}, st.handle)
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
		w.Header().Set("Retry-After", "5")
	}
	var te *ToolError
	if errors.As(err, &te) {
		writeAGUIToolError(w, status, te)
		return
	}
	writeAGUIError(w, status, code, msg, retry)
}
