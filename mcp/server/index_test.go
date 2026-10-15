package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var indexToolNames = []string{"ipfs_index_search", "ipfs_index_cid", "ipfs_index_status", "ipfs_index_reconcile", "ipfs_index_bulk_start", "ipfs_index_bulk_status", "ipfs_index_bulk_cancel"}

func TestIndexToolsAreOffUntilEmbeddingsAreConfigured(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{noEmbeddings: true})
	cs := e.connect(nil)
	listed := toolNames(t, cs)
	for _, name := range indexToolNames {
		assert.NotContains(t, listed, name)
		assert.Contains(t, e.s.UnavailableTools(), Unavailable{Name: name, Reason: ReasonNotConfigured})
		assert.False(t, e.s.toolRegistered(name))
	}

	e.s.attachEmbeddings(mustService(t, e))
	listed = toolNames(t, e.connect(nil))
	for _, name := range indexToolNames {
		assert.Contains(t, listed, name)
		assert.True(t, e.s.toolRegistered(name))
		assert.NotContains(t, e.s.UnavailableTools(), Unavailable{Name: name, Reason: ReasonNotConfigured})
	}
	// Catalog order is kept after the late registration.
	var idx []string
	for _, n := range e.s.ToolNames() {
		if strings.HasPrefix(n, "ipfs_index_") {
			idx = append(idx, n)
		}
	}
	assert.Equal(t, indexToolNames, idx)
}

func mustService(t *testing.T, e *indexEnv) *embeddings.Service {
	t.Helper()
	set := embeddings.Settings{
		Mode:     embeddings.ModeEmbedded,
		Embedder: embeddings.EmbedderSettings{BaseURL: e.embed.URL + "/v1", Model: "fake-embed", Dimensions: indexTestDims},
	}
	emb, err := embeddings.NewEmbedder(set.Embedder, nil)
	require.NoError(t, err)
	return embeddings.NewService(set, e.store, emb)
}

func TestIndexToolAnnotationsMatchTheContract(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	res, err := e.connect(nil).ListTools(t.Context(), nil)
	require.NoError(t, err)
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	for name, want := range map[string]struct{ readOnly, destructive, idempotent, openWorld bool }{
		"ipfs_index_search":    {true, false, true, false},
		"ipfs_index_cid":       {false, false, true, true},
		"ipfs_index_status":    {true, false, true, false},
		"ipfs_index_reconcile": {false, true, true, false},
	} {
		tool := byName[name]
		require.NotNil(t, tool, name)
		a := tool.Annotations
		assert.Equal(t, want.readOnly, a.ReadOnlyHint, name)
		assert.Equal(t, want.destructive, *a.DestructiveHint, name)
		assert.Equal(t, want.idempotent, a.IdempotentHint, name)
		assert.Equal(t, want.openWorld, *a.OpenWorldHint, name)
		assert.NotEmpty(t, tool.OutputSchema, name)
	}
}

// --- automatic indexing ---------------------------------------------------

func TestAddIsNotIndexedWhenAutoIsOff(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	id := indexIdentity("local")
	res := e.addText(e.connect(&id), "cats purr when content", "note.txt")
	require.False(t, res.IsError, "%+v", res.Content[0].(*mcp.TextContent).Text)
	assert.Zero(t, e.embed.requests.Load(), "no embedder request is made")
	assert.Empty(t, e.store.cids())
	assert.Empty(t, structured(t, res)["warnings"])
}

func TestAddIsIndexedByCIDWhenAutoIsOn(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{Auto: true}})
	id := indexIdentity(e.embed.host())
	res := e.addText(e.connect(&id), "cats purr when content", "note.txt")
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	validateOutput(t, "ipfs_add", out)
	cid := out["cid"].(string)
	assert.Empty(t, out["warnings"])
	assert.Equal(t, []string{cid}, e.store.cids(), "indexed under the CID ipfs_add returned")
	assert.EqualValues(t, 1, e.embed.requests.Load())
	assert.Equal(t, "/ipfs/"+cid+"/note.txt", e.store.rows[0].Path)
	assert.Equal(t, "text/plain", e.store.rows[0].MIME)
	assert.Equal(t, 1, e.d.count("add"), "indexing adds nothing to the node")
	assert.Zero(t, e.d.count("pin/add")+e.d.count("provide/once"))
}

func TestAddOutsidePolicyIsNotIndexed(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{Auto: true, MaxBytes: 64}})
	id := indexIdentity("local")
	cs := e.connect(&id)
	for name, c := range map[string]struct{ content, filename string }{
		"markup not on the allowlist": {"<html><body>cats</body></html>", "page.html"},
		"larger than max_bytes":       {strings.Repeat("cats ", 40), "big.txt"},
	} {
		res := e.addText(cs, c.content, c.filename)
		require.False(t, res.IsError, name)
		assert.Empty(t, structured(t, res)["warnings"], name+": leaving content out is not a warning")
	}
	res := toolCall(t, cs, "ipfs_add", map[string]any{"content": "gIA=", "encoding": "base64", "pin": true, "provide": false})
	require.False(t, res.IsError)
	assert.Zero(t, e.embed.requests.Load())
	assert.Empty(t, e.store.cids())
}

func TestEmptyAllowlistIndexesNothing(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{Auto: true, MIMEAllowlist: []string{}}})
	id := indexIdentity("local")
	e.addText(e.connect(&id), "cats purr", "note.txt")
	assert.Zero(t, e.embed.requests.Load())
}

func TestEmbedderDownIsAWarningNotAFailure(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{Auto: true}})
	e.embed.down.Store(true)
	id := indexIdentity(e.embed.host())
	res := e.addText(e.connect(&id), "cats purr when content", "note.txt")
	require.False(t, res.IsError, "the add succeeds: %+v", res.Content)
	out := structured(t, res)
	validateOutput(t, "ipfs_add", out)
	assert.NotEmpty(t, out["cid"])
	warnings, _ := out["warnings"].([]any)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "not indexed")
	assert.NotContains(t, warnings[0], "cats purr", "no content in a warning")
	assert.Empty(t, e.store.cids())
}

func TestAddIsNotIndexedWithoutEgressAck(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{Auto: true}})
	other := indexIdentity("elsewhere.example")
	for name, id := range map[string]*Identity{"operator": nil, "principal acking another host": &other} {
		res := e.addText(e.connect(id), "cats purr when "+name, "note.txt")
		require.False(t, res.IsError, name)
		warnings, _ := structured(t, res)["warnings"].([]any)
		require.Len(t, warnings, 1, name)
		assert.Contains(t, warnings[0], e.embed.host(), name)
	}
	assert.Zero(t, e.embed.requests.Load(), "nothing is sent")
	assert.Empty(t, e.store.cids())
}

// --- tools ----------------------------------------------------------------

func TestIndexStatus(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{Auto: true}})
	e.seed("bafkreieoswm5yqijue4v3qyit63bd76suvpinoixcelkdzzlfw3uimwtsu", 3)
	e.seed("bafkreiha5sgvksp5ooau53gcgplgt6ximremnu7jf2pchvwgdirglk5xoa", 1)
	cs := e.connect(nil)

	out := structured(t, toolCall(t, cs, "ipfs_index_status", nil))
	validateOutput(t, "ipfs_index_status", out)
	assert.Equal(t, map[string]any{
		"enabled": true, "mode": "embedded", "store_reachable": true, "auto": true,
		"model": "fake-embed", "dimensions": float64(indexTestDims), "indexed_cids": float64(2), "chunk_count": float64(4),
		"by_kind": map[string]any{"text": float64(2)},
	}, out)

	out = structured(t, toolCall(t, cs, "ipfs_index_status", map[string]any{"cid": "bafkreiha5sgvksp5ooau53gcgplgt6ximremnu7jf2pchvwgdirglk5xoa"}))
	assert.Equal(t, true, out["cid_indexed"])
	out = structured(t, toolCall(t, cs, "ipfs_index_status", map[string]any{"cid": "bafkreick7tl6wovuqsllgxing7gkp6wv6xxzkaltwfn5p2ujtb6qzikbgi"}))
	assert.Equal(t, false, out["cid_indexed"])
	assert.Zero(t, e.embed.requests.Load(), "status contacts no embedder")
}

func TestIndexStatusByKind(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{Auto: true}})
	e.seed("bafkreieoswm5yqijue4v3qyit63bd76suvpinoixcelkdzzlfw3uimwtsu", 3)
	rows := []embeddings.Row{
		{CID: "bafkreigqzecctngknadksgwbf6po52s6byvbz4oz5hpr7sky5aamillqcm", Path: "/ipfs/bafkreigqzecctngknadksgwbf6po52s6byvbz4oz5hpr7sky5aamillqcm", MIME: "application/pdf", ChunkIndex: 0, Model: "fake-embed", Vector: make([]float32, indexTestDims), Kind: "document", Source: "extracted", Text: "a"},
		{CID: "bafkreigqzecctngknadksgwbf6po52s6byvbz4oz5hpr7sky5aamillqcm", Path: "/ipfs/bafkreigqzecctngknadksgwbf6po52s6byvbz4oz5hpr7sky5aamillqcm", MIME: "application/pdf", ChunkIndex: 1, Model: "fake-embed", Vector: make([]float32, indexTestDims), Kind: "document", Source: "extracted", Text: "b"},
	}
	require.NoError(t, e.store.Replace(t.Context(), "bafkreigqzecctngknadksgwbf6po52s6byvbz4oz5hpr7sky5aamillqcm", rows))
	cs := e.connect(nil)

	out := structured(t, toolCall(t, cs, "ipfs_index_status", nil))
	validateOutput(t, "ipfs_index_status", out)
	assert.Equal(t, map[string]any{"text": float64(1), "document": float64(1)}, out["by_kind"], "files, not chunks; an unset kind is text")
}

func TestIndexCID(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{MaxBytes: 200, ChunkChars: 200}})
	id := indexIdentity(e.embed.host())
	cs := e.connect(&id)
	text := "cats " + strings.Repeat("purr and stretch in the sun. ", 5)
	e.files["/ipfs/bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"] = []byte(text)
	e.files["/ipfs/bafkreidvfd4h6cvomwmbtoqxwrk7q5qm5hb32xcjrrbhzrprwurlq6gjdm"] = []byte(strings.Repeat("x ", 150))
	e.files["/ipfs/bafkreid4gibwbzbde2b5gp4xzhsv24nd4zznurtd5z73pewbtlt3nffq54"] = []byte{0xff, 0xfe, 0x00}
	e.files["/ipfs/bafkreiaezmhplg6kujt4hyc4nxdunghwefve26eha267ot4smkugmivwyi"] = []byte("  \n ")

	call := func(cid string, force bool) map[string]any {
		res := toolCall(t, cs, "ipfs_index_cid", map[string]any{"cid": cid, "force": force})
		require.False(t, res.IsError, "%+v", res.Content)
		out := structured(t, res)
		validateOutput(t, "ipfs_index_cid", out)
		return out
	}

	out := call("bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i", false)
	assert.Equal(t, "fake-embed", out["model"])
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Equal(t, []string{"bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"}, e.store.cids())
	assert.Equal(t, "/ipfs/bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i", e.store.rows[0].Path)
	assert.Zero(t, e.d.count("add")+e.d.count("pin/add")+e.d.count("provide/once"), "indexing only reads")

	before := e.embed.requests.Load()
	assert.Equal(t, "already_indexed", call("bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i", false)["skipped_reason"])
	assert.Equal(t, before, e.embed.requests.Load(), "already indexed content is not embedded again")
	assert.EqualValues(t, 1, call("bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i", true)["chunks_indexed"], "force re-indexes")

	assert.Equal(t, "too_large", call("bafkreidvfd4h6cvomwmbtoqxwrk7q5qm5hb32xcjrrbhzrprwurlq6gjdm", false)["skipped_reason"])
	assert.Equal(t, "mime_not_allowed", call("bafkreid4gibwbzbde2b5gp4xzhsv24nd4zznurtd5z73pewbtlt3nffq54", false)["skipped_reason"])
	blank := call("bafkreiaezmhplg6kujt4hyc4nxdunghwefve26eha267ot4smkugmivwyi", false)
	assert.Equal(t, "empty", blank["skipped_reason"], "blank content has nothing to embed")
	assert.EqualValues(t, 0, blank["chunks_indexed"])
	assert.Equal(t, "not_a_file", call("dir", false)["skipped_reason"], "a directory is not a file")
	e.mu.Lock()
	offline, read := e.catOffline["/ipfs/bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"]
	e.mu.Unlock()
	require.True(t, read)
	assert.False(t, offline, "ipfs_index_cid reads with the normal, network-capable cat")
	assert.Equal(t, []string{"bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"}, e.store.cids())
}

func TestIndexCIDRefusesWithoutEgressAckBeforeReading(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	e.files["/ipfs/bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"] = []byte("cats purr")
	noAck := indexIdentity()
	for name, id := range map[string]*Identity{"operator": nil, "principal without egress_ack": &noAck} {
		res := toolCall(t, e.connect(id), "ipfs_index_cid", map[string]any{"cid": "bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"})
		require.True(t, res.IsError, name)
		assert.Equal(t, CodeEgressNotAcknowledged, decodeToolError(t, res).Code, name)
	}
	assert.Zero(t, e.d.count("files/stat")+e.d.count("cat"), "nothing is read from the node")
	assert.Zero(t, e.embed.requests.Load())
}

func TestIndexSearchReturnsChunkTextFromTheNode(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{ChunkChars: 200}})
	id := indexIdentity(e.embed.host())
	cs := e.connect(&id)

	doc := "cats purr. " + strings.Repeat("They nap all day. ", 8) + "\n\ndogs bark. " + strings.Repeat("They fetch sticks. ", 12)
	e.files["/ipfs/bafkreianjquaxe4youz372zjb6cf2rh3hqxhcqkys6chpf6wlh5dcsm6dy"] = []byte(doc)
	e.files["/ipfs/bafkreiaa26kgoounqrnig6ditsexxn2egosoplv7wlnsiszrrl5uotghbq"] = []byte("fish swim in the sea")
	for cid := range e.files {
		res := toolCall(t, cs, "ipfs_index_cid", map[string]any{"cid": strings.TrimPrefix(cid, "/ipfs/")})
		require.False(t, res.IsError, "%+v", res.Content)
	}
	chunks := embeddings.Chunk(doc, 200)
	require.GreaterOrEqual(t, len(chunks), 2)

	res := toolCall(t, cs, "ipfs_index_search", map[string]any{"query": "dogs love a walk", "top_k": 1})
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	validateOutput(t, "ipfs_index_search", out)
	assert.Equal(t, "fake-embed", out["model"])
	results := out["results"].([]any)
	require.Len(t, results, 1)
	hit := results[0].(map[string]any)
	assert.Equal(t, "bafkreianjquaxe4youz372zjb6cf2rh3hqxhcqkys6chpf6wlh5dcsm6dy", hit["cid"])
	assert.Equal(t, hit["text"], chunks[int(hit["chunk_index"].(float64))], "the text is the chunk that was embedded")
	assert.True(t, strings.HasPrefix(hit["text"].(string), "dogs bark"))
	assert.InDelta(t, 1.0, hit["score"], 1e-6)
	assert.Equal(t, true, res.Meta[untrustedMetaKey], "search results are untrusted content")

	// Restricting to one CID.
	out = structured(t, toolCall(t, cs, "ipfs_index_search", map[string]any{"query": "fish", "cid": "bafkreianjquaxe4youz372zjb6cf2rh3hqxhcqkys6chpf6wlh5dcsm6dy", "top_k": 50}))
	for _, r := range out["results"].([]any) {
		assert.Equal(t, "bafkreianjquaxe4youz372zjb6cf2rh3hqxhcqkys6chpf6wlh5dcsm6dy", r.(map[string]any)["cid"])
	}
	assert.Len(t, out["results"], len(chunks))
}

func TestIndexSearchDropsHitsWhoseContentIsGone(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	id := indexIdentity(e.embed.host())
	cs := e.connect(&id)
	e.seed("bafkreihb7sw42oji6bz3mn3qqr7iyjjsvyj3ryhhubt6yxrw4jtgdw6qeq", 1) // indexed, but the node cannot cat it
	e.seed("bafkreidjrp5dj7zmeb4bsuigtqcvthqthbo22vvikgokdwfz7sobt7jdea", 1)
	e.files["/ipfs/bafkreidjrp5dj7zmeb4bsuigtqcvthqthbo22vvikgokdwfz7sobt7jdea"] = []byte("dogs bark")
	out := structured(t, toolCall(t, cs, "ipfs_index_search", map[string]any{"query": "cats", "top_k": 5}))
	for _, r := range out["results"].([]any) {
		assert.Equal(t, "bafkreidjrp5dj7zmeb4bsuigtqcvthqthbo22vvikgokdwfz7sobt7jdea", r.(map[string]any)["cid"])
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	require.Contains(t, e.catOffline, "/ipfs/bafkreihb7sw42oji6bz3mn3qqr7iyjjsvyj3ryhhubt6yxrw4jtgdw6qeq")
	for p, offline := range e.catOffline {
		assert.True(t, offline, "search reads %s offline, so a collected block is not fetched from the network", p)
	}
}

func TestIndexSearchErrors(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	id := indexIdentity(e.embed.host())
	e.seed("bafkreieoswm5yqijue4v3qyit63bd76suvpinoixcelkdzzlfw3uimwtsu", 1)

	e.embed.down.Store(true)
	res := toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "cats"})
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeIndexUnavailable, te.Code)
	assert.True(t, te.Retryable)
	assert.NotContains(t, te.Message, "cats", "the query is not echoed")

	// The operator identity covers no host: refused, nothing sent.
	e.embed.down.Store(false)
	before := e.embed.requests.Load()
	res = toolCall(t, e.connect(nil), "ipfs_index_search", map[string]any{"query": "cats"})
	assert.Equal(t, CodeEgressNotAcknowledged, decodeToolError(t, res).Code)
	assert.Equal(t, before, e.embed.requests.Load())

	// A configured principal with no egress_ack is refused by the generic rule.
	noAck := indexIdentity()
	res = toolCall(t, e.connect(&noAck), "ipfs_index_search", map[string]any{"query": "cats"})
	assert.Equal(t, CodeEgressNotAcknowledged, decodeToolError(t, res).Code)
}

func TestIndexCIDAndReconcileNeedConfirmation(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	id := indexIdentity(e.embed.host())
	e.files["/ipfs/bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"] = []byte("cats purr")
	e.seed("bafkreieu3xsbh62zkujitrzgmqyfoqe3mefpramwrul6w6otjtj663qdm4", 1)

	declined := e.connectDeciding(&id, func(*mcp.ElicitRequest) *mcp.ElicitResult { return &mcp.ElicitResult{Action: "decline"} })
	res := toolCall(t, declined, "ipfs_index_cid", map[string]any{"cid": "bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"})
	assert.Equal(t, CodeConfirmDeclined, decodeToolError(t, res).Code)
	res = toolCall(t, declined, "ipfs_index_reconcile", nil)
	assert.Equal(t, CodeConfirmDeclined, decodeToolError(t, res).Code)
	assert.Equal(t, []string{"bafkreieu3xsbh62zkujitrzgmqyfoqe3mefpramwrul6w6otjtj663qdm4"}, e.store.cids(), "nothing changed")
	assert.Zero(t, e.d.count("pin/ls"), "nothing ran")

	var prompts []string
	accepting := e.connectDeciding(&id, func(req *mcp.ElicitRequest) *mcp.ElicitResult {
		prompts = append(prompts, req.Params.Message)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
	callOK(t, accepting, "ipfs_index_cid", map[string]any{"cid": "bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i"})
	callOK(t, accepting, "ipfs_index_reconcile", map[string]any{"dry_run": true})
	require.Len(t, prompts, 2)
	assert.Contains(t, prompts[0], "bafkreifqsh2pn3nv7r5oqcnfokuj4qubog4rqcnp25ggqmqkx762pi6b5i")
	assert.Contains(t, prompts[1], "dry run=true")

	// Without a client that can confirm, a write is refused.
	plain := connect(t, e.s)
	res = toolCall(t, plain, "ipfs_index_reconcile", nil)
	assert.Equal(t, CodeConfirmUnavailable, decodeToolError(t, res).Code)
}

func TestIndexStatusSurvivesUnreachableStore(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	e.s.embeddingsExt = embeddings.NewService(embeddings.Settings{Mode: embeddings.ModeRemote}, brokenStore{e.store}, mustEmbedder(t, e))
	out := structured(t, toolCall(t, e.connect(nil), "ipfs_index_status", nil))
	validateOutput(t, "ipfs_index_status", out)
	assert.Equal(t, false, out["store_reachable"])
	assert.NotContains(t, out, "chunk_count")
}

func mustEmbedder(t *testing.T, e *indexEnv) *embeddings.Embedder {
	t.Helper()
	emb, err := embeddings.NewEmbedder(embeddings.EmbedderSettings{BaseURL: e.embed.URL + "/v1", Model: "fake-embed", Dimensions: indexTestDims}, nil)
	require.NoError(t, err)
	return emb
}

// brokenStore fails every count, as an unreachable store would.
type brokenStore struct{ *memStore }

func (brokenStore) Count(context.Context) (int, error) { return 0, assert.AnError }

func TestIndexSearchInputIsValidatedAgainstTheContract(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	id := indexIdentity(e.embed.host())
	cs := e.connect(&id)
	for _, args := range []map[string]any{{"query": ""}, {"query": "x", "top_k": 51}, {"query": "x", "top_k": 0}, {"query": "x", "extra": 1}} {
		raw, _ := json.Marshal(args)
		res := toolCall(t, cs, "ipfs_index_search", args)
		require.True(t, res.IsError, string(raw))
		assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code, string(raw))
	}
}
