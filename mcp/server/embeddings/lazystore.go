package embeddings

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Retry schedule of the store connection. A store that is down at startup, or
// goes down later, is tried again on use, but never more often than the
// backoff allows, so a dead store costs one failed open per interval and not
// one per call.
const (
	storeRetryInitial = 2 * time.Second
	storeRetryMax     = time.Minute
	// startupStoreTimeout bounds one open of a remote store.
	startupStoreTimeout = 10 * time.Second
	// embeddedOpenTimeout bounds one open of the embedded store: the start
	// budget of the process (its health wait) and the schema definition after it.
	embeddedOpenTimeout = startTimeout + startupStoreTimeout
)

// ErrStoreUnavailable is returned by every store call while the store has not
// been reached and its schema defined, or after it exited. The wrapped error is
// the last cause.
var ErrStoreUnavailable = errors.New("embeddings store is unavailable")

// errOpening: the store is being opened and the caller stopped waiting for it.
var errOpening = errors.New("the store is still opening")

// exiter is implemented by a store that knows its backing process has ended.
type exiter interface{ exited() bool }

// lazyStore is the Store of a Service whose backing store may not be up yet.
// It runs open, which starts the embedded store if needed and defines the
// schema, on the first call that needs the store, and again after a failure
// once the backoff has passed, or after the store exited. Concurrent callers
// share one open in flight. The open runs under its own deadline and not under
// a caller's context, so a caller that gives up neither fails the open nor
// advances the backoff; each caller waits for it under its own context. While
// no open has succeeded every call fails closed with ErrStoreUnavailable;
// nothing is answered from partial state. A store that exited after opening is
// dropped, so the next use opens it again (replacing the embedded process),
// subject to the backoff.
type lazyStore struct {
	open    func(ctx context.Context) (Store, error)
	timeout time.Duration
	now     func() time.Time

	mu       sync.Mutex
	inner    Store
	inflight *openCall
	lastErr  error
	failures int
	retryAt  time.Time
	closed   bool
}

// openCall is one open in flight. done is closed when st and err are set.
type openCall struct {
	done   chan struct{}
	cancel context.CancelFunc
	st     Store
	err    error
}

func newLazyStore(open func(ctx context.Context) (Store, error), timeout time.Duration) *lazyStore {
	return &lazyStore{open: open, timeout: timeout, now: time.Now}
}

// get returns the open store, starting an open when the backoff allows and
// waiting for the one in flight under ctx.
func (l *lazyStore) get(ctx context.Context) (Store, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, fmt.Errorf("%w: the service is closed", ErrStoreUnavailable)
	}
	if st := l.inner; st != nil {
		if ex, ok := st.(exiter); !ok || !ex.exited() {
			l.mu.Unlock()
			return st, nil
		}
		l.dropLocked(st)
	}
	c := l.inflight
	if c == nil {
		if l.failures > 0 && l.now().Before(l.retryAt) {
			err := l.lastErr
			l.mu.Unlock()
			return nil, fmt.Errorf("%w: %w", ErrStoreUnavailable, err)
		}
		// The open must outlive this caller and carry its own deadline.
		octx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.timeout)
		c = &openCall{done: make(chan struct{}), cancel: cancel}
		l.inflight = c
		go l.run(octx, c)
	}
	l.mu.Unlock()
	select {
	case <-c.done:
		return c.st, c.err
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w: %w", ErrStoreUnavailable, errOpening, ctx.Err())
	}
}

// run performs one open and publishes its result.
func (l *lazyStore) run(ctx context.Context, c *openCall) {
	st, err := l.open(ctx)
	c.cancel()
	l.mu.Lock()
	l.inflight = nil
	if err != nil {
		l.failures++
		l.lastErr = err
		l.retryAt = l.now().Add(min(storeRetryInitial<<(l.failures-1), storeRetryMax))
		c.err = fmt.Errorf("%w: %w", ErrStoreUnavailable, err)
	} else {
		l.inner, l.lastErr, l.failures = st, nil, 0
		c.st = st
	}
	l.mu.Unlock()
	close(c.done)
}

// dropLocked forgets st, which exited, so the next use opens a store again once
// the initial backoff has passed. l.mu is held.
func (l *lazyStore) dropLocked(st Store) {
	if l.inner != st {
		return
	}
	l.inner = nil
	l.lastErr = ErrStoreExited
	l.failures = 1
	l.retryAt = l.now().Add(storeRetryInitial)
}

// note reports err of a call on st: a store that exited is dropped.
func (l *lazyStore) note(st Store, err error) error {
	if errors.Is(err, ErrStoreExited) {
		l.mu.Lock()
		l.dropLocked(st)
		l.mu.Unlock()
	}
	return err
}

// ready reports whether the store is open, trying to open it when needed.
func (l *lazyStore) ready(ctx context.Context) error {
	_, err := l.get(ctx)
	return err
}

// close stops further attempts and ends one in progress.
func (l *lazyStore) close() {
	l.mu.Lock()
	l.closed = true
	c := l.inflight
	l.mu.Unlock()
	if c != nil {
		c.cancel()
		<-c.done
	}
}

// Bootstrap implements Store: it opens the store.
func (l *lazyStore) Bootstrap(ctx context.Context) error { return l.ready(ctx) }

// Replace implements Store.
func (l *lazyStore) Replace(ctx context.Context, cid string, rows []Row) error {
	st, err := l.get(ctx)
	if err != nil {
		return err
	}
	return l.note(st, st.Replace(ctx, cid, rows))
}

// Search implements Store.
func (l *lazyStore) Search(ctx context.Context, vector []float32, k int) ([]Hit, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	hits, err := st.Search(ctx, vector, k)
	return hits, l.note(st, err)
}

// DeleteCID implements Store.
func (l *lazyStore) DeleteCID(ctx context.Context, cid string) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	n, err := st.DeleteCID(ctx, cid)
	return n, l.note(st, err)
}

// Count implements Store.
func (l *lazyStore) Count(ctx context.Context) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	n, err := st.Count(ctx)
	return n, l.note(st, err)
}

// SearchCID implements Store.
func (l *lazyStore) SearchCID(ctx context.Context, vector []float32, cid string, k int) ([]Hit, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	hits, err := st.SearchCID(ctx, vector, cid, k)
	return hits, l.note(st, err)
}

// CountCID implements Store.
func (l *lazyStore) CountCID(ctx context.Context, cid string) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	n, err := st.CountCID(ctx, cid)
	return n, l.note(st, err)
}

// CountCIDs implements Store.
func (l *lazyStore) CountCIDs(ctx context.Context) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	n, err := st.CountCIDs(ctx)
	return n, l.note(st, err)
}

// CountCIDsByKind implements Store.
func (l *lazyStore) CountCIDsByKind(ctx context.Context) (map[string]int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	m, err := st.CountCIDsByKind(ctx)
	return m, l.note(st, err)
}

// ListCIDs implements Store.
func (l *lazyStore) ListCIDs(ctx context.Context, after string, limit int) ([]string, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := st.ListCIDs(ctx, after, limit)
	return ids, l.note(st, err)
}

// SaveRun implements RunStore.
func (l *lazyStore) SaveRun(ctx context.Context, run Run) error {
	st, err := l.get(ctx)
	if err != nil {
		return err
	}
	rs, ok := st.(RunStore)
	if !ok {
		return errors.New("the store keeps no bulk run state")
	}
	return l.note(st, rs.SaveRun(ctx, run))
}

// LoadRun implements RunStore.
func (l *lazyStore) LoadRun(ctx context.Context) (*Run, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	rs, ok := st.(RunStore)
	if !ok {
		return nil, errors.New("the store keeps no bulk run state")
	}
	run, err := rs.LoadRun(ctx)
	return run, l.note(st, err)
}
