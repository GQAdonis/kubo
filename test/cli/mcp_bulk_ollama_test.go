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

// TestMCPBulkIndexOllama runs a bulk index against a real Ollama bge-m3 and the
// real surreal binary: documents the node pins or keeps in MFS are indexed by
// one confirmed run and found by meaning, and the pin set and block set are
// unchanged afterwards.
func TestMCPBulkIndexOllama(t *testing.T) {
	bin := surrealBinary(t)
	base, _ := startOllama(t) // a private Ollama on a free port; the chat model it found is unused
	resp, err := http.Get(strings.TrimSuffix(base, "/v1") + "/api/tags")
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
	docs := map[string]string{
		"cooking": "A recipe for sourdough bread: mix flour, water and salt, then let the dough rise overnight before baking.",
		"network": "IPFS content addressing identifies files by the hash of their bytes, and peers fetch blocks over bitswap.",
		"astro":   "The James Webb telescope observes distant galaxies in infrared light from an orbit around the Sun-Earth L2 point.",
	}
	cids := map[string]string{
		"cooking": node.IPFSAddStr(docs["cooking"], "--cid-version=1"),
		"network": node.IPFSAddStr(docs["network"], "--cid-version=1"),
		// The astronomy note is kept by MFS only: never pinned.
		"astro": node.IPFSAddStr(docs["astro"], "--cid-version=1", "--pin=false"),
	}
	node.IPFS("files", "mkdir", "-p", "/library")
	node.IPFS("files", "cp", "/ipfs/"+cids["astro"], "/library/astro.txt")
	pinsBefore := sortedLines(node.IPFS("pin", "ls").Stdout.String())
	refsBefore := sortedLines(localRefs(node))

	dataDir := filepath.Join(t.TempDir(), "idx")
	p := startIndexMCP(t, node, indexConfig(bin, dataDir, base, "bge-m3", 1024, `{}`))
	e := &elicitor{action: "accept"}
	cs := p.mustClient(indexTestToken, e.options())

	okResult(t, callMCP(t, cs, "ipfs_index_bulk_start", map[string]any{}))
	run := waitBulk(t, cs)
	assert.Equal(t, "completed", run["state"], "%v", run)
	assert.EqualValues(t, 3, run["files_indexed"], "%v", run)
	assert.EqualValues(t, 3, indexStatus(t, cs, "")["indexed_cids"])

	for query, want := range map[string]string{
		"how do I bake bread at home":           "cooking",
		"how do peers find content by its hash": "network",
		"infrared space observatory":            "astro",
	} {
		hits := searchHits(t, cs, query, map[string]any{"top_k": 3})
		require.NotEmpty(t, hits, query)
		assert.Equal(t, cids[want], hits[0]["cid"], query)
		assert.Equal(t, docs[want], hits[0]["text"], query)
	}

	assert.Equal(t, pinsBefore, sortedLines(node.IPFS("pin", "ls").Stdout.String()), "no pin was added or removed")
	assert.Equal(t, refsBefore, sortedLines(localRefs(node)), "no block was added")
}
