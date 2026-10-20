package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Routes on the HTTP listener. Only /mcp is served by this package; the AG-UI
// and A2A handlers mount themselves on the other paths through RouteMounter.
const (
	RouteMCP       = "/mcp"
	RouteDecisions = "/decisions"
	RouteAGUI      = "/agui"
	// RouteAGUICatalog is the A2UI catalog discovery route, named by the
	// contract (derivations/a2ui-components.json serve.route). It mounts with
	// /agui and only when agent.a2ui is on.
	RouteAGUICatalog = "/agui/catalog"
	RouteA2A         = "/a2a"
	RouteAgentCard   = "/.well-known/agent-card.json"
)

// Defaults of the listener, matching the contract's config schema.
const (
	DefaultMaxBodyBytes   int64 = 4 << 20
	DefaultSessionTimeout       = 30 * time.Minute

	readHeaderTimeout = 10 * time.Second
	// DefaultBodyReadTimeout is how long a request body may take to arrive.
	DefaultBodyReadTimeout = 30 * time.Second
	idleTimeout            = 2 * time.Minute
	shutdownTimeout        = 5 * time.Second
)

// MountOptions says how a mounted route is protected.
type MountOptions struct {
	// Public routes skip bearer authentication. Only public metadata may be
	// served on one: the A2A agent card and RFC 9728 metadata. Host, Origin and
	// body-size checks still apply.
	Public bool
	// Kinds are the principal kinds accepted on the route; a valid token of
	// another kind gets 403. Empty selects the route's default (see
	// defaultKinds), which keeps a token from one protocol out of another.
	Kinds []string
	// AllowedOrigins are browser origins accepted on this route in addition to
	// the listener-wide list, for example the Web UI chat on /agui.
	AllowedOrigins []string
	// CORS, when set, makes the route's browser origin policy strict: only this
	// mount's AllowedOrigins are accepted (neither the listener-wide list nor
	// the listener's own origin counts), and a listed origin gets exactly the
	// headers of the policy, with Vary: Origin and no credentials header.
	CORS *CORSPolicy
	// NoStore also covers refusals from the surrounding protection chain.
	NoStore bool
}

// CORSPolicy is the exact CORS answer of a strict route.
type CORSPolicy struct {
	// Methods and Headers are sent as Access-Control-Allow-Methods and
	// Access-Control-Allow-Headers.
	Methods []string
	Headers []string
	// Expose is sent as Access-Control-Expose-Headers when not empty.
	Expose []string
}

// RouteMounter is how later changes (AG-UI, A2A) add routes to the listener.
// Every mounted route passes through the same Host check, Origin policy, body
// limit and AuthVerifier as /mcp, so none can bypass them.
type RouteMounter interface {
	Mount(path string, h http.Handler, o MountOptions) error
}

// RouteProvider mounts the routes of one later change (AG-UI, A2A, the agent
// card) on a new HTTP server. It runs once per NewHTTPServer; a provider that is
// switched off mounts nothing, so its paths answer 404.
type RouteProvider func(s *Server, m RouteMounter) error

var routeProviders struct {
	mu   sync.Mutex
	list []RouteProvider
}

// RegisterRouteProvider adds a provider that every later NewHTTPServer calls.
func RegisterRouteProvider(p RouteProvider) {
	routeProviders.mu.Lock()
	defer routeProviders.mu.Unlock()
	routeProviders.list = append(routeProviders.list, p)
}

// HTTPOptions configures NewHTTPServer. The zero value serves loopback on
// DefaultHTTPListen without authentication, read-only.
type HTTPOptions struct {
	// Listen is host:port or a multiaddr. Empty selects DefaultHTTPListen.
	Listen string
	// Verifier authenticates bearer tokens. Nil is allowed only on a loopback
	// listener, where every request runs as a read-only anonymous principal.
	Verifier AuthVerifier
	// AllowedOrigins are the browser origins accepted on every route. Empty
	// rejects every cross-origin request.
	AllowedOrigins []string
	// MaxBodyBytes bounds a request body. Zero selects DefaultMaxBodyBytes.
	MaxBodyBytes int64
	// SessionTimeout ends idle MCP sessions. Zero selects
	// DefaultSessionTimeout.
	SessionTimeout time.Duration
	// BodyReadTimeout bounds how long one request body may take to arrive, so a
	// slow upload cannot hold a connection forever. It is a read deadline only,
	// cleared once the body is read: responses, including long-lived event
	// streams, have no write deadline. Zero selects DefaultBodyReadTimeout.
	BodyReadTimeout time.Duration
	// Logger receives diagnostics; nil discards them.
	Logger *slog.Logger
}

// HTTPServer serves the MCP server, and the routes mounted on it, over HTTP.
type HTTPServer struct {
	mcp     *Server
	opts    HTTPOptions
	listen  Listen
	verify  AuthVerifier
	origins []string
	log     *slog.Logger
	mux     *http.ServeMux
	bearer  func(http.Handler) http.Handler

	mu         sync.Mutex
	routes     map[string]struct{}
	ln         net.Listener
	serveHooks []func(context.Context) func()
}

// NewHTTPServer prepares the listener and mounts /mcp. It does not bind. A
// non-loopback address without a verifier is refused.
func NewHTTPServer(s *Server, opts HTTPOptions) (*HTTPServer, error) {
	listen, err := ParseListen(opts.Listen)
	if err != nil {
		return nil, err
	}
	if !listen.IsLoopback() && opts.Verifier == nil {
		return nil, fmt.Errorf("%w: %s is not a loopback address; configure a bearer token (--auth-token-env, --auth-token-file or a principal in --config) or listen on 127.0.0.1", errNonLoopbackWithoutAuth, listen)
	}
	if opts.MaxBodyBytes == 0 {
		opts.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if opts.MaxBodyBytes < 0 {
		return nil, errors.New("max body bytes must be positive")
	}
	if opts.BodyReadTimeout == 0 {
		opts.BodyReadTimeout = DefaultBodyReadTimeout
	}
	if opts.SessionTimeout == 0 {
		opts.SessionTimeout = DefaultSessionTimeout
	}
	if opts.SessionTimeout < 0 {
		return nil, errors.New("session timeout must be positive")
	}
	log := opts.Logger
	if log == nil {
		log = s.log
	}
	h := &HTTPServer{
		mcp:    s,
		opts:   opts,
		listen: listen,
		verify: opts.Verifier,
		log:    log,
		mux:    http.NewServeMux(),
		routes: make(map[string]struct{}),
	}
	if h.verify == nil {
		h.verify = anonymousVerifier{}
	}
	if h.origins, err = normalizeOrigins(opts.AllowedOrigins); err != nil {
		return nil, err
	}
	var skew time.Duration
	if c, ok := h.verify.(*ChainVerifier); ok {
		c.adoptLogger(log)
		c.warnOpenRoutes()
		skew = c.ClockSkew()
	}
	h.bearer = auth.RequireBearerToken(TokenVerifier(h.verify), &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: h.resourceMetadataURL(),
		Scopes:              []string{ScopeRead},
		ClockSkew:           skew,
	})

	if md := h.verify.ProtectedResourceMetadata(); md != nil {
		if err := h.Mount(ResourceMetadataPath, auth.ProtectedResourceMetadataHandler(md), MountOptions{Public: true}); err != nil {
			return nil, err
		}
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcp }, &mcp.StreamableHTTPOptions{
		Logger:              log,
		SessionTimeout:      opts.SessionTimeout,
		MaxRequestBodyBytes: opts.MaxBodyBytes,
	})
	if err := h.Mount(RouteMCP, mcpHandler, MountOptions{}); err != nil {
		return nil, err
	}
	// The session report exists only while auth.oidc is on; otherwise the path
	// is a plain 404.
	if c, ok := h.verify.(*ChainVerifier); ok && c.jwt != nil {
		cors := CORSPolicy{Methods: []string{http.MethodGet, http.MethodOptions}, Headers: []string{"authorization"}}
		err := h.Mount(RouteAuthSession, sessionHandler{log: log}, MountOptions{
			Kinds:          []string{KindUI, KindHumanMCP, KindAdmin},
			AllowedOrigins: c.jwt.origins,
			CORS:           &cors,
		})
		if err != nil {
			return nil, err
		}
	}
	routeProviders.mu.Lock()
	providers := slices.Clone(routeProviders.list)
	routeProviders.mu.Unlock()
	for _, p := range providers {
		if err := p(s, h); err != nil {
			return nil, fmt.Errorf("mount routes: %w", err)
		}
	}
	return h, nil
}

// normalizeOrigins lowercases and validates configured origins.
func normalizeOrigins(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, o := range in {
		o = strings.ToLower(strings.TrimSpace(o))
		u, err := url.Parse(o)
		if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" || u.RawQuery != "" || o == "null" {
			return nil, fmt.Errorf("allowed origin %q: want scheme://host[:port] with no path", o)
		}
		out = append(out, o)
	}
	return out, nil
}

// resourceMetadataURL is the absolute URL of the RFC 9728 document, or empty
// when the verifier supplies none.
func (h *HTTPServer) resourceMetadataURL() string {
	md := h.verify.ProtectedResourceMetadata()
	if md == nil {
		return ""
	}
	base, err := url.Parse(md.Resource)
	if err != nil || base.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: ResourceMetadataPath}).String()
}

// defaultKinds scopes principal kinds to routes. A token for the A2A peer
// principal does not open the MCP route, nor the other way around. The admin
// kind is deliberately absent from every default: its only route (the settings
// route) mounts with an explicit Kinds list (ADR-0009 section 3).
func defaultKinds(route string) []string {
	switch route {
	case RouteMCP:
		return []string{KindHumanMCP, KindUI, KindUAR}
	case RouteAGUI, RouteAGUICatalog:
		return []string{KindUI}
	case RouteA2A:
		return []string{KindA2A}
	}
	return []string{KindHumanMCP, KindUI, KindA2A, KindUAR}
}

// Mount adds a route behind the listener's protections. It implements
// RouteMounter.
func (h *HTTPServer) Mount(route string, handler http.Handler, o MountOptions) error {
	if handler == nil {
		return errors.New("mount: nil handler")
	}
	if !strings.HasPrefix(route, "/") || path.Clean(route) != route || strings.ContainsAny(route, " \t{}") {
		return fmt.Errorf("mount: %q is not a clean absolute path", route)
	}
	extra, err := normalizeOrigins(o.AllowedOrigins)
	if err != nil {
		return fmt.Errorf("mount %s: %w", route, err)
	}
	kinds := o.Kinds
	if len(kinds) == 0 {
		kinds = defaultKinds(route)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, dup := h.routes[route]; dup {
		return fmt.Errorf("mount: %s is already mounted", route)
	}
	h.routes[route] = struct{}{}
	protected := h.protect(handler, o.Public, kinds, extra, o.CORS)
	if o.NoStore {
		next := protected
		protected = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
	h.mux.Handle(route, protected)
	// Subtree routes (for example /a2a/stream) share the protection.
	h.mux.Handle(route+"/", protected)
	return nil
}

// protect wraps a handler in the shared chain: Host, Origin, body limit, then
// authentication and the route's principal-kind scope.
func (h *HTTPServer) protect(next http.Handler, public bool, kinds, extraOrigins []string, cors *CORSPolicy) http.Handler {
	origins := append(slices.Clone(h.origins), extraOrigins...)
	if cors != nil {
		origins = slices.Clone(extraOrigins)
	}
	guarded := next
	if !public {
		guarded = h.authenticate(next, kinds)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.checkHost(r) {
			h.log.Warn("rejected request", "reason", "host")
			http.Error(w, fmt.Sprintf("Forbidden: invalid Host header %q", r.Host), http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && !h.originOK(r, origin, origins, cors != nil) {
			h.log.Warn("rejected request", "reason", "origin")
			http.Error(w, fmt.Sprintf("Forbidden: origin %q is not allowed", origin), http.StatusForbidden)
			return
		}
		listed := origin != "" && slices.Contains(origins, strings.ToLower(origin))
		if !fetchSiteAllowed(r, listed) {
			h.log.Warn("rejected request", "reason", "cross-site")
			http.Error(w, "Forbidden: cross-site request", http.StatusForbidden)
			return
		}
		if listed {
			if cors != nil {
				setStrictCORS(w, origin, cors)
			} else {
				setCORS(w, origin)
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, h.opts.MaxBodyBytes)
			r.Body = h.withBodyDeadline(w, r.Body)
		}
		guarded.ServeHTTP(w, r)
	})
}

// checkHost rejects a Host header that does not name this listener when the
// listener is loopback-only: the DNS rebinding defence. A non-loopback
// listener is protected by its bearer token instead.
func (h *HTTPServer) checkHost(r *http.Request) bool {
	if !h.listen.IsLoopback() {
		return true
	}
	return isLoopbackHost(hostOnly(r.Host))
}

// fetchSiteAllowed applies browser fetch metadata. A browser marks requests
// that come from another origin or site; those pass only from an allowlisted
// origin. Requests without the header (curl, MCP clients) are not browsers and
// are covered by the Origin check and the bearer token.
func fetchSiteAllowed(r *http.Request, originListed bool) bool {
	switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
	case "", "same-origin", "none":
		return true
	}
	return originListed
}

// originAllowed accepts a browser origin that is allowlisted or is the
// listener's own origin. "null" and anything else is refused.
func (h *HTTPServer) originAllowed(r *http.Request, origin string, allowed []string) bool {
	origin = strings.ToLower(origin)
	if slices.Contains(allowed, origin) {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// originOK applies the listener's origin policy, or with strict only the
// route's own list.
func (h *HTTPServer) originOK(r *http.Request, origin string, allowed []string, strict bool) bool {
	if strict {
		return slices.Contains(allowed, strings.ToLower(origin))
	}
	return h.originAllowed(r, origin, allowed)
}

// setStrictCORS answers a listed origin with exactly the policy: the origin
// echoed (never *), the methods and headers named, and Vary: Origin. It sends
// the policy's exposed headers when it lists any, and no
// Access-Control-Allow-Credentials and no max-age.
func setStrictCORS(w http.ResponseWriter, origin string, c *CORSPolicy) {
	hd := w.Header()
	hd.Set("Access-Control-Allow-Origin", origin)
	hd.Add("Vary", "Origin")
	hd.Set("Access-Control-Allow-Methods", strings.Join(c.Methods, ", "))
	hd.Set("Access-Control-Allow-Headers", strings.Join(c.Headers, ", "))
	if len(c.Expose) > 0 {
		hd.Set("Access-Control-Expose-Headers", strings.Join(c.Expose, ", "))
	}
}

func setCORS(w http.ResponseWriter, origin string) {
	hd := w.Header()
	hd.Set("Access-Control-Allow-Origin", origin)
	hd.Add("Vary", "Origin")
	hd.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	hd.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Mcp-Session-Id, MCP-Protocol-Version, Last-Event-ID")
	hd.Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
	hd.Set("Access-Control-Max-Age", "600")
}

// authenticate requires a valid bearer token and a principal kind the route
// accepts. A loopback listener without a verifier runs every request as the
// anonymous read-only principal.
func (h *HTTPServer) authenticate(next http.Handler, kinds []string) http.Handler {
	kindGate := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := auth.TokenInfoFromContext(r.Context())
		if d := denialOf(info); d != nil {
			h.writeDenial(w, d)
			return
		}
		id, _ := info.Extra[identityExtraKey].(Identity)
		if !slices.Contains(kinds, id.Kind) {
			http.Error(w, fmt.Sprintf("Forbidden: principal %s (%s) is not accepted on %s", id.ID, id.Kind, r.URL.Path), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
	guarded := h.bearer(kindGate)
	if _, anonymous := h.verify.(anonymousVerifier); !anonymous {
		return guarded
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Anonymous access is for a client on this machine. A request that a
		// reverse proxy forwarded came from somewhere else, so it needs a
		// credential like any other remote caller.
		if isForwarded(r) {
			h.log.Warn("rejected request", "reason", "anonymous-forwarded")
			http.Error(w, "Forbidden: anonymous access does not accept proxied requests; configure a bearer token", http.StatusForbidden)
			return
		}
		// No verifier: the request carries no credential, so supply the
		// placeholder the anonymous verifier accepts.
		r.Header.Set("Authorization", "Bearer anonymous")
		guarded.ServeHTTP(w, r)
	})
}

// writeDenial answers a request whose token is valid but may not proceed. A
// step-up refusal uses the settings error shape of the contract; the rest are
// plain text. Neither says anything about the token.
func (h *HTTPServer) writeDenial(w http.ResponseWriter, d *Denial) {
	h.log.Warn("rejected request", "reason", d.Code)
	if d.Code == DenialStepUpRequired {
		writeSettingsJSON(w, d.Status, "", settingsErrorBody{Error: settingsErrorDetail{Code: d.Code, Message: d.Message}})
		return
	}
	http.Error(w, "Forbidden: "+d.Message, d.Status)
}

// Handler returns the listener's HTTP handler, for tests and for callers that
// run their own http.Server.
func (h *HTTPServer) Handler() http.Handler { return h.mux }

// onServe installs a lifecycle starter before Serve, without starting work on
// construction or opening another listener. Its stop function must join work.
func (h *HTTPServer) onServe(start func(context.Context) func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.serveHooks = append(h.serveHooks, start)
}

// Listen binds the listener. Call it before Serve to learn the address when
// the port was left to the OS.
func (h *HTTPServer) Listen() (net.Addr, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ln != nil {
		return h.ln.Addr(), nil
	}
	ln, err := net.Listen("tcp", h.listen.BindAddr())
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", h.listen, err)
	}
	h.ln = ln
	h.mcp.uar.setKuboListen(ln.Addr().String())
	return ln.Addr(), nil
}

// Addr returns the bound address, or nil before Listen.
func (h *HTTPServer) Addr() net.Addr {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ln == nil {
		return nil
	}
	return h.ln.Addr()
}

// Serve serves until ctx ends, then shuts down gracefully. It binds first if
// Listen was not called.
func (h *HTTPServer) Serve(ctx context.Context) error {
	if _, err := h.Listen(); err != nil {
		return err
	}
	// Cancel and join the goroutines the endpoints started once the listener
	// stopped, so none outlives Serve.
	defer h.mcp.runShutdownHooks()
	defer h.warmKeys(ctx)()
	h.mu.Lock()
	starters := slices.Clone(h.serveHooks)
	h.mu.Unlock()
	for _, start := range starters {
		stop := start(ctx)
		defer stop()
	}
	srv := &http.Server{
		Handler:           h.mux,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(h.ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close()
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// warmKeys starts the one prefetch of the session key set, when auth.oidc is
// on, and returns the function that waits for it. A failure only logs: JWT
// requests get 401 until a key set loads, and static bearers are unaffected.
func (h *HTTPServer) warmKeys(ctx context.Context) func() {
	c, ok := h.verify.(*ChainVerifier)
	if !ok || c.jwt == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := c.Warm(ctx); err != nil && ctx.Err() == nil {
			h.log.Warn("auth.oidc: the key set could not be fetched; session tokens get 401 until it loads", "jwks_url", c.JWKSURL(), "error", err.Error())
		}
	}()
	return func() { <-done }
}

// identitied is implemented by verifiers that can list the identities they
// authenticate.
type identitied interface{ Identities() []Identity }

// StartupReport returns the lines to print before serving: grants that policy
// withdraws and the UAR preflight outcome of each kind uar principal. It
// contacts only the UAR addresses the operator configured.
func (h *HTTPServer) StartupReport(ctx context.Context) []string {
	v, ok := h.verify.(identitied)
	if !ok {
		return nil
	}
	return h.mcp.StartupReport(ctx, v.Identities())
}

// forwardingHeaders are the headers a reverse proxy adds to say where a
// request really came from.
var forwardingHeaders = []string{"Forwarded", "X-Forwarded-For", "X-Real-IP"}

// isForwarded reports whether the request carries a forwarding header.
func isForwarded(r *http.Request) bool {
	return slices.ContainsFunc(forwardingHeaders, func(name string) bool {
		return len(r.Header.Values(name)) > 0
	})
}

// deadlineBody bounds the total time a request body may take to arrive. The read
// deadline is set only while a Read is in progress and lifted right after a
// successful one, so a
// response that follows (an event stream) never runs under it.
type deadlineBody struct {
	io.ReadCloser
	rc  *http.ResponseController
	end time.Time
}

func (b deadlineBody) Read(p []byte) (int, error) {
	if err := b.rc.SetReadDeadline(b.end); err != nil {
		return b.ReadCloser.Read(p) // the connection does not support deadlines
	}
	n, err := b.ReadCloser.Read(p)
	if err == nil || err == io.EOF {
		_ = b.rc.SetReadDeadline(time.Time{})
	}
	// After an error the deadline stays: when the body timed out, the server
	// must not wait for the rest while it discards the unread body, or the
	// error response would never be sent.
	return n, err
}

// withBodyDeadline bounds the time a request body may take to arrive. A request
// without a body (an SSE GET) gets no deadline.
func (h *HTTPServer) withBodyDeadline(w http.ResponseWriter, body io.ReadCloser) io.ReadCloser {
	if body == http.NoBody {
		return body
	}
	return deadlineBody{ReadCloser: body, rc: http.NewResponseController(w), end: time.Now().Add(h.opts.BodyReadTimeout)}
}
