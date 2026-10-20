package server

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTRouteScope(t *testing.T) {
	t.Run("default routes exclude mcp", func(t *testing.T) {
		e := newSessionEnv(t, sessionEnvOptions{})
		hits := e.iss.hits.Load()
		resp := e.do(t, rawReq{path: RouteMCP, token: e.token(otherSub, nil), body: initializeBody})
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a mapped JWT on /mcp")
		assert.Equal(t, hits, e.iss.hits.Load())
	})
	t.Run("an operator may add mcp", func(t *testing.T) {
		e := newSessionEnv(t, sessionEnvOptions{oidc: func(s *OIDCSettings) {
			s.Routes = append(s.Routes, OIDCRouteMCP)
			s.RequireOrigin = false // the MCP client sends no Origin
		}})
		resp := e.do(t, rawReq{path: RouteMCP, token: e.token(otherSub, nil), body: initializeBody})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
	t.Run("narrowed list", func(t *testing.T) {
		e := newSessionEnv(t, sessionEnvOptions{oidc: func(s *OIDCSettings) { s.Routes = []string{OIDCRouteSession} }})
		code, _ := e.call(t, http.MethodGet, RouteAGUI, e.token(sessionSub, nil), "")
		assert.Equal(t, http.StatusUnauthorized, code)
	})
	t.Run("empty list accepts a JWT nowhere", func(t *testing.T) {
		e := newSessionEnv(t, sessionEnvOptions{oidc: func(s *OIDCSettings) { s.Routes = []string{} }})
		for _, route := range []string{RouteAGUI, RouteSettings, RouteIndexSearch, RouteAuthSession} {
			code, _ := e.call(t, http.MethodGet, route, e.token(sessionSub, nil), "")
			assert.Equal(t, http.StatusUnauthorized, code, route)
		}
	})
}

// With auth.oidc.routes absent only the session report accepts a JWT.
func TestJWTRouteScopeDefaultIsSessionOnly(t *testing.T) {
	cfg, err := ParseConfig(oidcConfigJSON(`{` + validOIDCBody + `}`))
	require.NoError(t, err)
	resolved, err := cfg.Auth.OIDC.resolve()
	require.NoError(t, err)
	require.Equal(t, []string{OIDCRouteSession}, resolved.Routes)

	e := newSessionEnv(t, sessionEnvOptions{oidc: func(s *OIDCSettings) { s.Routes = resolved.Routes }})
	for _, route := range []string{RouteAGUI, RouteSettings, RouteIndexSearch} {
		code, _ := e.call(t, http.MethodGet, route, e.token(sessionSub, nil), "")
		assert.Equal(t, http.StatusUnauthorized, code, route)
	}
	resp := e.do(t, rawReq{path: RouteMCP, token: e.token(otherSub, nil), body: initializeBody})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "/mcp")
	code, _ := e.call(t, http.MethodGet, RouteAuthSession, e.token(sessionSub, nil), "")
	assert.Equal(t, http.StatusOK, code, "/auth/session")
}

// A route beyond session with an empty agent.origin_allowlist logs one Warn
// line naming both keys; startup continues. A listed origin, or routes limited
// to session, logs nothing.
func TestJWTRoutesWarnWhenOriginAllowlistIsEmpty(t *testing.T) {
	iss := newTestIssuer(t)
	build := func(routes []string, origins []string) string {
		logs := &lockedBuffer{}
		log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		cfg := iss.settings()
		cfg.Routes = routes
		v, err := NewJWTVerifier(JWTOptions{OIDC: cfg, Principals: defaultSessionPrincipals(), OriginAllowlist: origins})
		require.NoError(t, err)
		newHTTPEnv(t, func(h *HTTPOptions) {
			h.Verifier = NewChainVerifier(testVerifier(t), v)
			h.Logger = log
		})
		return logs.String()
	}
	count := func(out string) int { return strings.Count(out, "auth.oidc.routes") }

	out := build([]string{OIDCRouteAGUI, OIDCRouteSession}, nil)
	assert.Equal(t, 1, count(out), out)
	assert.Contains(t, out, "agent.origin_allowlist")
	assert.Contains(t, out, "level=WARN")

	assert.Zero(t, count(build([]string{OIDCRouteAGUI, OIDCRouteSession}, []string{sessionOrigin})), "allowlist set")
	assert.Zero(t, count(build([]string{OIDCRouteSession}, nil)), "session only")
	assert.Zero(t, count(build([]string{}, nil)), "no route")
}

func TestJWTOriginRule(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{origins: []string{sessionOrigin}})
	tok := e.token(sessionSub, nil)
	for _, tc := range []struct {
		name, method, origin string
		want                 int
	}{
		{"POST with no Origin", http.MethodPost, "", http.StatusForbidden},
		{"PUT with no Origin", http.MethodPut, "", http.StatusForbidden},
		{"DELETE with no Origin", http.MethodDelete, "", http.StatusForbidden},
		{"POST from a listed origin", http.MethodPost, sessionOrigin, http.StatusOK},
		{"POST from a listed origin, upper case", http.MethodPost, "HTTPS://CONSOLE.EXAMPLE.TEST", http.StatusOK},
		{"POST from an unlisted origin", http.MethodPost, "https://evil.example.test", http.StatusForbidden},
		{"POST from the null origin", http.MethodPost, "null", http.StatusForbidden},
		{"GET with no Origin", http.MethodGet, "", http.StatusOK},
		{"HEAD with no Origin", http.MethodHead, "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := e.call(t, tc.method, RouteAGUI, tok, tc.origin)
			assert.Equal(t, tc.want, code)
		})
	}
}

func TestJWTOriginRuleOnlyListsAgentOriginAllowlist(t *testing.T) {
	// http.allowed_origins admits an origin to the listener; it does not make
	// the origin acceptable for a session write.
	e := newSessionEnv(t, sessionEnvOptions{origins: []string{sessionOrigin}})
	require.NotNil(t, e)
	code, _ := e.call(t, http.MethodPost, RouteAGUI, e.token(sessionSub, nil), "http://"+e.base[len("http://"):])
	assert.Equal(t, http.StatusForbidden, code, "the listener's own origin is not on agent.origin_allowlist")
}

func TestJWTOriginRuleOff(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{oidc: func(s *OIDCSettings) { s.RequireOrigin = false }})
	code, _ := e.call(t, http.MethodPost, RouteAGUI, e.token(sessionSub, nil), "")
	assert.Equal(t, http.StatusOK, code, "require_origin false follows the static bearer behaviour")
}
