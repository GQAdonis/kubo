//go:build !windows

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-whole-node-indexing against a real daemon, the real
// ipfs mcp binary over HTTP and a real surreal binary, with a local fake
// embedder (deterministic, so the assertions are exact). Each test uses its own
// IPFS_PATH, OS-chosen loopback ports and a surreal on an OS-chosen port.

// bulkConfig is indexConfig with extra keys inside the embeddings section
// (bulk, search_route, describer) and an explicit read grant.
func bulkConfig(surrealBin, dataDir, embedBase, extra string) string {
	if extra != "" {
		extra = ", " + extra
	}
	return fmt.Sprintf(`{
  "principals": [{"name": "indexer", "kind": "human-mcp", "credential": {"env": "CLI_IDX_TOKEN"},
    "groups": ["*"], "write_groups": ["content", "pin", "index"], "destructive_groups": ["pin", "diag"], "egress_ack": ["local"]}],
  "embeddings": {"mode": "embedded", "embedded": {"surreal_binary": %q, "data_dir": %q},
    "embedder": {"base_url": %q, "model": "fake", "dimensions": 8}, "policy": {}%s}
}`, surrealBin, dataDir, embedBase, extra)
}

// bulkNode builds a node with pinned files, a pinned directory and an MFS file
// that is not pinned. It returns the CIDs of the files that hold text.
type bulkFixture struct {
	pinnedCats, pinnedDogs, inDir, mfsBirds string
}

func buildBulkNode(t *testing.T, node *harness.Node) bulkFixture {
	t.Helper()
	var f bulkFixture
	f.pinnedCats = node.IPFSAddStr("cats purr in the sun", "--cid-version=1")
	f.pinnedDogs = node.IPFSAddStr("dogs chase the ball", "--cid-version=1")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fish.txt"), []byte("fish swim in the pond"), 0o600))
	node.IPFS("add", "-r", "-Q", "--cid-version=1", dir) // pinned recursively
	f.inDir = node.IPFS("add", "-Q", "--only-hash", "--cid-version=1", filepath.Join(dir, "fish.txt")).Stdout.Trimmed()
	// birds: kept by MFS only, never pinned.
	f.mfsBirds = node.IPFSAddStr("birds sing at dawn", "--cid-version=1", "--pin=false")
	node.IPFS("files", "mkdir", "-p", "/notes")
	node.IPFS("files", "cp", "/ipfs/"+f.mfsBirds, "/notes/birds.txt")
	return f
}

// waitBulk polls ipfs_index_bulk_status until the run is no longer running.
func waitBulk(t *testing.T, cs *mcp.ClientSession) map[string]any {
	t.Helper()
	var run map[string]any
	eventually(t, 90*time.Second, func() bool {
		st := okResult(t, callMCP(t, cs, "ipfs_index_bulk_status", nil))
		r, _ := st["run"].(map[string]any)
		if r == nil || r["state"] == "running" {
			return false
		}
		run = r
		return true
	}, "the bulk run did not end")
	return run
}

// TestMCPBulkIndexNoInjection: a bulk run over pins and MFS reads only. The pin
// set, the local block set and the MFS tree are identical before and after, the
// content is found by search, and the embedder saw only chunks of that content.
func TestMCPBulkIndexNoInjection(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	f := buildBulkNode(t, node)

	pinsBefore := sortedLines(node.IPFS("pin", "ls").Stdout.String())
	refsBefore := sortedLines(localRefs(node))
	mfsBefore := sortedLines(node.IPFS("files", "ls", "-l", "/").Stdout.String())
	mfsRootBefore := node.IPFS("files", "stat", "--hash", "/").Stdout.Trimmed()

	embed := newCountingEmbed(t)
	p := startIndexMCP(t, node, bulkConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", ""))
	e := &elicitor{action: "accept"}
	cs := p.mustClient(indexTestToken, e.options())

	res := callMCP(t, cs, "ipfs_index_bulk_start", map[string]any{})
	started := okResult(t, res)
	assert.Equal(t, "running", started["state"])
	assert.Equal(t, []any{"pins", "mfs"}, started["sources"])
	require.Len(t, e.prompts, 1, "the start was confirmed")
	assert.Contains(t, e.prompts[0], "pins, mfs")
	assert.Contains(t, e.prompts[0], "Contacts only this machine")

	run := waitBulk(t, cs)
	assert.Equal(t, "completed", run["state"], "%v", run)
	assert.Equal(t, "indexer", run["started_by"])
	assert.EqualValues(t, 4, run["files_indexed"], "two pins, the file in the pinned directory and the MFS-only file: %v", run)

	// Nothing was added, pinned, written or announced.
	assert.Equal(t, pinsBefore, sortedLines(node.IPFS("pin", "ls").Stdout.String()), "the pin set")
	assert.Equal(t, refsBefore, sortedLines(localRefs(node)), "the local block set")
	assert.Equal(t, mfsBefore, sortedLines(node.IPFS("files", "ls", "-l", "/").Stdout.String()), "the MFS listing")
	assert.Equal(t, mfsRootBefore, node.IPFS("files", "stat", "--hash", "/").Stdout.Trimmed(), "the MFS root")

	// The content is searchable, including the MFS-only file.
	for query, want := range map[string]string{"cats": f.pinnedCats, "dogs": f.pinnedDogs, "fish": f.inDir, "birds": f.mfsBirds} {
		hits := searchHits(t, cs, query, nil)
		require.NotEmpty(t, hits, query)
		assert.Equal(t, want, hits[0]["cid"], query)
	}

	// A second run finds everything indexed and embeds nothing more.
	requestsBefore := embed.requests.Load()
	okResult(t, callMCP(t, cs, "ipfs_index_bulk_start", map[string]any{}))
	again := waitBulk(t, cs)
	assert.EqualValues(t, 0, again["files_indexed"])
	assert.EqualValues(t, 4, again["skipped_by_reason"].(map[string]any)["already_indexed"])
	assert.Equal(t, requestsBefore, embed.requests.Load(), "nothing was embedded again")
}

// TestMCPBulkIndexSkipsWhatIsNotLocal: a pinned file whose data blocks are not
// on the node is skipped not_local and nothing is fetched back.
func TestMCPBulkIndexSkipsWhatIsNotLocal(t *testing.T) {
	bin := surrealBinary(t)
	// A source node (no daemon) makes a multi-block file; the indexing node gets
	// only its root block and a direct pin, which needs just that block.
	source := harness.NewT(t).NewNode().Init()
	root := source.IPFSAddDeterministic("600KB", "not-local", "--cid-version=1", "--raw-leaves", "--chunker=size-262144")
	rootBlock := source.IPFS("block", "get", root).Stdout.Bytes()

	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	good := node.IPFSAddStr("cats purr in the sun", "--cid-version=1")
	put := node.PipeToIPFS(bytes.NewReader(rootBlock), "block", "put", "--cid-codec=dag-pb", "--mhtype=sha2-256")
	require.Equal(t, root, put.Stdout.Trimmed(), "the root block has the same CID on both nodes")
	node.IPFS("pin", "add", "--recursive=false", root)
	refsBefore := sortedLines(localRefs(node))

	embed := newCountingEmbed(t)
	p := startIndexMCP(t, node, bulkConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", ""))
	cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())
	okResult(t, callMCP(t, cs, "ipfs_index_bulk_start", map[string]any{"sources": []any{"pins"}}))
	run := waitBulk(t, cs)

	assert.Equal(t, "completed", run["state"], "%v", run)
	assert.EqualValues(t, 1, run["files_indexed"], "%v", run)
	assert.EqualValues(t, 1, run["skipped_by_reason"].(map[string]any)["not_local"], "%v", run)
	hits := searchHits(t, cs, "cats", nil)
	require.NotEmpty(t, hits)
	assert.Equal(t, good, hits[0]["cid"])
	assert.Equal(t, refsBefore, sortedLines(localRefs(node)), "no missing block was fetched")
}

// TestMCPBulkIndexMFSLazyCopyIsNeverFetched: the MFS walk is offline. A node
// copies a third party's directory into MFS with `files cp`, which keeps only the
// root block, and the source goes away. A bulk run over MFS finishes in bounded
// time, skips the entries whose blocks are not local as not_local, and the local
// block set is unchanged: nothing was requested from the network. (The daemon's
// files/ls would have loaded the children through bitswap and waited for the
// departed source.)
func TestMCPBulkIndexMFSLazyCopyIsNeverFetched(t *testing.T) {
	bin := surrealBinary(t)
	nodes := harness.NewT(t).NewNodes(2).Init().StartDaemons()
	source, node := nodes[0], nodes[1]
	defer node.StopDaemon()
	require.NoError(t, node.ConnectAndWait(source, 30*time.Second))

	dir := t.TempDir()
	for name, text := range map[string]string{"one.txt": "one lazy cats", "two.txt": "two lazy dogs", "three.txt": "three lazy fish"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600))
	}
	root := source.IPFS("add", "-r", "-Q", "--cid-version=1", dir).Stdout.Trimmed()

	node.IPFS("files", "cp", "/ipfs/"+root, "/lazy")
	local := node.IPFSAddStr("birds sing at dawn", "--cid-version=1", "--pin=false")
	node.IPFS("files", "cp", "/ipfs/"+local, "/birds.txt")
	refs := localRefs(node)
	locality := node.IPFS("files", "stat", "--with-local", "/lazy").Stdout.String()
	require.Contains(t, locality, "Local:", locality)
	require.NotContains(t, locality, "(100.00%)", "the copy is lazy: the children are not on the node")
	source.StopDaemon() // from here a request for a missing block would wait for nobody

	refsBefore := sortedLines(refs)
	embed := newCountingEmbed(t)
	p := startIndexMCP(t, node, bulkConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", ""))
	cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())
	started := time.Now()
	okResult(t, callMCP(t, cs, "ipfs_index_bulk_start", map[string]any{"sources": []any{"mfs"}}))
	run := waitBulk(t, cs)

	assert.Less(t, time.Since(started), 60*time.Second, "the run is bounded")
	assert.Equal(t, "completed", run["state"], "%v", run)
	assert.EqualValues(t, 1, run["files_indexed"], "only the local file: %v", run)
	assert.EqualValues(t, 3, run["skipped_by_reason"].(map[string]any)["not_local"], "each lazy child: %v", run)
	assert.Equal(t, refsBefore, sortedLines(localRefs(node)), "no missing block was fetched")
	hits := searchHits(t, cs, "birds", nil)
	require.NotEmpty(t, hits)
	assert.Equal(t, local, hits[0]["cid"])
}

// TestKuboFilesLsOfflineStillFetchesLazyMFSChildren pins the daemon behavior
// the MFS walk is built around (and the reason it does not call files/ls): the
// global offline option reaches only commands that take their API from
// cmdenv.GetApi, and `files ls -l` loads MFS directory children through the
// online DAG service. If Kubo ever makes it honor offline, this test fails and
// the walk may use files/ls again. The walk's own commands, `files stat` and
// `ls` on /ipfs/<root>/..., stay local.
func TestKuboFilesLsOfflineStillFetchesLazyMFSChildren(t *testing.T) {
	nodes := harness.NewT(t).NewNodes(2).Init().StartDaemons()
	source, node := nodes[0], nodes[1]
	defer node.StopDaemon()
	defer source.StopDaemon()
	require.NoError(t, node.ConnectAndWait(source, 30*time.Second))

	dir := t.TempDir()
	for _, name := range []string{"one.txt", "two.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name+" lazy"), 0o600))
	}
	root := source.IPFS("add", "-r", "-Q", "--cid-version=1", dir).Stdout.Trimmed()
	node.IPFS("files", "cp", "/ipfs/"+root, "/lazy")
	before := sortedLines(localRefs(node))

	// The walk's reads leave the local block set alone.
	rootCID := node.IPFS("files", "stat", "--hash", "/").Stdout.Trimmed()
	node.IPFS("ls", "--offline", "/ipfs/"+rootCID+"/lazy", "--resolve-type=false", "--size=false")
	res := node.RunIPFS("files", "stat", "--offline", "/ipfs/"+rootCID+"/lazy/one.txt")
	assert.NotZero(t, res.ExitCode(), "the child block is not on the node, so an offline stat fails")
	assert.Equal(t, before, sortedLines(localRefs(node)), "offline ls and stat fetched nothing")

	// files ls -l with --offline loads the children from the network.
	node.IPFS("files", "ls", "-l", "--offline", "/lazy")
	assert.Greater(t, len(sortedLines(localRefs(node))), len(before), "files ls ignored offline and fetched the children")
}

// TestMCPBulkIndexDisabled: bulk.enabled false leaves the three tools out of
// the tool list and their RPC paths out of the printed list; the other index
// tools keep working.
func TestMCPBulkIndexDisabled(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	embed := newCountingEmbed(t)
	cfg := bulkConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", `"bulk": {"enabled": false}`)
	p := startIndexMCP(t, node, cfg)
	cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

	listed := mcpListedTools(t, cs)
	for _, n := range []string{"ipfs_index_bulk_start", "ipfs_index_bulk_status", "ipfs_index_bulk_cancel"} {
		assert.NotContains(t, listed, n)
	}
	for _, n := range []string{"ipfs_index_search", "ipfs_index_cid", "ipfs_index_status", "ipfs_index_reconcile"} {
		assert.Contains(t, listed, n)
	}
	status := okResult(t, callMCP(t, cs, "ipfs_node_status", nil))
	reasons := map[string]string{}
	for _, u := range status["unavailable_tools"].([]any) {
		m := u.(map[string]any)
		reasons[m["name"].(string)] = m["reason"].(string)
	}
	assert.Equal(t, "not_configured", reasons["ipfs_index_bulk_start"], "ipfs_node_status says why the tool is not offered")

	// The printed RPC paths: the bulk-only commands are gone with the tools.
	cfgFile := writeEmbeddingsConfig(t, cfg)
	stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--print-rpc-allowed-paths", "--mcp-config", cfgFile, "--tools", "index")
	require.Zero(t, code, stderr)
	assert.NotContains(t, stdout, "/api/v0/files/ls")
	assert.Contains(t, stdout, "/api/v0/cat")

	enabled := writeEmbeddingsConfig(t, bulkConfig(bin, filepath.Join(t.TempDir(), "idx2"), embed.URL+"/v1", ""))
	stdout, stderr, code = runMCP(t, node.IPFSBin, node.Dir, "--print-rpc-allowed-paths", "--mcp-config", enabled, "--tools", "index")
	require.Zero(t, code, stderr)
	for _, path := range []string{"/api/v0/ls", "/api/v0/pin/ls", "/api/v0/files/stat", "/api/v0/cat"} {
		assert.Contains(t, stdout, path)
	}

	// With no embeddings section no index tool is served, so none of their
	// commands is allowed.
	none := writeEmbeddingsConfig(t, `{}`)
	stdout, stderr, code = runMCP(t, node.IPFSBin, node.Dir, "--print-rpc-allowed-paths", "--mcp-config", none, "--tools", "index")
	require.Zero(t, code, stderr)
	assert.NotContains(t, stdout, "/api/v0/cat")
}

// indexRoute sends one request to the search routes with the test token.
func indexRoute(t *testing.T, p *mcpHTTPProc, method, path, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, p.base+path, strings.NewReader(body))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(b)
}

// TestMCPIndexSearchRouteDisabled: search_route.enabled false answers 404 on
// both routes and dials nothing; left on, the routes answer.
func TestMCPIndexSearchRouteDisabled(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	node.IPFSAddStr("cats purr in the sun", "--cid-version=1")

	t.Run("switched off", func(t *testing.T) {
		embed := newCountingEmbed(t)
		p := startIndexMCP(t, node, bulkConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", `"search_route": {"enabled": false}`))
		for _, r := range []struct{ method, path, body string }{
			{http.MethodPost, "/index/search", `{"query":"cats"}`},
			{http.MethodGet, "/index/status", ""},
		} {
			code, _ := indexRoute(t, p, r.method, r.path, indexTestToken, r.body)
			assert.Equal(t, http.StatusNotFound, code, "%s %s", r.method, r.path)
		}
		assert.Zero(t, embed.requests.Load(), "nothing was dialed")
		// The same server still serves the tools.
		cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())
		assert.Contains(t, mcpListedTools(t, cs), "ipfs_index_search")
	})

	t.Run("on by default", func(t *testing.T) {
		embed := newCountingEmbed(t)
		p := startIndexMCP(t, node, bulkConfig(bin, filepath.Join(t.TempDir(), "idx"), embed.URL+"/v1", ""))
		code, _ := indexRoute(t, p, http.MethodPost, "/index/search", "", `{"query":"cats"}`)
		assert.Equal(t, http.StatusUnauthorized, code, "the route is mounted and needs a bearer")
		code, body := indexRoute(t, p, http.MethodGet, "/index/status", indexTestToken, "")
		require.Equal(t, http.StatusOK, code, body)
		var st map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &st))
		assert.Equal(t, true, st["enabled"])
		assert.Equal(t, "embedded", st["mode"])
	})
}
