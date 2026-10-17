package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"
)

const (
	testIssuerURL = "https://oauth.test.invalid"
	testAudience  = "ipfs-console"
	testKidRSA    = "rsa-1"
	testKidEC     = "ec-1"
)

// testIssuer is a local OpenID provider: it serves a JWKS over HTTP and signs
// tokens with keys generated for the test.
type testIssuer struct {
	t   *testing.T
	srv *httptest.Server

	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey

	mu      sync.Mutex
	hidden  map[string]bool // kids withheld from the key set
	down    bool            // answer 503
	garbage bool            // answer 200 with a body that is not a key set
	extra   []jose.JSONWebKey

	hits atomic.Int64
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	ek, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	i := &testIssuer{t: t, rsaKey: rk, ecKey: ek, hidden: map[string]bool{}}
	i.srv = httptest.NewServer(http.HandlerFunc(i.serveJWKS))
	t.Cleanup(i.srv.Close)
	return i
}

func (i *testIssuer) serveJWKS(w http.ResponseWriter, _ *http.Request) {
	i.hits.Add(1)
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.down {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if i.garbage {
		_, _ = w.Write([]byte("<html>not json</html>"))
		return
	}
	set := jose.JSONWebKeySet{}
	if !i.hidden[testKidRSA] {
		set.Keys = append(set.Keys, jose.JSONWebKey{Key: &i.rsaKey.PublicKey, KeyID: testKidRSA, Use: "sig", Algorithm: "RS256"})
	}
	if !i.hidden[testKidEC] {
		set.Keys = append(set.Keys, jose.JSONWebKey{Key: &i.ecKey.PublicKey, KeyID: testKidEC, Use: "sig", Algorithm: "ES256"})
	}
	set.Keys = append(set.Keys, i.extra...)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(set)
}

func (i *testIssuer) setDown(down bool) { i.mu.Lock(); i.down = down; i.mu.Unlock() }
func (i *testIssuer) hide(kid string, hide bool) {
	i.mu.Lock()
	i.hidden[kid] = hide
	i.mu.Unlock()
}

func (i *testIssuer) jwksURL() string { return i.srv.URL + "/jwks.json" }

// settings is a resolved auth.oidc section aimed at this issuer.
func (i *testIssuer) settings() OIDCSettings {
	return OIDCSettings{
		Issuer:        testIssuerURL,
		Audiences:     []string{testAudience},
		JWKSURL:       i.jwksURL(),
		Algorithms:    []jose.SignatureAlgorithm{jose.RS256},
		ClockSkew:     30 * time.Second,
		JWKSCacheTTL:  15 * time.Minute,
		Routes:        DefaultOIDCRoutes(),
		RequireOrigin: true,
	}
}

// claimsAt builds a valid claim set whose times are relative to now.
func claimsAt(now time.Time, sub string) map[string]any {
	return map[string]any{
		"iss":       testIssuerURL,
		"aud":       []string{testAudience},
		"sub":       sub,
		"iat":       now.Add(-time.Minute).Unix(),
		"nbf":       now.Add(-time.Minute).Unix(),
		"exp":       now.Add(10 * time.Minute).Unix(),
		"auth_time": now.Add(-5 * time.Minute).Unix(),
		"amr":       []string{"password", "totp"},
		"email":     "operator@example.test",
	}
}

// sign signs claims with the issuer key for kid (RS256 for the RSA key and
// ES256 for the EC key).
func (i *testIssuer) sign(kid string, claims map[string]any) string {
	i.t.Helper()
	switch kid {
	case testKidEC:
		return signWith(i.t, jose.SigningKey{Algorithm: jose.ES256, Key: i.ecKey}, kid, claims)
	default:
		return signWith(i.t, jose.SigningKey{Algorithm: jose.RS256, Key: i.rsaKey}, kid, claims)
	}
}

func signWith(t *testing.T, key jose.SigningKey, kid string, claims map[string]any) string {
	t.Helper()
	opts := (&jose.SignerOptions{}).WithType("JWT")
	if kid != "" {
		opts = opts.WithHeader(jose.HeaderKey("kid"), kid)
	}
	sig, err := jose.NewSigner(key, opts)
	require.NoError(t, err)
	tok, err := jwt.Signed(sig).Claims(claims).Serialize()
	require.NoError(t, err)
	return tok
}

// signatureSegment returns the third segment of a compact token.
func signatureSegment(tok string) string {
	for n, i := 0, 0; i < len(tok); i++ {
		if tok[i] == '.' {
			if n++; n == 2 {
				return tok[i+1:]
			}
		}
	}
	return ""
}

func jsonSeg(v any) string {
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// rewritePayload swaps the payload of tok for claims and keeps the signature.
func rewritePayload(tok string, claims map[string]any) string {
	var head string
	for i := 0; i < len(tok); i++ {
		if tok[i] == '.' {
			head = tok[:i]
			break
		}
	}
	return head + "." + jsonSeg(claims) + "." + signatureSegment(tok)
}

func newTestAlgorithms() []jose.SignatureAlgorithm { return []jose.SignatureAlgorithm{jose.RS256} }

// jwkSigningKey is the issuer's RSA signing key.
func jwkSigningKey(i *testIssuer) jose.SigningKey {
	return jose.SigningKey{Algorithm: jose.RS256, Key: i.rsaKey}
}
