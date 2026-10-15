package embeddings

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
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
		Embedded: &EmbeddedSettings{Binary: dir + "/no-such-surreal", DataDir: dir + "/data"},
		Embedder: EmbedderSettings{BaseURL: "http://" + closedAddr(t) + "/v1", Model: "bge-m3", Dimensions: 4},
	}
	svc, err := Open(t.Context(), s, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	st := svc.Stages(t.Context(), allHosts)[StageStore]
	require.False(t, st.Available)
	require.Contains(t, st.Reason, "surreal_binary")
	_, err = svc.Search(t.Context(), func(string) bool { return true }, "q", 1)
	require.ErrorIs(t, err, ErrStoreUnavailable)
	require.Nil(t, svc.Supervisor())
}

func TestEmbeddedExtractorThatCannotStartDoesNotFailOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := remoteSettings(closedAddr(t), closedAddr(t))
	s.Extractor = &ExtractorSettings{Mode: ModeEmbedded, Embedded: &ExtractorEmbedded{Binary: dir + "/no-such-xberg", CacheDir: dir + "/cache"}}
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
