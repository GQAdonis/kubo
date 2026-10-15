//go:build !windows

package embeddings

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestExtractorFailsClosedWhenXbergExits kills the supervised xberg after it
// has started. The proxy stops listening and Extract refuses to send, so a
// process that binds the freed port never receives a document or the token.
func TestExtractorFailsClosedWhenXbergExits(t *testing.T) {
	cfg := ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: t.TempDir()}
	x, err := OpenExtractor(t.Context(), ExtractorSettings{Mode: ModeEmbedded, Embedded: &cfg}, nil, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = x.Close() })
	sup := x.Supervisor()
	_, err = x.Extract(t.Context(), nil, ExtractRequest{Filename: "a.pdf", Data: []byte("%PDF")})
	require.NoError(t, err, "it works while xberg runs")
	proxyAddr := sup.proxy.host

	killProcess(sup.cmd)
	select {
	case <-sup.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("xberg did not exit")
	}

	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", proxyAddr, time.Second)
		if err == nil {
			_ = c.Close()
			return false
		}
		return true
	}, 5*time.Second, 20*time.Millisecond, "the proxy closes its listener when xberg exits")

	_, err = x.Extract(t.Context(), nil, ExtractRequest{Filename: "a.pdf", Data: []byte("%PDF")})
	require.ErrorIs(t, err, ErrExtractorUnavailable)
	require.ErrorContains(t, err, "exited")
}

func TestExtractFailsClosedBeforeSendingWhenSupervisorIsDone(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	x, err := newExtractor(ExtractorSettings{Mode: ModeEmbedded}, srv.URL, false, "tok", nil)
	require.NoError(t, err)
	exited := make(chan struct{})
	close(exited)
	x.sup = &ExtractorSupervisor{exited: exited}
	_, err = x.Extract(t.Context(), nil, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractorUnavailable)
	require.Zero(t, hits.Load(), "nothing is sent once the child is gone")
}

// TestSurrealClientFailsClosedWhenTheStoreExited: after the embedded store's
// process ends the client sends nothing, so its root credential cannot reach
// whatever binds the freed port.
func TestSurrealClientFailsClosedWhenTheStoreExited(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	t.Cleanup(srv.Close)
	c := newSurrealClient(srv.URL, "ns", "db", auth{user: "root", pass: "secret"}, nil)
	gone := make(chan struct{})
	c.gone = gone

	_, err := c.query(t.Context(), "RETURN 1", nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, hits.Load())

	close(gone)
	_, err = c.query(t.Context(), "RETURN 1", nil)
	require.ErrorIs(t, err, ErrStoreExited)
	require.EqualValues(t, 1, hits.Load(), "no request after the process exited")
}
