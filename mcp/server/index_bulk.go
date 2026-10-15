package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/kubo/mcp/server/embeddings"
)

// This file is built with and without the noagent tag, like index.go. It is
// the bulk run engine of ADR-0011 V4: an operator-started, capped, resumable
// walk over the node's pins and MFS that indexes each UnixFS file through the
// same per-file path as ipfs_index_cid. It only reads: every cat, ls and stat
// is offline, and it adds, pins, writes to MFS and announces nothing.

const (
	// bulkSaveEvery bounds how often a run's state is saved while it only
	// skips files that are already indexed.
	bulkSaveEvery = time.Second
	// bulkSaveTimeout bounds one save of the run state; it uses a context that
	// outlives the run, so a cancelled run still records where it stopped.
	bulkSaveTimeout = 10 * time.Second
	// bulkMaxDepth bounds how deep a walk descends into directories.
	bulkMaxDepth = 64
	// bulkListTimeout bounds listing the pins, which grows with the node.
	bulkListTimeout = 5 * time.Minute
)

// bulkCallTimeout bounds one stat or ls of the walk. A daemon command that
// stalls (a lock held by another client, a block that never answers) ends and
// the entry is skipped instead of the run hanging in "running". Tests shorten it.
var bulkCallTimeout = time.Minute

// bulkStopWait bounds how long a cancel waits for the run to end. Tests shorten
// it.
var bulkStopWait = 30 * time.Second

var (
	// errBulkCapReached ends a run that hit max_files_per_run or
	// max_bytes_per_run: the run is completed.
	errBulkCapReached = errors.New("a run cap was reached")
	// errBulkCancelled is the cause of a run stopped by ipfs_index_bulk_cancel.
	errBulkCancelled = errors.New("the bulk run was cancelled")
	// errBulkShutdown is the cause of a run stopped because ipfs mcp is exiting.
	errBulkShutdown = errors.New("ipfs mcp is shutting down")
)

// bulkState is the server's bulk run: at most one runner is live. A run that
// is paused after a restart has no runner until a confirmed resume starts one.
type bulkState struct {
	mu     sync.Mutex
	runner *bulkRunner
	// root outlives every tool call; runs and shutdown hang from it. It is made
	// on first use.
	once   sync.Once
	root   context.Context
	cancel context.CancelCauseFunc
	// cfg is embeddings.bulk, read at start.
	cfg BulkConfig
}

// active returns the live runner, or nil.
func (b *bulkState) active() *bulkRunner {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runner
}

// bulkNow and bulkWait are the clock and the wait of the rate limit; tests
// replace them.
func (s *Server) bulkNow() time.Time {
	if s.bulkClock != nil {
		return s.bulkClock()
	}
	return time.Now()
}

func (s *Server) bulkWait(ctx context.Context, d time.Duration) error {
	if s.bulkSleep != nil {
		return s.bulkSleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-t.C:
		return nil
	}
}

// bulkRunner is the live goroutine of one run.
type bulkRunner struct {
	s      *Server
	svc    *embeddings.Service
	store  embeddings.RunStore
	covers func(string) bool
	cancel context.CancelCauseFunc
	done   chan struct{}

	// run is the persisted state, owned by the run goroutine.
	run embeddings.Run
	// resumeAfter is the cursor the walk skips up to; it is the saved cursor of
	// a resumed run and empty for a new one.
	resumeAfter embeddings.RunCursor
	seen        map[string]struct{}
	next        time.Time
	lastSave    time.Time
}

// startBulkRun saves the run as running and starts its goroutine. The caller
// holds s.bulk.mu and has checked that no runner is live.
func (s *Server) startBulkRun(svc *embeddings.Service, rs embeddings.RunStore, covers func(string) bool, run embeddings.Run) (*bulkRunner, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.bulkRoot()), bulkSaveTimeout)
	err := rs.SaveRun(ctx, run)
	cancel()
	if err != nil {
		return nil, indexErr(err)
	}
	rctx, rcancel := context.WithCancelCause(s.bulkRoot())
	r := &bulkRunner{
		s: s, svc: svc, store: rs, covers: covers, cancel: rcancel, done: make(chan struct{}),
		run: run, resumeAfter: run.Cursor, seen: map[string]struct{}{},
	}
	s.bulk.runner = r
	go r.loop(rctx)
	return r, nil
}

// bulkRoot returns the context every run hangs from, creating it on first use.
func (s *Server) bulkRoot() context.Context {
	s.bulk.once.Do(func() {
		s.bulk.root, s.bulk.cancel = context.WithCancelCause(context.Background())
	})
	return s.bulk.root
}

// StopBulk stops the live bulk run, if any, and waits for it to record where
// it stopped. A run stopped this way is paused. It is safe to call when no
// embeddings are configured.
func (s *Server) StopBulk() {
	s.bulkRoot() // a stop with no run ever started still cancels what a later start would use
	r := s.bulk.active()
	if r != nil {
		r.stop(errBulkShutdown)
	}
	s.bulk.cancel(errBulkShutdown)
}

// stop cancels the runner with a cause and waits for it to end. It reports
// whether the run ended within bulkStopWait.
func (r *bulkRunner) stop(cause error) bool {
	r.cancel(cause)
	select {
	case <-r.done:
		return true
	case <-time.After(bulkStopWait):
		r.s.log.Warn("bulk index: the run did not stop in time")
		return false
	}
}

// loop runs the walk and records how it ended.
func (r *bulkRunner) loop(ctx context.Context) {
	defer close(r.done)
	err := r.walk(ctx)
	switch cause := context.Cause(ctx); {
	case errors.Is(err, errBulkCapReached), err == nil && ctx.Err() == nil:
		r.run.State = embeddings.RunCompleted
	case errors.Is(cause, errBulkCancelled):
		r.run.State = embeddings.RunCancelled
	case ctx.Err() != nil:
		// ipfs mcp is exiting. A run that stops with it is paused: it indexes
		// nothing until a confirmed resume.
		r.run.State = embeddings.RunPaused
	default:
		r.run.State = embeddings.RunFailed
		r.run.LastError = clipMessage(err.Error())
		r.s.log.Warn("bulk index: the run failed", "run", r.run.ID, "error", r.run.LastError)
	}
	r.saveFinal()
	r.s.bulk.mu.Lock()
	if r.s.bulk.runner == r {
		r.s.bulk.runner = nil
	}
	r.s.bulk.mu.Unlock()
}

// save records the run's state. force saves now; otherwise saves are spaced by
// bulkSaveEvery, which a stretch of skipped files would otherwise exceed.
func (r *bulkRunner) save(force bool) {
	now := r.s.bulkNow()
	if !force && now.Sub(r.lastSave) < bulkSaveEvery {
		return
	}
	r.lastSave = now
	r.saveFinal()
}

func (r *bulkRunner) saveFinal() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.s.bulkRoot()), bulkSaveTimeout)
	defer cancel()
	if err := r.store.SaveRun(ctx, r.run); err != nil {
		r.s.log.Warn("bulk index: cannot save the run state", "run", r.run.ID, "error", clipMessage(err.Error()))
	}
}

// walk visits the run's sources in order and indexes each file it finds.
func (r *bulkRunner) walk(ctx context.Context) error {
	for _, src := range r.run.Sources {
		if r.sourceDone(src) {
			continue
		}
		var err error
		switch src {
		case embeddings.SourcePins:
			err = r.walkPins(ctx)
		case embeddings.SourceMFS:
			err = r.walkMFS(ctx)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// sourceDone reports whether a resumed run already finished the source: the
// cursor is in a later one.
func (r *bulkRunner) sourceDone(src string) bool {
	if r.resumeAfter.Source == "" {
		return false
	}
	return slices.Index(r.run.Sources, src) < slices.Index(r.run.Sources, r.resumeAfter.Source)
}

// call is one daemon command of the walk, bounded by bulkCallTimeout so a
// command that stalls cannot hold the run (or an MFS directory lock) for good.
func (r *bulkRunner) call(ctx context.Context, timeout time.Duration, command string, args []string, opts map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return r.s.daemon.Call(ctx, command, args, opts, out)
}

// --- the tree walk ----------------------------------------------------------------

// treeWalk is one source's view of the walk: where a resumed run continues in
// that source's path space and how a finished entry moves the cursor.
type treeWalk struct {
	// after is the path of the last finished entry the resumed run recorded, or
	// "" for a walk from the start.
	after string
	// commit records that the entry at path is finished.
	commit func(path string)
}

// walkTree visits one entry of a source's tree and everything under it. ref is
// the path the node is read through and c its CID when the caller has it (a
// directory link, a pin); an empty c takes the CID from the stat. treePath is
// the entry's path in the source's own path space, the thing a cursor names.
// Every read is offline, so an entry whose blocks are not on the node is skipped
// not_local and nothing is fetched.
func (r *bulkRunner) walkTree(ctx context.Context, w *treeWalk, ref, c, name, treePath string, depth int) error {
	if depth > bulkMaxDepth {
		return nil
	}
	if c != "" {
		if _, dup := r.seen[c]; dup {
			return nil
		}
	}
	st, err := r.statOffline(ctx, ref)
	if err != nil {
		return r.fileFailure(ctx, cmp.Or(c, ref), err)
	}
	c = cmp.Or(c, st.Hash)
	if _, dup := r.seen[c]; dup {
		return nil
	}
	switch st.Type {
	case "directory":
		r.seen[c] = struct{}{}
		return r.walkChildren(ctx, w, ref, c, treePath, depth)
	case "file":
		return r.file(ctx, c, name, st.Size)
	}
	// Something that is not UnixFS (a DAG-CBOR node, a symlink).
	return r.countSkip(c, embeddings.SkipNotAFile)
}

// walkChildren lists a directory offline and visits its entries in name order.
func (r *bulkRunner) walkChildren(ctx context.Context, w *treeWalk, ref, c, treePath string, depth int) error {
	var resp lsResponse
	// resolve-type and size would make the daemon load every child, and one
	// missing block would hide the siblings that are on the node.
	err := r.call(ctx, bulkCallTimeout, "ls", []string{ref}, map[string]any{"offline": true, "resolve-type": false, "size": false}, &resp)
	if err != nil {
		return r.dirFailure(ctx, c, err)
	}
	type entry struct{ name, hash string }
	var entries []entry
	for _, o := range resp.Objects {
		for _, l := range o.Links {
			if safeEntryName(l.Name) && l.Hash != "" {
				entries = append(entries, entry{l.Name, l.Hash})
			}
		}
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.name, b.name) })
	for _, e := range entries {
		p := joinTreePath(treePath, e.name)
		if pathDone(w.after, p) {
			continue
		}
		// The stat decides what an entry is: an ls listing's type does not tell
		// a raw file from a symlink.
		if err := r.walkTree(ctx, w, "/ipfs/"+e.hash, e.hash, e.name, p, depth+1); err != nil {
			return err
		}
		w.commit(p)
	}
	return nil
}

// joinTreePath adds one entry name to a tree path ("" is a pin's root, "/" the
// MFS root).
func joinTreePath(base, name string) string {
	if base == "" {
		return name
	}
	return strings.TrimSuffix(base, "/") + "/" + name
}

// pathDone reports whether a resumed walk has already finished the entry at p,
// a file or a whole directory: the entry is the cursor, or sorts before it and
// does not contain it. A directory that contains the cursor is descended.
func pathDone(after, p string) bool {
	if after == "" {
		return false
	}
	if after == p {
		return true
	}
	if strings.HasPrefix(after, strings.TrimSuffix(p, "/")+"/") {
		return false
	}
	return segmentsLess(p, after)
}

func (r *bulkRunner) statOffline(ctx context.Context, p string) (statResponse, error) {
	var st statResponse
	err := r.call(ctx, bulkCallTimeout, "files/stat", []string{p}, map[string]any{"offline": true}, &st)
	return st, err
}

// --- pins -----------------------------------------------------------------

// walkPins visits every recursive and direct pin in CID order. The cursor names
// the last finished pin and, inside a pinned directory, the last finished entry,
// so a resume continues where the run stopped.
func (r *bulkRunner) walkPins(ctx context.Context) error {
	cids, err := r.pinnedRoots(ctx)
	if err != nil {
		return err
	}
	var after, resumePin, resumeInside string
	if cur := r.resumeAfter; cur.Source == embeddings.SourcePins {
		after, resumePin, resumeInside = cur.After, cur.Pin, cur.Inside
	}
	for _, c := range cids {
		if c <= after {
			continue
		}
		w := &treeWalk{commit: func(p string) {
			r.run.Cursor = embeddings.RunCursor{Source: embeddings.SourcePins, After: after, Pin: c, Inside: p}
			r.save(false)
		}}
		if c == resumePin {
			w.after = resumeInside
		}
		if err := r.walkTree(ctx, w, "/ipfs/"+c, c, "", "", 0); err != nil {
			return err
		}
		after = c
		r.run.Cursor = embeddings.RunCursor{Source: embeddings.SourcePins, After: c}
		r.save(false)
	}
	return nil
}

// pinnedRoots lists the CIDs of recursive and direct pins, sorted. Indirect
// pins are the insides of recursive ones and are reached by the walk.
func (r *bulkRunner) pinnedRoots(ctx context.Context) ([]string, error) {
	set := map[string]struct{}{}
	for _, typ := range []string{"recursive", "direct"} {
		var resp pinLsResponse
		if err := r.call(ctx, bulkListTimeout, "pin/ls", nil, map[string]any{"type": typ}, &resp); err != nil {
			return nil, fmt.Errorf("list %s pins: %w", typ, err)
		}
		for c := range resp.Keys {
			set[c] = struct{}{}
		}
	}
	cids := make([]string, 0, len(set))
	for c := range set {
		cids = append(cids, c)
	}
	slices.Sort(cids)
	return cids, nil
}

// safeEntryName reports whether a directory entry's name may be used: it names
// one path segment and holds no control character. The name only settles the
// media type; it never becomes a path the node is asked for.
func safeEntryName(n string) bool {
	if n == "" || n == "." || n == ".." || strings.ContainsAny(n, "/\\\x00") {
		return false
	}
	for _, r := range n {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// --- MFS ------------------------------------------------------------------

// walkMFS visits the MFS tree under the run's root, entries in name order. The
// daemon's files/ls and files/stat on an MFS path load directories through the
// online DAG service, so a lazily copied directory would be fetched from the
// network. The walk therefore asks once for the CID of the MFS root, which is a
// local node, and reads everything below it as /ipfs/<root>/<relative path>
// with the offline commands. An entry whose blocks are not on the node is
// skipped not_local, and the MFS path stays the cursor.
func (r *bulkRunner) walkMFS(ctx context.Context) error {
	var root statResponse
	if err := r.call(ctx, bulkCallTimeout, "files/stat", []string{"/"}, map[string]any{"hash": true, "offline": true}, &root); err != nil {
		return r.dirFailure(ctx, "/", err)
	}
	if root.Hash == "" {
		return nil
	}
	mfsRoot := cmp.Or(r.run.MFSRoot, "/")
	ref := "/ipfs/" + root.Hash
	rel := strings.Trim(mfsRoot, "/")
	if rel != "" {
		ref += "/" + rel
	}
	// A narrowed root that is not in the tree is an outcome of the whole run, not
	// one unreadable file that leaves a completed run with nothing indexed.
	if rel != "" {
		if _, err := r.statOffline(ctx, ref); err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			return fmt.Errorf("mfs_root %q cannot be read: %w", mfsRoot, err)
		}
	}
	w := &treeWalk{commit: func(p string) {
		r.run.Cursor = embeddings.RunCursor{Source: embeddings.SourceMFS, After: p}
		r.save(false)
	}}
	if r.resumeAfter.Source == embeddings.SourceMFS {
		w.after = r.resumeAfter.After
	}
	// The root has no CID of its own to key a duplicate check on until it is
	// stat-ed; the MFS root itself is never indexed.
	return r.walkTree(ctx, w, ref, "", path.Base(mfsRoot), mfsRoot, 0)
}

// segmentsLess orders two absolute paths segment by segment, the order a walk
// that sorts each directory's names visits them in.
func segmentsLess(a, b string) bool {
	as, bs := strings.Split(strings.Trim(a, "/"), "/"), strings.Split(strings.Trim(b, "/"), "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := strings.Compare(as[i], bs[i]); c != 0 {
			return c < 0
		}
	}
	return len(as) < len(bs)
}

// --- one file -------------------------------------------------------------

// countSkip records a file seen and skipped without reading it.
func (r *bulkRunner) countSkip(c, reason string) error {
	r.seen[c] = struct{}{}
	r.run.FilesSeen++
	r.skip(reason)
	return nil
}

func (r *bulkRunner) skip(reason string) {
	r.run.FilesSkipped++
	if r.run.SkippedByReason == nil {
		r.run.SkippedByReason = map[string]int{}
	}
	r.run.SkippedByReason[reason]++
}

// fileFailure maps an RPC failure on one entry: a block that is not on the node
// is a skip, a stopped daemon ends the run, and anything else skips the entry
// with a warning, because one unreadable entry should not end a long run.
func (r *bulkRunner) fileFailure(ctx context.Context, c string, err error) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	switch {
	case notLocal(err):
		return r.countSkip(c, embeddings.SkipNotLocal)
	case errors.Is(err, ErrDaemonUnreachable):
		return err
	case errors.Is(err, context.DeadlineExceeded):
		r.s.log.Warn("bulk index: reading an entry timed out", "run", r.run.ID, "cid", c)
		return r.countSkip(c, embeddings.SkipExtractionFailed)
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		r.s.log.Warn("bulk index: an entry could not be read", "run", r.run.ID, "cid", c, "error", clipMessage(err.Error()))
		return r.countSkip(c, embeddings.SkipExtractionFailed)
	}
	return err
}

// dirFailure maps an RPC failure on a directory the walk could not list. Its
// entries are not files the run tried, so only a directory the node does not
// hold counts, as not_local; any other failure is logged and the walk goes on
// with the next entry.
func (r *bulkRunner) dirFailure(ctx context.Context, c string, err error) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	switch {
	case notLocal(err):
		return r.countSkip(c, embeddings.SkipNotLocal)
	case errors.Is(err, ErrDaemonUnreachable):
		return err
	}
	r.s.log.Warn("bulk index: a directory could not be listed", "run", r.run.ID, "cid", c, "error", clipMessage(err.Error()))
	return nil
}

// file indexes one file through the per-file path of ipfs_index_cid.
func (r *bulkRunner) file(ctx context.Context, c, name string, size int64) error {
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	n, err := r.svc.Store().CountCID(ctx, c)
	if err != nil {
		return indexErr(fmt.Errorf("index %s: %w", c, err))
	}
	if n > 0 {
		return r.countSkip(c, embeddings.SkipAlreadyIndexed)
	}
	caps := r.run.Caps
	// What reading the file can cost: its size, up to the most any stage reads.
	// A large file that is skipped after a short sniff costs a short read.
	cost := min(size, int64(max(r.svc.Policy().MaxBytes, binaryReadMax(r.svc))))
	if cost > caps.MaxBytes {
		// It can never fit a run of this size.
		return r.countSkip(c, embeddings.SkipTooLarge)
	}
	if r.run.Attempted() >= caps.MaxFiles || r.run.BytesRead+cost > caps.MaxBytes {
		return errBulkCapReached
	}
	// The egress check repeats before each file: the principal's acknowledgement
	// is read at start, but the stages' hosts must cover every file.
	if err := r.svc.CheckStageEgress(r.covers); err != nil {
		return indexErr(err)
	}
	if err := r.pace(ctx); err != nil {
		return err
	}
	// Counted once the file is going to be read, so a run paused while it waits
	// for its turn does not count the file again when it resumes.
	r.seen[c] = struct{}{}
	r.run.FilesSeen++
	// A pause or cancel that ends the file before it is indexed leaves it for the
	// resume, which counts it then; take the count back so it is counted once.
	unsee := func() {
		delete(r.seen, c)
		r.run.FilesSeen--
	}
	t := indexTarget{CID: c, Path: "/ipfs/" + c, Name: name, Offline: true, Checked: true, memo: &stageMemo{}}
	st := statResponse{Hash: c, Size: size, Type: "file"}
	for {
		out, err := r.s.indexResolved(ctx, r.svc, r.covers, t, st)
		if err == nil {
			r.record(out)
			r.save(true)
			return nil
		}
		var te *ToolError
		switch {
		case ctx.Err() != nil:
			unsee()
			return context.Cause(ctx)
		case errors.As(err, &te) && te.Code == CodeResourceExhausted:
			// The describer's hourly limit is used up: wait for a request to
			// free and try the file again, so the description is not left out.
			if werr := r.s.bulkWait(ctx, r.svc.Describer().RetryAfter()); werr != nil {
				unsee()
				return werr
			}
			continue
		case errors.As(err, &te):
			return err
		}
		return r.fileFailure(ctx, c, err)
	}
}

// record adds the result of one file to the run.
func (r *bulkRunner) record(out indexCIDOutput) {
	if len(out.warnings) > 0 {
		// The result of a tool call carries these; a bulk run has no caller to
		// read them, so they go to the log with the run.
		r.s.log.Warn("bulk index: a stage failed on a file", "run", r.run.ID, "cid", out.CID, "warnings", out.warnings)
	}
	r.run.BytesRead += out.read
	if out.SkippedReason != "" {
		r.skip(out.SkippedReason)
		return
	}
	r.run.FilesIndexed++
	r.run.ChunksIndexed += out.ChunksIndexed
}

// pace spaces the files a run reads by files_per_minute.
func (r *bulkRunner) pace(ctx context.Context) error {
	gap := time.Minute / time.Duration(max(r.run.Caps.FilesPerMinute, 1))
	if now := r.s.bulkNow(); now.Before(r.next) {
		if err := r.s.bulkWait(ctx, r.next.Sub(now)); err != nil {
			return err
		}
	}
	r.next = r.s.bulkNow().Add(gap)
	return nil
}
