package server

import (
	"sync"
	"time"
)

// This file is built with and without the noagent tag, like index_routes.go. It
// is the abuse limit of POST /index/search (MCP-03): a token bucket per identity
// and a small cap on searches in flight, so one browser tab or token cannot
// drive the embedder and the vector store without bound.

const (
	// searchBurst is how many searches an identity may start at once.
	searchBurst = 10
	// searchRefill is how often an identity earns one more search: 60 a minute.
	searchRefill = time.Second
	// searchMaxInFlight bounds the searches running at the same time, across all
	// identities.
	searchMaxInFlight = 8
	// searchMaxPerIdentity bounds the searches one identity has running.
	searchMaxPerIdentity = 2
	// searchBusyRetry is the Retry-After of a search refused for being too many
	// in flight.
	searchBusyRetry = time.Second
)

type searchBucket struct {
	tokens   float64
	last     time.Time
	inflight int
}

type searchLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	buckets  map[string]*searchBucket
	inflight int
}

func newSearchLimiter(now func() time.Time) *searchLimiter {
	return &searchLimiter{now: now, buckets: map[string]*searchBucket{}}
}

// acquire admits one search for the identity. It returns a release function the
// caller defers, or nil and how long to wait before trying again.
func (l *searchLimiter) acquire(identity string) (release func(), retry time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.buckets[identity]
	if b == nil {
		b = &searchBucket{tokens: searchBurst, last: now}
		l.buckets[identity] = b
	}
	b.tokens = min(searchBurst, b.tokens+float64(now.Sub(b.last))/float64(searchRefill))
	b.last = now
	if l.inflight >= searchMaxInFlight || b.inflight >= searchMaxPerIdentity {
		return nil, searchBusyRetry
	}
	if b.tokens < 1 {
		return nil, time.Duration((1 - b.tokens) * float64(searchRefill))
	}
	b.tokens--
	b.inflight++
	l.inflight++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		b.inflight--
		l.inflight--
	}, 0
}
