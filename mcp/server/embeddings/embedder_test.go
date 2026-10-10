package embeddings

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func allow(string) bool { return true }

func testEmbedder(t *testing.T, base string, key Secret) *Embedder {
	t.Helper()
	e, err := NewEmbedder(EmbedderSettings{BaseURL: base, Model: "bge-m3", Dimensions: 4, APIKey: key}, nil)
	require.NoError(t, err)
	return e
}

func TestEmbedOrdersByIndexAndSendsKey(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t, reply{match: "bge-m3", body: fixture(t, "embeddings_ok.json")})
	e := testEmbedder(t, rs.URL+"/v1", "sk-test")
	vecs, err := e.Embed(t.Context(), allow, []string{"first", "second"})
	require.NoError(t, err)
	require.Equal(t, []float32{1, 0, 0, 0}, vecs[0])
	require.Equal(t, []float32{0.5, 0.25, 0.125, 0.0625}, vecs[1])
	r := rs.requests()[0]
	require.Equal(t, "/v1/embeddings", r.Path)
	require.Equal(t, "Bearer sk-test", r.Auth)
	require.Contains(t, r.Body, `"input":["first","second"]`)
}

func TestEmbedRefusesUnacknowledgedHostWithoutSending(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	e := testEmbedder(t, srv.URL, "")
	for name, covers := range map[string]func(string) bool{
		"nil":  nil,
		"none": func(string) bool { return false },
	} {
		_, err := e.Embed(t.Context(), covers, []string{"secret document text"})
		var eg *EgressError
		require.ErrorAs(t, err, &eg, name)
		require.ErrorIs(t, err, ErrEgressNotAcknowledged)
		require.Equal(t, []string{e.Host()}, eg.Hosts)
		require.Contains(t, err.Error(), e.Host())
	}
	require.Zero(t, hits.Load(), "nothing may reach an unacknowledged host")
}

func TestEmbedErrors(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t,
		reply{match: "wrong-dim", body: `{"data":[{"embedding":[1,2],"index":0}]}`},
		reply{match: "short", body: `{"data":[]}`},
		reply{match: "dup", body: `{"data":[{"embedding":[1,0,0,0],"index":0},{"embedding":[1,0,0,0],"index":0}]}`},
		reply{match: "noindex", body: `{"data":[{"embedding":[1,0,0,0]},{"embedding":[0,1,0,0]}]}`},
		reply{match: "text", status: 404, body: fixture(t, "embeddings_error.json")},
	)
	e := testEmbedder(t, rs.URL, "")
	ctx := t.Context()
	_, err := e.Embed(ctx, allow, []string{"wrong-dim"})
	require.ErrorContains(t, err, "2-dimensional")
	_, err = e.Embed(ctx, allow, []string{"short"})
	require.ErrorContains(t, err, "0 vectors")
	_, err = e.Embed(ctx, allow, []string{"dup", "dup"})
	require.ErrorContains(t, err, "unusable index")
	vecs, err := e.Embed(ctx, allow, []string{"noindex", "noindex"})
	require.NoError(t, err)
	require.Equal(t, []float32{0, 1, 0, 0}, vecs[1])

	_, err = e.Embed(ctx, allow, []string{"some text with private words"})
	var ee *EmbedError
	require.ErrorAs(t, err, &ee)
	require.Equal(t, 404, ee.Status)
	require.Contains(t, ee.Message, "not found")
	require.NotContains(t, err.Error(), "private words")
}

func TestEmbedDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	var other atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { other.Add(1) }))
	t.Cleanup(target.Close)
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redir.Close)
	e := testEmbedder(t, redir.URL, "sk")
	_, err := e.Embed(t.Context(), allow, []string{"x"})
	require.Error(t, err)
	require.Zero(t, other.Load())
}

func TestNewEmbedderRejectsBadSettings(t *testing.T) {
	t.Parallel()
	_, err := NewEmbedder(EmbedderSettings{}, nil)
	require.Error(t, err)
	_, err = NewEmbedder(EmbedderSettings{BaseURL: "http://u:p@h:1", Model: "m", Dimensions: 4}, nil)
	require.ErrorContains(t, err, "credentials")
	e, err := NewEmbedder(EmbedderSettings{BaseURL: "http://h:1/v1/", Model: "m", Dimensions: 4}, nil)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(e.endpoint, "/v1/embeddings"))
}
