//go:build !noagent

package cli

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance test of console-session-auth 2.8: the built binary, an
// `ipfs mcp --http` instance on an OS-chosen loopback port, and a local OpenID
// issuer on another. A token the config maps to a ui principal runs /agui and is
// refused on /mcp.

const (
	sessionTestIssuer   = "https://oauth.session-test.invalid"
	sessionTestAudience = "ipfs-console"
	sessionTestOrigin   = "https://console.session-test.invalid"
	sessionTestSub      = "0b1c2d3e-4f50-4a61-8b72-93a4b5c6d7e8"
	sessionTestKid      = "cli-key-1"
)

// sessionIssuer serves a JWKS and signs ID tokens.
type sessionIssuer struct {
	t    *testing.T
	key  *rsa.PrivateKey
	srv  *httptest.Server
	hits atomic.Int64
}

func newSessionIssuer(t *testing.T) *sessionIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	i := &sessionIssuer{t: t, key: key}
	i.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: sessionTestKid, Use: "sig", Algorithm: "RS256"},
		}})
	}))
	t.Cleanup(i.srv.Close)
	return i
}

func (i *sessionIssuer) token(sub string, mod func(map[string]any)) string {
	i.t.Helper()
	now := time.Now()
	claims := map[string]any{
		"iss": sessionTestIssuer, "aud": []string{sessionTestAudience}, "sub": sub,
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(10 * time.Minute).Unix(),
		"auth_time": now.Add(-time.Minute).Unix(), "amr": []string{"password"},
		"email": "operator@session-test.invalid",
	}
	if mod != nil {
		mod(claims)
	}
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: i.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), sessionTestKid))
	require.NoError(i.t, err)
	tok, err := jwt.Signed(sig).Claims(claims).Serialize()
	require.NoError(i.t, err)
	return tok
}

// sessionTestConfig writes the ipfs mcp config. routes nil leaves
// auth.oidc.routes absent, which accepts a JWT on /auth/session only.
func sessionTestConfig(t *testing.T, iss *sessionIssuer, llmBase string, oidcOn bool, routes []string) string {
	t.Helper()
	cfg := map[string]any{
		"agent": map[string]any{"agui": true, "origin_allowlist": []string{sessionTestOrigin}},
		"llm":   map[string]any{"base_url": llmBase, "model": "m"},
		"auth": map[string]any{"oidc": map[string]any{
			"enabled": oidcOn, "issuer": sessionTestIssuer, "audiences": []string{sessionTestAudience},
			"jwks_url": iss.srv.URL + "/jwks.json",
		}},
		"principals": []any{
			map[string]any{"name": "chat", "kind": "ui", "credential": map[string]any{"env": "CLI_AGUI_CHAT_TOKEN"}, "egress_ack": []string{"local"}},
			map[string]any{"name": "console-ui", "kind": "ui", "subjects": []string{sessionTestSub}, "egress_ack": []string{"local"}},
		},
	}
	if routes != nil {
		cfg["auth"].(map[string]any)["oidc"].(map[string]any)["routes"] = routes
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "mcp-session.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func TestMCPSessionToken(t *testing.T) {
	t.Run("a mapped token runs /agui and is refused on /mcp", func(t *testing.T) {
		t.Parallel()
		iss := newSessionIssuer(t)
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("one"), cliSayStep("two"), cliSayStep("three"))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", sessionTestConfig(t, iss, model.baseURL(), true, []string{"agui", "session"}))
		assert.Contains(t, p.stderr.String(), "auth.oidc is on")
		assert.NotContains(t, p.stderr.String(), "operator@session-test.invalid")
		mapped := iss.token(sessionTestSub, nil)
		origin := map[string]string{"Origin": sessionTestOrigin}

		r := aguiPost(t, p, mapped, aguiInput("t-session", "r1", "hello"), origin)
		assert.Equal(t, http.StatusOK, r.Status, r.Body)
		assert.Contains(t, r.types(), "RUN_FINISHED")

		assert.Equal(t, http.StatusUnauthorized, p.status(mcpHTTPRequest{path: "/mcp", token: mapped, body: "{}"}),
			"auth.oidc.routes does not list mcp")

		noOrigin := aguiPost(t, p, mapped, aguiInput("t-session", "r2", "hello"), nil)
		assert.Equal(t, http.StatusForbidden, noOrigin.Status, "a session write needs a listed Origin")

		static := aguiPost(t, p, aguiChatToken, aguiInput("t-static", "r3", "hello"), nil)
		assert.Equal(t, http.StatusOK, static.Status, "a static bearer without Origin still works")

		other := aguiPost(t, p, iss.token("registrant", nil), aguiInput("t-other", "r4", "hello"), origin)
		assert.Equal(t, http.StatusForbidden, other.Status, "a valid token whose sub maps to no principal")
		expired := aguiPost(t, p, iss.token(sessionTestSub, func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }),
			aguiInput("t-exp", "r5", "hello"), origin)
		assert.Equal(t, http.StatusUnauthorized, expired.Status)

		// The report names the principal and nothing secret.
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, p.base+"/auth/session", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+mapped)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var rep map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&rep))
		assert.Equal(t, "not_eligible", rep["admin"])
		assert.Equal(t, []any{map[string]any{"name": "console-ui", "kind": "ui"}}, rep["principals"])
		assert.NotContains(t, rep, "sub")
		assert.NotContains(t, p.stderr.String(), mapped, "no token on the process's stderr")
	})

	t.Run("off: no JWT is accepted, nothing is fetched and /auth/session is 404", func(t *testing.T) {
		t.Parallel()
		iss := newSessionIssuer(t)
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("one"))
		// auth.oidc.enabled false; the section is present but inert. The
		// static chat principal and the session principal both stay: the
		// session one authenticates nothing while the verifier is off.
		cfgPath := sessionTestConfig(t, iss, model.baseURL(), false, []string{"agui", "session"})

		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", cfgPath)
		assert.NotContains(t, p.stderr.String(), "auth.oidc is on")
		mapped := iss.token(sessionTestSub, nil)
		r := aguiPost(t, p, mapped, aguiInput("t-off", "r1", "hello"), map[string]string{"Origin": sessionTestOrigin})
		assert.Equal(t, http.StatusUnauthorized, r.Status)
		assert.Equal(t, http.StatusNotFound, p.status(mcpHTTPRequest{method: http.MethodGet, path: "/auth/session", token: mapped}))
		assert.Zero(t, iss.hits.Load(), "no request reached the issuer")
	})

	t.Run("routes absent: a mapped token reaches /auth/session only", func(t *testing.T) {
		t.Parallel()
		iss := newSessionIssuer(t)
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("one"))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", sessionTestConfig(t, iss, model.baseURL(), true, nil))
		mapped := iss.token(sessionTestSub, nil)
		assert.Equal(t, http.StatusOK, p.status(mcpHTTPRequest{method: http.MethodGet, path: "/auth/session", token: mapped}))
		r := aguiPost(t, p, mapped, aguiInput("t-default", "r1", "hello"), map[string]string{"Origin": sessionTestOrigin})
		assert.Equal(t, http.StatusUnauthorized, r.Status, "agui is not on the default routes")
	})
}
