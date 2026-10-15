package server

import (
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinnedUnder reports whether c is a pinned root or reachable from one, which
// is what pin ls --type=all answers for a CID.
func (n *fakeNode) pinnedUnder(c string) bool {
	var reach func(string) bool
	reach = func(from string) bool {
		if from == c {
			return true
		}
		for _, en := range n.dirs[from] {
			if reach(en.CID) {
				return true
			}
		}
		return false
	}
	for _, root := range append(append([]string{}, n.recursive...), n.direct...) {
		if reach(root) {
			return true
		}
	}
	return false
}

// installPinRm makes the fake node's pins answer per CID and removable.
func (n *fakeNode) installPinRm(e *indexEnv) {
	base := e.d.handle["pin/ls"]
	e.d.handle["pin/ls"] = func(args []string, opts map[string]any) (any, error) {
		if len(args) == 0 {
			return base(args, opts)
		}
		c := rootOf(args[0])
		if n.pinnedUnder(c) {
			return map[string]any{"Keys": map[string]any{c: map[string]any{"Type": "recursive"}}}, nil
		}
		return nil, &RPCError{Command: "pin/ls", Message: "path '" + args[0] + "' is not pinned"}
	}
	e.d.handle["pin/rm"] = func(args []string, _ map[string]any) (any, error) {
		c := rootOf(args[0])
		kept := n.recursive[:0]
		for _, p := range n.recursive {
			if p != c {
				kept = append(kept, p)
			}
		}
		n.recursive = kept
		return map[string]any{"Pins": []string{c}}, nil
	}
}

// M1: unpinning a directory through ipfs mcp removes the derived entries of
// every file indexed under it that nothing else covers.
func TestPinRmOfAPinnedDirectoryDropsTheIndexedFilesBelowIt(t *testing.T) {
	n := newFakeNode()
	only := n.file(filler("cats", 20))    // reachable only through the directory
	inMFS := n.file(filler("dogs", 20))   // also a file in the MFS
	alsoPin := n.file(filler("fish", 20)) // also pinned on its own
	nested := n.file(filler("they", 20))  // below a subdirectory
	sub := n.dir("sub", nodeEntry{"nested.txt", nested})
	root := n.dir("root", nodeEntry{"a.txt", only}, nodeEntry{"b.txt", inMFS}, nodeEntry{"c.txt", alsoPin}, nodeEntry{"sub", sub})
	n.pinRecursive(root, alsoPin)
	n.mfs["/"] = []nodeMFS{{Name: "kept.txt", CID: inMFS}}

	b := newTextBulkEnv(t, n, nil)
	n.installPinRm(b.indexEnv)
	b.run(b.principal(), map[string]any{"sources": []any{"pins"}})
	require.ElementsMatch(t, sortedCIDs(only, inMFS, alsoPin, nested), b.indexedCIDs(), "the run indexed each file under its own CID")

	res := toolCall(t, b.connect(nil), "pin_rm", map[string]any{"path": "/ipfs/" + root})
	require.False(t, res.IsError, "%+v", res.Content)
	assert.Empty(t, warningsOf(res))

	assert.ElementsMatch(t, sortedCIDs(inMFS, alsoPin), b.indexedCIDs(),
		"files only the unpinned directory covered are gone; the MFS file and the separately pinned file stay")
	id := b.principal()
	out := structured(t, toolCall(t, b.connect(&id), "ipfs_index_search", map[string]any{"query": "cats", "top_k": 10}))
	for _, h := range out["results"].([]any) {
		cid := h.(map[string]any)["cid"]
		assert.NotContains(t, []any{only, nested}, cid, "no hit of an unpinned file returns")
	}
	for _, cmd := range b.commands() {
		assert.NotEqual(t, "files/ls", cmd, "the MFS is read through its root CID, offline")
	}
}

func TestPinRmWarnsWhenTheUnpinnedDirectoryCannotBeWalked(t *testing.T) {
	n := newFakeNode()
	f := n.file(filler("cats", 20))
	root := n.dir("root", nodeEntry{"a.txt", f})
	n.pinRecursive(root)
	b := newTextBulkEnv(t, n, nil)
	n.installPinRm(b.indexEnv)
	b.run(b.principal(), map[string]any{"sources": []any{"pins"}})
	n.absent[root] = true // the directory block is gone from this node

	res := toolCall(t, b.connect(nil), "pin_rm", map[string]any{"path": "/ipfs/" + root})
	require.False(t, res.IsError, "the unpin itself succeeded: %+v", res.Content)
	warnings := warningsOf(res)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "ipfs_index_reconcile")
	assert.ElementsMatch(t, []string{f}, b.indexedCIDs(), "never delete on a guess")
}

// M1: reconcile treats a file in the MFS as covered.
func TestReconcileKeepsFilesStillInTheMFS(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	inMFS, gone := testCID(t, 4201), testCID(t, 4202)
	e.seed(inMFS, 1)
	e.seed(gone, 1)
	e.mfsFiles = []string{inMFS}
	cs := e.connect(nil)

	res := toolCall(t, cs, "ipfs_index_reconcile", nil)
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	validateOutput(t, "ipfs_index_reconcile", out)
	assert.Equal(t, []any{gone}, out["deleted"])
	assert.EqualValues(t, 1, out["kept"])
	assert.True(t, e.indexed(inMFS), "a file the MFS holds is covered")
	assert.False(t, e.indexed(gone))
	assert.Empty(t, warningsOf(res))
}

func TestReconcileKeepsEntriesWhenTheMFSCannotBeRead(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	c := testCID(t, 4203)
	e.seed(c, 1)
	stat := e.d.handle["files/stat"]
	e.d.handle["files/stat"] = func(args []string, opts map[string]any) (any, error) {
		if args[0] == "/" {
			return nil, &RPCError{Command: "files/stat", Message: "mfs unavailable"}
		}
		return stat(args, opts)
	}
	res := toolCall(t, e.connect(nil), "ipfs_index_reconcile", nil)
	require.False(t, res.IsError, "%+v", res.Content)
	assert.True(t, e.indexed(c), "an unknown answer is not a reason to delete")
	warnings := warningsOf(res)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "MFS")
}

// L3: the confirmation texts name where a file may go.
func TestIndexConfirmationNamesStageHostsAndModelDownloads(t *testing.T) {
	remote := embeddings.Settings{
		Mode:   embeddings.ModeRemote,
		Remote: &embeddings.RemoteSettings{Endpoint: "https://store.example.test:8000"},
	}
	svc := embeddings.NewService(remote, &kindStore{}, mustTestEmbedder(t, "https://embed.example.test/v1"))
	x, err := embeddings.NewExtractor(embeddings.ExtractorSettings{Mode: embeddings.ModeRemote, Remote: &embeddings.ExtractorRemote{BaseURL: "https://extract.example.test"}}, nil)
	require.NoError(t, err)
	d, err := embeddings.NewDescriber(embeddings.DescriberSettings{BaseURL: "https://describe.example.test/v1", Model: "m"}, nil)
	require.NoError(t, err)
	svc.WithExtractor(x).WithDescriber(d)
	s := &Server{embeddingsExt: svc}

	_, extra := s.summaryArgs("ipfs_index_cid", map[string]any{"cid": testCID(t, 1)})
	for _, want := range []string{"embedder embed.example.test", "store store.example.test:8000", "extractor extract.example.test", "describer describe.example.test"} {
		assert.Contains(t, extra, want)
	}
	assert.NotContains(t, extra, "only this machine")

	_, extra = s.summaryArgs("ipfs_index_bulk_start", map[string]any{})
	assert.Contains(t, extra, "extractor extract.example.test")
	assert.Contains(t, extra, "describer describe.example.test")
}

func TestIndexConfirmationNamesTheHuggingFaceHubWhenDownloadsAreOn(t *testing.T) {
	t.Setenv("HF_ENDPOINT", "")
	for _, tc := range []struct {
		name   string
		allow  bool
		mirror string
		want   string
	}{
		{"downloads off", false, "", "Contacts only this machine."},
		{"downloads on", true, "", "model downloads from huggingface.co"},
		{"downloads on with a mirror", true, "https://hf-mirror.example.test", "model downloads from hf-mirror.example.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HF_ENDPOINT", tc.mirror)
			set := embeddings.Settings{
				Mode:      embeddings.ModeEmbedded,
				Extractor: &embeddings.ExtractorSettings{Mode: embeddings.ModeEmbedded, Embedded: &embeddings.ExtractorEmbedded{AllowModelDownloads: tc.allow}},
			}
			svc := embeddings.NewService(set, &kindStore{}, mustTestEmbedder(t, "http://127.0.0.1:9/v1"))
			s := &Server{embeddingsExt: svc}
			for _, tool := range []string{"ipfs_index_cid", "ipfs_index_bulk_start"} {
				_, extra := s.summaryArgs(tool, map[string]any{})
				assert.Contains(t, extra, tc.want, tool)
				if !tc.allow {
					assert.NotContains(t, extra, "huggingface", tool)
				}
			}
		})
	}
}

// L4: a search reads only the start of a file, and no more than its budget.
func TestSearchReadsOnlyThePrefixItNeeds(t *testing.T) {
	pol := embeddings.Policy{ChunkChars: 200, MaxBytes: 1 << 20}
	e := newIndexEnv(t, indexEnvOptions{policy: pol})
	c := testCID(t, 4301)
	long := "cats " + strings.Repeat("filler words go here. ", 20000) // about 440 KB
	e.files["/ipfs/"+c] = []byte(long)
	want := embeddings.Chunk(long, 200)
	require.Greater(t, len(want), 100)
	const deep = 60
	require.NoError(t, e.store.Replace(t.Context(), c, []embeddings.Row{{
		CID: c, Path: "/ipfs/" + c, MIME: "text/plain", ChunkIndex: deep, Model: "fake-embed", Vector: axisVector(0),
	}}))

	id := indexIdentity(e.embed.host())
	out := structured(t, toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "cats", "top_k": 1}))
	hits := out["results"].([]any)
	require.Len(t, hits, 1)
	assert.Equal(t, want[deep], hits[0].(map[string]any)["text"], "the prefix read gives the same chunk as the whole file")

	reads := e.catCalls()
	require.NotEmpty(t, reads)
	last := reads[len(reads)-1]
	assert.EqualValues(t, (deep+2)*200*4, last.length, "only as much as the chunk needs is asked for")
	assert.Less(t, last.length, int64(len(long)))
	assert.True(t, last.offline)
}

func TestSearchPrefixChunksEqualTheWholeFileChunks(t *testing.T) {
	// Chunk is prefix-stable, which is what lets a search read a prefix.
	text := strings.Repeat("alpha beta gamma.\n\ndelta epsilon zeta eta theta iota kappa lambda mu nu xi. ", 400)
	whole := embeddings.Chunk(text, 150)
	for _, upto := range []int{0, 1, 7, 40} {
		prefix := text[:min(len(text), (upto+2)*150*4)]
		got := embeddings.Chunk(prefix, 150)
		require.Greater(t, len(got), upto)
		assert.Equal(t, whole[:upto+1], got[:upto+1], "upto=%d", upto)
	}
}

func TestReadChunksRefusesOnceTheBudgetIsSpent(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{ChunkChars: 200}})
	c := testCID(t, 4302)
	e.files["/ipfs/"+c] = []byte(strings.Repeat("cats purr. ", 1000))
	pol := e.s.embeddingsExt.Policy()

	budget := 100
	assert.Nil(t, e.s.readChunks(t.Context(), "/ipfs/"+c, pol, 0, &budget), "the read would exceed the budget")
	assert.Zero(t, e.d.count("cat"), "nothing is asked of the node")
	assert.Equal(t, 100, budget)

	budget = 10 * 200 * 4
	got := e.s.readChunks(t.Context(), "/ipfs/"+c, pol, 0, &budget)
	assert.NotEmpty(t, got)
	assert.Equal(t, 10*200*4-2*200*4, budget, "the bytes asked for are spent")
}

// I1: offline reads are built from the parsed CID, not the stored path.
func TestSafeIndexPath(t *testing.T) {
	c := testCID(t, 4401)
	other := testCID(t, 4402)
	for _, tc := range []struct{ name, stored, cid, want string }{
		{"empty path", "", c, "/ipfs/" + c},
		{"plain path", "/ipfs/" + c, c, "/ipfs/" + c},
		{"path with a file name", "/ipfs/" + c + "/notes.txt", c, "/ipfs/" + c + "/notes.txt"},
		{"ipns path", "/ipns/evil.example/x", c, "/ipfs/" + c},
		{"another CID", "/ipfs/" + other, c, "/ipfs/" + c},
		{"deep path", "/ipfs/" + c + "/a/b", c, "/ipfs/" + c},
		{"dot dot", "/ipfs/" + c + "/..", c, "/ipfs/" + c},
		{"control character", "/ipfs/" + c + "/a\nb", c, "/ipfs/" + c},
		{"a CID that does not parse", "/ipfs/not-a-cid", "not-a-cid", ""},
		{"path in the CID field", "", "/ipns/evil", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, safeIndexPath(tc.stored, tc.cid))
		})
	}
}

func TestSearchNeverReadsAStoredIPNSPath(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{ChunkChars: 200}})
	c := testCID(t, 4403)
	e.files["/ipfs/"+c] = []byte("cats purr in the sun")
	e.files["/ipns/evil.example/secret"] = []byte("cats secret")
	require.NoError(t, e.store.Replace(t.Context(), c, []embeddings.Row{{
		CID: c, Path: "/ipns/evil.example/secret", MIME: "text/plain", ChunkIndex: 0, Model: "fake-embed", Vector: axisVector(0),
	}}))
	id := indexIdentity(e.embed.host())
	out := structured(t, toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "cats", "top_k": 1}))
	hit := out["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "cats purr in the sun", hit["text"])
	for _, r := range e.catCalls() {
		assert.False(t, strings.HasPrefix(r.path, "/ipns/"), "read %s", r.path)
	}
}
