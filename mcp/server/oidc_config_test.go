package server

import (
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oidcConfigJSON wraps an auth.oidc body in a minimal config file.
func oidcConfigJSON(body string) []byte {
	return []byte(`{"auth":{"oidc":` + body + `}}`)
}

const validOIDCBody = `"enabled":true,"issuer":"https://oauth.example.test","audiences":["ipfs-console"],"jwks_url":"https://oauth.example.test/.well-known/jwks.json"`

func TestParseConfigOIDCDefaultsAndOffByDefault(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{}`))
	require.NoError(t, err)
	assert.False(t, cfg.Auth.OIDC.Enabled, "off by default")

	cfg, err = ParseConfig(oidcConfigJSON(`{` + validOIDCBody + `}`))
	require.NoError(t, err)
	s, err := cfg.Auth.OIDC.resolve()
	require.NoError(t, err)
	assert.Equal(t, []jose.SignatureAlgorithm{jose.RS256}, s.Algorithms)
	assert.Equal(t, 30*time.Second, s.ClockSkew)
	assert.Equal(t, 15*time.Minute, s.JWKSCacheTTL)
	assert.Equal(t, 24*time.Hour, s.JWKSMaxStale)
	assert.Equal(t, []string{OIDCRouteSession}, s.Routes, "a JWT is accepted on the session report only")
	assert.Equal(t, DefaultOIDCRoutes(), s.Routes)
	assert.NotContains(t, s.Routes, OIDCRouteMCP)
	assert.True(t, s.RequireOrigin)
}

func TestParseConfigOIDCRejections(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"missing jwks_url", `{"enabled":true,"issuer":"https://oauth.example.test","audiences":["a"]}`, "jwks_url"},
		{"missing issuer", `{"enabled":true,"audiences":["a"],"jwks_url":"https://x.example.test/jwks"}`, "issuer"},
		{"no audiences", `{"enabled":true,"issuer":"https://oauth.example.test","audiences":[],"jwks_url":"https://x.example.test/jwks"}`, "audiences"},
		{"HS256", `{` + validOIDCBody + `,"algorithms":["HS256"]}`, "algorithms"},
		{"none", `{` + validOIDCBody + `,"algorithms":["none"]}`, "algorithms"},
		{"clock skew above 2m", `{` + validOIDCBody + `,"clock_skew":"2m1s"}`, "auth.oidc.clock_skew"},
		{"plain http jwks off loopback", `{"enabled":true,"issuer":"https://oauth.example.test","audiences":["a"],"jwks_url":"http://keys.example.test/jwks"}`, "jwks_url"},
		{"unknown key", `{` + validOIDCBody + `,"fallback_secret":"x"}`, "fallback_secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig(oidcConfigJSON(tc.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestParseConfigOIDCAccepts(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"skew at the limit", `{` + validOIDCBody + `,"clock_skew":"2m"}`},
		{"zero skew", `{` + validOIDCBody + `,"clock_skew":"0s"}`},
		{"loopback http jwks", `{"enabled":true,"issuer":"https://oauth.example.test","audiences":["a"],"jwks_url":"http://127.0.0.1:9/jwks"}`},
		{"empty routes", `{` + validOIDCBody + `,"routes":[]}`},
		{"asymmetric list", `{` + validOIDCBody + `,"algorithms":["RS256","ES256","EdDSA"]}`},
		{"disabled with nothing else", `{"enabled":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig(oidcConfigJSON(tc.body))
			require.NoError(t, err)
		})
	}
}
