package embeddings

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The embedder, extractor, describer and vector store dial the configured host themselves:
// HTTP_PROXY and HTTPS_PROXY in the process environment never see document text,
// media or keys.
func TestStageClientsIgnoreTheProxyEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")

	emb, err := NewEmbedder(EmbedderSettings{BaseURL: "https://embed.example.test/v1", Model: "m", Dimensions: 4}, nil)
	require.NoError(t, err)
	desc, err := NewDescriber(DescriberSettings{BaseURL: "https://describe.example.test/v1", Model: "m"}, nil)
	require.NoError(t, err)
	ext, err := NewExtractor(ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: "https://xberg.example.test"}}, nil)
	require.NoError(t, err)

	store := newSurrealClient("https://store.example.test:8000", "ns", "db", auth{}, nil)

	for name, hc := range map[string]*http.Client{"embedder": emb.hc, "describer": desc.hc, "extractor": ext.hc, "store": store.hc} {
		tr, ok := hc.Transport.(*http.Transport)
		require.True(t, ok, name)
		assert.Nil(t, tr.Proxy, name)
	}
}
