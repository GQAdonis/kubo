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
// backoff allows, so a dead store costs one failed request per interval and
// not one per call.
const (
	storeRetryInitial = 2 * time.Second
	storeRetryMax     = time.Minute
	// startupStoreTimeout bounds the one attempt Open makes on a remote store.
	startupStoreTimeout = 10 * time.Second
)

// ErrStoreUnavailable is returned by every store call while the store has not
// been reached and its schema defined. The wrapped error is the last cause.
var ErrStoreUnavailable = errors.New("embeddings store is unavailable")

// lazyStore is the Store of a Service whose backing store may not be up yet.
// It runs open, which starts the embedded store if needed and defines the
// schema, on the first call that needs the store, and again after a failure
// once the backoff has passed. open succeeds exactly once: the schema is
// defined once, then every call is forwarded. While it has not succeeded every
// call fails closed with ErrStoreUnavailable; nothing is answered from partial
// state.
type lazyStore struct {
	open func(ctx context.Context) (Store, error)
	now  func() time.Time

	mu       sync.Mutex
	inner    Store
	lastErr  error
	failures int
	retryAt  time.Time
	closed   bool
}

func newLazyStore(open func(ctx context.Context) (Store, error)) *lazyStore {
	return &lazyStore{open: open, now: time.Now}
}

// get returns the open store, trying to open it when the backoff allows.
func (l *lazyStore) get(ctx context.Context) (Store, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inner != nil {
		return l.inner, nil
	}
	if l.closed {
		return nil, fmt.Errorf("%w: the service is closed", ErrStoreUnavailable)
	}
	if l.failures > 0 && l.now().Before(l.retryAt) {
		return nil, fmt.Errorf("%w: %w", ErrStoreUnavailable, l.lastErr)
	}
	st, err := l.open(ctx)
	if err != nil {
		l.failures++
		l.lastErr = err
		l.retryAt = l.now().Add(min(storeRetryInitial<<(l.failures-1), storeRetryMax))
		return nil, fmt.Errorf("%w: %w", ErrStoreUnavailable, err)
	}
	l.inner, l.lastErr, l.failures = st, nil, 0
	return st, nil
}

// ready reports whether the store is open, trying to open it when needed.
func (l *lazyStore) ready(ctx context.Context) error {
	_, err := l.get(ctx)
	return err
}

// close stops further attempts. It waits for one in progress.
func (l *lazyStore) close() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
}

// Bootstrap implements Store: it opens the store.
func (l *lazyStore) Bootstrap(ctx context.Context) error { return l.ready(ctx) }

// Replace implements Store.
func (l *lazyStore) Replace(ctx context.Context, cid string, rows []Row) error {
	st, err := l.get(ctx)
	if err != nil {
		return err
	}
	return st.Replace(ctx, cid, rows)
}

// Search implements Store.
func (l *lazyStore) Search(ctx context.Context, vector []float32, k int) ([]Hit, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	return st.Search(ctx, vector, k)
}

// DeleteCID implements Store.
func (l *lazyStore) DeleteCID(ctx context.Context, cid string) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	return st.DeleteCID(ctx, cid)
}

// Count implements Store.
func (l *lazyStore) Count(ctx context.Context) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	return st.Count(ctx)
}

// SearchCID implements Store.
func (l *lazyStore) SearchCID(ctx context.Context, vector []float32, cid string, k int) ([]Hit, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	return st.SearchCID(ctx, vector, cid, k)
}

// CountCID implements Store.
func (l *lazyStore) CountCID(ctx context.Context, cid string) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	return st.CountCID(ctx, cid)
}

// CountCIDs implements Store.
func (l *lazyStore) CountCIDs(ctx context.Context) (int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return 0, err
	}
	return st.CountCIDs(ctx)
}

// CountCIDsByKind implements Store.
func (l *lazyStore) CountCIDsByKind(ctx context.Context) (map[string]int, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	return st.CountCIDsByKind(ctx)
}

// ListCIDs implements Store.
func (l *lazyStore) ListCIDs(ctx context.Context, after string, limit int) ([]string, error) {
	st, err := l.get(ctx)
	if err != nil {
		return nil, err
	}
	return st.ListCIDs(ctx, after, limit)
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
	return rs.SaveRun(ctx, run)
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
	return rs.LoadRun(ctx)
}
