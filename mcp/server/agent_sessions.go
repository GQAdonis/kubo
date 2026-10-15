//go:build !noagent

package server

import (
	"errors"
	"time"
)

// Session table bounds. The global cap bounds memory; the per-principal quota
// and the run rate keep one caller from using up the table or the operator's
// model key for everyone else.
const (
	maxSessions = 64
	// maxSessionsPerPrincipal is the number of open sessions one principal may
	// hold. A principal at its quota evicts its own least recently used idle
	// session; it never evicts another principal's.
	maxSessionsPerPrincipal = 16
	sessionIdleTTL          = 30 * time.Minute
	maxSessionMessages      = 256
	maxSessionIDLen         = 128

	// defaultRunBurst and defaultRunsPerMinute are the per-principal run rate: a
	// token bucket that holds defaultRunBurst runs and refills at
	// defaultRunsPerMinute.
	defaultRunBurst      = 120
	defaultRunsPerMinute = 120
)

// ErrRateLimited is the error of a run refused by the per-principal run rate.
// The concrete error is a *RateLimitedError that says when to retry.
var ErrRateLimited = errors.New("too many agent runs; retry later")

// RateLimitedError reports a refused run and when the principal may try again.
// Transports answer 429 with Retry-After.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string { return ErrRateLimited.Error() }

// Is makes errors.Is(err, ErrRateLimited) true.
func (e *RateLimitedError) Is(target error) bool { return target == ErrRateLimited }

// RetryAfterSeconds is RetryAfter rounded up to whole seconds, at least one.
func (e *RateLimitedError) RetryAfterSeconds() int {
	secs := int((e.RetryAfter + time.Second - 1) / time.Second)
	return max(secs, 1)
}

// runBucket is the token bucket of one principal.
type runBucket struct {
	tokens float64
	last   time.Time
}

// agentSession is the in-memory state of one conversation. It is never
// persisted.
type agentSession struct {
	id        string
	principal string
	touched   time.Time
	busy      bool

	messages []ChatMessage
	// queue holds the tool calls of the last model reply that have no answer
	// yet, in order. The first one may be waiting on interrupt.
	queue     []queuedCall
	interrupt *Interrupt
	// snap retains the backend snapshot of the run that opened interrupt, with
	// one snapshot reference held, so a resume after a backend swap still talks
	// to the backend the run started with. nil whenever interrupt is nil.
	snap *agentSnapshot
	// backendChanged is set when a swap revoked snap (the endpoint or key moved
	// while the interrupt was open). A resume then fails with ErrBackendChanged
	// instead of ErrNoOpenInterrupt. Cleared when the interrupt ends.
	backendChanged bool
}

// dropSnapshot releases the session's retained snapshot, if any. Callers hold
// a.mu, or the session is being discarded.
func (s *agentSession) dropSnapshot() {
	if s.snap != nil {
		s.snap.release()
		s.snap = nil
	}
}

type queuedCall struct {
	call ChatToolCall
	// skip, when set, answers the call with this error instead of running it.
	skip string
}

// takeRun charges one run to the principal's bucket, or says when to retry.
// Callers hold a.mu.
func (a *Agent) takeRun(principal string, now time.Time) error {
	burst, perMinute := a.runBurst, a.runsPerMinute
	if burst <= 0 {
		burst = defaultRunBurst
	}
	if perMinute <= 0 {
		perMinute = defaultRunsPerMinute
	}
	perSecond := float64(perMinute) / 60
	b, ok := a.buckets[principal]
	if !ok {
		b = &runBucket{tokens: float64(burst), last: now}
		a.buckets[principal] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(float64(burst), b.tokens+elapsed*perSecond)
	}
	b.last = now
	if b.tokens < 1 {
		wait := time.Duration((1 - b.tokens) / perSecond * float64(time.Second))
		return &RateLimitedError{RetryAfter: wait}
	}
	b.tokens--
	return nil
}

// pruneBuckets drops the buckets that have refilled completely, which are
// indistinguishable from a principal never seen. Callers hold a.mu.
func (a *Agent) pruneBuckets(now time.Time) {
	burst, perMinute := a.runBurst, a.runsPerMinute
	if burst <= 0 {
		burst = defaultRunBurst
	}
	if perMinute <= 0 {
		perMinute = defaultRunsPerMinute
	}
	perSecond := float64(perMinute) / 60
	for p, b := range a.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*perSecond >= float64(burst) {
			delete(a.buckets, p)
		}
	}
}

// acquire finds or creates the session and marks it busy. It charges the run
// to the principal's rate before it touches the table.
func (a *Agent) acquire(sessionID, principal string, create bool) (*agentSession, func(), error) {
	if len(sessionID) > maxSessionIDLen {
		return nil, nil, ErrSessionNotFound
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	for id, s := range a.sessions {
		if !s.busy && now.Sub(s.touched) > sessionIdleTTL {
			s.dropSnapshot()
			delete(a.sessions, id)
		}
	}
	a.pruneBuckets(now)
	s, ok := a.sessions[sessionID]
	switch {
	case ok && s.principal != principal:
		// Do not reveal that the ID exists for someone else.
		return nil, nil, ErrSessionNotFound
	case ok && s.busy:
		return nil, nil, ErrSessionBusy
	case !ok && !create:
		return nil, nil, ErrSessionNotFound
	}
	if err := a.takeRun(principal, now); err != nil {
		return nil, nil, err
	}
	if !ok {
		if err := a.makeRoom(principal); err != nil {
			return nil, nil, err
		}
		if sessionID == "" {
			sessionID = randomID(12)
		}
		s = &agentSession{id: sessionID, principal: principal}
		a.sessions[sessionID] = s
	}
	s.busy, s.touched = true, now
	return s, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		s.busy, s.touched = false, a.now()
	}, nil
}

// makeRoom frees a slot for a new session of the principal. It evicts only the
// principal's own idle sessions: at its per-principal quota, or, when the table
// is full, the principal's oldest. A full table that the principal cannot free
// by itself is ErrTooManySessions; another principal's conversation is never
// the price of a new one. Callers hold a.mu.
func (a *Agent) makeRoom(principal string) error {
	own := 0
	for _, s := range a.sessions {
		if s.principal == principal {
			own++
		}
	}
	if own >= maxSessionsPerPrincipal || len(a.sessions) >= maxSessions {
		if !a.evictOldestIdle(principal) {
			return ErrTooManySessions
		}
	}
	return nil
}

// OpenInterrupt returns the interrupt the session is waiting on, for the
// session's owner only. A transport must call it, and match the interrupt ID the
// client names against the result, before it records any decision for the
// confirmation: it is what keeps a caller from answering another principal's
// or another thread's pending confirmation. A session that does not exist, has
// expired or belongs to someone else is ErrSessionNotFound, without saying which.
func (a *Agent) OpenInterrupt(id Identity, sessionID string) (*Interrupt, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[sessionID]
	if !ok || s.principal != id.ID {
		return nil, ErrSessionNotFound
	}
	if s.busy {
		return nil, ErrSessionBusy
	}
	if a.now().Sub(s.touched) > sessionIdleTTL {
		return nil, ErrSessionNotFound
	}
	if s.interrupt == nil {
		return nil, ErrNoOpenInterrupt
	}
	cp := *s.interrupt
	cp.Resolvers = append([]string(nil), cp.Resolvers...)
	return &cp, nil
}

// evictOldestIdle drops the principal's least recently used idle session. It
// reports whether it dropped one. Callers hold a.mu.
func (a *Agent) evictOldestIdle(principal string) bool {
	oldest := ""
	for id, s := range a.sessions {
		if s.busy || s.principal != principal {
			continue
		}
		if oldest == "" || s.touched.Before(a.sessions[oldest].touched) {
			oldest = id
		}
	}
	if oldest == "" {
		return false
	}
	a.sessions[oldest].dropSnapshot()
	delete(a.sessions, oldest)
	return true
}
