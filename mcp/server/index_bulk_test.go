package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// textPolicy lets the bulk tests index the text types a directory walk meets.
var textPolicy = embeddings.Policy{MIMEAllowlist: []string{"text/plain", "text/markdown"}}

// threeFiles is a node with three pinned text files of ten bytes each.
func threeFiles() (*fakeNode, []string) {
	n := newFakeNode()
	cids := []string{n.file(filler("cats", 10)), n.file(filler("dogs", 10)), n.file(filler("fish", 10))}
	n.pinRecursive(cids...)
	return n, sortedCIDs(cids...)
}

func newTextBulkEnv(t *testing.T, n *fakeNode, mut func(*mediaOpts)) *bulkEnv {
	t.Helper()
	o := mediaOpts{policy: textPolicy}
	if mut != nil {
		mut(&o)
	}
	return newBulkEnv(t, bulkOpts{media: o, node: n})
}

// --- 1.1: the run engine ---------------------------------------------------------------

func TestBulkRunIndexesPinsAndMFS(t *testing.T) {
	n := newFakeNode()
	pinned := n.file(filler("cats", 20))
	direct := n.file(filler("dogs", 20))
	inDir := n.file(filler("fish", 20))
	nested := n.file(filler("they", 20))
	sub := n.dir("sub", nodeEntry{"nested.txt", nested})
	root := n.dir("root", nodeEntry{"in.txt", inDir}, nodeEntry{"sub", sub})
	missing := "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
	n.absent[missing] = true
	n.other["bafkreidt5gbjix64gfd5cuuda5twbncvyfxuozykyutbfpv376mw3qkqla"] = "symlink"
	n.pinRecursive(pinned, root, missing, "bafkreidt5gbjix64gfd5cuuda5twbncvyfxuozykyutbfpv376mw3qkqla")
	n.pinDirect(direct)
	mfsOne := n.file(filler("cats", 30))
	mfsTwo := n.file(filler("dogs", 30))
	n.mfs["/"] = []nodeMFS{{Name: "docs", Dir: true}, {Name: "top.txt", CID: mfsTwo}}
	n.mfs["/docs"] = []nodeMFS{{Name: "note.md", CID: mfsOne}}

	b := newTextBulkEnv(t, n, nil)
	out, run := b.run(b.principal(), nil)

	assert.Equal(t, "running", out["state"])
	assert.Equal(t, false, out["resumed"])
	assert.Equal(t, []any{"pins", "mfs"}, out["sources"])
	assert.Equal(t, "/", out["mfs_root"])
	assert.NotContains(t, out, "egress_hosts", "every stage is on this machine")
	assert.Equal(t, map[string]any{"max_files": float64(1000), "max_bytes": float64(1 << 30), "files_per_minute": float64(30)}, out["caps"])

	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.ElementsMatch(t, sortedCIDs(pinned, direct, inDir, nested, mfsOne, mfsTwo), b.indexedCIDs())
	assert.Equal(t, 8, run.FilesSeen)
	assert.Equal(t, 6, run.FilesIndexed)
	assert.Equal(t, 2, run.FilesSkipped)
	assert.Equal(t, map[string]int{"not_local": 1, "not_a_file": 1}, run.SkippedByReason)
	assert.Equal(t, 6, run.ChunksIndexed)
	assert.EqualValues(t, 20*4+30*2, run.BytesRead)
	assert.Equal(t, "indexer", run.StartedBy)

	st := b.status(b.principal())
	assert.Equal(t, true, st["enabled"])
	view := st["run"].(map[string]any)
	assert.Equal(t, "completed", view["state"])
	assert.EqualValues(t, 6, view["files_indexed"])
	assert.Equal(t, map[string]any{"not_local": float64(1), "not_a_file": float64(1)}, view["skipped_by_reason"])
	assert.NotContains(t, view, "cursor", "the cursor is internal")
}

func TestBulkRunSkipsFilesThatAreAlreadyIndexed(t *testing.T) {
	n, cids := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	b.run(b.principal(), nil)
	before := b.embed.requests.Load()

	_, run := b.run(b.principal(), nil)
	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.Equal(t, 3, run.FilesSeen)
	assert.Equal(t, 0, run.FilesIndexed)
	assert.Equal(t, map[string]int{"already_indexed": 3}, run.SkippedByReason)
	assert.Equal(t, before, b.embed.requests.Load(), "nothing was embedded again")
	assert.Equal(t, cids, b.indexedCIDs())
}

func TestBulkRunSourcesAndMFSRootAreBoundedByTheConfiguration(t *testing.T) {
	n := newFakeNode()
	pinned := n.file(filler("cats", 10))
	n.pinRecursive(pinned)
	a := n.file(filler("dogs", 10))
	other := n.file(filler("fish", 10))
	n.mfs["/"] = []nodeMFS{{Name: "a", Dir: true}, {Name: "b", Dir: true}}
	n.mfs["/a"] = []nodeMFS{{Name: "x.txt", CID: a}}
	n.mfs["/b"] = []nodeMFS{{Name: "y.txt", CID: other}}

	t.Run("sources narrowed by the input", func(t *testing.T) {
		b := newTextBulkEnv(t, n, nil)
		out, run := b.run(b.principal(), map[string]any{"sources": []any{"pins"}})
		assert.Equal(t, []any{"pins"}, out["sources"])
		assert.NotContains(t, out, "mfs_root")
		assert.Equal(t, []string{"pins"}, run.Sources)
		assert.Equal(t, []string{pinned}, b.indexedCIDs())
		assert.Zero(t, b.d.count("files/ls"), "MFS was not walked")
	})

	t.Run("a source the configuration leaves out is refused", func(t *testing.T) {
		b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.Sources = []string{"pins"} })
		res := b.start(b.principal(), map[string]any{"sources": []any{"mfs"}})
		assert.Equal(t, CodeInvalidArgument, toolErr(t, res).Code)
		assert.Nil(t, b.store.stored(t), "no run exists")
	})

	t.Run("empty configured sources index nothing", func(t *testing.T) {
		b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.Sources = []string{} })
		res := b.start(b.principal(), nil)
		assert.Equal(t, CodeFailedPrecondition, toolErr(t, res).Code)
		assert.Nil(t, b.store.stored(t))
	})

	t.Run("mfs_root narrows the walk to a subtree", func(t *testing.T) {
		b := newTextBulkEnv(t, n, nil)
		out, _ := b.run(b.principal(), map[string]any{"sources": []any{"mfs"}, "mfs_root": "/a"})
		assert.Equal(t, "/a", out["mfs_root"])
		assert.Equal(t, []string{a}, b.indexedCIDs())
	})

	t.Run("mfs_root cannot leave the configured root", func(t *testing.T) {
		b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.MFSRoot = "/a" })
		res := b.start(b.principal(), map[string]any{"mfs_root": "/b"})
		assert.Equal(t, CodeInvalidArgument, toolErr(t, res).Code)
		res = b.start(b.principal(), map[string]any{"mfs_root": "/ab"})
		assert.Equal(t, CodeInvalidArgument, toolErr(t, res).Code, "/ab is not under /a")
		assert.Nil(t, b.store.stored(t))
	})
}

// --- caps ------------------------------------------------------------------------------

func TestBulkRunStopsAtMaxFilesAndTheNextRunContinues(t *testing.T) {
	n, cids := threeFiles()
	b := newTextBulkEnv(t, n, nil)

	out, run := b.run(b.principal(), map[string]any{"max_files": 2})
	assert.Equal(t, map[string]any{"max_files": float64(2), "max_bytes": float64(1 << 30), "files_per_minute": float64(30)}, out["caps"])
	assert.Equal(t, embeddings.RunCompleted, run.State, "a run that hits its cap is completed")
	assert.Equal(t, 2, run.FilesIndexed)
	assert.Len(t, b.indexedCIDs(), 2)

	_, run = b.run(b.principal(), map[string]any{"max_files": 2})
	assert.Equal(t, 1, run.FilesIndexed, "the files the first run indexed do not count against the cap")
	assert.Equal(t, 2, run.SkippedByReason["already_indexed"])
	assert.Equal(t, cids, b.indexedCIDs())
}

func TestBulkRunStopsBeforeExceedingMaxBytes(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)

	_, run := b.run(b.principal(), map[string]any{"max_bytes": 25})
	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.Equal(t, 2, run.FilesIndexed, "two 10-byte files fit in 25 bytes, a third would not")
	assert.EqualValues(t, 20, run.BytesRead)
	assert.Len(t, b.indexedCIDs(), 2)
}

func TestBulkRunSkipsAFileLargerThanTheWholeByteCap(t *testing.T) {
	n := newFakeNode()
	n.pinRecursive(n.file(filler("bigx", 100)))
	b := newTextBulkEnv(t, n, nil)
	_, run := b.run(b.principal(), map[string]any{"max_bytes": 25})
	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.Equal(t, map[string]int{"too_large": 1}, run.SkippedByReason, "it can never fit a run of this size, so the run moves on")
	assert.Zero(t, b.d.count("cat"))
}

func TestBulkRunCapsAreClampedToTheConfiguration(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, func(o *mediaOpts) {
		o.bulk = BulkConfig{MaxFilesPerRun: 2, MaxBytesPerRun: 15, FilesPerMinute: 600}
	})
	out, run := b.run(b.principal(), map[string]any{"max_files": 100, "max_bytes": 1 << 40, "files_per_minute": 6000})
	assert.Equal(t, map[string]any{"max_files": float64(2), "max_bytes": float64(15), "files_per_minute": float64(600)}, out["caps"],
		"a run never exceeds the configured caps")
	assert.Equal(t, 1, run.FilesIndexed, "the byte cap of 15 stops it after one 10-byte file")

	out, _ = b.run(b.principal(), map[string]any{"max_files": 1})
	assert.Equal(t, float64(1), out["caps"].(map[string]any)["max_files"], "the input may lower a cap")
}

func TestBulkRunPacesFilesByFilesPerMinute(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	_, run := b.run(b.principal(), map[string]any{"files_per_minute": 2})
	assert.Equal(t, 3, run.FilesIndexed)
	assert.Equal(t, []time.Duration{30 * time.Second, 30 * time.Second}, b.waited(),
		"the first file starts at once, each next waits 60s/2 after the previous")
}

func TestBulkRunDoesNotWaitForFilesItSkips(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	b.run(b.principal(), nil)
	b.mu.Lock()
	b.waits = nil
	b.mu.Unlock()
	b.run(b.principal(), nil)
	assert.Empty(t, b.waited(), "already indexed files cost nothing, so they are not rate limited")
}

// --- one run at a time -----------------------------------------------------------------

// blockingBulk makes the run block in its first wait until release is called,
// so a test can act while a run is active.
func blockingBulk(b *bulkEnv) (reached <-chan struct{}, release func()) {
	reachedCh, releaseCh := make(chan struct{}), make(chan struct{})
	var once sync.Once
	b.s.bulkSleep = func(ctx context.Context, d time.Duration) error {
		b.mu.Lock()
		b.waits = append(b.waits, d)
		b.mu.Unlock()
		once.Do(func() { close(reachedCh) })
		select {
		case <-releaseCh:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
	return reachedCh, sync.OnceFunc(func() { close(releaseCh) })
}

func TestBulkSecondStartIsRefusedWhileARunIsActive(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	reached, release := blockingBulk(b)
	t.Cleanup(release)

	res := b.start(b.principal(), nil)
	require.False(t, res.IsError, "%+v", res.Content)
	first := structured(t, res)["run_id"]
	<-reached

	res = b.start(b.principal(), nil)
	te := toolErr(t, res)
	assert.Equal(t, CodeFailedPrecondition, te.Code)
	assert.Contains(t, te.Message, "already running")
	assert.Equal(t, first, b.store.stored(t).ID, "the active run continues")

	st := b.status(b.principal())
	assert.Equal(t, "running", st["run"].(map[string]any)["state"])

	release()
	b.wait()
	assert.Equal(t, embeddings.RunCompleted, b.store.stored(t).State)
	assert.Equal(t, 3, b.store.stored(t).FilesIndexed)
}

func TestBulkNewStartIsRefusedWhileARunIsPaused(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "paused-run", State: embeddings.RunPaused, Sources: []string{"pins"}, Caps: embeddings.RunCaps{MaxFiles: 10, MaxBytes: 1000, FilesPerMinute: 60},
	}))
	res := b.start(b.principal(), nil)
	te := toolErr(t, res)
	assert.Equal(t, CodeFailedPrecondition, te.Code)
	assert.Contains(t, te.Message, "resume")
	assert.Zero(t, b.embed.requests.Load())
}

// --- restart, pause and resume -------------------------------------------------------------

func TestBulkRestartLeavesARunningRunPausedAndIndexingNothing(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	// What a crashed or killed ipfs mcp leaves behind: a run stored as running.
	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "crashed", State: embeddings.RunRunning, StartedBy: "indexer", Sources: []string{"pins"},
		Caps: embeddings.RunCaps{MaxFiles: 10, MaxBytes: 1000, FilesPerMinute: 60}, FilesIndexed: 1,
	}))

	b.s.recoverBulkRun(t.Context())
	stored := b.store.stored(t)
	assert.Equal(t, embeddings.RunPaused, stored.State, "the stored state is corrected at start")
	assert.Nil(t, b.s.bulk.active(), "nothing resumes it")
	assert.Zero(t, b.embed.requests.Load())
	assert.Zero(t, b.d.count("pin/ls")+b.d.count("cat")+b.d.count("ls"), "no walk ran")

	st := b.status(b.principal())
	assert.Equal(t, "paused", st["run"].(map[string]any)["state"])
}

func TestBulkStartSeesAStaleRunningRunAsPausedAndDoesNotHang(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "stale", State: embeddings.RunRunning, Sources: []string{"pins"}, Caps: embeddings.RunCaps{MaxFiles: 1, MaxBytes: 1, FilesPerMinute: 1},
	}))
	// No recoverBulkRun: the first request after a crash meets a stored run that
	// says running and has no runner here.
	res := b.start(b.principal(), nil)
	te := toolErr(t, res)
	assert.Equal(t, CodeFailedPrecondition, te.Code)
	assert.Contains(t, te.Message, "paused")
	assert.Equal(t, embeddings.RunPaused, b.store.stored(t).State)
}

func TestBulkStatusReportsAStaleRunningRunAsPaused(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "stale", State: embeddings.RunRunning, Sources: []string{"pins"}, Caps: embeddings.RunCaps{MaxFiles: 1, MaxBytes: 1, FilesPerMinute: 1},
	}))
	st := b.status(b.principal())
	assert.Equal(t, "paused", st["run"].(map[string]any)["state"], "a run with no live runner is paused whatever the store says")
}

func TestBulkResumeContinuesFromTheCursorWithoutReindexing(t *testing.T) {
	n, cids := threeFiles()
	first := newTextBulkEnv(t, n, nil)
	reached, _ := blockingBulk(first)
	// Stop the run in the wait before its second file: the way ipfs mcp exiting
	// pauses it.
	res := first.start(first.principal(), nil)
	require.False(t, res.IsError, "%+v", res.Content)
	<-reached
	first.s.StopBulk()
	paused := first.store.stored(t)
	require.Equal(t, embeddings.RunPaused, paused.State)
	require.Equal(t, 1, paused.FilesIndexed)
	assert.Equal(t, embeddings.RunCursor{Source: "pins", After: cids[0]}, paused.Cursor)
	assert.Equal(t, []string{cids[0]}, first.indexedCIDs())

	// A second ipfs mcp over the same store and node: restart.
	second := newTextBulkEnv(t, n, func(o *mediaOpts) { o.store = first.store })
	second.s.recoverBulkRun(t.Context())
	require.Equal(t, embeddings.RunPaused, second.store.stored(t).State)

	// A plain start is refused, a resume continues.
	res = second.start(second.principal(), nil)
	assert.Equal(t, CodeFailedPrecondition, toolErr(t, res).Code)
	out := structured(t, second.start(second.principal(), map[string]any{"resume": true}))
	assert.Equal(t, true, out["resumed"])
	validateOutput(t, "ipfs_index_bulk_start", out)
	second.wait()

	final := second.store.stored(t)
	assert.Equal(t, embeddings.RunCompleted, final.State)
	assert.Equal(t, 3, final.FilesIndexed)
	assert.Equal(t, 0, final.SkippedByReason["already_indexed"], "the cursor skipped the done file without even a store lookup")
	assert.Equal(t, cids, second.indexedCIDs())
	assert.EqualValues(t, 2, second.embed.requests.Load(), "only the two remaining files were embedded")
	assert.Equal(t, final.ID, paused.ID, "the same run")
}

func TestBulkResumeRefusesNewLimitsAndNeedsAPausedRun(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)

	res := b.start(b.principal(), map[string]any{"resume": true})
	assert.Equal(t, CodeFailedPrecondition, toolErr(t, res).Code, "there is nothing to resume")

	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "p", State: embeddings.RunPaused, Sources: []string{"pins"}, Caps: embeddings.RunCaps{MaxFiles: 10, MaxBytes: 1000, FilesPerMinute: 60},
	}))
	res = b.start(b.principal(), map[string]any{"resume": true, "max_files": 5})
	assert.Equal(t, CodeInvalidArgument, toolErr(t, res).Code, "a resume continues with the limits the run started with")
}

// --- cancel -----------------------------------------------------------------------------

func TestBulkCancelStopsTheActiveRun(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.FilesPerMinute = 600000 })
	reached, _ := blockingBulk(b)
	require.False(t, b.start(b.principal(), nil).IsError)
	<-reached

	id := b.principal()
	res := toolCall(t, b.connect(&id), "ipfs_index_bulk_cancel", nil)
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	validateOutput(t, "ipfs_index_bulk_cancel", out)
	assert.Equal(t, "cancelled", out["state"])
	assert.EqualValues(t, 1, out["files_indexed"], "the file indexed before the cancel stays indexed")
	assert.Nil(t, b.s.bulk.active())
	assert.Equal(t, embeddings.RunCancelled, b.store.stored(t).State)

	// A later run skips what was indexed.
	b.s.bulkSleep = nil
	b.s.bulkClock = nil
	_, run := b.run(b.principal(), nil)
	assert.Equal(t, 1, run.SkippedByReason["already_indexed"])
	assert.Equal(t, 2, run.FilesIndexed)
}

func TestBulkCancelAPausedRunAndWithNoRun(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	id := b.principal()
	res := toolCall(t, b.connect(&id), "ipfs_index_bulk_cancel", nil)
	assert.Equal(t, CodeFailedPrecondition, toolErr(t, res).Code)

	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "p", State: embeddings.RunPaused, Sources: []string{"pins"}, FilesIndexed: 4, Caps: embeddings.RunCaps{MaxFiles: 10, MaxBytes: 1000, FilesPerMinute: 60},
	}))
	res = toolCall(t, b.connect(&id), "ipfs_index_bulk_cancel", nil)
	require.False(t, res.IsError, "%+v", res.Content)
	assert.EqualValues(t, 4, structured(t, res)["files_indexed"])
	assert.Equal(t, embeddings.RunCancelled, b.store.stored(t).State)
}

func TestBulkStatusWithNoRunHasNone(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	st := b.status(b.principal())
	assert.Equal(t, true, st["enabled"])
	assert.NotContains(t, st, "run")
}

// --- 1.4: no injection, no network fetch ---------------------------------------------------

func TestBulkRunReadsOnlyAndAlwaysOffline(t *testing.T) {
	n := newFakeNode()
	f := n.file(filler("cats", 10))
	inDir := n.file(filler("dogs", 10))
	root := n.dir("root", nodeEntry{"a.txt", inDir})
	missing := "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
	n.absent[missing] = true
	partial := n.file(filler("fish", 10))
	n.partial[partial] = true
	n.pinRecursive(f, root, missing, partial)
	mfsF := n.file(filler("they", 10))
	n.mfs["/"] = []nodeMFS{{Name: "m.txt", CID: mfsF}}

	b := newTextBulkEnv(t, n, nil)
	_, run := b.run(b.principal(), nil)
	assert.Equal(t, 2, run.SkippedByReason["not_local"], "a missing root and a missing data block are both not_local")
	assert.Len(t, b.indexedCIDs(), 3)

	forbidden := []string{
		"add", "block/put", "dag/put", "dag/import", "pin/add", "pin/update", "pin/rm", "files/write", "files/cp", "files/mkdir",
		"files/rm", "files/mv", "files/chcid", "routing/provide", "routing/put", "name/publish", "refs/local", "repo/gc",
	}
	for _, c := range b.commands() {
		assert.NotContains(t, forbidden, c, "a bulk run issues no mutating or announcing command")
	}
	b.d.mu.Lock()
	defer b.d.mu.Unlock()
	reads := 0
	for _, c := range b.d.log {
		switch c.Command {
		case "cat", "ls", "files/stat", "files/ls":
			reads++
			assert.Equal(t, true, c.Opts["offline"], "%s %v is offline", c.Command, c.Args)
		}
	}
	assert.Positive(t, reads)
}

func TestBulkNeverReadsAFileWhoseBlocksAreMissing(t *testing.T) {
	n := newFakeNode()
	gone := n.file(filler("cats", 10))
	n.absent[gone] = true
	n.pinRecursive(gone)
	b := newTextBulkEnv(t, n, nil)
	_, run := b.run(b.principal(), nil)
	assert.Equal(t, map[string]int{"not_local": 1}, run.SkippedByReason)
	assert.Zero(t, b.d.count("cat"), "nothing was read, so nothing could be fetched")
	assert.Empty(t, b.rows())
}

// --- 1.3: principal and egress ----------------------------------------------------------------

func TestBulkStartIsRefusedNamingTheMissingHosts(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, func(o *mediaOpts) {
		o.extractor = func(*embeddings.ExtractorSettings) {}
		o.describer = func(*embeddings.DescriberSettings) {}
	})
	partial := indexIdentity(b.embed.host())
	res := b.start(partial, nil)
	te := toolErr(t, res)
	assert.Equal(t, CodeEgressNotAcknowledged, te.Code)
	missing, _ := te.Details["missing_hosts"].([]any)
	assert.ElementsMatch(t, []any{b.x.host(), b.chat.host()}, missing, "both the extractor and the describer are named")
	assert.Nil(t, b.store.stored(t), "no run was created")
	assert.Zero(t, b.embed.requests.Load())

	ok := b.principal()
	res = b.start(ok, nil)
	require.False(t, res.IsError, "%+v", res.Content)
	b.wait()
	assert.Equal(t, embeddings.RunCompleted, b.store.stored(t).State)
}

func TestBulkStartIsRefusedForTheOperatorIdentityBeforeAnyConfirmation(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	asked := 0
	cs := b.connectDeciding(nil, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		asked++
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
	res := toolCall(t, cs, "ipfs_index_bulk_start", nil)
	te := toolErr(t, res)
	assert.Equal(t, CodeEgressNotAcknowledged, te.Code, "the operator identity carries no egress_ack, as in G12")
	assert.Zero(t, asked, "nobody was asked to confirm a start that would be refused")
	assert.Nil(t, b.store.stored(t))
	assert.Zero(t, b.embed.requests.Load())
}

func TestBulkRunChecksEgressAgainBeforeEachFile(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	id := b.principal()
	// The ack list is shared with the run; withdrawing the embedder after the
	// first file makes the next file's check fail.
	b.onWait = func(time.Duration) { id.EgressAck[0] = "withdrawn.example.test" }

	res := b.start(id, nil)
	require.False(t, res.IsError, "%+v", res.Content)
	b.wait()
	run := b.store.stored(t)
	assert.Equal(t, embeddings.RunFailed, run.State)
	assert.Contains(t, run.LastError, b.embed.host(), "the failure names the host")
	assert.Equal(t, 1, run.FilesIndexed, "the first file was indexed before the acknowledgement was withdrawn")
	st := b.status(b.principal())["run"].(map[string]any)
	assert.Equal(t, "failed", st["state"])
	assert.Contains(t, st["last_error"], b.embed.host())
}

func TestBulkEgressHostsNameEveryNonLoopbackStage(t *testing.T) {
	svc := embeddings.NewService(
		embeddings.Settings{Mode: embeddings.ModeRemote, Remote: &embeddings.RemoteSettings{Endpoint: "https://store.example.test:8000"}},
		&kindStore{}, mustTestEmbedder(t, "https://embed.example.test/v1"))
	d, err := embeddings.NewDescriber(embeddings.DescriberSettings{BaseURL: "https://describe.example.test/v1", Model: "m"}, nil)
	require.NoError(t, err)
	svc.WithDescriber(d)
	s := &Server{}
	assert.ElementsMatch(t, []string{"embed.example.test", "store.example.test:8000", "describe.example.test"}, s.egressHosts(svc))
}

func mustTestEmbedder(t *testing.T, base string) *embeddings.Embedder {
	t.Helper()
	e, err := embeddings.NewEmbedder(embeddings.EmbedderSettings{BaseURL: base, Model: "m", Dimensions: indexTestDims}, nil)
	require.NoError(t, err)
	return e
}

// --- failures that end or do not end a run -------------------------------------------------------

func TestBulkEmbedderDownFailsTheRunWithAnError(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	b.embed.down.Store(true)
	_, run := b.run(b.principal(), nil)
	assert.Equal(t, embeddings.RunFailed, run.State)
	assert.NotEmpty(t, run.LastError)
	assert.Equal(t, 0, run.FilesIndexed)
	assert.Equal(t, 1, run.FilesSeen, "it stopped at the first file instead of skipping every file")
}

func TestBulkUnreadableEntryIsSkippedAndTheRunContinues(t *testing.T) {
	n := newFakeNode()
	ok := n.file(filler("cats", 10))
	n.pinRecursive(ok, "bafkreicfv4moaxed3rasc6o4qu4zwqzjwd7trbez5wp4givlnlogeyoi4m") // stat answers "path not found"
	b := newTextBulkEnv(t, n, nil)
	_, run := b.run(b.principal(), nil)
	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.Equal(t, 1, run.FilesIndexed)
	assert.Equal(t, 1, run.SkippedByReason["not_local"], "path not found is a block the node does not hold")
}

func TestBulkWaitsForTheDescriberHourlyLimitInsteadOfSkipping(t *testing.T) {
	n := newFakeNode()
	one := n.file(string(pngBytes) + "1")
	two := n.file(string(pngBytes) + "2")
	n.pinRecursive(one, two)
	limit := 1
	b := newBulkEnv(t, bulkOpts{node: n, media: mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(s *embeddings.DescriberSettings) { s.MaxRequestsPerHour = &limit },
	}})
	// The second image hits the limit; the run waits for a request to free. The
	// wait is where this test stops the run, so it does not sleep an hour.
	var waited time.Duration
	b.onWait = func(d time.Duration) {
		if d > time.Minute {
			waited = d
			b.s.bulk.active().cancel(errBulkCancelled)
		}
	}
	res := b.start(b.principal(), map[string]any{"files_per_minute": 6000})
	require.False(t, res.IsError, "%+v", res.Content)
	b.wait()
	run := b.store.stored(t)
	assert.Greater(t, waited, 50*time.Minute, "the run asked to wait for the hourly window")
	assert.Equal(t, embeddings.RunCancelled, run.State)
	assert.Equal(t, 1, run.FilesIndexed)
	assert.Zero(t, run.SkippedByReason["extraction_failed"]+run.SkippedByReason["kind_not_enabled"], "the second image was not skipped")
	assert.Len(t, b.chat.requests(), 1)
	assert.Len(t, b.indexedCIDs(), 1, "the second image is not half-indexed")
}

// --- 1.8: file names reach classification -----------------------------------------------------------

func TestBulkWalkClassifiesFilesByName(t *testing.T) {
	n := newFakeNode()
	py := n.file("def hello():\n    return 'cats'\n")
	yaml := n.file("animal: cats\ncount: 3\n")
	toml := n.file("[pets]\nanimal = \"cats\"\n")
	md := n.file("# Cats\n\ncats purr in the sun\n")
	root := n.dir("code", nodeEntry{"hello.py", py}, nodeEntry{"pets.yaml", yaml}, nodeEntry{"pets.toml", toml}, nodeEntry{"README.md", md})
	n.pinRecursive(root)

	b := newBulkEnv(t, bulkOpts{node: n, media: mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"text/plain", "text/markdown", "text/x-python", "application/yaml", "application/toml"}},
		extractor: func(*embeddings.ExtractorSettings) {},
	}})
	b.x.setReply(func(f extractedFile) (int, string) {
		return 200, `{"results":[{"content":"cats extracted from ` + f.Filename + `","mime_type":"x","metadata":{}}]}`
	})
	_, run := b.run(b.principal(), map[string]any{"sources": []any{"pins"}})
	require.Equal(t, 4, run.FilesIndexed, "%+v", run)

	kinds := map[string]string{}
	mimes := map[string]string{}
	for _, r := range b.rows() {
		kinds[r.CID] = r.Kind
		mimes[r.CID] = r.MIME
	}
	assert.Equal(t, "code", kinds[py])
	assert.Equal(t, "text/x-python", mimes[py], "a shebang-less .py is code by its name")
	assert.Equal(t, "data", kinds[yaml])
	assert.Equal(t, "application/yaml", mimes[yaml])
	assert.Equal(t, "data", kinds[toml])
	assert.Equal(t, "application/toml", mimes[toml])
	assert.Equal(t, "text", kinds[md])
	assert.Equal(t, "text/markdown", mimes[md])

	var sent []string
	for _, f := range b.x.requests() {
		sent = append(sent, f.Filename+"|"+f.PartType)
	}
	assert.ElementsMatch(t, []string{
		"content.py|text/x-python", "content.yaml|application/yaml", "content.toml|application/toml",
	}, sent, "the markdown file stays on the native path")

	// The CID-only tool on the same bytes has no name to go by: it is plain text.
	b2 := newMediaEnv(t, mediaOpts{policy: embeddings.Policy{MIMEAllowlist: []string{"text/plain"}}, extractor: func(*embeddings.ExtractorSettings) {}})
	b2.files["/ipfs/bafkreidx3s3rteeuhhcjboxeztbao6plxbng74qphi4b4tu4upkd3fhuca"] = []byte("def hello():\n    return 'cats'\n")
	out := b2.index(b2.principal(), "bafkreidx3s3rteeuhhcjboxeztbao6plxbng74qphi4b4tu4upkd3fhuca", false)
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Empty(t, b2.x.requests(), "without a name the Python source is text and never reaches the extractor")
	assert.Equal(t, "text", b2.rows()[0].Kind)
}

func TestBulkWalkRefusesHostileEntryNames(t *testing.T) {
	assert.False(t, safeEntryName(""))
	assert.False(t, safeEntryName(".."))
	assert.False(t, safeEntryName("a/b"))
	assert.False(t, safeEntryName("a\\b"))
	assert.False(t, safeEntryName("bad\x00name"))
	assert.False(t, safeEntryName("new\nline"))
	assert.True(t, safeEntryName("notes (final).md"))
}

// --- cursor order -----------------------------------------------------------------------------------

func TestMFSCursorSkipsWhatTheWalkAlreadyFinished(t *testing.T) {
	const after = "/b/m.txt"
	for p, done := range map[string]bool{
		"/a":        true, // sorts before, does not contain the cursor
		"/a/z.txt":  true,
		"/b":        false, // contains the cursor: descend
		"/b/m.txt":  true,  // the cursor itself
		"/b/n.txt":  false,
		"/b/a.txt":  true,
		"/b/m.txt2": false, // sorts after the cursor
		"/c":        false,
	} {
		assert.Equal(t, done, pathDone(after, p), p)
	}
	assert.False(t, pathDone("", "/a"), "a new run skips nothing")
	assert.True(t, segmentsLess("/a/b", "/a/c"))
	assert.True(t, segmentsLess("/a", "/b/x"))
	assert.False(t, segmentsLess("/b/x", "/a"))
}
