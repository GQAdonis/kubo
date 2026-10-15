//go:build !noagent

package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Limits of the settings surface (MCP-03). The route is admin-only and the
// admin token is 256 bits, so these bound a runaway or compromised client, not
// a crowd: a probe can hold 10 s of outbound I/O, and a stream holds a
// connection and a goroutine.
const (
	// settingsRouteBurst and settingsRoutePerMinute are the per-principal token
	// bucket of the route itself (read, write, change stream).
	settingsRouteBurst     = 60
	settingsRoutePerMinute = 60
	// settingsProbeBurst and settingsProbePerMinute are the tighter bucket of
	// test-connection and model-list, each of which dials a host.
	settingsProbeBurst     = 10
	settingsProbePerMinute = 10
	// settingsProbeMaxInFlight is the number of probes one principal may have
	// running at once.
	settingsProbeMaxInFlight = 2
	// maxSettingsSubscribers is the cap on open change streams.
	maxSettingsSubscribers = 16
	// settingsStreamRetry is the Retry-After of a refused stream.
	settingsStreamRetry = 5 * time.Second
)

// settingsLimiter is a per-principal token bucket with an optional cap on
// concurrent holders. The zero value is not usable; use newSettingsLimiter.
type settingsLimiter struct {
	mu        sync.Mutex
	now       func() time.Time
	burst     float64
	perSecond float64
	maxActive int
	buckets   map[string]*limiterBucket
}

type limiterBucket struct {
	tokens float64
	last   time.Time
	active int
}

// newSettingsLimiter builds a limiter that holds burst tokens per principal and
// refills at perMinute. maxActive 0 means no cap on concurrent holders.
func newSettingsLimiter(burst, perMinute, maxActive int) *settingsLimiter {
	return &settingsLimiter{
		now:       time.Now,
		burst:     float64(burst),
		perSecond: float64(perMinute) / 60,
		maxActive: maxActive,
		buckets:   map[string]*limiterBucket{},
	}
}

// take charges one token to the principal. On refusal it says when to retry;
// otherwise it returns the function that ends the hold (a no-op when the
// limiter has no concurrency cap). Call the function exactly once.
func (l *settingsLimiter) take(principal string) (done func(), retry time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	// A bucket that has refilled completely and holds nothing is
	// indistinguishable from a principal never seen: drop it.
	for p, b := range l.buckets {
		if b.active == 0 && b.tokens+now.Sub(b.last).Seconds()*l.perSecond >= l.burst {
			delete(l.buckets, p)
		}
	}
	b, ok := l.buckets[principal]
	if !ok {
		b = &limiterBucket{tokens: l.burst, last: now}
		l.buckets[principal] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(l.burst, b.tokens+elapsed*l.perSecond)
	}
	b.last = now
	if l.maxActive > 0 && b.active >= l.maxActive {
		return nil, settingsStreamRetry
	}
	if b.tokens < 1 {
		return nil, time.Duration((1 - b.tokens) / l.perSecond * float64(time.Second))
	}
	b.tokens--
	if l.maxActive == 0 {
		return func() {}, 0
	}
	b.active++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		b.active--
	}, 0
}

// refuseRate answers 429 with Retry-After, rounded up to whole seconds and at
// least one.
func refuseRate(w http.ResponseWriter, retry time.Duration) {
	secs := max(int((retry+time.Second-1)/time.Second), 1)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	http.Error(w, "too many settings requests; retry later", http.StatusTooManyRequests)
}

// settingsCORS is the strict CORS answer of the settings route
// (agent.origin_allowlist, ADR-0009 section 4): the page origins listed there
// and no others, exactly these methods and request headers, ETag readable so a
// page can send If-Match, and no credentials header.
var settingsCORS = CORSPolicy{
	Methods: []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodOptions},
	Headers: []string{"authorization", "content-type", "accept", "if-match"},
	Expose:  []string{"ETag", "Retry-After"},
}
