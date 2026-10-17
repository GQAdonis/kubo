package server

import (
	"net/http"
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
		for _, route := range []string{RouteAGUI, RouteSettings, RouteIndexSearch} {
			code, _ := e.call(t, http.MethodGet, route, e.token(sessionSub, nil), "")
			assert.Equal(t, http.StatusUnauthorized, code, route)
		}
	})
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
