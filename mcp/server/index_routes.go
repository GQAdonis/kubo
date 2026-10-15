package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

// This file is built with and without the noagent tag, like index.go. It is the
// HTTP projection of the index tools (ADR-0011 V5, contract routes/index.json):
// the Web UI search box is not an MCP session, so it posts to /index/search and
// reads /index/status. Both routes are the tools' own functions behind the
// listener's protections; they add no second contract.

// Routes of the search surface on the --http listener.
const (
	RouteIndexSearch = "/index/search"
	RouteIndexStatus = "/index/status"
)

// indexRoutesConfig is the state EnableIndexRoutes sets: the routes mount only
// when it exists.
type indexRoutesConfig struct {
	origins []string
}

// EnableIndexRoutes turns the search routes on. They also need an embeddings
// section (EnableEmbeddings) and the --http listener, which mounts them in
// NewHTTPServer; stdio never serves them. origins is agent.origin_allowlist:
// the page origins that may call the routes from a browser. Call it only when
// embeddings.search_route.enabled is not false.
func (s *Server) EnableIndexRoutes(origins []string) error {
	norm, err := normalizeOrigins(origins)
	if err != nil {
		return fmt.Errorf("agent.origin_allowlist: %w", err)
	}
	s.indexRoutesExt = &indexRoutesConfig{origins: norm}
	return nil
}

func init() { RegisterRouteProvider(mountIndexRoutes) }

// indexRoutesCORS is the contract's CORS answer for the routes.
var indexRoutesCORS = CORSPolicy{
	Methods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
	Headers: []string{"authorization", "content-type"},
	Expose:  []string{"Retry-After"},
}

// mountIndexRoutes mounts the routes when EnableIndexRoutes was called and an
// embeddings section exists. With either missing it mounts nothing: both paths
// are a plain 404 and nothing is read or dialed.
func mountIndexRoutes(s *Server, m RouteMounter) error {
	cfg := s.indexRoutesExt
	if cfg == nil || s.embeddingsExt == nil {
		return nil
	}
	h := &indexRoutesHandler{srv: s, limit: newSearchLimiter(time.Now)}
	cors := indexRoutesCORS
	opts := MountOptions{Kinds: []string{KindUI, KindHumanMCP}, AllowedOrigins: cfg.origins, CORS: &cors}
	for _, route := range []string{RouteIndexSearch, RouteIndexStatus} {
		if err := m.Mount(route, h, opts); err != nil {
			return err
		}
	}
	return nil
}

type indexRoutesHandler struct {
	srv   *Server
	limit *searchLimiter
}

// ServeHTTP implements http.Handler. The route's protection (Host, Origin,
// bearer, principal kind, body limit) already ran.
func (h *indexRoutesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case RouteIndexSearch:
		h.search(w, r)
	case RouteIndexStatus:
		h.status(w, r)
	default:
		http.NotFound(w, r)
	}
}

// caller resolves the identity of the request and the context the tools run in.
func (h *indexRoutesHandler) caller(r *http.Request) (context.Context, Identity, bool) {
	var id Identity
	if info := auth.TokenInfoFromContext(r.Context()); info != nil {
		id, _ = info.Extra[identityExtraKey].(Identity)
	}
	if id.ID == "" {
		return nil, id, false
	}
	ctx := WithPrincipal(WithIdentity(r.Context(), id), id.Principal)
	return ctx, id, true
}

// tool returns the registered tool a route runs, or writes 404: a tool the
// selection leaves out has no route either.
func (h *indexRoutesHandler) tool(w http.ResponseWriter, r *http.Request, name string) (registeredTool, bool) {
	reg, ok := h.srv.registry[name]
	if !ok {
		http.NotFound(w, r)
	}
	return reg, ok
}

// authorize applies the tool's own checks to the caller: its group grant and
// its egress rule. It writes the refusal.
func (h *indexRoutesHandler) authorize(ctx context.Context, w http.ResponseWriter, id Identity, t contract.Tool) bool {
	if !id.Allows(t) {
		writeRouteError(w, &ToolError{Code: CodePermissionDenied, Message: "principal " + id.ID + " may not call " + t.Name + ": it has no grant for the " + t.Group + " group"})
		return false
	}
	if te := h.srv.egressRefusal(ctx, nil, t); te != nil {
		te.Tool = t.Name
		writeRouteError(w, te)
		return false
	}
	return true
}

// search serves POST /index/search: the body is the ipfs_index_search input and
// the answer is its output.
func (h *indexRoutesHandler) search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost, "POST the ipfs_index_search input as JSON")
		return
	}
	reg, ok := h.tool(w, r, "ipfs_index_search")
	if !ok {
		return
	}
	ctx, id, ok := h.caller(r)
	if !ok {
		writeRouteError(w, &ToolError{Code: CodePermissionDenied, Message: "this route serves ui and human-mcp principals only"})
		return
	}
	if !h.authorize(ctx, w, id, reg.tool) {
		return
	}
	// Every search embeds the query and reads the store, and the route answers a
	// browser: one identity may not turn it into a way to spend the embedder.
	release, retry := h.limit.acquire(id.ID)
	if release == nil {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
		writeRouteStatus(w, http.StatusTooManyRequests, &ToolError{
			Code: CodeResourceExhausted, Message: "too many searches from this identity; retry after the delay in Retry-After", Retryable: true,
		})
		return
	}
	defer release()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeRouteError(w, invalidArgument("the request body could not be read: "+err.Error()))
		return
	}
	args, err := reg.sc.validateArgs(body)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	out, err := h.srv.indexSearch(ctx, args)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	writeRouteJSON(w, out)
}

// status serves GET /index/status: the ipfs_index_status output, plus the bulk
// run summary as bulk_run when the caller may read it and a run exists.
func (h *indexRoutesHandler) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet, "GET the status")
		return
	}
	reg, ok := h.tool(w, r, "ipfs_index_status")
	if !ok {
		return
	}
	ctx, id, ok := h.caller(r)
	if !ok {
		writeRouteError(w, &ToolError{Code: CodePermissionDenied, Message: "this route serves ui and human-mcp principals only"})
		return
	}
	if !h.authorize(ctx, w, id, reg.tool) {
		return
	}
	raw, _ := json.Marshal(map[string]any{})
	if c := r.URL.Query().Get("cid"); c != "" {
		raw, _ = json.Marshal(map[string]any{"cid": c})
	}
	args, err := reg.sc.validateArgs(raw)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	out, err := h.srv.indexStatus(ctx, args)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	doc, err := toMap(out)
	if err != nil {
		writeRouteError(w, err)
		return
	}
	if run := h.bulkRun(ctx, id); run != nil {
		doc["bulk_run"] = run
	}
	writeRouteJSON(w, doc)
}

// bulkRun is the bulk run summary for a caller allowed to read it, or nil.
func (h *indexRoutesHandler) bulkRun(ctx context.Context, id Identity) *bulkRunView {
	reg, ok := h.srv.registry["ipfs_index_bulk_status"]
	if !ok || !id.Allows(reg.tool) {
		return nil
	}
	out, err := h.srv.indexBulkStatus(ctx, nil)
	if err != nil {
		h.srv.log.Debug("index status: bulk run unavailable", "error", clipMessage(err.Error()))
		return nil
	}
	st, _ := out.(bulkStatusOutput)
	return st.Run
}

func toMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// routeStatus maps a tool error to the HTTP status of the route descriptor.
func routeStatus(te *ToolError) int {
	switch te.Code {
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodePermissionDenied, CodeEgressNotAcknowledged:
		return http.StatusForbidden
	case CodeIndexUnavailable:
		return http.StatusServiceUnavailable
	case CodeResourceExhausted:
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}

// methodNotAllowed answers a request whose method the route does not serve.
func methodNotAllowed(w http.ResponseWriter, allow, msg string) {
	w.Header().Set("Allow", allow)
	writeRouteStatus(w, http.StatusMethodNotAllowed, &ToolError{Code: CodeInvalidArgument, Message: msg})
}

// writeRouteError writes the ToolError shape. An unexpected failure is
// reduced to a code and a clipped message.
func writeRouteError(w http.ResponseWriter, err error) {
	var te *ToolError
	if !errors.As(err, &te) {
		te = toToolError("", err)
	}
	writeRouteStatus(w, routeStatus(te), te)
}

func writeRouteStatus(w http.ResponseWriter, status int, te *ToolError) {
	clean := *te
	clean.Confirmation = nil
	if clean.Code == CodeIndexUnavailable {
		w.Header().Set("Retry-After", strconv.Itoa(5))
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(toolErrorResult{Error: &clean})
}

func writeRouteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
