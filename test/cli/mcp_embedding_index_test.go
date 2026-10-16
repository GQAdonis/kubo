//go:build !windows

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-embedding-index-lifecycle against a real daemon, the
// real ipfs mcp binary over HTTP, and a real surreal binary on an OS-chosen
// loopback port. The embedder is a deterministic fake for the lifecycle test
// (so the assertions are exact) and a real Ollama bge-m3 for the Ollama test.
// Every process a test starts is in its own process group; ipfs mcp is asked
// to stop (which stops its surreal) and the group is killed if it does not.

const indexTestToken = "cli-index-token-0123456789abcdef"

// countingEmbed is an OpenAI-compatible embeddings endpoint whose vectors are
// one-hot on an axis chosen by the first word of the text. It counts the
// embedding requests (POSTs) it gets and can be switched off. The reachability
// probe of ipfs_index_status, one bare GET with no body, is counted apart in
// probes and is not an embedding request.
type countingEmbed struct {
	*httptest.Server
	requests  atomic.Int32
	probes    atomic.Int32
	probeBody atomic.Int32
	down      atomic.Bool
}

var countingAxes = map[string]int{"cats": 0, "dogs": 1, "fish": 2, "birds": 3}

func newCountingEmbed(t *testing.T) *countingEmbed {
	t.Helper()
	e := &countingEmbed{}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			e.probes.Add(1)
			if r.ContentLength > 0 {
				e.probeBody.Add(1)
			}
			_, _ = w.Write([]byte(`{}`))
			return
		}
		e.requests.Add(1)
		if e.down.Load() {
			http.Error(w, `{"error":{"message":"model not loaded"}}`, http.StatusInternalServerError)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		for i, text := range req.Input {
			v := make([]float32, 8)
			first, _, _ := strings.Cut(text, " ")
			if axis, ok := countingAxes[strings.ToLower(first)]; ok {
				v[axis] = 1
			} else {
				v[7] = 1
			}
			data = append(data, item{i, v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(e.Close)
	return e
}

// indexConfig is the ipfs mcp config: one principal that may write content,
// pins and the index, delete pins and collect garbage, and whose egress_ack
// covers loopback; and an embedded surreal store.
func indexConfig(surrealBin, dataDir, embedBase, model string, dims int, policy string) string {
	return fmt.Sprintf(`{
  "principals": [{"name": "indexer", "kind": "human-mcp", "credential": {"env": "CLI_IDX_TOKEN"},
    "write_groups": ["content", "pin", "index"], "destructive_groups": ["pin", "diag"], "egress_ack": ["local"]}],
  "embeddings": {"mode": "embedded", "embedded": {"surreal_binary": %q, "data_dir": %q},
    "embedder": {"base_url": %q, "model": %q, "dimensions": %d}, "policy": %s}
}`, surrealBin, dataDir, embedBase, model, dims, policy)
}

// startIndexMCP starts `ipfs mcp --http` with the config in its own process
// group, waits for it to listen, and stops it (and so its surreal) on cleanup.
func startIndexMCP(t *testing.T, node *harness.Node, cfgJSON string) *mcpHTTPProc {
	t.Helper()
	cfg := writeEmbeddingsConfig(t, cfgJSON)
	cmd := exec.Command(node.IPFSBin, "mcp", "--http=127.0.0.1:0", "--mcp-config", cfg,
		"--tools", "node,content,pin,diag,index,destructive")
	cmd.Env = append(mcpEnv(node.Dir), append(noAmbientConfig, "CLI_IDX_TOKEN="+indexTestToken)...)
	cmd.Dir = node.Dir
	p := &mcpHTTPProc{t: t, cmd: cmd, stderr: &syncBuffer{}}
	cmd.Stderr = p.stderr
	ownProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(20 * time.Second):
			killProcessGroup(cmd)
			<-exited
		}
	})
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if m := mcpHTTPListening.FindStringSubmatch(p.stderr.String()); m != nil {
			p.base = strings.TrimSuffix(m[1], "/mcp")
			return p
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("ipfs mcp --http did not report its address; stderr: %s", p.stderr.String())
	return nil
}

func okResult(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	require.False(t, res.IsError, "%+v", res.Content)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func indexStatus(t *testing.T, cs *mcp.ClientSession, cid string) map[string]any {
	t.Helper()
	args := map[string]any{}
	if cid != "" {
		args["cid"] = cid
	}
	res := callMCP(t, cs, "ipfs_index_status", args)
	requireMatchesOutputSchema(t, "ipfs_index_status", res)
	return okResult(t, res)
}

// mcpAdd adds text through ipfs_add and returns the result.
func mcpAdd(t *testing.T, cs *mcp.ClientSession, text string, pin bool) map[string]any {
	t.Helper()
	res := callMCP(t, cs, "ipfs_add", map[string]any{"content": text, "pin": pin, "provide": false})
	requireMatchesOutputSchemaWithLinks(t, "ipfs_add", res)
	return okResult(t, res)
}

func searchHits(t *testing.T, cs *mcp.ClientSession, query string, args map[string]any) []map[string]any {
	t.Helper()
	a := map[string]any{"query": query}
	for k, v := range args {
		a[k] = v
	}
	res := callMCP(t, cs, "ipfs_index_search", a)
	requireMatchesOutputSchema(t, "ipfs_index_search", res)
	var hits []map[string]any
	for _, r := range okResult(t, res)["results"].([]any) {
		hits = append(hits, r.(map[string]any))
	}
	return hits
}

func pinTypeOf(node *harness.Node, cid string) string {
	r := node.RunIPFS("pin", "ls", "--type=all", cid)
	if r.ExitCode() != 0 {
		return ""
	}
	return strings.TrimSpace(r.Stdout.String())
}

func localRefs(node *harness.Node) string {
	return node.IPFS("refs", "local").Stdout.String()
}

func TestMCPEmbeddingIndexOffByDefault(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	// Principals only: no embeddings section.
	p := startIndexMCP(t, node, `{"principals":[{"name":"indexer","kind":"human-mcp","credential":{"env":"CLI_IDX_TOKEN"},
	  "write_groups":["content"],"egress_ack":["local"]}]}`)
	e := &elicitor{action: "accept"}
	cs := p.mustClient(indexTestToken, e.options())

	for _, n := range mcpListedTools(t, cs) {
		assert.False(t, strings.HasPrefix(n, "ipfs_index_"), "%s is listed while embeddings are off", n)
	}
	status := okResult(t, callMCP(t, cs, "ipfs_node_status", nil))
	unavailable, _ := status["unavailable_tools"].([]any)
	reasons := map[string]string{}
	for _, u := range unavailable {
		m := u.(map[string]any)
		reasons[m["name"].(string)] = m["reason"].(string)
	}
	for _, n := range []string{"ipfs_index_search", "ipfs_index_cid", "ipfs_index_status", "ipfs_index_reconcile"} {
		assert.Equal(t, "not_configured", reasons[n], n)
	}

	// Adding works and says nothing about indexing.
	out := mcpAdd(t, cs, "cats purr", true)
	assert.Empty(t, out["warnings"])
	for _, e := range psTable(t) {
		if e.ppid == p.cmd.Process.Pid {
			t.Errorf("ipfs mcp has a child process while embeddings are off: %s", e.args)
		}
	}
}

// TestMCPEmbeddingIndexAutoOffStillIndexesExplicitly: with embeddings
// configured and policy.auto at its default (off), ipfs_add makes no embedder
// request; ipfs_index_cid indexes on request and the content is found.
func TestMCPEmbeddingIndexAutoOffStillIndexesExplicitly(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	embed := newCountingEmbed(t)
	dataDir := filepath.Join(t.TempDir(), "idx")
	p := startIndexMCP(t, node, indexConfig(bin, dataDir, embed.URL+"/v1", "fake", 8, `{}`))
	e := &elicitor{action: "accept"}
	cs := p.mustClient(indexTestToken, e.options())

	status := indexStatus(t, cs, "")
	assert.Equal(t, false, status["auto"])
	assert.Equal(t, true, status["store_reachable"])

	added := mcpAdd(t, cs, "cats nap in the afternoon sun", false)
	cid := added["cid"].(string)
	assert.Empty(t, added["warnings"])
	assert.Zero(t, embed.requests.Load(), "no embedder request when auto is off")
	assert.EqualValues(t, 0, indexStatus(t, cs, cid)["indexed_cids"])
	assert.Zero(t, embed.probeBody.Load(), "a status probe sends no body")
	assert.LessOrEqual(t, embed.probes.Load(), int32(2), "each status call sends at most one probe")

	refsBefore := localRefs(node)
	res := callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": cid})
	requireMatchesOutputSchema(t, "ipfs_index_cid", res)
	out := okResult(t, res)
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Equal(t, "fake", out["model"])
	assert.Equal(t, refsBefore, localRefs(node), "indexing reads content; it adds no block")
	assert.Equal(t, "", pinTypeOf(node, cid), "indexing pins nothing")

	again := okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": cid}))
	assert.Equal(t, "already_indexed", again["skipped_reason"])

	hits := searchHits(t, cs, "cats", nil)
	require.Len(t, hits, 1)
	assert.Equal(t, cid, hits[0]["cid"])
	assert.Equal(t, "cats nap in the afternoon sun", hits[0]["text"])
	assert.True(t, strings.HasPrefix(hits[0]["path"].(string), "/ipfs/"), "path is a content path")

	// Restricting a search to one CID uses the store's per-CID search.
	assert.Len(t, searchHits(t, cs, "cats", map[string]any{"cid": cid}), 1)
	other := mcpAdd(t, cs, "dogs chase balls", false)["cid"].(string)
	callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": other})
	only := searchHits(t, cs, "cats", map[string]any{"cid": other, "top_k": 5})
	require.Len(t, only, 1)
	assert.Equal(t, other, only[0]["cid"])
}

// TestMCPEmbeddingIndexLifecycle: an added text is indexed by CID and found; a
// second pin keeps the vectors when the first is removed (G12-4); reconcile
// after a CLI unpin deletes them; repo_gc deletes the vectors of what it
// removed.
func TestMCPEmbeddingIndexLifecycle(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	embed := newCountingEmbed(t)
	dataDir := filepath.Join(t.TempDir(), "idx")
	p := startIndexMCP(t, node, indexConfig(bin, dataDir, embed.URL+"/v1", "fake", 8,
		`{"auto": true, "max_bytes": 4096}`))
	e := &elicitor{action: "accept"}
	cs := p.mustClient(indexTestToken, e.options())
	surrealPID, ok := surrealProcess(t, dataDir)
	require.True(t, ok)
	assert.Equal(t, p.cmd.Process.Pid, surrealPID.ppid)

	assert.Equal(t, true, indexStatus(t, cs, "")["auto"])

	// 1. An added text is chunked, embedded and found by CID. It is added
	// unpinned so the pin steps below start from a known state.
	const catsText = "cats purr softly on warm windowsills"
	added := mcpAdd(t, cs, catsText, false)
	x := added["cid"].(string)
	assert.Empty(t, added["warnings"])
	assert.EqualValues(t, 1, embed.requests.Load())
	st := indexStatus(t, cs, x)
	assert.Equal(t, true, st["cid_indexed"])
	assert.EqualValues(t, 1, st["indexed_cids"])
	assert.EqualValues(t, 1, st["chunk_count"])
	hits := searchHits(t, cs, "cats", nil)
	require.Len(t, hits, 1)
	assert.Equal(t, x, hits[0]["cid"])
	assert.Equal(t, catsText, hits[0]["text"])
	assert.InDelta(t, 1.0, hits[0]["score"], 1e-3)

	// 2. Outside the policy nothing is embedded and nothing is said.
	before := embed.requests.Load()
	big := mcpAdd(t, cs, "dogs "+strings.Repeat("bark ", 1000), false)
	assert.Empty(t, big["warnings"], "larger than max_bytes: left out, not a warning")
	bin64 := callMCP(t, cs, "ipfs_add", map[string]any{"content": "gIA=", "encoding": "base64", "pin": false, "provide": false})
	assert.Empty(t, okResult(t, bin64)["warnings"])
	assert.Equal(t, before, embed.requests.Load())

	// 3. A failing embedder is a warning; the add succeeds.
	embed.down.Store(true)
	failed := mcpAdd(t, cs, "birds sing at dawn", false)
	assert.NotEmpty(t, failed["cid"])
	warnings, _ := failed["warnings"].([]any)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "not indexed")
	embed.down.Store(false)
	assert.EqualValues(t, 1, indexStatus(t, cs, "")["indexed_cids"], "the failed add left no vectors")

	// 4. G12-4: pinned twice, one pin removed, vectors stay. X is pinned
	// directly and, through a directory that holds the same bytes, indirectly.
	require.Zero(t, node.RunIPFS("pin", "add", "--recursive=false", x).ExitCode())
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cats.txt"), []byte(catsText), 0o600))
	d := strings.TrimSpace(node.IPFS("add", "-r", "-Q", "--pin=true", dir).Stdout.String())
	require.Contains(t, node.IPFS("refs", d).Stdout.String(), x, "the directory holds the same file CID")
	rm := callMCP(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + x, "recursive": false})
	requireMatchesOutputSchema(t, "pin_rm", rm)
	assert.Contains(t, pinTypeOf(node, x), "indirect", "the direct pin is gone, an indirect one remains")
	assert.Equal(t, true, indexStatus(t, cs, x)["cid_indexed"], "another pin covers it: vectors stay")

	// 5. Unpinned outside ipfs mcp: the vectors drift until reconcile.
	require.Zero(t, node.RunIPFS("pin", "rm", d).ExitCode())
	assert.Equal(t, "", pinTypeOf(node, x))
	assert.Equal(t, true, indexStatus(t, cs, x)["cid_indexed"], "no pin event reaches ipfs mcp")
	pinned := mcpAdd(t, cs, "fish swim in the sea", true)
	y := pinned["cid"].(string)

	dry := okResult(t, callMCP(t, cs, "ipfs_index_reconcile", map[string]any{"dry_run": true}))
	assert.Equal(t, true, dry["dry_run"])
	assert.Equal(t, []any{x}, dry["deleted"])
	assert.EqualValues(t, 1, dry["kept"])
	assert.Equal(t, true, indexStatus(t, cs, x)["cid_indexed"], "a dry run deletes nothing")
	// One indexed CID per page: the cursor walks the store in CID order.
	var walked []any
	cursor := ""
	for pages := 0; ; pages++ {
		require.Less(t, pages, 5)
		args := map[string]any{"dry_run": true, "limit": 1}
		if cursor != "" {
			args["cursor"] = cursor
		}
		page := okResult(t, callMCP(t, cs, "ipfs_index_reconcile", args))
		assert.EqualValues(t, 1, page["checked"])
		walked = append(walked, page["deleted"].([]any)...)
		info := page["page"].(map[string]any)
		if info["has_more"] != true {
			break
		}
		cursor = info["next_cursor"].(string)
	}
	assert.Equal(t, []any{x}, walked)
	res := callMCP(t, cs, "ipfs_index_reconcile", nil)
	requireMatchesOutputSchema(t, "ipfs_index_reconcile", res)
	assert.Equal(t, []any{x}, okResult(t, res)["deleted"])
	assert.Equal(t, false, indexStatus(t, cs, x)["cid_indexed"])
	assert.Equal(t, true, indexStatus(t, cs, y)["cid_indexed"], "pinned content keeps its vectors")

	// 6. repo_gc deletes the vectors of the indexed content it removed.
	z := mcpAdd(t, cs, "birds nest in the old oak tree", false)["cid"].(string)
	require.Equal(t, true, indexStatus(t, cs, z)["cid_indexed"])
	gc := callMCP(t, cs, "repo_gc", nil)
	requireMatchesOutputSchema(t, "repo_gc", gc)
	assert.Equal(t, false, indexStatus(t, cs, z)["cid_indexed"], "its blocks were collected")
	assert.Equal(t, true, indexStatus(t, cs, y)["cid_indexed"], "pinned content survives")
	assert.Equal(t, "fish swim in the sea", searchHits(t, cs, "fish", nil)[0]["text"])

	// 7. Every write and destructive call above was confirmed with the person.
	assert.NotEmpty(t, e.prompts)
	var sawDryRun bool
	for _, m := range e.prompts {
		sawDryRun = sawDryRun || strings.Contains(m, "dry run=true")
	}
	assert.True(t, sawDryRun, "the reconcile prompt says whether it is a dry run: %v", e.prompts)

	// 8. A search with the embedder down is index_unavailable, not a crash.
	embed.down.Store(true)
	res = callMCP(t, cs, "ipfs_index_search", map[string]any{"query": "fish"})
	assert.Equal(t, "index_unavailable", mcpToolErrorCode(t, res))
	embed.down.Store(false)

	// 9. The store is the supervised child and stops with ipfs mcp.
	_, running := surrealProcess(t, dataDir)
	assert.True(t, running)
	assert.NotContains(t, p.stderr.String(), catsText, "no content in the log")
}
