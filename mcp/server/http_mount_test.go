package server

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoRoute stands in for the AG-UI and A2A handlers of later changes.
func echoRoute() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := auth.TokenInfoFromContext(r.Context())
		who := "public"
		if info != nil {
			who = info.UserID
			// A session identity's UserID is its binding key; the echo names
			// the principal.
			if id, ok := info.Extra[identityExtraKey].(Identity); ok {
				who = id.ID
			}
		}
		_, _ = w.Write([]byte("ok " + who))
	})
}

func TestMountedRouteInheritsProtections(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.AllowedOrigins = []string{"http://app.example"} })
	require.NoError(t, e.hs.Mount("/test", echoRoute(), MountOptions{}))

	cases := []struct {
		name string
		req  rawReq
		want int
	}{
		{"no token", rawReq{path: "/test"}, http.StatusUnauthorized},
		{"unknown token", rawReq{path: "/test", token: "nope-nope-nope-nope"}, http.StatusUnauthorized},
		{"valid token", rawReq{path: "/test", token: tokReader}, http.StatusOK},
		{"subtree", rawReq{path: "/test/deeper", token: tokReader}, http.StatusOK},
		{"subtree without token", rawReq{path: "/test/deeper"}, http.StatusUnauthorized},
		{"bad host", rawReq{path: "/test", token: tokReader, host: "evil.example"}, http.StatusForbidden},
		{"bad origin", rawReq{path: "/test", token: tokReader, header: map[string]string{"Origin": "https://evil.example"}}, http.StatusForbidden},
		{"allowed origin", rawReq{path: "/test", token: tokReader, header: map[string]string{"Origin": "http://app.example"}}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := e.do(t, tc.req)
			assert.Equal(t, tc.want, resp.StatusCode)
		})
	}
}

func TestMountedRouteBodyLimitApplies(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.MaxBodyBytes = 2048 })
	reader := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	require.NoError(t, e.hs.Mount("/sink", reader, MountOptions{}))
	small := e.do(t, rawReq{path: "/sink", token: tokReader, body: "small"})
	assert.Equal(t, http.StatusOK, small.StatusCode)
	big := e.do(t, rawReq{path: "/sink", token: tokReader, body: string(make([]byte, 16384))})
	assert.Equal(t, http.StatusRequestEntityTooLarge, big.StatusCode)
}

func TestMountedRouteIsScopedByPrincipalKind(t *testing.T) {
	e := newHTTPEnv(t, nil)
	require.NoError(t, e.hs.Mount(RouteA2A, echoRoute(), MountOptions{}))
	require.NoError(t, e.hs.Mount(RouteAGUI, echoRoute(), MountOptions{}))

	// A token is accepted only on the routes of its principal's kind (G9-4).
	matrix := []struct {
		route, token string
		want         int
	}{
		{RouteA2A, tokPeer, http.StatusOK},
		{RouteA2A, tokReader, http.StatusForbidden},
		{RouteA2A, tokChat, http.StatusForbidden},
		{RouteAGUI, tokChat, http.StatusOK},
		{RouteAGUI, tokPeer, http.StatusForbidden},
		{RouteAGUI, tokReader, http.StatusForbidden},
		{RouteMCP, tokPeer, http.StatusForbidden},
		{RouteMCP, tokReader, http.StatusOK},
		{RouteA2A, "", http.StatusUnauthorized},
		{RouteAGUI, "", http.StatusUnauthorized},
	}
	for _, m := range matrix {
		method, body := http.MethodGet, ""
		if m.route == RouteMCP {
			method, body = http.MethodPost, initializeBody
		}
		resp := e.do(t, rawReq{method: method, path: m.route, token: m.token, body: body})
		assert.Equal(t, m.want, resp.StatusCode, "%s with %q", m.route, m.token)
	}
}

func TestMountOptionsOverrideKindsAndOrigins(t *testing.T) {
	e := newHTTPEnv(t, nil)
	require.NoError(t, e.hs.Mount("/chat", echoRoute(), MountOptions{
		Kinds:          []string{KindUI},
		AllowedOrigins: []string{"webui://localhost"},
	}))
	ok := e.do(t, rawReq{path: "/chat", token: tokChat, header: map[string]string{"Origin": "webui://localhost"}})
	assert.Equal(t, http.StatusOK, ok.StatusCode)
	assert.Equal(t, "webui://localhost", ok.Header.Get("Access-Control-Allow-Origin"))
	other := e.do(t, rawReq{path: "/chat", token: tokChat, header: map[string]string{"Origin": "http://app.example"}})
	assert.Equal(t, http.StatusForbidden, other.StatusCode, "the route's origin is not listener-wide")
	assert.Equal(t, http.StatusForbidden, e.do(t, rawReq{path: "/chat", token: tokReader}).StatusCode)
}

func TestPublicRouteSkipsAuthButNotHostOrOrigin(t *testing.T) {
	e := newHTTPEnv(t, nil)
	require.NoError(t, e.hs.Mount(RouteAgentCard, echoRoute(), MountOptions{Public: true}))
	assert.Equal(t, http.StatusOK, e.do(t, rawReq{method: http.MethodGet, path: RouteAgentCard}).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do(t, rawReq{method: http.MethodGet, path: RouteAgentCard, host: "evil.example"}).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.do(t, rawReq{method: http.MethodGet, path: RouteAgentCard, header: map[string]string{"Origin": "https://evil.example"}}).StatusCode)
}

func TestMountRejectsBadRoutes(t *testing.T) {
	e := newHTTPEnv(t, nil)
	require.NoError(t, e.hs.Mount("/once", echoRoute(), MountOptions{}))
	for _, route := range []string{"/once", RouteMCP, "relative", "/a/../b", "/trailing/", "", "/sp ace"} {
		require.Error(t, e.hs.Mount(route, echoRoute(), MountOptions{}), route)
	}
	require.Error(t, e.hs.Mount("/nilhandler", nil, MountOptions{}))
	require.Error(t, e.hs.Mount("/badorigin", echoRoute(), MountOptions{AllowedOrigins: []string{"nope"}}))
}

func TestRouteProvidersMountOnNewServers(t *testing.T) {
	routeProviders.mu.Lock()
	saved := routeProviders.list
	routeProviders.list = nil
	routeProviders.mu.Unlock()
	t.Cleanup(func() {
		routeProviders.mu.Lock()
		routeProviders.list = saved
		routeProviders.mu.Unlock()
	})
	RegisterRouteProvider(func(_ *Server, m RouteMounter) error {
		return m.Mount("/provided", echoRoute(), MountOptions{})
	})
	e := newHTTPEnv(t, nil)
	assert.Equal(t, http.StatusUnauthorized, e.do(t, rawReq{path: "/provided"}).StatusCode, "mounted behind auth")
	assert.Equal(t, http.StatusOK, e.do(t, rawReq{path: "/provided", token: tokReader}).StatusCode)
}

// metadataVerifier is a stand-in for the Ory Hydra verifier: it accepts one
// token and supplies RFC 9728 metadata.
type metadataVerifier struct{ *StaticVerifier }

func (metadataVerifier) ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata {
	return &oauthex.ProtectedResourceMetadata{
		Resource:             "https://mcp.example.test/mcp",
		AuthorizationServers: []string{"https://hydra.example.test"},
		ScopesSupported:      []string{ScopeRead, ScopeWrite, ScopeAdmin},
	}
}

func TestProtectedResourceMetadataHook(t *testing.T) {
	static := testVerifier(t)
	t.Run("static verifier serves none", func(t *testing.T) {
		e := newHTTPEnv(t, nil)
		resp := e.do(t, rawReq{method: http.MethodGet, path: ResourceMetadataPath})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.NotContains(t, e.do(t, rawReq{body: initializeBody}).Header.Get("WWW-Authenticate"), "resource_metadata")
	})
	t.Run("verifier with metadata", func(t *testing.T) {
		e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = metadataVerifier{static} })
		resp := e.do(t, rawReq{method: http.MethodGet, path: ResourceMetadataPath})
		require.Equal(t, http.StatusOK, resp.StatusCode, "metadata is public")
		assert.Contains(t, resp.Header.Get("Content-Type"), "json")

		denied := e.do(t, rawReq{body: initializeBody})
		require.Equal(t, http.StatusUnauthorized, denied.StatusCode)
		challenge := denied.Header.Get("WWW-Authenticate")
		u, err := url.Parse("https://mcp.example.test" + ResourceMetadataPath)
		require.NoError(t, err)
		assert.Contains(t, challenge, `resource_metadata="`+u.String()+`"`)
		assert.Equal(t, http.StatusOK, e.do(t, rawReq{token: tokReader, body: initializeBody}).StatusCode)
	})
}

func TestSessionTimeoutIsConfigurable(t *testing.T) {
	d := newFakeDaemon(t)
	s := mustNew(t, d, Options{})
	hs, err := NewHTTPServer(s, HTTPOptions{Listen: "127.0.0.1:0", SessionTimeout: time.Minute})
	require.NoError(t, err)
	assert.Equal(t, time.Minute, hs.opts.SessionTimeout)
	hs, err = NewHTTPServer(mustNew(t, d, Options{}), HTTPOptions{Listen: "127.0.0.1:0"})
	require.NoError(t, err)
	assert.Equal(t, DefaultSessionTimeout, hs.opts.SessionTimeout)
	assert.Equal(t, DefaultMaxBodyBytes, hs.opts.MaxBodyBytes)
	_, err = NewHTTPServer(mustNew(t, d, Options{}), HTTPOptions{Listen: "127.0.0.1:0", SessionTimeout: -time.Second})
	require.Error(t, err)
}

func TestServeStopsWhenContextEnds(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{})
	hs, err := NewHTTPServer(s, HTTPOptions{Listen: "127.0.0.1:0"})
	require.NoError(t, err)
	_, err = hs.Listen()
	require.NoError(t, err)
	addr := hs.Addr()
	require.NotNil(t, addr)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- hs.Serve(ctx) }()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after the context ended")
	}
}
