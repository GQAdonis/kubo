//go:build !windows && !noagent

package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var xbergLine = regexp.MustCompile(`extractor xberg on 127\.0\.0\.1:(\d+)`)

// TestMCPExtractorXberg runs the real xberg v1.3.10 behind ipfs mcp: xberg is
// supervised on loopback on an ephemeral port with its own cache, the store is
// an embedded surreal, the embedder is a private Ollama bge-m3, and one PDF,
// one DOCX and one CSV are indexed by CID and each is found by a phrase it
// contains. It skips, naming the reason, when IPFS_MCP_TEST_XBERG (the absolute
// path of the xberg binary) is unset, or when surreal or bge-m3 are missing.
func TestMCPExtractorXberg(t *testing.T) {
	xbergBin := os.Getenv("IPFS_MCP_TEST_XBERG")
	if xbergBin == "" {
		t.Skip("IPFS_MCP_TEST_XBERG (absolute path of an xberg v1.3.10 binary) is not set")
	}
	require.True(t, filepath.IsAbs(xbergBin), "IPFS_MCP_TEST_XBERG must be an absolute path")
	surreal := surrealBinary(t)
	base, _ := startOllama(t) // a private Ollama on a free port; its chat model is unused
	requireOllamaModel(t, strings.TrimSuffix(base, "/v1"), "bge-m3")

	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	work := t.TempDir()
	cache := filepath.Join(work, "xberg-cache")
	dataDir := filepath.Join(work, "idx")

	fixtures := map[string]struct {
		data   []byte
		phrase string // a phrase the file contains
		query  string
		mime   string
		kind   string
	}{
		"pdf": {extractorTestPDF("The zanzibar quokka migrates quietly across the lattice"), "zanzibar quokka", "where does the zanzibar quokka migrate",
			"application/pdf", "document"},
		"docx": {extractorTestDOCX(t, "The marmalade lighthouse keeper polishes brass lanterns at dawn"), "marmalade lighthouse keeper", "who polishes brass lanterns at the lighthouse",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "document"},
		"csv": {[]byte("creature,sound\nvelociraptor,harmonics of a distant cello\ncapybara,gentle humming\n"), "velociraptor", "which creature makes cello harmonics",
			"text/csv", "data"},
	}
	cids := map[string]string{}
	for name, f := range fixtures {
		cids[name] = addFixture(node, f.data)
	}
	refs, pins := sortedLines(localRefs(node)), sortedLines(node.IPFS("pin", "ls", "--type=all", "-q").Stdout.String())

	allow, _ := json.Marshal([]string{"text/plain", "application/pdf", fixtures["docx"].mime, "text/csv"})
	extractor, _ := json.Marshal(map[string]any{"mode": "embedded", "embedded": map[string]any{"xberg_binary": xbergBin, "cache_dir": cache}})
	p := startIndexMCP(t, node, extractorConfig(surreal, dataDir, base, "bge-m3", 1024,
		`{"mime_allowlist": `+string(allow)+`}`, string(extractor)))
	cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

	// xberg is a child of ipfs mcp, on loopback only, on a non-default port.
	m := xbergLine.FindStringSubmatch(p.stderr.String())
	require.NotNil(t, m, "the startup line names the xberg address; stderr: %s", p.stderr.String())
	port, _ := strconv.Atoi(m[1])
	assert.NotContains(t, slices.Concat(mcpserver.ReservedPorts, []int{8000}), port)
	assertNotReachableOffLoopback(t, port)
	var xbergPID int
	for _, e := range processesMentioning(t, cache) {
		if strings.Contains(e.args, " serve ") {
			xbergPID = e.pid
			assert.Equal(t, p.cmd.Process.Pid, e.ppid, "xberg is a child of ipfs mcp")
			assert.Contains(t, e.args, "--host 127.0.0.1")
		}
	}
	require.NotZero(t, xbergPID, "no xberg process found for cache %s", cache)
	assert.DirExists(t, cache)

	for name, f := range fixtures {
		res := callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": cids[name]})
		requireMatchesOutputSchema(t, "ipfs_index_cid", res)
		out := okResult(t, res)
		require.EqualValues(t, 1, out["chunks_indexed"], "%s: %+v", name, out)
		assert.NotContains(t, out, "skipped_reason", name)

		hits := searchHits(t, cs, f.query, map[string]any{"top_k": 3})
		require.NotEmpty(t, hits, name)
		top := hits[0]
		t.Logf("%s: %s found by %q: score %.3f kind %v source %v mime_type %v text %q", name, cids[name], f.query, top["score"], top["kind"], top["source"], top["mime_type"], top["text"])
		assert.Equal(t, cids[name], top["cid"], "%s: the file is found by its phrase", name)
		assert.Contains(t, strings.ToLower(top["text"].(string)), strings.ToLower(f.phrase), name)
		assert.Equal(t, f.kind, top["kind"], name)
		assert.Equal(t, "extracted", top["source"], name)
		assert.Equal(t, f.mime, top["mime_type"], name)
	}

	// Extraction added, pinned and announced nothing.
	assert.Equal(t, refs, sortedLines(localRefs(node)))
	assert.Equal(t, pins, sortedLines(node.IPFS("pin", "ls", "--type=all", "-q").Stdout.String()))

	// SIGKILL of ipfs mcp leaves no xberg behind (the watchdog stops it).
	require.NoError(t, p.cmd.Process.Kill())
	_ = p.cmd.Wait()
	eventually(t, 30*time.Second, func() bool { return !pidAlive(xbergPID) }, "xberg pid %d outlived a SIGKILLed ipfs mcp", xbergPID)
	assert.Empty(t, grepServe(processesMentioning(t, cache)), "no xberg process is left for the test cache")
}

func grepServe(entries []psEntry) []psEntry {
	var out []psEntry
	for _, e := range entries {
		if strings.Contains(e.args, " serve ") {
			out = append(out, e)
		}
	}
	return out
}

// requireOllamaModel skips the test unless the Ollama at api has the model
// (name prefix) pulled.
func requireOllamaModel(t *testing.T, api, prefix string) {
	t.Helper()
	resp, err := http.Get(api + "/api/tags")
	require.NoError(t, err)
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tags))
	for _, m := range tags.Models {
		if strings.HasPrefix(m.Name, prefix) {
			return
		}
	}
	t.Skipf("%s is not pulled into Ollama: run 'ollama pull %s'", prefix, prefix)
}
