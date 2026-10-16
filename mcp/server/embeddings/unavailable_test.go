package embeddings

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// closedAddr returns a loopback host:port that nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// flakyStore is a fake SurrealDB that drops connections while down and counts
// the schema definitions it receives.
type flakyStore struct {
	*httptest.Server
	up      atomic.Bool
	defines atomic.Int32
	t       *testing.T
}

func newFlakyStore(t *testing.T) *flakyStore {
	t.Helper()
	f := &flakyStore{t: t}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.up.Load() {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		b := new(strings.Builder)
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		body := b.String()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body, "INFO FOR TABLE"):
			_, _ = w.Write([]byte(fixture(t, "rpc_info_table_dim4.json")))
		case strings.Contains(body, "DEFINE TABLE IF NOT EXISTS "+chunkTable):
			f.defines.Add(1)
			_, _ = w.Write([]byte(fixture(t, "rpc_ddl_ok.json")))
		case strings.Contains(body, "DEFINE"):
			_, _ = w.Write([]byte(fixture(t, "rpc_ddl_ok.json")))
		default:
			_, _ = w.Write([]byte(fixture(t, "rpc_count_empty.json")))
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func remoteSettings(store, embedder string) Settings {
	return Settings{
		Mode:     ModeRemote,
		Remote:   &RemoteSettings{Endpoint: "http://" + store, Namespace: "ipfs", Database: "idx"},
		Embedder: EmbedderSettings{BaseURL: "http://" + embedder + "/v1", Model: "bge-m3", Dimensions: 4},
	}
}

func TestOpenStartsWithUnreachableStoreEmbedderAndExtractor(t *testing.T) {
	t.Parallel()
	s := remoteSettings(closedAddr(t), closedAddr(t))
	s.Extractor = &ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: "http://" + closedAddr(t)}}

	svc, err := Open(t.Context(), s, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })

	stages := svc.Stages(t.Context(), allHosts)
	for _, name := range []string{StageStore, StageEmbedder, StageExtractor} {
		require.Contains(t, stages, name)
		require.False(t, stages[name].Available, name)
		require.NotEmpty(t, stages[name].Reason, name)
	}
	require.ErrorIs(t, svc.Ready(t.Context()), ErrStoreUnavailable)
}

func TestOpenSchemaDefinedOnceWhenStoreComesUp(t *testing.T) {
	t.Parallel()
	f := newFlakyStore(t)
	svc, err := Open(t.Context(), remoteSettings(strings.TrimPrefix(f.URL, "http://"), closedAddr(t)), Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	require.Zero(t, f.defines.Load())
	require.False(t, svc.Stages(t.Context(), allHosts)[StageStore].Available)

	// The store comes up; the retry schedule lets the next call through.
	f.up.Store(true)
	now := time.Now()
	svc.lazy.now = func() time.Time { return now.Add(storeRetryMax + time.Second) }
	require.NoError(t, svc.Ready(t.Context()))
	require.True(t, svc.Stages(t.Context(), allHosts)[StageStore].Available)
	n, err := svc.Store().Count(t.Context())
	require.NoError(t, err)
	require.Zero(t, n)
	for range 3 {
		require.NoError(t, svc.Ready(t.Context()))
	}
	require.EqualValues(t, 1, f.defines.Load(), "the schema is defined exactly once")
}

func TestUnavailableStoreFailsClosed(t *testing.T) {
	t.Parallel()
	emb := newReplayServer(t, reply{match: "bge-m3", body: `{"data":[{"embedding":[1,0,0,0],"index":0}]}`})
	s := remoteSettings(closedAddr(t), strings.TrimPrefix(emb.URL, "http://"))
	svc, err := Open(t.Context(), s, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	all := func(string) bool { return true }

	hits, err := svc.Search(t.Context(), all, "query", 3)
	require.ErrorIs(t, err, ErrStoreUnavailable)
	require.Empty(t, hits)
	_, err = svc.Index(t.Context(), all, Document{CID: "bafyA", Chunks: []string{"one"}})
	require.ErrorIs(t, err, ErrStoreUnavailable)
	_, err = svc.Store().DeleteCID(t.Context(), "bafyA")
	require.ErrorIs(t, err, ErrStoreUnavailable)
	require.Empty(t, emb.requests(), "no text reaches the embedder while the store is down")
}

func TestUnavailableStoreIsRetriedNoOftenerThanTheBackoff(t *testing.T) {
	t.Parallel()
	f := newFlakyStore(t)
	svc, err := Open(t.Context(), remoteSettings(strings.TrimPrefix(f.URL, "http://"), closedAddr(t)), Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })

	// Within the backoff the store is not contacted even though it is up.
	f.up.Store(true)
	require.ErrorIs(t, svc.Ready(t.Context()), ErrStoreUnavailable)
	require.Zero(t, f.defines.Load())
}

func TestConfigurationErrorsStillFailOpen(t *testing.T) {
	t.Parallel()
	// A rejected credential is a configuration error, not an outage.
	rs := newReplayServer(t, reply{match: "", status: 401, body: "no"})
	svc, err := Open(t.Context(), remoteSettings(strings.TrimPrefix(rs.URL, "http://"), closedAddr(t)), Options{})
	require.ErrorIs(t, err, ErrAuth)
	require.Nil(t, svc)

	// So is a malformed setting.
	bad := remoteSettings("", closedAddr(t))
	bad.Remote.Endpoint = ""
	_, err = Open(t.Context(), bad, Options{})
	require.ErrorContains(t, err, "embeddings.remote.endpoint")

	// And an index of another dimension.
	f := newFlakyStore(t)
	f.up.Store(true)
	s := remoteSettings(strings.TrimPrefix(f.URL, "http://"), closedAddr(t))
	s.Embedder.Dimensions = 1024
	_, err = Open(t.Context(), s, Options{})
	require.ErrorIs(t, err, ErrDimensionMismatch)
}

func TestEmbeddedStoreThatCannotStartDoesNotFailOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := Settings{
		Mode:     ModeEmbedded,
		Embedded: &EmbeddedSettings{Binary: failingBinary(t, dir, "surreal"), DataDir: dir + "/data"},
		Embedder: EmbedderSettings{BaseURL: "http://" + closedAddr(t) + "/v1", Model: "bge-m3", Dimensions: 4},
	}
	svc, err := Open(t.Context(), s, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	st := svc.Stages(t.Context(), allHosts)[StageStore]
	require.False(t, st.Available)
	require.Equal(t, ReasonNotStarted, st.Reason)
	require.NotContains(t, st.Reason, dir)
	_, err = svc.Search(t.Context(), func(string) bool { return true }, "q", 1)
	require.ErrorIs(t, err, ErrStoreUnavailable)
	require.Nil(t, svc.Supervisor())
}

func TestEmbeddedExtractorThatCannotStartDoesNotFailOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := remoteSettings(closedAddr(t), closedAddr(t))
	s.Extractor = &ExtractorSettings{Mode: ModeEmbedded, Embedded: &ExtractorEmbedded{Binary: failingBinary(t, dir, "xberg"), CacheDir: dir + "/cache"}}
	svc, err := Open(t.Context(), s, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	require.Nil(t, svc.Extractor())
	st := svc.Stages(t.Context(), allHosts)[StageExtractor]
	require.False(t, st.Available)
	require.NotEmpty(t, st.Reason)
}

func allHosts(string) bool { return true }

func TestStagesProbeOnlyAcknowledgedHosts(t *testing.T) {
	t.Parallel()
	emb := newReplayServer(t, reply{match: "", body: `{}`})
	s := remoteSettings(closedAddr(t), strings.TrimPrefix(emb.URL, "http://"))
	svc, err := Open(t.Context(), s, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })

	stages := svc.Stages(t.Context(), nil)
	require.NotContains(t, stages, StageEmbedder, "an unacknowledged host is not contacted")
	require.Empty(t, emb.requests())

	stages = svc.Stages(t.Context(), allHosts)
	require.True(t, stages[StageEmbedder].Available)
	for _, r := range emb.requests() {
		require.Equal(t, http.MethodGet, r.Method)
		require.Empty(t, r.Auth, "the probe sends no credential")
		require.Empty(t, r.Body, "the probe sends no content")
	}
}

// mortalStore is a Store whose process can be made to exit.
type mortalStore struct {
	Store
	id   int
	dead atomic.Bool
}

func (f *mortalStore) exited() bool { return f.dead.Load() }

func (f *mortalStore) Count(context.Context) (int, error) {
	if f.dead.Load() {
		return 0, ErrStoreExited
	}
	return f.id, nil
}

// gatedOpen is an open function that blocks until released and counts calls.
type gatedOpen struct {
	calls   atomic.Int32
	release chan struct{}
	ctxErrs chan error
	fail    error
}

func newGatedOpen() *gatedOpen {
	return &gatedOpen{release: make(chan struct{}), ctxErrs: make(chan error, 8)}
}

func (g *gatedOpen) open(ctx context.Context) (Store, error) {
	n := g.calls.Add(1)
	select {
	case <-g.release:
		if g.fail != nil {
			return nil, g.fail
		}
		return &mortalStore{id: int(n)}, nil
	case <-ctx.Done():
		g.ctxErrs <- ctx.Err()
		return nil, ctx.Err()
	}
}

func TestCancelledCallerDoesNotFailTheOpenOrAdvanceTheBackoff(t *testing.T) {
	t.Parallel()
	g := newGatedOpen()
	l := newLazyStore(g.open, time.Minute)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := l.ready(ctx)
	require.ErrorIs(t, err, ErrStoreUnavailable)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, ReasonStarting, reasonOf(err))

	close(g.release)
	require.NoError(t, l.ready(t.Context()))
	require.EqualValues(t, 1, g.calls.Load(), "the open the cancelled caller started is the one that served")
	l.mu.Lock()
	defer l.mu.Unlock()
	require.Zero(t, l.failures, "a caller's cancellation is not a store failure")
	require.Empty(t, g.ctxErrs, "the open did not see the caller's cancellation")
}

func TestSlowEmbeddedOpenSurvivesATenSecondStatusCaller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const openTime = 25 * time.Second
		var calls atomic.Int32
		l := newLazyStore(func(ctx context.Context) (Store, error) {
			calls.Add(1)
			select {
			case <-time.After(openTime):
				return &mortalStore{id: 1}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}, embeddedOpenTimeout)

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		err := l.ready(ctx)
		require.Equal(t, ReasonStarting, reasonOf(err), "the status caller gives up at 10s")

		time.Sleep(openTime)
		synctest.Wait()
		require.NoError(t, l.ready(t.Context()), "the open went on and finished")
		require.EqualValues(t, 1, calls.Load())
		l.mu.Lock()
		require.Zero(t, l.failures)
		l.mu.Unlock()
	})
}

func TestOpenHasItsOwnDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGatedOpen()
		l := newLazyStore(g.open, startupStoreTimeout)
		// The caller waits longer than the open is allowed to run.
		err := l.ready(t.Context())
		require.ErrorIs(t, err, ErrStoreUnavailable)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, ReasonTimeout, reasonOf(err))
		l.mu.Lock()
		require.Equal(t, 1, l.failures, "a real open failure counts")
		l.mu.Unlock()
	})
}

func TestWaitersHonorTheirOwnContextWhileOneOpenProceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGatedOpen()
		l := newLazyStore(g.open, time.Minute)

		const waiters = 8
		var wg sync.WaitGroup
		errs := make([]error, waiters)
		for i := range waiters {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(t.Context(), time.Duration(i+1)*time.Second)
				defer cancel()
				errs[i] = l.ready(ctx)
			}()
		}
		wg.Wait()
		for i, err := range errs {
			require.ErrorIs(t, err, context.DeadlineExceeded, "waiter %d returned on its own deadline", i)
		}
		require.EqualValues(t, 1, g.calls.Load(), "the waiters share one open")

		close(g.release)
		synctest.Wait()
		require.NoError(t, l.ready(t.Context()))
		require.EqualValues(t, 1, g.calls.Load())
	})
}

// close() ends an open in flight and the store never serves afterwards, even
// when the open would have succeeded.
func TestClosedLazyStoreDoesNotServe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGatedOpen()
		l := newLazyStore(g.open, time.Minute)

		var first error
		var wg sync.WaitGroup
		wg.Go(func() { first = l.ready(t.Context()) })
		synctest.Wait()
		require.EqualValues(t, 1, g.calls.Load())

		l.close() // returns only once the open saw the cancellation
		require.ErrorIs(t, <-g.ctxErrs, context.Canceled)
		wg.Wait()
		require.ErrorIs(t, first, ErrStoreUnavailable)

		err := l.ready(t.Context())
		require.ErrorIs(t, err, ErrStoreUnavailable)
		require.ErrorContains(t, err, "closed")
		require.EqualValues(t, 1, g.calls.Load(), "no second open after close")

		// A store that was already open stops serving too.
		l2 := newLazyStore(func(context.Context) (Store, error) { return &mortalStore{id: 1}, nil }, time.Minute)
		require.NoError(t, l2.ready(t.Context()))
		l2.close()
		require.ErrorContains(t, l2.ready(t.Context()), "closed")
	})
}

func TestSchemaDefinedOnceUnderConcurrentUse(t *testing.T) {
	t.Parallel()
	f := newFlakyStore(t)
	f.up.Store(true)
	svc, err := Open(t.Context(), remoteSettings(strings.TrimPrefix(f.URL, "http://"), closedAddr(t)), Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() { require.NoError(t, svc.Ready(t.Context())) })
	}
	wg.Wait()
	require.EqualValues(t, 1, f.defines.Load(), "DEFINE runs once")
}

func TestExitedStoreIsReopenedAndReportedUnavailableMeanwhile(t *testing.T) {
	t.Parallel()
	var opens atomic.Int32
	var current atomic.Pointer[mortalStore]
	l := newLazyStore(func(context.Context) (Store, error) {
		st := &mortalStore{id: int(opens.Add(1))}
		current.Store(st)
		return st, nil
	}, time.Minute)
	svc := &Service{lazy: l, store: l}
	now := time.Now()
	l.now = func() time.Time { return now }

	require.NoError(t, svc.Ready(t.Context()))
	require.True(t, svc.Stages(t.Context(), nil)[StageStore].Available)

	current.Load().dead.Store(true)
	st := svc.Stages(t.Context(), nil)[StageStore]
	require.False(t, st.Available, "an exited store is not reported available")
	require.Equal(t, ReasonExited, st.Reason)
	_, err := svc.Store().Count(t.Context())
	require.ErrorIs(t, err, ErrStoreUnavailable)
	require.EqualValues(t, 1, opens.Load(), "the backoff holds the reopen")

	now = now.Add(storeRetryInitial + time.Second)
	n, err := svc.Store().Count(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, n, "the next use went through open and got a new store")
	require.True(t, svc.Stages(t.Context(), nil)[StageStore].Available)
}

func TestErrStoreExitedFromACallDropsTheStore(t *testing.T) {
	t.Parallel()
	var opens atomic.Int32
	l := newLazyStore(func(context.Context) (Store, error) { return &exitOnCount{id: int(opens.Add(1))}, nil }, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }
	_, err := l.Count(t.Context())
	require.ErrorIs(t, err, ErrStoreExited)
	_, err = l.Count(t.Context())
	require.ErrorIs(t, err, ErrStoreUnavailable)
	now = now.Add(storeRetryInitial + time.Second)
	_, err = l.Count(t.Context())
	require.ErrorIs(t, err, ErrStoreExited, "the first store's replacement was opened")
	require.EqualValues(t, 2, opens.Load())
}

// exitOnCount reports its process gone through the error of a call only.
type exitOnCount struct {
	Store
	id int
}

func (*exitOnCount) Count(context.Context) (int, error) { return 0, ErrStoreExited }

func TestStageReasonsAreCodesWithoutPathsOrAddresses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	storeAddr, embAddr, extAddr := closedAddr(t), closedAddr(t), closedAddr(t)
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	remote := remoteSettings(storeAddr, embAddr)
	remote.Extractor = &ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: "http://" + extAddr}}
	svc, err := Open(t.Context(), remote, Options{Logger: log})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	embedded := Settings{
		Mode:     ModeEmbedded,
		Embedded: &EmbeddedSettings{Binary: failingBinary(t, dir, "surreal"), DataDir: dir + "/data"},
		Embedder: EmbedderSettings{BaseURL: "http://" + embAddr + "/v1", Model: "bge-m3", Dimensions: 4},
		Extractor: &ExtractorSettings{Mode: ModeEmbedded, Embedded: &ExtractorEmbedded{
			Binary: failingBinary(t, dir, "xberg"), CacheDir: dir + "/cache"}},
	}
	esvc, err := Open(t.Context(), embedded, Options{Logger: log})
	require.NoError(t, err)
	t.Cleanup(func() { _ = esvc.Close() })

	allowed := []string{ReasonUnreachable, ReasonNotStarted, ReasonStarting, ReasonExited, ReasonAuthFailed, ReasonTimeout, ReasonUnavailable}
	for _, stages := range []map[string]StageStatus{svc.Stages(t.Context(), allHosts), esvc.Stages(t.Context(), allHosts)} {
		require.Len(t, stages, 3)
		for name, st := range stages {
			require.False(t, st.Available, name)
			require.Contains(t, allowed, st.Reason, name)
			for _, leak := range []string{dir, storeAddr, embAddr, extAddr, "127.0.0.1", "/"} {
				require.NotContains(t, st.Reason, leak, name)
			}
		}
	}
	require.Equal(t, ReasonUnreachable, svc.Stages(t.Context(), allHosts)[StageStore].Reason)
	require.Equal(t, ReasonNotStarted, esvc.Stages(t.Context(), allHosts)[StageStore].Reason)
	require.Equal(t, ReasonNotStarted, esvc.Stages(t.Context(), allHosts)[StageExtractor].Reason)

	// The detail is in the server log, once per change and not per poll.
	require.Contains(t, logs.String(), "surreal_binary: running")
	before := strings.Count(logs.String(), "embeddings stage is unavailable")
	for range 3 {
		svc.Stages(t.Context(), allHosts)
		esvc.Stages(t.Context(), allHosts)
	}
	require.Equal(t, before, strings.Count(logs.String(), "embeddings stage is unavailable"), "polling does not flood the log")
}

// failingBinary writes an executable that exits 1 and returns its path: a
// binary that exists but cannot start a store.
func failingBinary(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\nexit 1\n"), 0o755))
	return p
}

func TestMissingOrUnrunnableBinaryIsAConfigurationError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o644))
	for name, bin := range map[string]string{"missing": filepath.Join(dir, "nope"), "directory": dir, "not executable": plain} {
		s := Settings{
			Mode:     ModeEmbedded,
			Embedded: &EmbeddedSettings{Binary: bin, DataDir: filepath.Join(dir, "data")},
			Embedder: EmbedderSettings{BaseURL: "http://" + closedAddr(t) + "/v1", Model: "bge-m3", Dimensions: 4},
		}
		svc, err := Open(t.Context(), s, Options{})
		require.ErrorContains(t, err, "embeddings.embedded.surreal_binary", name)
		require.Nil(t, svc, name)
	}
	s := remoteSettings(closedAddr(t), closedAddr(t))
	s.Extractor = &ExtractorSettings{Mode: ModeEmbedded, Embedded: &ExtractorEmbedded{Binary: filepath.Join(dir, "nope"), CacheDir: dir + "/c"}}
	_, err := Open(t.Context(), s, Options{})
	require.ErrorContains(t, err, "embeddings.extractor.embedded.xberg_binary")
}
