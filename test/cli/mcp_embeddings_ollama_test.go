//go:build !windows && !noagent

package cli

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPEmbeddingsStoreOllamaBGEM3 embeds with the real bge-m3 model of a
// private Ollama (OS-chosen port, own process group, killed on cleanup) into
// the supervised real surreal store, then runs a semantic KNN search. It is
// skipped, with the reason, when ollama, bge-m3 or surreal is missing.
func TestMCPEmbeddingsStoreOllamaBGEM3(t *testing.T) {
	bin := surrealBinary(t)
	base, _ := startOllama(t) // base ends in /v1; the chat model it picked is unused here
	api := strings.TrimSuffix(base, "/v1")
	resp, err := http.Get(api + "/api/tags")
	require.NoError(t, err)
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tags))
	hasBGE := slices.ContainsFunc(tags.Models, func(m struct {
		Name string `json:"name"`
	}) bool {
		return strings.HasPrefix(m.Name, "bge-m3")
	})
	if !hasBGE {
		t.Skip("bge-m3 is not pulled into Ollama: run 'ollama pull bge-m3'")
	}

	dataDir := filepath.Join(t.TempDir(), "idx")
	set := embeddings.Settings{
		Mode:     embeddings.ModeEmbedded,
		Embedded: &embeddings.EmbeddedSettings{Binary: bin, DataDir: dataDir, AvoidPorts: mcpserver.ReservedPorts},
		Embedder: embeddings.EmbedderSettings{BaseURL: base, Model: "bge-m3", Dimensions: 1024},
	}
	svc, err := embeddings.Open(t.Context(), set, embeddings.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	covers := coversOnly(svc.Embedder().Host())

	docs := map[string]string{
		"bafyCooking": "A recipe for sourdough bread: mix flour, water and salt, then let the dough rise overnight before baking.",
		"bafyNetwork": "IPFS content addressing identifies files by the hash of their bytes, and peers fetch blocks over bitswap.",
		"bafyAstro":   "The James Webb telescope observes distant galaxies in infrared light from an orbit around the Sun-Earth L2 point.",
	}
	for cid, text := range docs {
		n, err := svc.Index(t.Context(), covers, embeddings.Document{CID: cid, Path: "/" + cid, MIME: "text/plain", Chunks: []string{text}})
		require.NoError(t, err, cid)
		require.Equal(t, 1, n)
	}
	for query, want := range map[string]string{
		"how do I bake bread at home":           "bafyCooking",
		"how do peers find content by its hash": "bafyNetwork",
		"infrared space observatory":            "bafyAstro",
	} {
		hits, err := svc.Search(t.Context(), covers, query, 3)
		require.NoError(t, err)
		require.Len(t, hits, 3)
		assert.Equal(t, want, hits[0].CID, "query %q", query)
		assert.Less(t, hits[0].Distance, hits[1].Distance)
	}
}
