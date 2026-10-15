//go:build !noagent

package server

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	embeddingsWithInsecure = `"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"/b/surreal","data_dir":"/d"},` +
		`"embedder":{"base_url":"http://127.0.0.1:11434/v1","model":"e","dimensions":8},"insecure_http_hosts":`
	hardenedHTTPSConfig = `{"llm":{"base_url":"https://models.example.com/v1","model":"m"},` +
		embeddingsWithInsecure + `["models.example.com"]}}`
	consoleOrigin = "https://console.example"
)

// stateOf returns the settings state behind the route, for tests that replace
// its limiters or clock before any request.
func (e *settingsEnv) stateOf(t *testing.T) *settingsState {
	t.Helper()
	e.s.agentMu.RLock()
	defer e.s.agentMu.RUnlock()
	st, _ := e.s.settingsExt.(*settingsState)
	require.NotNil(t, st)
	return st
}

// ---- L1: scheme rule on writes, scheme change is a host change for D4 ----

func TestSettingsWriteRefusesPlainHTTPToRemoteHost(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	etag, before := getSettings(t, e)
	for _, base := range []string{
		"http://models.example.com/v1",
		"http://llm.localhost/v1", // *.localhost is not loopback (I5)
		"http://10.0.0.7:8000/v1",
	} {
		resp := e.do(t, http.MethodPut, tokSettingsAdmin,
			`{"backend":{"base_url":"`+base+`"},"egress_confirmation":{"host":"x"}}`, map[string]string{"If-Match": etag})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, base)
		assert.Contains(t, bodyString(t, resp), "plain http only on a loopback host", base)
	}
	_, after := getSettings(t, e)
	assert.Equal(t, before, after, "a refused write changes nothing")
}

func TestSettingsWriteAcceptsPlainHTTPOnlyForListedHost(t *testing.T) {
	cfg := `{"llm":{"base_url":"http://127.0.0.1:59998/v1","model":"m"},` + embeddingsWithInsecure + `["llm.svc.cluster.local","other.svc:9000"]}}`
	e := newSettingsEnv(t, cfg)
	etag, _ := getSettings(t, e)

	// Exact match: a sibling name, a different port on a port-pinned entry and a
	// wildcard-looking host are not listed.
	for _, base := range []string{
		"http://evil.svc.cluster.local/v1", "http://other.svc:9001/v1", "http://sub.llm.svc.cluster.local/v1",
	} {
		resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"base_url":"`+base+`"}}`, map[string]string{"If-Match": etag})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, base)
	}
	resp := e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"base_url":"http://llm.svc.cluster.local:8000/v1"},"egress_confirmation":{"host":"llm.svc.cluster.local:8000"}}`,
		map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode, bodyString(t, resp))
	assert.Equal(t, "http://llm.svc.cluster.local:8000/v1", currentBaseURL(t, e))
}

func TestSettingsProbeAppliesTheSameSchemeRule(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	resp := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, `{"base_url":"http://llm.localhost/v1"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, bodyString(t, resp), "plain http only on a loopback host")
}

// A scheme change on an already acknowledged host is still a D4 event: the
// typed confirmation names the host, the refusal says why, and no confirmation
// is accepted when nothing changes.
func TestSettingsSchemeChangeNeedsConfirmationEvenWhenHostAcknowledged(t *testing.T) {
	e := newSettingsEnvWith(t, hardenedHTTPSConfig, "", adminVerifierWithAck(t, []string{"models.example.com"}))
	etag, before := getSettings(t, e)
	write := `{"backend":{"base_url":"http://models.example.com/v1"}`

	resp := e.do(t, http.MethodPut, tokSettingsAdmin, write+`}`, map[string]string{"If-Match": etag})
	details := requireSettingsError(t, resp, http.StatusConflict, "egress_confirmation_required")
	assert.Equal(t, "models.example.com", details["host"])
	_, after := getSettings(t, e)
	assert.Equal(t, before, after)

	wrong := e.do(t, http.MethodPut, tokSettingsAdmin, write+`,"egress_confirmation":{"host":"other.example.com"}}`, map[string]string{"If-Match": etag})
	requireSettingsError(t, wrong, http.StatusConflict, "egress_confirmation_required")

	resp = e.do(t, http.MethodPut, tokSettingsAdmin, write+`,"egress_confirmation":{"host":"models.example.com"}}`, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode, bodyString(t, resp))
	assert.Equal(t, "http://models.example.com/v1", currentBaseURL(t, e))

	// A path-only or model-only write on the same scheme and host needs none.
	etag2, _ := getSettings(t, e)
	resp = e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"model":"m2"}}`, map[string]string{"If-Match": etag2})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// A downgrade from https to http on the same hostname needs the typed
// confirmation whatever the port: a bare egress_ack host covers every port, so
// a port that differs must not turn the downgrade into an acknowledged move (N1).
func TestSettingsHTTPSDowngradeNeedsConfirmationWhateverThePort(t *testing.T) {
	for _, base := range []string{
		"http://models.example.com/v1",
		"http://models.example.com:80/v1",
		"http://models.example.com:8080/v1",
	} {
		t.Run(base, func(t *testing.T) {
			e := newSettingsEnvWith(t, hardenedHTTPSConfig, "", adminVerifierWithAck(t, []string{"models.example.com"}))
			etag, before := getSettings(t, e)
			write := `{"backend":{"base_url":"` + base + `"}`

			resp := e.do(t, http.MethodPut, tokSettingsAdmin, write+`}`, map[string]string{"If-Match": etag})
			requireSettingsError(t, resp, http.StatusConflict, "egress_confirmation_required")
			_, after := getSettings(t, e)
			assert.Equal(t, before, after, "the unconfirmed downgrade changed nothing")

			u, err := url.Parse(base)
			require.NoError(t, err)
			resp = e.do(t, http.MethodPut, tokSettingsAdmin, write+`,"egress_confirmation":{"host":"`+u.Host+`"}}`, map[string]string{"If-Match": etag})
			require.Equal(t, http.StatusOK, resp.StatusCode, bodyString(t, resp))
			assert.Equal(t, base, currentBaseURL(t, e))
		})
	}
}

// ---- L2: audit line ----

func settingsAuditLines(e *settingsEnv) []string {
	var out []string
	for _, l := range strings.Split(e.logs.String(), "\n") {
		if strings.Contains(l, "agent settings applied") {
			out = append(out, l)
		}
	}
	return out
}

func TestSettingsWriteAuditLine(t *testing.T) {
	dir := t.TempDir()
	e := newSettingsEnvDir(t, settingsBackendJSON, dir)
	etag, _ := getSettings(t, e)

	// A model-only write is Info, names the principal and the field.
	resp := e.do(t, http.MethodPut, tokSettingsAdmin, `{"backend":{"model":"m-private-model"}}`, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	etag = resp.Header.Get("ETag")
	lines := settingsAuditLines(e)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "level=INFO")
	assert.Contains(t, lines[0], "principal=settings-admin")
	assert.Contains(t, lines[0], "fields=model")
	assert.Contains(t, lines[0], "key_not_carried=false")

	// Store a key, then move the host: Warn, old and new host, key_not_carried.
	resp = e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"api_key":"`+settingsTestKey+`"},"system_prompt":"a private system prompt"}`, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode, bodyString(t, resp))
	etag = resp.Header.Get("ETag")
	resp = e.do(t, http.MethodPut, tokSettingsAdmin,
		`{"backend":{"base_url":"http://127.0.0.1:59997/v1"},"egress_confirmation":{"host":"127.0.0.1:59997"}}`, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, resp.StatusCode, bodyString(t, resp))

	lines = settingsAuditLines(e)
	require.Len(t, lines, 3)
	move := lines[2]
	for _, want := range []string{
		"level=WARN", "principal=settings-admin", "fields=base_url,egress_ack",
		"old_host=127.0.0.1:59998", "new_host=127.0.0.1:59997", "key_not_carried=true",
	} {
		assert.Contains(t, move, want)
	}
	logs := e.logs.String()
	for _, leaked := range []string{settingsTestKey, "a private system prompt", "m-private-model", "/v1"} {
		assert.NotContains(t, logs, leaked, "the audit trail must not carry %q", leaked)
	}
}

// ---- L3: strict CORS over agent.origin_allowlist ----

func (e *settingsEnv) withOrigin(t *testing.T, method, token, origin string) *http.Response {
	t.Helper()
	return e.do(t, method, token, "", map[string]string{"Origin": origin})
}

func TestSettingsRouteStrictCORS(t *testing.T) {
	cfg := `{"agent":{"origin_allowlist":["` + consoleOrigin + `"]},"llm":{"base_url":"http://127.0.0.1:59998/v1","model":"m"}}`
	e := newSettingsEnv(t, cfg)

	pre := e.withOrigin(t, http.MethodOptions, "", consoleOrigin)
	require.Equal(t, http.StatusNoContent, pre.StatusCode)
	hd := pre.Header
	assert.Equal(t, consoleOrigin, hd.Get("Access-Control-Allow-Origin"), "the origin is echoed, never *")
	assert.Equal(t, "GET, PUT, POST, OPTIONS", hd.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "authorization, content-type, accept, if-match", hd.Get("Access-Control-Allow-Headers"))
	assert.Contains(t, hd.Get("Access-Control-Expose-Headers"), "ETag")
	assert.Empty(t, hd.Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, hd.Values("Vary"), "Origin")

	get := e.withOrigin(t, http.MethodGet, tokSettingsAdmin, consoleOrigin)
	require.Equal(t, http.StatusOK, get.StatusCode)
	assert.Equal(t, consoleOrigin, get.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, get.Header.Get("Access-Control-Expose-Headers"), "ETag")
	assert.NotEmpty(t, get.Header.Get("ETag"))

	// Anything not on the route's own list is refused, the listener's own origin
	// included, and a valid token does not change that.
	for _, origin := range []string{"https://evil.example", strings.TrimPrefix(e.base, "")} {
		resp := e.withOrigin(t, http.MethodGet, tokSettingsAdmin, origin)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, origin)
		assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"), origin)
	}
	assert.Equal(t, http.StatusForbidden, e.withOrigin(t, http.MethodOptions, "", "https://evil.example").StatusCode)
}

func TestSettingsRouteWithoutAllowlistRefusesEveryBrowserOrigin(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	assert.Equal(t, http.StatusForbidden, e.withOrigin(t, http.MethodGet, tokSettingsAdmin, consoleOrigin).StatusCode)
	assert.Equal(t, http.StatusForbidden, e.withOrigin(t, http.MethodOptions, "", consoleOrigin).StatusCode)
	assert.Equal(t, http.StatusOK, e.do(t, http.MethodGet, tokSettingsAdmin, "", nil).StatusCode, "a client without an Origin is not a browser")
}

// ---- L4: rate and subscriber limits ----

type stepClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *stepClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestSettingsRouteRateLimit429(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	st := e.stateOf(t)
	clock := &stepClock{t: time.Unix(1_000_000, 0)}
	st.route = newSettingsLimiter(2, 60, 0)
	st.route.now = clock.now

	assert.Equal(t, http.StatusOK, e.do(t, http.MethodGet, tokSettingsAdmin, "", nil).StatusCode)
	assert.Equal(t, http.StatusOK, e.do(t, http.MethodGet, tokSettingsAdmin, "", nil).StatusCode)
	resp := e.do(t, http.MethodGet, tokSettingsAdmin, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "1", resp.Header.Get("Retry-After"))

	// A refused method does not spend a token, and time refills the bucket.
	clock.advance(2 * time.Second)
	assert.Equal(t, http.StatusOK, e.do(t, http.MethodGet, tokSettingsAdmin, "", nil).StatusCode)
}

func TestSettingsProbeRateAndConcurrencyLimit(t *testing.T) {
	e := newSettingsEnv(t, settingsBackendJSON)
	st := e.stateOf(t)
	clock := &stepClock{t: time.Unix(1_000_000, 0)}
	st.probe = newSettingsLimiter(1, 60, 1)
	st.probe.now = clock.now

	body := `{"base_url":"http://127.0.0.1:1/v1"}`
	first := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, body)
	require.Equal(t, http.StatusOK, first.StatusCode, "the probe runs (and fails to connect) once")
	second := e.doPath(t, http.MethodPost, RouteSettingsTestConnection, tokSettingsAdmin, body)
	require.Equal(t, http.StatusTooManyRequests, second.StatusCode)
	assert.NotEmpty(t, second.Header.Get("Retry-After"))
	models := e.do(t, http.MethodGet, tokSettingsAdmin, "", nil)
	assert.Equal(t, http.StatusOK, models.StatusCode, "the route bucket is separate from the probe bucket")
}

func TestSettingsLimiterIsPerPrincipalAndCapsConcurrency(t *testing.T) {
	l := newSettingsLimiter(1, 60, 1)
	clock := &stepClock{t: time.Unix(1_000_000, 0)}
	l.now = clock.now

	done, retry := l.take("a")
	require.NotNil(t, done)
	assert.Zero(t, retry)
	_, retry = l.take("a")
	assert.Positive(t, retry, "a principal with a probe in flight is refused")
	done2, retry2 := l.take("b")
	require.NotNil(t, done2, "another principal is unaffected")
	assert.Zero(t, retry2)
	done()
	done2()

	_, retry = l.take("a")
	assert.Positive(t, retry, "the token spent by the first probe is not back yet")
	clock.advance(time.Second)
	again, retry := l.take("a")
	require.NotNil(t, again)
	assert.Zero(t, retry)
	again()
}

// A refusal whose computed wait rounds to zero is still a refusal: it never
// returns a nil release function together with a zero retry (N3).
func TestSettingsLimiterRefusalNeverRoundsToZero(t *testing.T) {
	l := newSettingsLimiter(1, 1, 2)
	clock := &stepClock{t: time.Unix(1_000_000, 0)}
	l.now = clock.now
	l.buckets["a"] = &limiterBucket{tokens: 1 - 1e-12, last: clock.now()}

	done, retry := l.take("a")
	assert.Nil(t, done)
	assert.Positive(t, retry, "a token short by 1e-12 is a refusal with a positive retry")
}

func TestSettingsEventSubscriberCap(t *testing.T) {
	b := newSettingsEvents()
	b.max = 2
	c1, ok := b.subscribe()
	require.True(t, ok)
	_, ok = b.subscribe()
	require.True(t, ok)
	_, ok = b.subscribe()
	assert.False(t, ok, "the cap refuses the third stream")
	b.unsubscribe(c1)
	_, ok = b.subscribe()
	assert.True(t, ok, "a closed stream frees its slot")

	e := newSettingsEnv(t, settingsBackendJSON)
	e.stateOf(t).events.max = 1
	stream, resp := e.openEvents(t, tokSettingsAdmin)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = stream.nextEvent(t)
	_, refused := e.openEvents(t, tokSettingsAdmin)
	require.Equal(t, http.StatusTooManyRequests, refused.StatusCode)
	assert.Equal(t, "5", refused.Header.Get("Retry-After"))
}
