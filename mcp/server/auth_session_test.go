package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionSchema resolves the contract's response schema.
func sessionSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), "routes/session-response.schema.json")
	require.NoError(t, err)
	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(raw, &s))
	r, err := s.Resolve(nil)
	require.NoError(t, err)
	return r
}

func TestAuthSessionReport(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{})
	schema := sessionSchema(t)

	validate := func(t *testing.T, body string) map[string]any {
		t.Helper()
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &doc))
		require.NoError(t, schema.Validate(doc), body)
		return doc
	}

	t.Run("ui and admin subject", func(t *testing.T) {
		tok := e.token(sessionSub, nil)
		code, body := e.call(t, http.MethodGet, RouteAuthSession, tok, "")
		require.Equal(t, http.StatusOK, code)
		doc := validate(t, body)
		assert.Equal(t, "ok", doc["admin"])
		assert.Equal(t, "operator@example.test", doc["email"])
		assert.ElementsMatch(t, []any{
			map[string]any{"name": "console-ui", "kind": "ui"},
			map[string]any{"name": "console-admin", "kind": "admin"},
		}, doc["principals"])
		assert.NotContains(t, body, sessionSub, "the sub is never returned")
		assert.NotContains(t, body, signatureSegment(tok), "the token is never returned")
		assert.NotContains(t, body, "sub")
	})
	t.Run("ui only", func(t *testing.T) {
		code, body := e.call(t, http.MethodGet, RouteAuthSession, e.token(viewerSub, nil), "")
		require.Equal(t, http.StatusOK, code)
		doc := validate(t, body)
		assert.Equal(t, "not_eligible", doc["admin"])
		assert.Equal(t, []any{map[string]any{"name": "viewer", "kind": "ui"}}, doc["principals"])
	})
	t.Run("admin needs a second factor", func(t *testing.T) {
		tok := e.token(sessionSub, func(c map[string]any) { c["amr"] = []string{"password"} })
		code, body := e.call(t, http.MethodGet, RouteAuthSession, tok, "")
		require.Equal(t, http.StatusOK, code, "the report itself never fails on step-up")
		assert.Equal(t, "step_up_required", validate(t, body)["admin"])
	})
	t.Run("no email claim", func(t *testing.T) {
		tok := e.token(viewerSub, func(c map[string]any) { delete(c, "email") })
		_, body := e.call(t, http.MethodGet, RouteAuthSession, tok, "")
		assert.NotContains(t, validate(t, body), "email")
	})
	t.Run("exp is the token exp", func(t *testing.T) {
		_, body := e.call(t, http.MethodGet, RouteAuthSession, e.token(viewerSub, nil), "")
		doc := validate(t, body)
		assert.Greater(t, doc["exp"], float64(0))
	})
}

func TestAuthSessionRefusals(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{})
	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"static bearer only", tokChat, http.StatusUnauthorized},
		{"invalid token", e.token(viewerSub, nil)[:40] + "x", http.StatusUnauthorized},
		{"valid token, unmapped subject", e.token("registrant", nil), http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := e.call(t, http.MethodGet, RouteAuthSession, tc.token, "")
			assert.Equal(t, tc.want, code)
			assert.NotContains(t, body, "principals")
		})
	}
	code, _ := e.call(t, http.MethodPost, RouteAuthSession, e.token(viewerSub, nil), sessionOrigin)
	assert.Equal(t, http.StatusMethodNotAllowed, code)
}

func TestAuthSessionRouteOffIs404(t *testing.T) {
	// With auth.oidc off the listener has a static verifier only.
	e := newHTTPEnv(t, nil)
	for _, tok := range []string{"", tokChat, jwsShapedToken} {
		resp := e.do(t, rawReq{method: http.MethodGet, path: RouteAuthSession, token: tok})
		// 404 before authentication would be ideal, but every mounted route is
		// behind the bearer; an unmounted path is the mux's 404.
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, strings.TrimSpace(tok))
	}
}

func TestAuthSessionRouteHonoursRoutesList(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{oidc: func(s *OIDCSettings) { s.Routes = []string{OIDCRouteAGUI} }})
	code, _ := e.call(t, http.MethodGet, RouteAuthSession, e.token(viewerSub, nil), "")
	assert.Equal(t, http.StatusUnauthorized, code, "session is off auth.oidc.routes")
}
