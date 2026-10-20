//go:build !nomcp

package kubo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	cmds "github.com/ipfs/go-ipfs-cmds"
	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const jwsShaped = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImsxIn0.eyJzdWIiOiJ4In0.c2lnbmF0dXJl"

func oidcConfig(jwksURL string, enabled bool) string {
	return fmt.Sprintf(`{"http":{"enabled":true,"listen":"127.0.0.1:5251"},
	"auth":{"oidc":{"enabled":%t,"issuer":"https://oauth.example.test","audiences":["ipfs-console"],"jwks_url":%q}},
	"principals":[
		{"name":"reader","kind":"human-mcp","credential":{"env":"OIDC_STATIC_TOKEN"}},
		{"name":"console-ui","kind":"ui","subjects":["sub-1"]}
	]}`, enabled, jwksURL)
}

func verifyAt(t *testing.T, v mcpserver.AuthVerifier, token, path string) error {
	t.Helper()
	_, _, err := v.Verify(t.Context(), token, &http.Request{Method: http.MethodGet, URL: &url.URL{Path: path}, Header: http.Header{}})
	return err
}

// With auth.oidc off nothing is built and nothing is fetched, whatever the
// section says.
func TestHTTPStartupOIDCOffFetchesNothing(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	t.Setenv("OIDC_STATIC_TOKEN", s5TokenA)
	var hits atomic.Int64
	jwks := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(jwks.Close)
	file := writeMCPConfig(t, oidcConfig(jwks.URL, false))

	s, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	_, isStatic := s.http.opts.Verifier.(*mcpserver.StaticVerifier)
	assert.True(t, isStatic, "no chain, no JWT verifier")
	assert.Empty(t, s.http.notes)

	require.NoError(t, verifyAt(t, s.http.opts.Verifier, s5TokenA, "/mcp"))
	require.ErrorIs(t, verifyAt(t, s.http.opts.Verifier, jwsShaped, "/agui"), auth.ErrInvalidToken)
	assert.Zero(t, hits.Load(), "no request reached the JWKS server")
}

// An unreachable key set never stops startup; static bearers keep working and
// a session token is 401.
func TestHTTPStartupOIDCUnreachableKeysDoNotStopStartup(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	t.Setenv("OIDC_STATIC_TOKEN", s5TokenA)
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL + "/jwks.json"
	dead.Close()
	file := writeMCPConfig(t, oidcConfig(url, true))

	s, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	require.Len(t, s.http.notes, 1)
	assert.Contains(t, s.http.notes[0], "auth.oidc is on")
	assert.False(t, s.http.anonymous)

	require.NoError(t, verifyAt(t, s.http.opts.Verifier, s5TokenA, "/mcp"))
	require.ErrorIs(t, verifyAt(t, s.http.opts.Verifier, jwsShaped, "/agui"), auth.ErrInvalidToken)
}

// A config with only session principals is not an anonymous listener when
// auth.oidc is on, and is refused when it is off.
func TestHTTPStartupSessionOnlyPrincipals(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	body := func(enabled bool) string {
		return fmt.Sprintf(`{"http":{"enabled":true,"listen":"127.0.0.1:5251"},
		"auth":{"oidc":{"enabled":%t,"issuer":"https://oauth.example.test","audiences":["a"],"jwks_url":"http://127.0.0.1:1/jwks"}},
		"principals":[{"name":"console-ui","kind":"ui","subjects":["sub-1"]}]}`, enabled)
	}
	s, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: writeMCPConfig(t, body(true))}))
	require.NoError(t, err)
	assert.False(t, s.http.anonymous)
	assert.NotNil(t, s.http.opts.Verifier)

	_, err = loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: writeMCPConfig(t, body(false))}))
	require.Error(t, err, "subjects alone do not authenticate anyone while auth.oidc is off")
	assert.Contains(t, err.Error(), "auth.oidc.enabled", "the message names the key that turns the verifier on")
	assert.Contains(t, err.Error(), "console-ui", "and the session-only principal it concerns")
	assert.NotContains(t, err.Error(), "resolvable credential", "no empty credential list")
}
