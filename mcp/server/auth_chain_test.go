package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainKeepsStaticBearersExactlyAsBefore(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{})
	// A static ui bearer POSTs to /agui with no Origin: the JWT Origin rule does
	// not apply to it.
	code, body := e.call(t, http.MethodPost, RouteAGUI, tokChat, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok chat", body)
	// A static token of the wrong kind is refused as before.
	code, _ = e.call(t, http.MethodPost, RouteAGUI, tokReader, "")
	assert.Equal(t, http.StatusForbidden, code)
	// A static token never reaches the JWT verifier.
	assert.EqualValues(t, 1, e.iss.hits.Load(), "only the startup prefetch reached the issuer")
}

func TestChainOpaqueTokenNeverReachesTheJWTPath(t *testing.T) {
	e := newSessionEnv(t, sessionEnvOptions{})
	base := e.iss.hits.Load()
	for _, tok := range []string{"opaque-token-0123456789", "a.b", "a.b.c.d", "a..c"} {
		code, _ := e.call(t, http.MethodGet, RouteAGUI, tok, "")
		assert.Equal(t, http.StatusUnauthorized, code, tok)
	}
	assert.Equal(t, base, e.iss.hits.Load())
}

func TestChainJWKSDownDoesNotStopStartup(t *testing.T) {
	iss := newTestIssuer(t)
	iss.setDown(true)
	e := newSessionEnv(t, sessionEnvOptions{issuer: iss})
	// The listener is up (newHTTPEnv served it), static bearers work, and a
	// session token is 401 until keys load.
	code, _ := e.call(t, http.MethodGet, RouteAGUI, tokChat, "")
	assert.Equal(t, http.StatusOK, code)
	code, _ = e.call(t, http.MethodGet, RouteAGUI, e.token(sessionSub, nil), "")
	assert.Equal(t, http.StatusUnauthorized, code)

	err := e.jwt.Warm(t.Context())
	if err != nil {
		assert.Contains(t, err.Error(), "/jwks.json")
	}
}

func TestChainWarmErrorNamesURLNotToken(t *testing.T) {
	iss := newTestIssuer(t)
	iss.setDown(true)
	v := newTestJWTVerifier(t, iss, nil)
	err := v.Warm(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), iss.jwksURL())
}

func TestChainWithoutJWTIsStatic(t *testing.T) {
	c := NewChainVerifier(testVerifier(t), nil)
	_, _, err := c.Verify(t.Context(), tokReader, reqTo(RouteMCP))
	require.NoError(t, err)
	_, _, err = c.Verify(t.Context(), jwsShapedToken, reqTo(RouteAGUI))
	require.Error(t, err)
	assert.Zero(t, c.ClockSkew())
	require.NoError(t, c.Warm(t.Context()))
}

const jwsShapedToken = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImsxIn0.eyJzdWIiOiJ4In0.c2lnbmF0dXJl"
