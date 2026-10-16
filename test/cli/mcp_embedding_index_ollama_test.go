//go:build !windows && !noagent

package cli

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Ollama helpers (startOllama) live in a file that the noagent build
// leaves out, so this test does too.

// TestMCPEmbeddingIndexOllama runs the lifecycle's core against a real Ollama
// bge-m3 and the real surreal binary: three added texts are indexed by CID
// and found by meaning, and unpinning one removes its vectors.
func TestMCPEmbeddingIndexOllama(t *testing.T) {
	bin := surrealBinary(t)
	base, _ := startOllama(t) // a private Ollama on a free port; the chat model it found is unused
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
	hasBGE := false
	for _, m := range tags.Models {
		hasBGE = hasBGE || strings.HasPrefix(m.Name, "bge-m3")
	}
	if !hasBGE {
		t.Skip("bge-m3 is not pulled into Ollama: run 'ollama pull bge-m3'")
	}

	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	dataDir := filepath.Join(t.TempDir(), "idx")
	p := startIndexMCP(t, node, indexConfig(bin, dataDir, base, "bge-m3", 1024, `{"auto": true}`))
	e := &elicitor{action: "accept"}
	cs := p.mustClient(indexTestToken, e.options())

	docs := map[string]string{
		"cooking": "A recipe for sourdough bread: mix flour, water and salt, then let the dough rise overnight before baking.",
		"network": "IPFS content addressing identifies files by the hash of their bytes, and peers fetch blocks over bitswap.",
		"astro":   "The James Webb telescope observes distant galaxies in infrared light from an orbit around the Sun-Earth L2 point.",
	}
	cids := map[string]string{}
	for name, text := range docs {
		out := mcpAdd(t, cs, text, true)
		assert.Empty(t, out["warnings"], name)
		cids[name] = out["cid"].(string)
	}
	assert.EqualValues(t, 3, indexStatus(t, cs, "")["indexed_cids"])

	for query, want := range map[string]string{
		"how do I bake bread at home":           "cooking",
		"how do peers find content by its hash": "network",
		"infrared space observatory":            "astro",
	} {
		hits := searchHits(t, cs, query, map[string]any{"top_k": 3})
		require.Len(t, hits, 3, query)
		assert.Equal(t, cids[want], hits[0]["cid"], query)
		assert.Equal(t, docs[want], hits[0]["text"], query)
		assert.Equal(t, "bge-m3", okResult(t, callMCP(t, cs, "ipfs_index_search", map[string]any{"query": query}))["model"])
	}

	// Unpin one through ipfs mcp: its vectors go, the others stay.
	rm := callMCP(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + cids["astro"]})
	require.False(t, rm.IsError, "%+v", rm.Content)
	assert.Equal(t, false, indexStatus(t, cs, cids["astro"])["cid_indexed"])
	assert.EqualValues(t, 2, indexStatus(t, cs, "")["indexed_cids"])
	for _, h := range searchHits(t, cs, "infrared space observatory", map[string]any{"top_k": 3}) {
		assert.NotEqual(t, cids["astro"], h["cid"])
	}
}
