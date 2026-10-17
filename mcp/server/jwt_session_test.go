package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionEmailNeverSelectsAPrincipal(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{})
	// A valid token whose email is the operator's but whose sub is in no
	// principal's subjects.
	tok := e.token("some-registrant", func(c map[string]any) { c["email"] = "operator@example.test" })
	for _, route := range []string{RouteAGUI, RouteSettings, RouteIndexSearch} {
		code, body := e.call(t, http.MethodGet, route, tok, "")
		assert.Equal(t, http.StatusForbidden, code, route)
		assert.NotContains(t, body, "ok ", "no principal resolved on %s", route)
	}
}

func TestSessionOneSubjectTwoCapabilities(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{})
	tok := e.token(sessionSub, nil)
	code, body := e.call(t, http.MethodGet, RouteAGUI, tok, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok console-ui", body, "/agui runs as the ui principal")
	code, body = e.call(t, http.MethodGet, RouteSettings, tok, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok console-admin", body, "the settings route evaluates the admin principal")
	code, body = e.call(t, http.MethodGet, RouteIndexSearch, tok, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok console-ui", body)
}

func TestSessionSubjectOfAnotherKindIsRefusedOnTheRoute(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{})
	tok := e.token(otherSub, nil) // a human-mcp subject only
	for _, route := range []string{RouteAGUI, RouteSettings} {
		code, _ := e.call(t, http.MethodGet, route, tok, "")
		assert.Equal(t, http.StatusForbidden, code, "%s does not accept a human-mcp principal", route)
	}
	code, body := e.call(t, http.MethodGet, RouteIndexSearch, tok, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok laptop", body)
}

func TestSessionRouteWithNoSubjectKindIs403AndUnlistedRouteIs401(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{oidc: func(s *OIDCSettings) { s.Routes = append(s.Routes, OIDCRouteA2A) }})
	tok := e.token(sessionSub, nil)
	// a2a is listed but no a2a principal takes a subject.
	code, _ := e.call(t, http.MethodGet, RouteA2A, tok, "")
	assert.Equal(t, http.StatusForbidden, code)
	// /mcp is not listed by default: the route check comes first.
	hits := e.iss.hits.Load()
	code, _ = e.call(t, http.MethodPost, RouteMCP, e.token(sessionSub, nil), "")
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, hits, e.iss.hits.Load(), "an unlisted route triggers no key fetch")
}

func TestParseConfigRejectsDuplicateSubjectsOfOneKind(t *testing.T) {
	const sub = `"0b1c2d3e-4f50-4a61-8b72-93a4b5c6d7e8"`
	cfg := func(principals string) []byte { return []byte(`{"principals":[` + principals + `]}`) }

	// Two admin principals with one subject.
	_, err := ParseConfig(cfg(`{"name":"a1","kind":"admin","subjects":[` + sub + `]},{"name":"a2","kind":"admin","subjects":[` + sub + `]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subjects")
	assert.Contains(t, err.Error(), "a1")
	assert.Contains(t, err.Error(), "a2")

	// One subject in a ui and an admin principal is the intended shape.
	c, err := ParseConfig(cfg(`{"name":"u","kind":"ui","subjects":[` + sub + `]},{"name":"a","kind":"admin","subjects":[` + sub + `]}`))
	require.NoError(t, err)
	_, err = NewJWTVerifier(JWTOptions{OIDC: newTestIssuerSettings(), Principals: c.SessionPrincipals()})
	require.NoError(t, err)

	// A verifier built by hand refuses the same duplicate.
	_, err = NewJWTVerifier(JWTOptions{OIDC: newTestIssuerSettings(), Principals: []SessionPrincipal{
		{Identity: NewIdentity(Principal{ID: "x1", Kind: KindUI}, nil, nil, nil), Subjects: []string{"s"}},
		{Identity: NewIdentity(Principal{ID: "x2", Kind: KindUI}, nil, nil, nil), Subjects: []string{"s"}},
	}})
	require.Error(t, err)
}

func TestParseConfigPrincipalNeedsCredentialOrSubjects(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"neither", `{"name":"x","kind":"ui"}`, "credential"},
		{"empty subjects", `{"name":"x","kind":"ui","subjects":[]}`, "credential"},
		{"subjects on a2a", `{"name":"x","kind":"a2a","credential":{"env":"X_TOKEN"},"subjects":["s"]}`, "subjects"},
		{"bad max_auth_age", `{"name":"x","kind":"admin","subjects":["s"],"max_auth_age":"-1h"}`, "max_auth_age"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(`{"principals":[` + tc.body + `]}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
	// Subjects alone are enough, and a session-only principal has no static
	// credential to resolve.
	c, err := ParseConfig([]byte(`{"principals":[{"name":"x","kind":"ui","subjects":["s"]}]}`))
	require.NoError(t, err)
	creds, warnings := c.Credentials()
	assert.Empty(t, creds)
	assert.Empty(t, warnings)
}

func newTestIssuerSettings() OIDCSettings {
	return OIDCSettings{Issuer: testIssuerURL, Audiences: []string{testAudience}, JWKSURL: "http://127.0.0.1:1/jwks", Algorithms: newTestAlgorithms(), JWKSCacheTTL: 1}
}
