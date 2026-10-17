package server

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	sessionOrigin = "https://console.example.test"
	sessionSub    = jwtTestSub
	otherSub      = "7f1e2d3c-0000-4000-8000-00000000beef"
)

// sessionEnv is an HTTP listener whose verifier accepts session tokens from a
// local issuer, with echo routes standing in for the real handlers.
type sessionEnv struct {
	*httpEnv
	iss *testIssuer
	jwt *JWTVerifier
}

// sessionEnvOptions adjusts the environment. Zero values are the defaults.
type sessionEnvOptions struct {
	oidc       func(*OIDCSettings)
	principals []SessionPrincipal
	// origins is the agent.origin_allowlist the verifier enforces.
	origins []string
}

func defaultSessionPrincipals() []SessionPrincipal {
	return []SessionPrincipal{
		{
			Identity: NewIdentity(Principal{ID: "console-ui", Kind: KindUI}, []string{AllGroups}, nil, nil),
			Subjects: []string{sessionSub},
		},
		{
			Identity: NewIdentity(Principal{ID: "console-admin", Kind: KindAdmin}, nil, nil, nil),
			Subjects: []string{sessionSub},
		},
		{
			Identity: NewIdentity(Principal{ID: "laptop", Kind: KindHumanMCP}, []string{AllGroups}, nil, nil),
			Subjects: []string{otherSub},
		},
	}
}

func newSessionEnv(t *testing.T, o sessionEnvOptions) *sessionEnv {
	t.Helper()
	iss := newTestIssuer(t)
	cfg := iss.settings()
	if o.oidc != nil {
		o.oidc(&cfg)
	}
	principals := o.principals
	if principals == nil {
		principals = defaultSessionPrincipals()
	}
	origins := o.origins
	if origins == nil {
		origins = []string{sessionOrigin}
	}
	v, err := NewJWTVerifier(JWTOptions{OIDC: cfg, Principals: principals, OriginAllowlist: origins})
	require.NoError(t, err)
	e := newHTTPEnv(t, func(h *HTTPOptions) { h.Verifier = v })
	for _, m := range []struct {
		route string
		opts  MountOptions
	}{
		{RouteAGUI, MountOptions{}},
		{RouteA2A, MountOptions{}},
		{RouteSettings, MountOptions{Kinds: []string{KindAdmin}}},
		{RouteIndexSearch, MountOptions{Kinds: []string{KindUI, KindHumanMCP}}},
	} {
		require.NoError(t, e.hs.Mount(m.route, echoRoute(), m.opts))
	}
	return &sessionEnv{httpEnv: e, iss: iss, jwt: v}
}

// token signs a valid token for sub, with now-relative times, after mod.
func (e *sessionEnv) token(sub string, mod func(map[string]any)) string {
	c := claimsAt(time.Now(), sub)
	if mod != nil {
		mod(c)
	}
	return e.iss.sign(testKidRSA, c)
}

// get and post send a request with a bearer token and an optional Origin.
func (e *sessionEnv) call(t *testing.T, method, path, token, origin string) (int, string) {
	t.Helper()
	r := rawReq{method: method, path: path, token: token}
	if origin != "" {
		r.header = map[string]string{"Origin": origin}
	}
	resp := e.do(t, r)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}
