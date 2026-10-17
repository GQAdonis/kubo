package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var jwtTestNow = time.Unix(1_800_000_000, 0)

const jwtTestSub = "0b1c2d3e-4f50-4a61-8b72-93a4b5c6d7e8"

// newTestJWTVerifier builds a verifier on a fixed clock that maps jwtTestSub to
// a ui identity.
func newTestJWTVerifier(t *testing.T, iss *testIssuer, mutate func(*OIDCSettings)) *JWTVerifier {
	t.Helper()
	cfg := iss.settings()
	if mutate != nil {
		mutate(&cfg)
	}
	v, err := NewJWTVerifier(JWTOptions{
		OIDC: cfg,
		Now:  func() time.Time { return jwtTestNow },
		Subjects: map[string]Identity{
			jwtTestSub: NewIdentity(Principal{ID: "console-ui", Kind: KindUI}, []string{AllGroups}, nil, nil),
		},
	})
	require.NoError(t, err)
	return v
}

func TestJWTVerifierAcceptsOnlyTheValidToken(t *testing.T) {
	iss := newTestIssuer(t)
	v := newTestJWTVerifier(t, iss, nil)

	// The public key bytes an attacker can download from the JWKS, used as an
	// HMAC secret (the algorithm-confusion attack).
	pub, err := json.Marshal(jose.JSONWebKey{Key: &iss.rsaKey.PublicKey, KeyID: testKidRSA})
	require.NoError(t, err)

	mod := func(f func(map[string]any)) string {
		c := claimsAt(jwtTestNow, jwtTestSub)
		f(c)
		return iss.sign(testKidRSA, c)
	}
	valid := iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub))
	noneHeader := jsonSeg(map[string]any{"alg": "none", "kid": testKidRSA, "typ": "JWT"})

	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid token", valid, true},
		{"wrong audience", mod(func(c map[string]any) { c["aud"] = []string{"someone-else"} }), false},
		{"no audience", mod(func(c map[string]any) { delete(c, "aud") }), false},
		{"wrong issuer", mod(func(c map[string]any) { c["iss"] = testIssuerURL + "/" }), false},
		{"expired", mod(func(c map[string]any) { c["exp"] = jwtTestNow.Add(-time.Hour).Unix() }), false},
		{"expired beyond the skew", mod(func(c map[string]any) { c["exp"] = jwtTestNow.Add(-31 * time.Second).Unix() }), false},
		{"expired within the skew", mod(func(c map[string]any) { c["exp"] = jwtTestNow.Add(-29 * time.Second).Unix() }), true},
		{"not valid yet", mod(func(c map[string]any) { c["nbf"] = jwtTestNow.Add(time.Hour).Unix() }), false},
		{"issued in the future", mod(func(c map[string]any) { c["iat"] = jwtTestNow.Add(time.Hour).Unix() }), false},
		{"no expiry", mod(func(c map[string]any) { delete(c, "exp") }), false},
		{"no subject", mod(func(c map[string]any) { delete(c, "sub") }), false},
		{"alg none", noneHeader + "." + jsonSeg(claimsAt(jwtTestNow, jwtTestSub)) + ".c2ln", false},
		{"alg none without signature", noneHeader + "." + jsonSeg(claimsAt(jwtTestNow, jwtTestSub)) + ".", false},
		{"HS256 signed with the public key", signWith(t, jose.SigningKey{Algorithm: jose.HS256, Key: pub}, testKidRSA, claimsAt(jwtTestNow, jwtTestSub)), false},
		{"unknown kid", signWith(t, jose.SigningKey{Algorithm: jose.RS256, Key: iss.rsaKey}, "no-such-kid", claimsAt(jwtTestNow, jwtTestSub)), false},
		{"no kid", signWith(t, jose.SigningKey{Algorithm: jose.RS256, Key: iss.rsaKey}, "", claimsAt(jwtTestNow, jwtTestSub)), false},
		{"tampered payload", rewritePayload(valid, func() map[string]any {
			c := claimsAt(jwtTestNow, jwtTestSub)
			c["sub"] = "someone-else"
			return c
		}()), false},
		{"tampered signature", valid[:len(valid)-4] + strings.Repeat("A", 4), false},
		{"algorithm not allowed", iss.sign(testKidEC, claimsAt(jwtTestNow, jwtTestSub)), false},
		{"opaque token", "just-a-static-bearer-token", false},
		{"empty", "", false},
		{"oversized", strings.Repeat("a", maxJWTBytes) + "." + strings.Repeat("b", 10) + "." + strings.Repeat("c", 10), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, exp, err := v.Verify(t.Context(), tc.token, nil)
			if !tc.ok {
				require.ErrorIs(t, err, auth.ErrInvalidToken)
				assert.Empty(t, id.ID)
				if tc.token != "" {
					assert.NotContains(t, err.Error(), tc.token, "the error never carries the token")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "console-ui", id.ID)
			assert.WithinDuration(t, time.Unix(claimsInt(t, tc.token, "exp"), 0), exp, 0, "the verdict ends at the token exp")
		})
	}
}

// claimsInt reads an integer claim from the payload of a signed test token.
func claimsInt(t *testing.T, tok, name string) int64 {
	t.Helper()
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	raw, err := jose.ParseSignedCompact(tok, []jose.SignatureAlgorithm{jose.RS256})
	require.NoError(t, err)
	var c map[string]any
	require.NoError(t, json.Unmarshal(raw.UnsafePayloadWithoutVerification(), &c))
	f, ok := c[name].(float64)
	require.True(t, ok)
	return int64(f)
}

func TestJWTVerifierAlgorithmSelection(t *testing.T) {
	iss := newTestIssuer(t)
	only := func(a ...jose.SignatureAlgorithm) func(*OIDCSettings) {
		return func(s *OIDCSettings) { s.Algorithms = a }
	}
	ec := iss.sign(testKidEC, claimsAt(jwtTestNow, jwtTestSub))
	rs := iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub))

	_, _, err := newTestJWTVerifier(t, iss, only(jose.ES256)).Verify(t.Context(), ec, nil)
	require.NoError(t, err, "ES256 token with ES256 allowed")
	_, _, err = newTestJWTVerifier(t, iss, only(jose.ES256)).Verify(t.Context(), rs, nil)
	require.ErrorIs(t, err, auth.ErrInvalidToken, "RS256 token with only ES256 allowed")
	_, _, err = newTestJWTVerifier(t, iss, only(jose.RS256, jose.ES256)).Verify(t.Context(), ec, nil)
	require.NoError(t, err, "both allowed")
}

func TestJWTVerifierKeyAlgorithmMismatchIsRejected(t *testing.T) {
	iss := newTestIssuer(t)
	// The key set declares the RSA key for PS256 only; an RS256 token signed
	// with it must not be accepted.
	iss.mu.Lock()
	iss.extra = append(iss.extra, jose.JSONWebKey{Key: &iss.rsaKey.PublicKey, KeyID: "ps-only", Use: "sig", Algorithm: "PS256"})
	iss.mu.Unlock()
	v := newTestJWTVerifier(t, iss, nil)
	tok := signWith(t, jose.SigningKey{Algorithm: jose.RS256, Key: iss.rsaKey}, "ps-only", claimsAt(jwtTestNow, jwtTestSub))
	_, _, err := v.Verify(t.Context(), tok, nil)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestJWTVerifierRefusesPrivateAndNonSigningKeys(t *testing.T) {
	iss := newTestIssuer(t)
	iss.mu.Lock()
	iss.extra = append(iss.extra,
		jose.JSONWebKey{Key: iss.rsaKey, KeyID: "private", Use: "sig"},
		jose.JSONWebKey{Key: &iss.rsaKey.PublicKey, KeyID: "enc-key", Use: "enc"},
	)
	iss.mu.Unlock()
	v := newTestJWTVerifier(t, iss, nil)
	for _, kid := range []string{"private", "enc-key"} {
		tok := signWith(t, jose.SigningKey{Algorithm: jose.RS256, Key: iss.rsaKey}, kid, claimsAt(jwtTestNow, jwtTestSub))
		_, _, err := v.Verify(t.Context(), tok, nil)
		require.ErrorIs(t, err, auth.ErrInvalidToken, kid)
	}
}

func TestJWKSCacheFetchesOnceForManyTokens(t *testing.T) {
	iss := newTestIssuer(t)
	v := newTestJWTVerifier(t, iss, nil)
	tok := iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub))
	for range 20 {
		_, _, err := v.Verify(t.Context(), tok, nil)
		require.NoError(t, err)
	}
	assert.EqualValues(t, 1, iss.hits.Load())
}

// fakeJWKS is a RoundTripper that serves a key set without a socket, so the
// timer tests can run in a synctest bubble.
type fakeJWKS struct {
	set  jose.JSONWebKeySet
	hits int
	fail bool
}

func (f *fakeJWKS) RoundTrip(req *http.Request) (*http.Response, error) {
	f.hits++
	if f.fail {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody, Request: req}, nil
	}
	raw, _ := json.Marshal(f.set)
	return &http.Response{StatusCode: http.StatusOK, Body: nopBody(raw), Request: req, Header: http.Header{}}, nil
}

type nopBodyReader struct{ *strings.Reader }

func (nopBodyReader) Close() error { return nil }

func nopBody(b []byte) nopBodyReader { return nopBodyReader{strings.NewReader(string(b))} }

func TestJWKSCacheRefetchIsRateLimitedAndTTLBounded(t *testing.T) {
	iss := newTestIssuer(t)
	pubA := jose.JSONWebKey{Key: &iss.rsaKey.PublicKey, KeyID: "a", Use: "sig"}
	pubB := jose.JSONWebKey{Key: &iss.rsaKey.PublicKey, KeyID: "b", Use: "sig"}

	synctest.Test(t, func(t *testing.T) {
		rt := &fakeJWKS{set: jose.JSONWebKeySet{Keys: []jose.JSONWebKey{pubA}}}
		c := newJWKSCache("https://keys.test.invalid/jwks.json", 15*time.Minute, &http.Client{Transport: rt}, time.Now)

		_, err := c.key(t.Context(), "a")
		require.NoError(t, err)
		require.Equal(t, 1, rt.hits, "first use fetches")

		// An unknown kid inside the window costs no request.
		_, err = c.key(t.Context(), "b")
		require.ErrorIs(t, err, errNoSuchKey)
		require.Equal(t, 1, rt.hits)

		// The issuer rotates; after the window one request finds the new key.
		rt.set = jose.JSONWebKeySet{Keys: []jose.JSONWebKey{pubA, pubB}}
		time.Sleep(jwksMinRefetch + time.Second)
		_, err = c.key(t.Context(), "b")
		require.NoError(t, err)
		require.Equal(t, 2, rt.hits)

		// Many unknown kids in a burst still make no further request.
		for i := range 50 {
			_, err = c.key(t.Context(), "junk-"+string(rune('a'+i%26)))
			require.ErrorIs(t, err, errNoSuchKey)
		}
		require.Equal(t, 2, rt.hits)

		// Past the TTL a known kid refreshes the set once; an outage then
		// keeps serving the keys already held.
		time.Sleep(16 * time.Minute)
		rt.fail = true
		_, err = c.key(t.Context(), "a")
		require.NoError(t, err, "stale keys survive an outage")
		require.Equal(t, 3, rt.hits)
		_, err = c.key(t.Context(), "a")
		require.NoError(t, err)
		require.Equal(t, 3, rt.hits, "and the failed attempt also starts the window")
	})
}

func TestJWKSCacheFailureNamesTheURLWithoutUserinfo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &fakeJWKS{fail: true}
		c := newJWKSCache("https://user:hunter2@keys.test.invalid/jwks.json", time.Minute, &http.Client{Transport: rt}, time.Now)
		_, err := c.key(t.Context(), "a")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "keys.test.invalid/jwks.json")
		assert.NotContains(t, err.Error(), "hunter2")
	})
}

func TestJWTVerifierKeysUnavailableIs401NotAFault(t *testing.T) {
	iss := newTestIssuer(t)
	iss.setDown(true)
	v := newTestJWTVerifier(t, iss, nil)
	_, _, err := v.Verify(t.Context(), iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub)), nil)
	require.ErrorIs(t, err, auth.ErrInvalidToken)

	// Garbage instead of a key set is the same.
	iss.setDown(false)
	iss.mu.Lock()
	iss.garbage = true
	iss.mu.Unlock()
	v = newTestJWTVerifier(t, iss, nil)
	_, _, err = v.Verify(t.Context(), iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub)), nil)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestNewJWTVerifierRequiresItsSettings(t *testing.T) {
	_, err := NewJWTVerifier(JWTOptions{})
	require.Error(t, err)
}
