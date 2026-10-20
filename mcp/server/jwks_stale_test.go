package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClock is a clock the test moves by hand.
type testClock struct{ ns atomic.Int64 }

func newTestClock(at time.Time) *testClock {
	c := &testClock{}
	c.ns.Store(at.UnixNano())
	return c
}

func (c *testClock) Now() time.Time          { return time.Unix(0, c.ns.Load()) }
func (c *testClock) Advance(d time.Duration) { c.ns.Add(int64(d)) }

const staleLogMessage = "older than auth.oidc.jwks_max_stale"

func TestJWKSMaxStaleCapsHeldKeys(t *testing.T) {
	iss := newTestIssuer(t)
	clock := newTestClock(jwtTestNow)
	logs := &lockedBuffer{}
	cfg := iss.settings()
	cfg.JWKSMaxStale = 2 * time.Hour
	v, err := NewJWTVerifier(JWTOptions{
		OIDC:   cfg,
		Now:    clock.Now,
		Logger: newDebugLogger(logs),
		Principals: []SessionPrincipal{{
			Identity: NewIdentity(Principal{ID: "console-ui", Kind: KindUI}, []string{AllGroups}, nil, nil),
			Subjects: []string{jwtTestSub},
		}},
	})
	require.NoError(t, err)
	chain := NewChainVerifier(testVerifier(t), v)

	c := claimsAt(jwtTestNow, jwtTestSub)
	c["exp"] = jwtTestNow.Add(72 * time.Hour).Unix()
	tok := iss.sign(testKidRSA, c)
	verify := func() error {
		_, _, err := v.Verify(t.Context(), tok, reqTo(RouteAGUI))
		return err
	}
	staleRecords := func() int { return strings.Count(logs.String(), staleLogMessage) }

	require.NoError(t, verify(), "a fresh fetch")
	require.EqualValues(t, 1, iss.hits.Load())

	// The issuer stops answering. The key held inside the cap keeps working.
	iss.setDown(true)
	clock.Advance(time.Hour)
	require.NoError(t, verify(), "a held key works inside the cap while the JWKS server is down")
	assert.Zero(t, staleRecords())

	// Past the cap the same token is refused, and the expiry is logged once.
	clock.Advance(time.Hour + time.Minute)
	require.ErrorIs(t, verify(), auth.ErrInvalidToken)
	require.ErrorIs(t, verify(), auth.ErrInvalidToken)
	clock.Advance(time.Minute)
	require.ErrorIs(t, verify(), auth.ErrInvalidToken)
	assert.Equal(t, 1, staleRecords(), "one record per stale period")
	assert.NotContains(t, logs.String(), tok)

	// A static bearer does not depend on the key set.
	_, _, err = chain.Verify(t.Context(), tokChat, reqTo(RouteAGUI))
	require.NoError(t, err)

	// The issuer comes back; the next fetch restores access.
	iss.setDown(false)
	clock.Advance(jwksMinRefetch + time.Second)
	require.NoError(t, verify())
	assert.Equal(t, 1, staleRecords())

	// A second outage past the cap is a new period and logs again.
	iss.setDown(true)
	clock.Advance(3 * time.Hour)
	require.ErrorIs(t, verify(), auth.ErrInvalidToken)
	assert.Equal(t, 2, staleRecords())
}

// A hand-built OIDCSettings with no cap gets the default, not "no cap".
func TestJWKSMaxStaleHasNoOffValue(t *testing.T) {
	iss := newTestIssuer(t)
	cfg := iss.settings()
	cfg.JWKSMaxStale = 0
	v, err := NewJWTVerifier(JWTOptions{OIDC: cfg})
	require.NoError(t, err)
	assert.Equal(t, DefaultOIDCJWKSMaxStale, v.keys.maxStale)
}

func TestJWKSMaxStaleConfig(t *testing.T) {
	parse := func(extra string) (OIDCSettings, error) {
		cfg, err := ParseConfig(oidcConfigJSON(`{` + validOIDCBody + extra + `}`))
		if err != nil {
			return OIDCSettings{}, err
		}
		return cfg.Auth.OIDC.resolve()
	}
	s, err := parse(``)
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, s.JWKSMaxStale, "default")

	s, err = parse(`,"jwks_max_stale":"90m"`)
	require.NoError(t, err)
	assert.Equal(t, 90*time.Minute, s.JWKSMaxStale)

	s, err = parse(`,"jwks_cache_ttl":"15m","jwks_max_stale":"15m"`)
	require.NoError(t, err, "equal to the TTL is the minimum")
	assert.Equal(t, 15*time.Minute, s.JWKSMaxStale)

	for _, extra := range []string{
		`,"jwks_cache_ttl":"15m","jwks_max_stale":"14m"`,
		`,"jwks_max_stale":"0s"`,
		`,"jwks_max_stale":"-1h"`,
		`,"jwks_max_stale":"forever"`,
	} {
		_, err := parse(extra)
		require.Error(t, err, extra)
		assert.Contains(t, err.Error(), "jwks_max_stale", extra)
	}

	// Validated while disabled, so a bad value never waits for the switch.
	_, err = ParseConfig(oidcConfigJSON(`{"enabled":false,"jwks_cache_ttl":"15m","jwks_max_stale":"1m"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth.oidc.jwks_max_stale")
}

// With auth.oidc off and the key set, no verifier exists and nothing is fetched.
func TestJWKSMaxStaleDisabledFetchesNothing(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	cfg, err := ParseConfig(oidcConfigJSON(`{"enabled":false,"jwks_url":"` + srv.URL + `/jwks","jwks_max_stale":"48h"}`))
	require.NoError(t, err)
	oidc, err := cfg.OIDCSettings()
	require.NoError(t, err)
	assert.Nil(t, oidc, "no settings, so no verifier")
	assert.Zero(t, hits.Load())
}

// The key fetch does not take the cancellation of the request that triggered
// it: a client that hangs up must not burn the refetch window.
func TestJWKSFetchSurvivesCancelledRequest(t *testing.T) {
	iss := newTestIssuer(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startOnce.Do(func() { close(started) })
		<-release
		iss.serveJWKS(w, r)
	}))
	t.Cleanup(slow.Close)
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)

	v := newTestJWTVerifier(t, iss, func(s *OIDCSettings) { s.JWKSURL = slow.URL + "/jwks.json" })
	tok := iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub))

	ctx, cancel := context.WithCancel(t.Context())
	first := make(chan error, 1)
	go func() {
		_, _, err := v.Verify(ctx, tok, reqTo(RouteAGUI))
		first <- err
	}()
	<-started
	cancel() // the client drops while the fetch is in flight
	releaseOnce()
	<-first

	// The fixed test clock keeps every later request inside the 30 s window.
	_, _, err := v.Verify(t.Context(), tok, reqTo(RouteAGUI))
	require.NoError(t, err, "the second request finds the keys the first one fetched")
	assert.EqualValues(t, 1, iss.hits.Load())
}

// blockingRT never answers; it returns when the request context ends.
type blockingRT struct{}

func (blockingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestJWKSFetchStaysBoundedByTheTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newJWKSCache("https://keys.test.invalid/jwks.json", time.Minute, time.Hour, &http.Client{Transport: blockingRT{}}, time.Now)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		start := time.Now()
		err := c.fetch(ctx)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded), "the timeout ended it: %v", err)
		assert.Equal(t, jwksFetchTimeout, time.Since(start))
	})
}

// The default client follows no redirect: a second destination is never asked.
func TestJWKSDefaultClientRefusesRedirects(t *testing.T) {
	iss := newTestIssuer(t) // serves the valid key set the redirect points at
	var second atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		second.Add(1)
		iss.serveJWKS(w, r)
	}))
	t.Cleanup(target.Close)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/jwks.json", http.StatusFound)
	}))
	t.Cleanup(first.Close)

	v := newTestJWTVerifier(t, iss, func(s *OIDCSettings) { s.JWKSURL = first.URL + "/jwks.json" })
	tok := iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub))
	_, _, err := v.Verify(t.Context(), tok, reqTo(RouteAGUI))
	require.ErrorIs(t, err, auth.ErrInvalidToken)
	assert.Zero(t, second.Load(), "the redirect target saw no request")
}

// A key set over 1 MiB is "not a key set".
func TestJWKSDefaultClientBoundsTheBody(t *testing.T) {
	iss := newTestIssuer(t)
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"keys":[],"pad":"` + strings.Repeat("a", jwksMaxBytes+10) + `"}`))
	}))
	t.Cleanup(big.Close)

	v := newTestJWTVerifier(t, iss, func(s *OIDCSettings) { s.JWKSURL = big.URL + "/jwks.json" })
	err := v.Warm(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a key set")

	tok := iss.sign(testKidRSA, claimsAt(jwtTestNow, jwtTestSub))
	_, _, err = v.Verify(t.Context(), tok, reqTo(RouteAGUI))
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}
