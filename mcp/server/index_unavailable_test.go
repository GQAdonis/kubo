package server

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedHostPort is a loopback host:port that nothing listens on.
func closedHostPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// unreachableEmbeddings is an embeddings section whose store, embedder and
// extractor all refuse connections.
func unreachableEmbeddings(t *testing.T) *EmbeddingsConfig {
	t.Helper()
	store, emb, ext := closedHostPort(t), closedHostPort(t), closedHostPort(t)
	cfg, err := ParseConfig([]byte(`{"embeddings":{"mode":"remote",
	  "remote":{"endpoint":"http://` + store + `","namespace":"ipfs","database":"index"},
	  "embedder":{"base_url":"http://` + emb + `/v1","model":"bge-m3","dimensions":1024},
	  "extractor":{"mode":"remote","remote":{"base_url":"http://` + ext + `"}}}}`))
	require.NoError(t, err)
	return cfg.Embeddings
}

func TestServerStartsAndReportsUnavailableStagesWhenDependenciesAreDown(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{noEmbeddings: true})
	cfg := unreachableEmbeddings(t)
	require.NoError(t, e.s.EnableEmbeddings(t.Context(), cfg, nil), "an outage is not a startup error")
	t.Cleanup(func() { _ = e.s.CloseEmbeddings() })
	id := indexIdentity("local")
	cs := e.connect(&id)
	out := structured(t, toolCall(t, cs, "ipfs_index_status", nil))
	assert.Equal(t, true, out["enabled"])
	assert.Equal(t, false, out["store_reachable"])
	stages, ok := out["stages"].(map[string]any)
	require.True(t, ok, "status names the stages that are down")
	for _, name := range []string{"store", "embedder", "extractor"} {
		st, ok := stages[name].(map[string]any)
		require.True(t, ok, name)
		assert.Equal(t, false, st["available"], name)
		assert.NotEmpty(t, st["reason"], name)
	}
}

func TestSearchFailsClosedWhileTheStoreIsDown(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{noEmbeddings: true})
	cfg := unreachableEmbeddings(t)
	require.NoError(t, e.s.EnableEmbeddings(t.Context(), cfg, nil))
	t.Cleanup(func() { _ = e.s.CloseEmbeddings() })

	id := indexIdentity("local")
	res := toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "cats"})
	require.True(t, res.IsError)
	out := structured(t, res)
	errObj, _ := out["error"].(map[string]any)
	assert.Equal(t, CodeIndexUnavailable, errObj["code"])
	assert.Equal(t, true, errObj["retryable"])
	assert.NotContains(t, out, "results", "no partial results")
}

func TestUnpinPathsDoNotErrorWhileTheStoreIsDown(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{noEmbeddings: true})
	cfg := unreachableEmbeddings(t)
	require.NoError(t, e.s.EnableEmbeddings(t.Context(), cfg, nil))
	t.Cleanup(func() { _ = e.s.CloseEmbeddings() })

	svc := e.s.Embeddings()
	warnings := e.s.dropUncovered(t.Context(), svc, []string{"bafkreieoswm5yqijue4v3qyit63bd76suvpinoixcelkdzzlfw3uimwtsu"}, nil)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "were not checked")
}
