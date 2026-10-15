package server

import (
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connectPlain opens a session as id whose client cannot elicit.
func (b *bulkEnv) connectPlain(id *Identity) *mcp.ClientSession {
	b.t.Helper()
	ctx := b.t.Context()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := b.s.MCP().Connect(ctx, st, nil)
	require.NoError(b.t, err)
	b.t.Cleanup(func() { _ = ss.Close() })
	if id != nil {
		b.mu.Lock()
		b.ids[ss] = *id
		b.mu.Unlock()
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "plain", Version: "0"}, nil).Connect(ctx, ct, nil)
	require.NoError(b.t, err)
	b.t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// --- 1.2: both write tools are confirmed ------------------------------------------------------------

func TestBulkWriteToolsAreRefusedWithoutAConfirmation(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	id := b.principal()

	t.Run("a client that declines", func(t *testing.T) {
		cs := b.connectDeciding(&id, func(*mcp.ElicitRequest) *mcp.ElicitResult { return &mcp.ElicitResult{Action: "decline"} })
		for _, tool := range []string{"ipfs_index_bulk_start", "ipfs_index_bulk_cancel"} {
			res := toolCall(t, cs, tool, nil)
			assert.Equal(t, CodeConfirmDeclined, toolErr(t, res).Code, tool)
		}
	})

	t.Run("a client that cannot elicit and no resolver", func(t *testing.T) {
		cs := b.connectPlain(&id)
		for _, tool := range []string{"ipfs_index_bulk_start", "ipfs_index_bulk_cancel"} {
			res := toolCall(t, cs, tool, nil)
			assert.Equal(t, CodeConfirmUnavailable, toolErr(t, res).Code, tool)
		}
	})

	t.Run("the operator over stdio", func(t *testing.T) {
		cs := b.connectPlain(nil)
		res := toolCall(t, cs, "ipfs_index_bulk_cancel", nil)
		assert.Equal(t, CodeConfirmUnavailable, toolErr(t, res).Code)
	})

	assert.Nil(t, b.store.stored(t), "nothing started")
	assert.Nil(t, b.s.bulk.active())
	assert.Zero(t, b.embed.requests.Load())
	assert.Zero(t, b.d.count("pin/ls"), "the walk never began")
}

func TestBulkStartAfterAnApprovedConfirmationRunsAndTheTextNamesWhatItDoes(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	id := b.principal()
	var asked []string
	cs := b.connectDeciding(&id, func(r *mcp.ElicitRequest) *mcp.ElicitResult {
		asked = append(asked, r.Params.Message)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
	res := toolCall(t, cs, "ipfs_index_bulk_start", map[string]any{"sources": []any{"pins"}, "max_files": 7})
	require.False(t, res.IsError, "%+v", res.Content)
	b.wait()

	require.Len(t, asked, 1)
	text := asked[0]
	assert.Contains(t, text, "pins", "the sources")
	assert.NotContains(t, text, "mfs", "only the sources in effect")
	assert.Contains(t, text, "7 files", "the effective file cap")
	assert.Contains(t, text, "1073741824 bytes", "the byte cap is the configured default, not the word default")
	assert.Contains(t, text, "30 files/min")
	assert.Contains(t, text, "Contacts only this machine", "every host the run may contact is named, or that there is none")
	assert.NotContains(t, text, "default")
	assert.Equal(t, embeddings.RunCompleted, b.store.stored(t).State)
}

func TestBulkConfirmationNamesEveryNonLoopbackHost(t *testing.T) {
	svc := embeddings.NewService(
		embeddings.Settings{Mode: embeddings.ModeRemote, Remote: &embeddings.RemoteSettings{Endpoint: "https://store.example.test:8000"}},
		&kindStore{}, mustTestEmbedder(t, "https://embed.example.test/v1"))
	d, err := embeddings.NewDescriber(embeddings.DescriberSettings{BaseURL: "https://describe.example.test/v1", Model: "m"}, nil)
	require.NoError(t, err)
	svc.WithDescriber(d)
	s := &Server{embeddingsExt: svc}

	args, extra := s.summaryArgs("ipfs_index_bulk_start", map[string]any{"max_files": 5})
	for _, host := range []string{"embed.example.test", "store.example.test:8000", "describe.example.test"} {
		assert.Contains(t, extra, host)
	}
	assert.Equal(t, "pins, mfs", args["sources"])
	assert.Equal(t, 5, args["max_files"])

	other, extra := s.summaryArgs("pin_add", map[string]any{"path": "x"})
	assert.Empty(t, extra, "other tools are not touched")
	assert.Equal(t, map[string]any{"path": "x"}, other)

	// The text a person reads still renders from the catalog's template.
	tool, ok := mustCatalog(t).Tool("ipfs_index_bulk_start")
	require.True(t, ok)
	sum := buildSummary(tool, nil, args)
	assert.True(t, strings.HasPrefix(sum.Text, "Bulk-index pins, mfs under MFS root /"), sum.Text)
}

func TestBulkConfirmationTextOfAResumeShowsTheStoredRun(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "p", State: embeddings.RunPaused, Sources: []string{"pins"}, Caps: embeddings.RunCaps{MaxFiles: 3, MaxBytes: 99, FilesPerMinute: 4},
	}))
	args, _ := b.s.summaryArgs("ipfs_index_bulk_start", map[string]any{"resume": true})
	assert.Equal(t, "pins", args["sources"])
	assert.Equal(t, 3, args["max_files"])
	assert.Equal(t, int64(99), args["max_bytes"])
	assert.Equal(t, true, args["resume"])
}

func mustCatalog(t *testing.T) *contract.Catalog {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	return cat
}

// --- 1.7: disabled states ---------------------------------------------------------------------------

func TestBulkToolsAreNotListedWhenBulkIsDisabled(t *testing.T) {
	off := false
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.Enabled = &off })
	id := b.principal()
	listed := toolNames(t, b.connect(&id))
	for _, name := range bulkTools {
		assert.NotContains(t, listed, name)
		assert.False(t, b.s.toolRegistered(name))
		assert.Contains(t, b.s.UnavailableTools(), Unavailable{Name: name, Reason: ReasonNotConfigured})
	}
	for _, name := range []string{"ipfs_index_search", "ipfs_index_cid", "ipfs_index_status", "ipfs_index_reconcile"} {
		assert.Contains(t, listed, name, "the other index tools stay")
	}

	_, err := b.connect(&id).CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_index_bulk_start"})
	assert.Error(t, err, "the tool does not exist")
	assert.Zero(t, b.d.count("pin/ls"))
}

func TestBulkRunIsInertWithoutRunState(t *testing.T) {
	// A store that keeps no run state cannot back the bulk tools.
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	id := b.principal()
	b.s.embeddingsExt = embeddings.NewService(embeddings.Settings{Mode: embeddings.ModeEmbedded}, noRunStore{b.store}, mustTestEmbedder(t, b.embed.URL+"/v1"))
	res := toolCall(t, b.connect(&id), "ipfs_index_bulk_status", nil)
	assert.Equal(t, CodeToolUnavailable, toolErr(t, res).Code)
}

// noRunStore hides the run state of a store.
type noRunStore struct{ embeddings.Store }

// --- 1.6: the printed RPC paths ------------------------------------------------------------------------

func TestPrintedRPCPathsIncludeTheBulkCommandsOnlyWithTheBulkTools(t *testing.T) {
	with, err := RPCAllowedPaths(Options{Tools: []string{"index"}})
	require.NoError(t, err)
	for _, p := range []string{"cat", "files/stat", "ls", "pin/ls"} {
		assert.Contains(t, with, "/api/v0/"+p)
	}

	without, err := RPCAllowedPaths(Options{Tools: []string{"index"}, Disabled: BulkTools()})
	require.NoError(t, err)
	assert.NotContains(t, with, "/api/v0/files/ls", "the offline MFS walk reads through the root CID and never needs files/ls")
	assert.NotContains(t, without, "/api/v0/files/ls")
	assert.NotContains(t, without, "/api/v0/ls", "only the bulk start lists directories")
	assert.Contains(t, without, "/api/v0/pin/ls", "reconcile still checks pins")
	assert.Contains(t, without, "/api/v0/cat")

	for _, p := range with {
		assert.NotContains(t, []string{
			"/api/v0/add", "/api/v0/pin/add", "/api/v0/files/write", "/api/v0/files/cp", "/api/v0/routing/provide", "/api/v0/block/put", "/api/v0/dag/put",
		}, p, "the bulk tools need no command that writes or announces")
	}
}

func TestBulkToolsNeedNoCommandThatWritesOrAnnounces(t *testing.T) {
	cat := mustCatalog(t)
	for _, name := range bulkTools {
		tool, _ := cat.Tool(name)
		for _, c := range tool.RPC {
			assert.Contains(t, []string{"cat", "files/stat", "ls", "pin/ls"}, c, name)
		}
	}
}
