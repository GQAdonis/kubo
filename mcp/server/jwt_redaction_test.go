package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionNeverLogsATokenOrEmail drives every outcome of the session path
// (accepted, each refusal, the report, a settings write) against a listener
// that logs at debug level, then looks for any part of a token or the email in
// the log, which also carries the settings audit record.
func TestSessionNeverLogsATokenOrEmail(t *testing.T) {
	iss := newTestIssuer(t)
	t.Setenv(settingsKeyEnv, settingsTestKey)
	cfg := mustParse(t, fmt.Sprintf(`{
		"auth":{"oidc":{"enabled":true,"issuer":%q,"audiences":[%q],"jwks_url":%q}},
		"principals":[
			{"name":"console-ui","kind":"ui","subjects":[%q]},
			{"name":"console-admin","kind":"admin","subjects":[%q]}
		],
		"agent":{"origin_allowlist":[%q]},
		"llm":{"base_url":"http://127.0.0.1:59998/v1","model":"m-old","api_key":{"env":%q}}
	}`, testIssuerURL, testAudience, iss.jwksURL(), sessionSub, sessionSub, sessionOrigin, settingsKeyEnv))
	oidc, err := cfg.OIDCSettings()
	require.NoError(t, err)

	logs := &lockedBuffer{}
	log := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	jwt, err := NewJWTVerifier(JWTOptions{OIDC: *oidc, Principals: cfg.SessionPrincipals(), OriginAllowlist: cfg.Agent.OriginAllowlist})
	require.NoError(t, err)

	resolved, err := cfg.ResolveAgentWithOverlay(LLMOverrides{}, nil)
	require.NoError(t, err)
	d := newFakeDaemon(t)
	s := mustNew(t, d, Options{Tools: []string{"node"}, Resolvers: []Resolver{newFakeResolver(ResolverLinuxPolkit)}, Transport: "http", Logger: log})
	require.NoError(t, s.EnableAgent(resolved))
	require.NoError(t, s.EnableAgentSettings(AgentSettingsAPIConfig{Cfg: cfg, Resolved: resolved}))
	hs, err := NewHTTPServer(s, HTTPOptions{Listen: "127.0.0.1:0", Verifier: NewChainVerifier(testVerifier(t), jwt), Logger: log})
	require.NoError(t, err)
	addr, err := hs.Listen()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- hs.Serve(ctx) }()
	t.Cleanup(func() { cancel(); require.NoError(t, <-done) })
	base := "http://" + addr.String()

	now := time.Now()
	sign := func(mod func(map[string]any)) string {
		c := claimsAt(now, sessionSub)
		if mod != nil {
			mod(c)
		}
		return iss.sign(testKidRSA, c)
	}
	good := sign(nil)
	tokens := map[string]string{
		"good":        good,
		"expired":     sign(func(c map[string]any) { c["exp"] = now.Add(-time.Hour).Unix() }),
		"wrong aud":   sign(func(c map[string]any) { c["aud"] = []string{"x"} }),
		"unmapped":    sign(func(c map[string]any) { c["sub"] = "registrant-9" }),
		"weak amr":    sign(func(c map[string]any) { c["amr"] = []string{"password"} }),
		"unknown kid": signWith(t, jwkSigningKey(iss), "no-such-kid", claimsAt(now, sessionSub)),
		"tampered":    good[:len(good)-6] + "AAAAAA",
	}
	send := func(method, path, token, origin, body string) (int, string) {
		req, err := http.NewRequestWithContext(t.Context(), method, base+path, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		return resp.StatusCode, b.String()
	}

	var responses strings.Builder
	for name, tok := range tokens {
		for _, path := range []string{RouteAuthSession, RouteSettings, RouteAGUI} {
			code, body := send(http.MethodGet, path, tok, "", "")
			responses.WriteString(body)
			if name != "good" && (name != "weak amr" || path != RouteAuthSession) {
				assert.GreaterOrEqual(t, code, 400, "%s on %s", name, path)
			}
		}
		_, body := send(http.MethodPost, RouteAGUI, tok, "", "{}") // no Origin
		responses.WriteString(body)
	}
	// An accepted settings write: the audit record names principal and sub.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+RouteSettings, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+good)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	etag := resp.Header.Get("ETag")
	req, err = http.NewRequestWithContext(t.Context(), http.MethodPut, base+RouteSettings, strings.NewReader(`{"backend":{"model":"m-new"}}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+good)
	req.Header.Set("Origin", sessionOrigin)
	req.Header.Set("If-Match", etag)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The JWKS outage path logs the URL, not a token.
	iss.setDown(true)
	down, err := NewJWTVerifier(JWTOptions{OIDC: *oidc, Logger: log})
	require.NoError(t, err)
	_ = down.Warm(t.Context())
	_, _, _ = down.Verify(t.Context(), good, reqTo(RouteAGUI))

	out := logs.String()
	require.NotEmpty(t, out)
	assert.Contains(t, out, "principal=console-admin")
	assert.Contains(t, out, "sub="+sessionSub, "the audit record names the subject")
	assert.Contains(t, out, "agent settings applied")
	for name, tok := range tokens {
		for i, seg := range strings.Split(tok, ".") {
			if len(seg) < 8 {
				continue
			}
			assert.NotContains(t, out, seg, "%s token segment %d in the log", name, i)
			assert.NotContains(t, responses.String(), seg, "%s token segment %d in a response", name, i)
		}
	}
	assert.NotContains(t, out, "operator@example.test", "the email is display-only and never logged")
}
