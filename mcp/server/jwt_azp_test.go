package server

import (
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDebugLogger logs everything to w, as text.
func newDebugLogger(w *lockedBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// TestJWTVerifierAZPRule covers OIDC Core 3.1.3.7 steps 4 and 5: an ID token
// with several audiences needs an azp, and an azp must be a configured
// audience.
func TestJWTVerifierAZPRule(t *testing.T) {
	iss := newTestIssuer(t)
	logs := &lockedBuffer{}
	v := newTestJWTVerifier(t, iss, func(s *OIDCSettings) { s.Audiences = []string{testAudience, "ipfs-console-b"} })
	v.log = newDebugLogger(logs)

	tok := func(mod func(map[string]any)) string {
		c := claimsAt(jwtTestNow, jwtTestSub)
		mod(c)
		return iss.sign(testKidRSA, c)
	}
	for _, tc := range []struct {
		name string
		mod  func(map[string]any)
		ok   bool
	}{
		{"one aud, no azp", func(c map[string]any) {}, true},
		{"several aud, no azp", func(c map[string]any) { c["aud"] = []string{testAudience, "other-api"} }, false},
		{"several aud, foreign azp", func(c map[string]any) { c["aud"] = []string{testAudience, "other-api"}; c["azp"] = "other-api" }, false},
		{"several aud, azp in audiences", func(c map[string]any) { c["aud"] = []string{testAudience, "other-api"}; c["azp"] = testAudience }, true},
		{"one aud, foreign azp", func(c map[string]any) { c["azp"] = "someone-else" }, false},
		{"one aud, azp in audiences", func(c map[string]any) { c["azp"] = "ipfs-console-b" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := tok(tc.mod)
			_, _, err := v.Verify(t.Context(), token, reqTo(RouteAGUI))
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, auth.ErrInvalidToken)
			assert.NotContains(t, logs.String(), token, "a refusal never logs the token")
			assert.NotContains(t, logs.String(), signatureSegment(token))
		})
	}
	assert.Contains(t, logs.String(), "reason=azp")
}
