package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-jose/go-jose/v4"
)

const (
	// jwksMinRefetch is the shortest time between two fetch attempts of the key
	// set, whether the first succeeded or not. An unknown kid, which anyone can
	// put in a token, therefore costs at most one outbound request every 30 s.
	jwksMinRefetch = 30 * time.Second
	// jwksFetchTimeout bounds one fetch.
	jwksFetchTimeout = 10 * time.Second
	// jwksMaxBytes bounds the key set document.
	jwksMaxBytes = 1 << 20
)

// errNoSuchKey means the cache has no usable key for a kid.
var errNoSuchKey = errors.New("no key for kid")

// jwksCache holds the issuer's public keys. It fetches the key set only when a
// token needs a key it does not have or the cached set outlived its TTL, and it
// never fetches more often than jwksMinRefetch. The URL is the operator's
// configured value; nothing is derived from a token.
type jwksCache struct {
	url string
	ttl time.Duration
	// maxStale is auth.oidc.jwks_max_stale: past this age since the last
	// successful fetch no key is served, fresh or stale.
	maxStale time.Duration
	client   *http.Client
	now      func() time.Time
	// onStale reports that the cap passed. It runs once per stale period, that
	// is until the next successful fetch.
	onStale     func(age time.Duration)
	staleLogged atomic.Bool

	// fetchMu serializes fetches, so concurrent requests with an unknown kid
	// share one attempt.
	fetchMu sync.Mutex

	mu          sync.RWMutex
	keys        map[string]jose.JSONWebKey
	fetchedAt   time.Time
	lastAttempt time.Time
}

func newJWKSCache(rawURL string, ttl, maxStale time.Duration, client *http.Client, now func() time.Time) *jwksCache {
	if client == nil {
		client = &http.Client{
			Timeout: jwksFetchTimeout,
			// The URL is configured; a redirect would be a second, unreviewed
			// destination (and might downgrade https).
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if now == nil {
		now = time.Now
	}
	return &jwksCache{url: rawURL, ttl: ttl, maxStale: maxStale, client: client, now: now, keys: map[string]jose.JSONWebKey{}}
}

// key returns the public key for kid. A key in a fresh cache is returned
// without any network access. Otherwise the set is refetched, at most once per
// jwksMinRefetch; inside that window a key the cache still holds is returned
// and an absent one is reported as errNoSuchKey.
func (c *jwksCache) key(ctx context.Context, kid string) (jose.JSONWebKey, error) {
	if k, ok := c.lookup(kid, true); ok {
		return k, nil
	}
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()
	// Another request may have refreshed the set while this one waited.
	if k, ok := c.lookup(kid, true); ok {
		return k, nil
	}
	c.mu.RLock()
	last := c.lastAttempt
	c.mu.RUnlock()
	if !last.IsZero() && c.now().Sub(last) < jwksMinRefetch {
		if k, ok := c.lookup(kid, false); ok {
			return k, nil
		}
		c.reportStale()
		return jose.JSONWebKey{}, errNoSuchKey
	}
	if err := c.fetch(ctx); err != nil {
		// Keep serving the keys already held: an outage of the issuer's JWKS
		// endpoint must not sign everyone out before their tokens expire.
		if k, ok := c.lookup(kid, false); ok {
			return k, nil
		}
		c.reportStale()
		return jose.JSONWebKey{}, err
	}
	if k, ok := c.lookup(kid, true); ok {
		return k, nil
	}
	return jose.JSONWebKey{}, errNoSuchKey
}

// lookup returns the cached key for kid. With fresh it requires the cache to be
// within its TTL. Past maxStale it returns no key at all, so a key the issuer
// rotated out or revoked cannot outlive an unreachable key set forever.
func (c *jwksCache) lookup(kid string, fresh bool) (jose.JSONWebKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	k, ok := c.keys[kid]
	if !ok {
		return jose.JSONWebKey{}, false
	}
	age := c.now().Sub(c.fetchedAt)
	if c.maxStale > 0 && age >= c.maxStale {
		// Not logged here: the caller may still refetch and succeed. key()
		// reports the expiry only when the stale answer is final.
		return jose.JSONWebKey{}, false
	}
	if fresh && age >= c.ttl {
		return jose.JSONWebKey{}, false
	}
	return k, true
}

// reportStale logs the cap's expiry, once per stale period, when a held key set
// older than maxStale is the final answer: the refetch failed or the refetch
// window forbids one. A stale set that a refetch replaces is not an outage and
// is not reported.
func (c *jwksCache) reportStale() {
	c.mu.RLock()
	age, held := c.now().Sub(c.fetchedAt), !c.fetchedAt.IsZero()
	c.mu.RUnlock()
	if !held || c.maxStale <= 0 || age < c.maxStale || c.onStale == nil {
		return
	}
	if c.staleLogged.CompareAndSwap(false, true) {
		c.onStale(age)
	}
}

// warm fetches the key set once, for a prefetch at startup. It obeys the same
// rate limit as a request-driven fetch.
func (c *jwksCache) warm(ctx context.Context) error {
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()
	c.mu.RLock()
	last := c.lastAttempt
	c.mu.RUnlock()
	if !last.IsZero() && c.now().Sub(last) < jwksMinRefetch {
		return nil
	}
	return c.fetch(ctx)
}

// fetch downloads the key set and replaces the cache. The caller holds
// fetchMu. Only public signing keys with a kid are kept. The error never
// contains the response body.
func (c *jwksCache) fetch(ctx context.Context) error {
	c.mu.Lock()
	c.lastAttempt = c.now()
	c.mu.Unlock()

	// The fetch outlives the request that triggered it: a client that hangs up
	// must not use up the refetch window and leave the next request without a
	// key. jwksFetchTimeout still bounds it.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jwksFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("jwks %s: %w", redactURL(c.url), err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		// url.Error repeats the URL, which is already named once below.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("jwks %s: fetch: %w", redactURL(c.url), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks %s: status %d", redactURL(c.url), resp.StatusCode)
	}
	var set jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(resp.Body, jwksMaxBytes)).Decode(&set); err != nil {
		return fmt.Errorf("jwks %s: not a key set: %w", redactURL(c.url), err)
	}
	keys := make(map[string]jose.JSONWebKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.KeyID == "" || !k.Valid() || !k.IsPublic() || (k.Use != "" && k.Use != "sig") {
			continue
		}
		keys[k.KeyID] = k
	}
	if len(keys) == 0 {
		return fmt.Errorf("jwks %s: no usable signing key", redactURL(c.url))
	}
	c.mu.Lock()
	c.keys, c.fetchedAt = keys, c.now()
	c.mu.Unlock()
	c.staleLogged.Store(false)
	return nil
}

// redactURL drops userinfo from a URL, for messages.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid url>"
	}
	return u.Redacted()
}
