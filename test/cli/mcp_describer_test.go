//go:build !windows

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-media-describer against a real daemon, the real
// ipfs mcp binary over HTTP and a real surreal binary. The describer is a fake
// OpenAI-compatible endpoint except in TestMCPDescriberRealService, which runs
// only against a describer the operator configures by environment.

// testPNG is a small real PNG: a red square on white.
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := range 96 {
		for x := range 96 {
			c := color.RGBA{255, 255, 255, 255}
			if x > 24 && x < 72 && y > 24 && y < 72 {
				c = color.RGBA{220, 20, 20, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// fakeChat is an OpenAI-compatible /chat/completions endpoint that records its
// requests and answers with reply.
type fakeChat struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	reply  string
}

func newFakeChat(t *testing.T, reply string) *fakeChat {
	t.Helper()
	c := &fakeChat{reply: reply}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.bodies = append(c.bodies, string(b))
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		text, _ := json.Marshal(c.reply)
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%s}}]}`, text)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *fakeChat) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

// describerConfig is bulkConfig with a policy that allows PNG and a describer
// block.
func describerConfig(surrealBin, dataDir, embedBase, describer string) string {
	policy := `"policy": {"mime_allowlist": ["text/plain", "image/png"]}`
	cfg := bulkConfig(surrealBin, dataDir, embedBase, "")
	cfg = strings.Replace(cfg, `"policy": {}`, policy, 1)
	if describer != "" {
		cfg = strings.Replace(cfg, `"policy"`, `"describer": `+describer+`, "policy"`, 1)
	}
	return cfg
}

// TestMCPDescriberDisabled: with no describer block, or one that describes
// nothing (kinds [] or a zero hourly limit), an allowlisted PNG is skipped
// describer_not_configured and no request leaves ipfs mcp; with the block on,
// the same PNG is described and found.
func TestMCPDescriberDisabled(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	pngCID := addFixture(node, testPNG(t))

	for name, block := range map[string]string{
		"no describer block":     "",
		"kinds is empty":         `{"base_url": "%s/v1", "model": "vision-test", "kinds": []}`,
		"hourly limit is zero":   `{"base_url": "%s/v1", "model": "vision-test", "max_requests_per_hour": 0}`,
		"video only, image file": `{"base_url": "%s/v1", "model": "vision-test", "kinds": ["video"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			chat := newFakeChat(t, "cats asleep on a sofa")
			embed := newCountingEmbed(t)
			if block != "" {
				block = fmt.Sprintf(block, chat.URL)
			}
			p := startIndexMCP(t, node, describerConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", block))
			cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

			out := okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": pngCID}))
			assert.Equal(t, "describer_not_configured", out["skipped_reason"])
			assert.EqualValues(t, 0, out["chunks_indexed"])
			assert.Zero(t, chat.count(), "no description request was made")
			assert.Zero(t, embed.requests.Load(), "nothing was embedded")
		})
	}

	t.Run("describer on", func(t *testing.T) {
		chat := newFakeChat(t, "cats asleep on a red sofa")
		embed := newCountingEmbed(t)
		block := fmt.Sprintf(`{"base_url": "%s/v1", "model": "vision-test"}`, chat.URL)
		p := startIndexMCP(t, node, describerConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", block))
		cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

		out := okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": pngCID}))
		assert.EqualValues(t, 1, out["chunks_indexed"])
		require.Equal(t, 1, chat.count())
		assert.Contains(t, chat.bodies[0], "data:image/png;base64,", "the bytes travel in the request")
		assert.NotContains(t, chat.bodies[0], pngCID, "the CID does not")

		hits := searchHits(t, cs, "cats", nil)
		require.Len(t, hits, 1)
		assert.Equal(t, pngCID, hits[0]["cid"])
		assert.Equal(t, "description", hits[0]["source"])
		assert.Equal(t, "vision-test", hits[0]["described_by"])
		assert.Equal(t, "image", hits[0]["kind"])
		assert.Equal(t, "cats asleep on a red sofa", hits[0]["text"])
	})
}

// bagEmbed is an OpenAI-compatible embeddings endpoint whose vector is a hashed
// bag of words, so a query that shares a word with a text is near it. It is
// deterministic and needs no model.
func newBagEmbed(t *testing.T, dims int) *httptest.Server {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		data := make([]item, 0, len(req.Input))
		for i, text := range req.Input {
			v := make([]float32, dims)
			for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
				h := uint32(2166136261)
				for _, c := range []byte(word) {
					h = (h ^ uint32(c)) * 16777619
				}
				v[h%uint32(dims)]++
			}
			data = append(data, item{i, v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestMCPDescriberRealService describes one PNG with a describer the operator
// configures, and finds the image by a word from its description. It runs only
// when IPFS_MCP_TEST_DESCRIBER_URL (the OpenAI-compatible base URL) and
// IPFS_MCP_TEST_DESCRIBER_MODEL are set; IPFS_MCP_TEST_DESCRIBER_KEY_VAR names
// the environment variable that holds the API key, if the endpoint needs one.
// Video is not exercised: no hosted video describer is approved (OG-17).
func TestMCPDescriberRealService(t *testing.T) {
	base := os.Getenv("IPFS_MCP_TEST_DESCRIBER_URL")
	model := os.Getenv("IPFS_MCP_TEST_DESCRIBER_MODEL")
	if base == "" || model == "" {
		t.Skip("IPFS_MCP_TEST_DESCRIBER_URL and IPFS_MCP_TEST_DESCRIBER_MODEL are not set: no vision model is configured for the real-service describer test")
	}
	u, err := url.Parse(base)
	require.NoError(t, err)
	hosted := !strings.HasPrefix(u.Hostname(), "127.") && u.Hostname() != "localhost" && u.Hostname() != "::1"
	t.Logf("describer host %s model %s hosted=%t", u.Host, model, hosted)

	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	pngCID := addFixture(node, testPNG(t))

	block := fmt.Sprintf(`{"base_url": %q, "model": %q, "max_output_tokens": 200, "timeout_seconds": 300`, base, model)
	if keyVar := os.Getenv("IPFS_MCP_TEST_DESCRIBER_KEY_VAR"); keyVar != "" {
		block += fmt.Sprintf(`, "api_key": {"env": %q}`, keyVar)
	}
	block += "}"
	embed := newBagEmbed(t, 256)
	cfg := strings.Replace(describerConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", block), `"dimensions": 8`, `"dimensions": 256`, 1)
	cfg = strings.Replace(cfg, `"egress_ack": ["local"]`, fmt.Sprintf(`"egress_ack": ["local", %q]`, u.Host), 1)
	p := startIndexMCP(t, node, cfg)
	cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

	out := okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": pngCID}))
	require.EqualValues(t, 1, out["chunks_indexed"], "%v", out)

	// Take a word from the description itself, then find the image with it.
	hits := searchHits(t, cs, "image", map[string]any{"top_k": 5})
	require.NotEmpty(t, hits)
	text := hits[0]["text"].(string)
	t.Logf("description by %v: %s", hits[0]["described_by"], text)
	var word string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
		if len(w) > len(word) {
			word = w
		}
	}
	require.NotEmpty(t, word)
	found := searchHits(t, cs, word, map[string]any{"top_k": 3})
	require.NotEmpty(t, found)
	assert.Equal(t, pngCID, found[0]["cid"])
	assert.Equal(t, "description", found[0]["source"])
	assert.Equal(t, model, found[0]["described_by"])
}
