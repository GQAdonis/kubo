package server

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- N1: a file is counted once across a pause -------------------------------------------------------

func TestBulkPauseInsideAFileCountsItOnceAcrossTheResume(t *testing.T) {
	newNode := func() *fakeNode {
		n := newFakeNode()
		n.pinRecursive(n.file(string(pngBytes)+"1"), n.file(string(pngBytes)+"2"))
		return n
	}
	describer := func(first *bulkEnv) bulkOpts {
		limit := 1
		o := bulkOpts{node: newNode(), media: mediaOpts{
			policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
			describer: func(s *embeddings.DescriberSettings) { s.MaxRequestsPerHour = &limit },
		}}
		if first != nil {
			o.media.store = first.store
		}
		return o
	}

	first := newBulkEnv(t, describer(nil))
	// The second image hits the hourly limit and the run waits for it; the pause
	// arrives inside that wait, with the file counted but not indexed.
	first.onWait = func(d time.Duration) {
		if d > time.Minute {
			first.s.bulk.active().cancel(errBulkShutdown)
		}
	}
	res := first.start(first.principal(), map[string]any{"files_per_minute": 6000})
	require.False(t, res.IsError, "%+v", res.Content)
	first.wait()
	paused := first.store.stored(t)
	require.Equal(t, embeddings.RunPaused, paused.State)
	require.Equal(t, 1, paused.FilesIndexed)
	assert.Equal(t, 1, paused.FilesSeen, "the file that was cut short is not counted yet")

	second := newBulkEnv(t, describer(first))
	out := structured(t, second.start(second.principal(), map[string]any{"resume": true}))
	assert.Equal(t, true, out["resumed"])
	second.wait()
	final := second.store.stored(t)
	assert.Equal(t, embeddings.RunCompleted, final.State)
	assert.Equal(t, 2, final.FilesIndexed)
	assert.Equal(t, 2, final.FilesSeen, "each file is counted once, not once per attempt")
}

// --- N2: a missing mfs_root is an outcome of the run ---------------------------------------------------

func TestBulkMissingMFSRootFailsTheRunInsteadOfCompletingIt(t *testing.T) {
	n := newFakeNode()
	n.mfs["/"] = []nodeMFS{{Name: "a", Dir: true}}
	n.mfs["/a"] = []nodeMFS{{Name: "x.txt", CID: n.file(filler("cats", 10))}}
	b := newTextBulkEnv(t, n, nil)

	_, run := b.run(b.principal(), map[string]any{"sources": []any{"mfs"}, "mfs_root": "/gone"})
	assert.Equal(t, embeddings.RunFailed, run.State)
	assert.Contains(t, run.LastError, `mfs_root "/gone"`)
	assert.Zero(t, run.FilesSeen, "no file was tried")
	assert.Zero(t, run.FilesSkipped, "the root is not one skipped file")
	assert.Empty(t, b.indexedCIDs())
}

// --- N3: SearchFiltered widens its read until top_k rows pass the filter ------------------------------

// countingStore records the row counts a search asks the store for.
type countingStore struct {
	*kindStore
	mu      sync.Mutex
	fetches []int
}

func (c *countingStore) Search(ctx context.Context, v []float32, n int) ([]embeddings.Hit, error) {
	c.mu.Lock()
	c.fetches = append(c.fetches, n)
	c.mu.Unlock()
	return c.kindStore.Search(ctx, v, n)
}

func TestSearchFilteredWidensItsReadUntilTopKPassTheFilter(t *testing.T) {
	axis := func(i int) []float32 {
		v := make([]float32, indexTestDims)
		v[i] = 1
		return v
	}
	// descriptions nearer to the query "cats" than the one text row.
	build := func(t *testing.T, near int) (*embeddings.Service, *countingStore) {
		t.Helper()
		st := &countingStore{kindStore: &kindStore{}}
		svc := embeddings.NewService(embeddings.Settings{Mode: embeddings.ModeEmbedded}, st, mustTestEmbedder(t, newEmbedServer(t).URL+"/v1"))
		rows := make([]embeddings.Row, 0, near)
		for i := range near {
			rows = append(rows, embeddings.Row{CID: "bafyD", ChunkIndex: i, Model: "m", Vector: axis(0), Kind: "image", Source: embeddings.SourceDescription, Text: "d"})
		}
		require.NoError(t, st.Replace(t.Context(), "bafyD", rows))
		require.NoError(t, st.Replace(t.Context(), "bafyT", []embeddings.Row{{CID: "bafyT", Model: "m", Vector: axis(1)}}))
		return svc, st
	}
	covers := func(string) bool { return true }
	textOnly := embeddings.Filter{Sources: []string{"text"}}

	t.Run("a filter reads wider until enough rows pass", func(t *testing.T) {
		svc, st := build(t, 50)
		hits, err := svc.SearchFiltered(t.Context(), covers, "cats are here", "", 1, textOnly)
		require.NoError(t, err)
		require.Len(t, hits, 1)
		assert.Equal(t, "bafyT", hits[0].CID)
		assert.Equal(t, []int{40, 160}, st.fetches, "the first read of 40 held only descriptions, so the second read was wider")
	})

	t.Run("no filter reads exactly top_k", func(t *testing.T) {
		svc, st := build(t, 50)
		hits, err := svc.SearchFiltered(t.Context(), covers, "cats are here", "", 3, embeddings.Filter{})
		require.NoError(t, err)
		assert.Len(t, hits, 3)
		assert.Equal(t, []int{3}, st.fetches)
	})

	t.Run("the read is capped and the answer can hold fewer than top_k", func(t *testing.T) {
		svc, st := build(t, 1100)
		hits, err := svc.SearchFiltered(t.Context(), covers, "cats are here", "", 1, textOnly)
		require.NoError(t, err)
		assert.Empty(t, hits, "the one text row ranks beyond the nearest 1000 rows")
		assert.Equal(t, []int{40, 160, 640, 1000}, st.fetches, "no more than 1000 rows are read, in at most four reads")
	})
}

// --- LOW-7: the walk lists directories without loading their children ------------------------------

func TestBulkLsDoesNotLoadChildrenSoOneMissingBlockHidesNoSibling(t *testing.T) {
	n := newFakeNode()
	here := n.file(filler("cats", 10))
	gone := n.file(filler("dogs", 10))
	n.absent[gone] = true
	root := n.dir("root", nodeEntry{"a-gone.txt", gone}, nodeEntry{"b-here.txt", here})
	n.pinRecursive(root)
	b := newTextBulkEnv(t, n, nil)

	_, run := b.run(b.principal(), nil)
	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.Equal(t, []string{here}, b.indexedCIDs(), "the sibling of the missing block was indexed")
	assert.Equal(t, map[string]int{"not_local": 1}, run.SkippedByReason)

	b.d.mu.Lock()
	defer b.d.mu.Unlock()
	listings := 0
	for _, c := range b.d.log {
		if c.Command != "ls" {
			continue
		}
		listings++
		assert.Equal(t, true, c.Opts["offline"], "ls %v is offline", c.Args)
		assert.Equal(t, false, c.Opts["resolve-type"], "ls %v does not resolve types", c.Args)
		assert.Equal(t, false, c.Opts["size"], "ls %v does not load sizes", c.Args)
	}
	assert.Positive(t, listings)
}

// --- LOW-8: a directory that cannot be listed -----------------------------------------------------

func TestBulkDirectoryListingFailures(t *testing.T) {
	setup := func(t *testing.T, failWith func() error) (*bulkEnv, string) {
		t.Helper()
		n := newFakeNode()
		file := n.file(filler("cats", 10))
		inner := n.file(filler("dogs", 10))
		bad := n.dir("bad", nodeEntry{"i.txt", inner})
		n.pinRecursive(file, bad)
		b := newTextBulkEnv(t, n, nil)
		base := b.d.handle["ls"]
		b.d.handle["ls"] = func(args []string, opts map[string]any) (any, error) {
			if rootOf(args[0]) == bad {
				return nil, failWith()
			}
			return base(args, opts)
		}
		return b, file
	}

	t.Run("an unexpected failure is logged and the walk goes on", func(t *testing.T) {
		b, file := setup(t, func() error { return &RPCError{Command: "ls", Message: "merkledag: internal failure"} })
		_, run := b.run(b.principal(), nil)
		assert.Equal(t, embeddings.RunCompleted, run.State)
		assert.Equal(t, []string{file}, b.indexedCIDs())
		assert.Zero(t, run.FilesSkipped, "a directory is not a file the run tried")
		assert.Empty(t, run.SkippedByReason)
	})

	t.Run("a directory the node does not hold is skipped not_local", func(t *testing.T) {
		b, file := setup(t, func() error { return &RPCError{Command: "ls", Message: offlineMessage + "bafyx"} })
		_, run := b.run(b.principal(), nil)
		assert.Equal(t, embeddings.RunCompleted, run.State)
		assert.Equal(t, []string{file}, b.indexedCIDs())
		assert.Equal(t, map[string]int{"not_local": 1}, run.SkippedByReason)
	})

	t.Run("a daemon that is gone fails the run", func(t *testing.T) {
		b, _ := setup(t, func() error { return ErrDaemonUnreachable })
		_, run := b.run(b.principal(), nil)
		assert.Equal(t, embeddings.RunFailed, run.State)
		assert.NotEmpty(t, run.LastError)
	})
}

// --- N7: a cancel that cannot end the run says so --------------------------------------------------

// stuckDaemon never returns from files/stat until released, whatever its context.
type stuckDaemon struct {
	Daemon
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *stuckDaemon) Call(ctx context.Context, command string, args []string, opts map[string]any, out any) error {
	if command == "files/stat" {
		s.once.Do(func() { close(s.entered) })
		<-s.release
		return ctx.Err()
	}
	return s.Daemon.Call(ctx, command, args, opts, out)
}

func (s *stuckDaemon) CallRaw(ctx context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	return s.Daemon.(rawDaemon).CallRaw(ctx, command, args, opts, upload, fn)
}

func TestBulkCancelThatCannotEndTheRunDoesNotClaimAnotherRunStarted(t *testing.T) {
	old := bulkStopWait
	bulkStopWait = 50 * time.Millisecond
	t.Cleanup(func() { bulkStopWait = old })

	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	sd := &stuckDaemon{Daemon: b.s.daemon, entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-sd.release:
		default:
			close(sd.release)
		}
	})
	b.s.daemon = sd
	require.False(t, b.start(b.principal(), nil).IsError)
	<-sd.entered

	id := b.principal()
	te := toolErr(t, toolCall(t, b.connect(&id), "ipfs_index_bulk_cancel", nil))
	assert.Equal(t, CodeFailedPrecondition, te.Code)
	assert.Contains(t, te.Message, "still ending")
	assert.NotContains(t, te.Message, "another bulk run started")

	close(sd.release)
	b.wait()
	assert.Equal(t, embeddings.RunCancelled, b.store.stored(t).State, "once the command returns the run ends cancelled")
}
