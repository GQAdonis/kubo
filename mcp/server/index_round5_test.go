package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- search filters (kinds and sources) ---------------------------------------------------------

func TestIndexSearchSourcesFilterKeepsDescriptionsOut(t *testing.T) {
	e := newRouteEnv(t, routeOpts{media: mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"text/plain", "image/png"}},
		describer: func(*embeddings.DescriberSettings) {},
	}})
	e.seedIndex(t)
	e.files["/ipfs/bafkreicekrpml5sqim726ua5vf3ta3lee5pcydv6iz3m67l2dcgvwgwm7y"] = pngBytes
	e.index(e.principal(), "bafkreicekrpml5sqim726ua5vf3ta3lee5pcydv6iz3m67l2dcgvwgwm7y", false)

	search := func(extra string) map[string]any {
		resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, `{"query":"cats",`+extra+`}`, nil)
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		out := resp.json(t)
		validateOutput(t, "ipfs_index_search", out)
		return out
	}
	sourcesOf := func(out map[string]any) []string {
		var got []string
		for _, r := range out["results"].([]any) {
			got = append(got, r.(map[string]any)["source"].(string))
		}
		return got
	}

	assert.ElementsMatch(t, []string{"text", "description"}, sourcesOf(search(`"top_k":5`)), "without a filter both are returned")
	assert.Equal(t, []string{"text"}, sourcesOf(search(`"sources":["text"]`)), "sources text returns no description hit")
	assert.Equal(t, []string{"description"}, sourcesOf(search(`"sources":["description"]`)))
	assert.Empty(t, sourcesOf(search(`"sources":["ocr"]`)))
	assert.Equal(t, []string{"text"}, sourcesOf(search(`"kinds":["text"]`)))
	assert.Equal(t, []string{"description"}, sourcesOf(search(`"kinds":["image"]`)))
	assert.Empty(t, sourcesOf(search(`"kinds":["image"],"sources":["text"]`)), "both filters must match")
}

func TestSearchFilteredWidensUntilTopKPassTheFilter(t *testing.T) {
	// Twenty description rows sit nearer the query than the single text row; a
	// top_k of 1 with sources text must still find it.
	st := &kindStore{}
	emb := mustTestEmbedder(t, "http://127.0.0.1:1/v1")
	svc := embeddings.NewService(embeddings.Settings{Mode: embeddings.ModeEmbedded}, st, emb)
	axis := func(i int) []float32 {
		v := make([]float32, indexTestDims)
		v[i] = 1
		return v
	}
	var rows []embeddings.Row
	for i := range 20 {
		rows = append(rows, embeddings.Row{CID: "bafkreigqzecctngknadksgwbf6po52s6byvbz4oz5hpr7sky5aamillqcm", ChunkIndex: i, Model: "m", Vector: axis(0), Kind: "image", Source: embeddings.SourceDescription, Text: "d"})
	}
	require.NoError(t, st.Replace(t.Context(), "bafkreigqzecctngknadksgwbf6po52s6byvbz4oz5hpr7sky5aamillqcm", rows))
	require.NoError(t, st.Replace(t.Context(), "bafkreicegysetgaysvp3255ncz6tifye5uoubvufwrrvehfbicm56y2xl4", []embeddings.Row{{CID: "bafkreicegysetgaysvp3255ncz6tifye5uoubvufwrrvehfbicm56y2xl4", Model: "m", Vector: axis(1)}}))

	f := embeddings.Filter{Sources: []string{"text"}}
	assert.True(t, f.Match(embeddings.Hit{}), "a row from before derived content is text")
	assert.False(t, f.Match(embeddings.Hit{Source: "description"}))
	hits := filterByStore(t, svc, axis(0), 1, f)
	require.Len(t, hits, 1, "top_k is counted after the filter")
	assert.Equal(t, "bafkreicegysetgaysvp3255ncz6tifye5uoubvufwrrvehfbicm56y2xl4", hits[0].CID)
}

// filterByStore runs the widening loop of SearchFiltered over a stored vector.
func filterByStore(t *testing.T, svc *embeddings.Service, q []float32, k int, f embeddings.Filter) []embeddings.Hit {
	t.Helper()
	hits, err := svc.Store().Search(t.Context(), q, 1000)
	require.NoError(t, err)
	var out []embeddings.Hit
	for _, h := range hits {
		if f.Match(h) && len(out) < k {
			out = append(out, h)
		}
	}
	return out
}

// --- /index/search abuse limit -------------------------------------------------------------------

func TestSearchLimiterTokenBucketAndConcurrency(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	l := newSearchLimiter(func() time.Time { return now })

	var releases []func()
	for range searchBurst {
		rel, _ := l.acquire("a")
		if rel == nil {
			// The per-identity concurrency cap is below the burst: finish the
			// ones in flight first.
			for _, r := range releases {
				r()
			}
			releases = nil
			rel, _ = l.acquire("a")
		}
		require.NotNil(t, rel)
		releases = append(releases, rel)
		if len(releases) == searchMaxPerIdentity {
			for _, r := range releases {
				r()
			}
			releases = nil
		}
	}
	for _, r := range releases {
		r()
	}
	rel, retry := l.acquire("a")
	assert.Nil(t, rel, "the burst is spent")
	assert.Positive(t, retry)
	assert.LessOrEqual(t, retry, searchRefill)

	other, _ := l.acquire("b")
	assert.NotNil(t, other, "another identity has its own bucket")
	other()

	now = now.Add(searchRefill)
	again, _ := l.acquire("a")
	assert.NotNil(t, again, "one search is earned back per refill")
	again()

	// Concurrency: an identity may have searchMaxPerIdentity in flight.
	now = now.Add(time.Hour)
	var held []func()
	for range searchMaxPerIdentity {
		r, _ := l.acquire("c")
		require.NotNil(t, r)
		held = append(held, r)
	}
	r, busy := l.acquire("c")
	assert.Nil(t, r)
	assert.Equal(t, searchBusyRetry, busy)
	for _, r := range held {
		r()
	}
	r, _ = l.acquire("c")
	assert.NotNil(t, r)
	r()
}

func TestIndexSearchRouteAnswers429WithRetryAfter(t *testing.T) {
	e := newRouteEnv(t, routeOpts{})
	e.seedIndex(t)
	var limited *routeResp
	for range 3 * searchBurst {
		resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, searchBody, nil)
		if resp.status == http.StatusTooManyRequests {
			limited = &resp
			break
		}
		require.Equal(t, http.StatusOK, resp.status, resp.body)
	}
	require.NotNil(t, limited, "a burst of searches from one identity is limited")
	assert.Equal(t, CodeResourceExhausted, limited.errorCode(t))
	assert.NotEmpty(t, limited.header.Get("Retry-After"))
	before := e.embed.requests.Load()
	resp := e.do(t, http.MethodPost, RouteIndexSearch, tokUI, searchBody, nil)
	assert.Equal(t, http.StatusTooManyRequests, resp.status)
	assert.Equal(t, before, e.embed.requests.Load(), "a refused search is not embedded")

	// Another identity is not affected.
	other := e.do(t, http.MethodPost, RouteIndexSearch, tokHuman, searchBody, nil)
	assert.Equal(t, http.StatusOK, other.status, other.body)
}

// --- bulk: the MFS walk is offline ---------------------------------------------------------------

func TestBulkMFSWalkReadsThroughTheRootCIDAndNeverAsksFilesLs(t *testing.T) {
	n := newFakeNode()
	local := n.file(filler("cats", 10))
	lazyFile := n.file(filler("dogs", 10))
	n.absent[lazyFile] = true // files cp /ipfs/<cid> copies lazily: the root block is not here
	innerLazy := n.file(filler("fish", 10))
	lazyDir := n.dir("lazydir", nodeEntry{"inner.txt", innerLazy})
	n.absent[lazyDir] = true
	n.mfs["/"] = []nodeMFS{
		{Name: "a-local.txt", CID: local},
		{Name: "b-lazy.txt", CID: lazyFile},
		{Name: "c-lazydir", CID: lazyDir, Dir: true},
		{Name: "d-sub", Dir: true},
	}
	nested := n.file(filler("they", 10))
	n.mfs["/d-sub"] = []nodeMFS{{Name: "n.txt", CID: nested}}

	b := newTextBulkEnv(t, n, nil)
	_, run := b.run(b.principal(), map[string]any{"sources": []any{"mfs"}})

	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.ElementsMatch(t, sortedCIDs(local, nested), b.indexedCIDs())
	assert.Equal(t, map[string]int{"not_local": 2}, run.SkippedByReason, "the lazy file and the lazy directory are skipped, not fetched")
	assert.Zero(t, b.d.count("files/ls"), "files/ls loads MFS through the online DAG service")

	b.d.mu.Lock()
	defer b.d.mu.Unlock()
	roots := 0
	for _, c := range b.d.log {
		if c.Command == "files/stat" && len(c.Args) == 1 && c.Args[0] == "/" {
			roots++
			continue
		}
		switch c.Command {
		case "files/stat", "ls", "cat":
			assert.Equal(t, true, c.Opts["offline"], "%s %v is offline", c.Command, c.Args)
			assert.True(t, strings.HasPrefix(c.Args[0], "/ipfs/"), "%s reads %s through the root CID, not an MFS path", c.Command, c.Args[0])
		}
	}
	assert.Equal(t, 1, roots, "the MFS root is resolved once per walk")
}

func TestBulkMFSRootNarrowsTheOfflineWalk(t *testing.T) {
	n := newFakeNode()
	a := n.file(filler("cats", 10))
	other := n.file(filler("dogs", 10))
	n.mfs["/"] = []nodeMFS{{Name: "a", Dir: true}, {Name: "b", Dir: true}}
	n.mfs["/a"] = []nodeMFS{{Name: "x.txt", CID: a}}
	n.mfs["/b"] = []nodeMFS{{Name: "y.txt", CID: other}}
	b := newTextBulkEnv(t, n, nil)
	b.run(b.principal(), map[string]any{"sources": []any{"mfs"}, "mfs_root": "/a"})
	assert.Equal(t, []string{a}, b.indexedCIDs())
}

// --- bulk: resume inside a pinned directory --------------------------------------------------------

func TestBulkResumeContinuesInsidePinnedDirectoryWithUnchangedCounts(t *testing.T) {
	n := newFakeNode()
	aa := n.file(filler("cats", 10))
	bb := n.file(filler("dogs", 10))
	cc := n.file(filler("fish", 10))
	xx := n.file(filler("they", 10))
	yy := n.file(filler("rain", 10))
	sub := n.dir("sub", nodeEntry{"x.txt", xx}, nodeEntry{"y.txt", yy})
	root := n.dir("root", nodeEntry{"a.txt", aa}, nodeEntry{"b.txt", bb}, nodeEntry{"c.txt", cc}, nodeEntry{"sub", sub})
	n.pinRecursive(root)

	first := newTextBulkEnv(t, n, nil)
	reached, _ := blockingBulk(first)
	res := first.start(first.principal(), nil)
	require.False(t, res.IsError, "%+v", res.Content)
	<-reached
	first.s.StopBulk()
	paused := first.store.stored(t)
	require.Equal(t, embeddings.RunPaused, paused.State)
	require.Equal(t, 1, paused.FilesIndexed)
	assert.Equal(t, embeddings.RunCursor{Source: "pins", Pin: root, Inside: "a.txt"}, paused.Cursor, "the cursor is inside the pinned directory")

	second := newTextBulkEnv(t, n, func(o *mediaOpts) { o.store = first.store })
	second.s.recoverBulkRun(t.Context())
	out := structured(t, second.start(second.principal(), map[string]any{"resume": true}))
	assert.Equal(t, true, out["resumed"])
	second.wait()

	final := second.store.stored(t)
	assert.Equal(t, embeddings.RunCompleted, final.State)
	assert.Equal(t, 5, final.FilesIndexed)
	assert.Equal(t, 5, final.FilesSeen, "the files before the cursor were not walked again")
	assert.Zero(t, final.SkippedByReason["already_indexed"])
	assert.Equal(t, 0, final.FilesSkipped)
	assert.EqualValues(t, 4, second.embed.requests.Load(), "only the four files after the cursor were embedded")
	assert.Equal(t, sortedCIDs(aa, bb, cc, xx, yy), second.indexedCIDs())
}

func TestPathDoneOrdersCursorsInsideADirectory(t *testing.T) {
	const after = "sub/x.txt"
	for p, done := range map[string]bool{
		"0.txt":     true,  // sorts before
		"b.txt":     true,  // sorts before "sub"
		"sub":       false, // contains the cursor: descend
		"sub/x.txt": true,  // the cursor itself
		"sub/a.txt": true,
		"sub/y.txt": false,
		"t":         false,
	} {
		assert.Equal(t, done, pathDone(after, p), "%s after %s", p, after)
	}
	assert.False(t, pathDone("", "a"))
}

// --- bulk: resume honours the configuration ----------------------------------------------------------

func TestBulkResumeNarrowsToTheCurrentConfiguration(t *testing.T) {
	stored := embeddings.Run{
		ID: "p", State: embeddings.RunPaused, StartedBy: "indexer", Sources: []string{"pins", "mfs"}, MFSRoot: "/a",
		Caps:   embeddings.RunCaps{MaxFiles: 10000, MaxBytes: 1 << 40, FilesPerMinute: 6000},
		Cursor: embeddings.RunCursor{Source: "mfs", After: "/a/x"},
	}
	n, _ := threeFiles()

	t.Run("a source the configuration dropped is not walked", func(t *testing.T) {
		b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.Sources = []string{"pins"} })
		require.NoError(t, b.store.SaveRun(t.Context(), stored))
		out := structured(t, b.start(b.principal(), map[string]any{"resume": true}))
		b.wait()
		assert.Equal(t, []any{"pins"}, out["sources"])
		assert.NotContains(t, out, "mfs_root")
		run := b.store.stored(t)
		assert.Equal(t, []string{"pins"}, run.Sources)
		assert.Equal(t, embeddings.RunCursor{Source: "pins", After: run.Cursor.After}, run.Cursor, "a cursor in a dropped source is reset")
		for _, c := range b.d.log {
			assert.NotEqual(t, []string{"/"}, c.Args, "MFS was not walked")
		}
	})

	t.Run("no remaining source is refused", func(t *testing.T) {
		b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.Sources = []string{} })
		require.NoError(t, b.store.SaveRun(t.Context(), stored))
		te := toolErr(t, b.start(b.principal(), map[string]any{"resume": true}))
		assert.Equal(t, CodeFailedPrecondition, te.Code)
		assert.Equal(t, embeddings.RunPaused, b.store.stored(t).State)
	})

	t.Run("a stored MFS root outside the configured root is refused", func(t *testing.T) {
		b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.bulk.MFSRoot = "/b" })
		require.NoError(t, b.store.SaveRun(t.Context(), stored))
		te := toolErr(t, b.start(b.principal(), map[string]any{"resume": true}))
		assert.Equal(t, CodeFailedPrecondition, te.Code)
		assert.Contains(t, te.Message, "mfs_root")
	})

	t.Run("the confirmation shows the clamped values", func(t *testing.T) {
		b := newTextBulkEnv(t, n, func(o *mediaOpts) {
			o.bulk = BulkConfig{MaxFilesPerRun: 7, MaxBytesPerRun: 77, FilesPerMinute: 3, Sources: []string{"pins"}}
		})
		require.NoError(t, b.store.SaveRun(t.Context(), stored))
		args, _ := b.s.summaryArgs("ipfs_index_bulk_start", map[string]any{"resume": true})
		assert.Equal(t, 7, args["max_files"])
		assert.EqualValues(t, 77, args["max_bytes"])
		assert.Equal(t, 3, args["files_per_minute"])
		assert.Equal(t, "pins", args["sources"])
	})
}

// --- bulk: counting and caps ---------------------------------------------------------------------------

func TestBulkNotAFileEntriesDoNotCountAgainstMaxFiles(t *testing.T) {
	n := newFakeNode()
	n.other["bafkreigeidx5xv6wggfym2jjtvrcnamqbanrvvjlqxty5fxd57cwkzsbum"] = "symlink"
	text := n.file(filler("cats", 10))
	root := n.dir("root", nodeEntry{"a-link", "bafkreigeidx5xv6wggfym2jjtvrcnamqbanrvvjlqxty5fxd57cwkzsbum"}, nodeEntry{"b.txt", text})
	n.pinRecursive(root)
	b := newTextBulkEnv(t, n, nil)
	_, run := b.run(b.principal(), map[string]any{"max_files": 1})
	assert.Equal(t, 1, run.FilesIndexed, "the symlink is not a file the run tried")
	assert.Equal(t, 1, run.SkippedByReason["not_a_file"])
	assert.Equal(t, []string{text}, b.indexedCIDs())
}

func TestBulkByteCapUsesWhatReadingCanCostNotTheUnixFSSize(t *testing.T) {
	n := newFakeNode()
	small := n.file(filler("cats", 40))
	big := n.file(filler("dogs", 220)) // over policy.max_bytes: skipped after a short check
	last := n.file(filler("fish", 10))
	root := n.dir("root", nodeEntry{"a.txt", small}, nodeEntry{"b.txt", big}, nodeEntry{"c.txt", last})
	n.pinRecursive(root)
	b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.policy.MaxBytes = 50 })
	_, run := b.run(b.principal(), map[string]any{"max_bytes": 250})
	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.Equal(t, 2, run.FilesIndexed, "the run went on to the file after the large one")
	assert.Equal(t, 1, run.SkippedByReason["too_large"])
	assert.Equal(t, sortedCIDs(small, last), b.indexedCIDs())
}

// --- bulk: per-call deadlines ------------------------------------------------------------------------------

// hangDaemon never answers the commands it names until the call's context ends.
type hangDaemon struct {
	Daemon
	hang map[string]bool
}

func (h *hangDaemon) Call(ctx context.Context, command string, args []string, opts map[string]any, out any) error {
	if h.hang[command] {
		<-ctx.Done()
		return ctx.Err()
	}
	return h.Daemon.Call(ctx, command, args, opts, out)
}

func (h *hangDaemon) CallRaw(ctx context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	return h.Daemon.(rawDaemon).CallRaw(ctx, command, args, opts, upload, fn)
}

func TestBulkWalkCallsHaveADeadlineSoAStalledOneIsSkipped(t *testing.T) {
	old := bulkCallTimeout
	bulkCallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { bulkCallTimeout = old })

	n := newFakeNode()
	hung := n.file(filler("dogs", 10))
	ok := n.file(filler("cats", 10))
	n.pinRecursive(hung, ok)
	b := newTextBulkEnv(t, n, nil)
	b.s.daemon = &hangDaemon{Daemon: b.s.daemon, hang: map[string]bool{"files/stat": true}}

	start := time.Now()
	_, run := b.run(b.principal(), nil)
	assert.Less(t, time.Since(start), 10*time.Second, "the run ended instead of waiting for the daemon")
	assert.Equal(t, embeddings.RunCompleted, run.State)
	assert.Equal(t, 2, run.SkippedByReason["extraction_failed"], "each stalled stat is one skipped entry")
}

// --- describer limit: the extractor result is kept across a retry -----------------------------------------

func TestRetryAfterTheDescriberLimitDoesNotSendTheFileToTheExtractorAgain(t *testing.T) {
	one := 1
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		extractor: func(s *embeddings.ExtractorSettings) { s.OCR = true },
		describer: func(s *embeddings.DescriberSettings) { s.MaxRequestsPerHour = &one },
	})
	m.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":"words in the image","mime_type":"image/png","metadata":{}}]}`
	})
	id := m.principal()
	covers := EgressCovers(id)
	ctx := WithIdentity(WithPrincipal(t.Context(), id.Principal), id)
	m.files["/ipfs/bafkreieoswm5yqijue4v3qyit63bd76suvpinoixcelkdzzlfw3uimwtsu"] = append(append([]byte{}, pngBytes...), 'a')
	m.files["/ipfs/bafkreiha5sgvksp5ooau53gcgplgt6ximremnu7jf2pchvwgdirglk5xoa"] = append(append([]byte{}, pngBytes...), 'b')

	run := func(c string, memo *stageMemo) error {
		t := indexTarget{CID: c, Path: "/ipfs/" + c, Offline: true, Checked: true, memo: memo}
		_, err := m.s.indexResolved(ctx, m.svc, covers, t, statResponse{Hash: c, Size: int64(len(m.files["/ipfs/"+c])), Type: "file"})
		return err
	}
	require.NoError(t, run("bafkreieoswm5yqijue4v3qyit63bd76suvpinoixcelkdzzlfw3uimwtsu", nil), "the first image takes the hourly slot")
	requests := len(m.x.requests())

	memo := &stageMemo{}
	err1 := run("bafkreiha5sgvksp5ooau53gcgplgt6ximremnu7jf2pchvwgdirglk5xoa", memo)
	var te *ToolError
	require.ErrorAs(t, indexErr(err1), &te)
	assert.Equal(t, CodeResourceExhausted, te.Code)
	assert.Len(t, m.x.requests(), requests+1, "the first attempt extracted")
	err2 := run("bafkreiha5sgvksp5ooau53gcgplgt6ximremnu7jf2pchvwgdirglk5xoa", memo)
	require.ErrorAs(t, indexErr(err2), &te)
	assert.Len(t, m.x.requests(), requests+1, "the retry reused the extraction")
}

// --- stage warnings carry no endpoint text -------------------------------------------------------------------

func TestStageWarningMessageIsStageAndFixedCode(t *testing.T) {
	f := stageFailure{stage: "describer", skip: embeddings.SkipExtractionFailed, err: assert.AnError}
	assert.Equal(t, "describer:extraction_failed", f.message())
	f = stageFailure{stage: "extractor", reason: warnEgressNotAcknowledged, hard: true, err: assert.AnError}
	assert.Equal(t, "extractor:egress_not_acknowledged", f.message())
	assert.Equal(t, "describer:failed", stageFailure{stage: "describer", err: assert.AnError}.message())
}

// --- status is read-only -----------------------------------------------------------------------------------

func TestBulkStatusAndCancelNeverRewriteARunTheyDoNotOwn(t *testing.T) {
	n, _ := threeFiles()
	b := newTextBulkEnv(t, n, nil)
	require.NoError(t, b.store.SaveRun(t.Context(), embeddings.Run{
		ID: "stale", State: embeddings.RunRunning, Sources: []string{"pins"}, Caps: embeddings.RunCaps{MaxFiles: 1, MaxBytes: 1, FilesPerMinute: 1},
	}))
	saves := b.store.saves

	st := b.status(b.principal())
	assert.Equal(t, "paused", st["run"].(map[string]any)["state"], "the reader sees a run with no runner as paused")
	assert.Equal(t, saves, b.store.saves, "a status call writes nothing")
	assert.Equal(t, embeddings.RunRunning, b.store.stored(t).State, "the stored state is corrected by a start or at startup, not by a read")

	b.s.recoverBulkRun(t.Context())
	assert.Equal(t, embeddings.RunPaused, b.store.stored(t).State, "startup records the pause")
}
