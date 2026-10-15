package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/mcp/server/embeddings"
)

const (
	// autoIndexTimeout bounds the indexing that follows one ipfs_add. The add
	// has already succeeded; indexing that runs out of time is a warning.
	autoIndexTimeout = 45 * time.Second
	// lifecycleTimeout bounds the index housekeeping that follows pin_rm,
	// pin_update and repo_gc. It runs even when the client has given up.
	lifecycleTimeout = 60 * time.Second
	// watchPageSize is how many indexed CIDs are read per store request when
	// repo_gc loads the set of indexed CIDs.
	watchPageSize = 500
)

// --- automatic indexing after ipfs_add -------------------------------------

// autoIndex indexes content that ipfs_add has just stored, when
// embeddings.policy.auto is on. It returns warnings for the add's result: an
// indexing failure never fails the add. It reads nothing from the node and
// adds, pins and announces nothing; the bytes are the ones the caller sent.
//
// Text reaches the embedder only if the egress_ack of the calling identity
// covers it. The operator identity (stdio, --auth-token-*) has no egress_ack,
// so an add from it is not indexed and says why.
func (s *Server) autoIndex(ctx context.Context, rootCID, filename string, data []byte) []string {
	svc := s.embeddingsExt
	if svc == nil || !svc.Policy().Auto {
		return nil
	}
	pol := svc.Policy()
	mime, skip := pol.CheckContent(filename, data)
	if skip != "" {
		return nil // outside the policy: nothing to report
	}
	covers := callerCovers(ctx)
	if err := svc.CheckEgress(covers); err != nil {
		return []string{"content was added but not indexed: " + indexErr(err).Error()}
	}
	chunks := embeddings.Chunk(string(data), pol.ChunkChars)
	if len(chunks) == 0 {
		return nil
	}
	src := "/ipfs/" + rootCID
	if filename != "" {
		src += "/" + filename
	}
	ictx, cancel := context.WithTimeout(ctx, autoIndexTimeout)
	defer cancel()
	if _, err := svc.Index(ictx, covers, embeddings.Document{CID: rootCID, Path: src, MIME: mime, Chunks: chunks, Kind: kindText, Source: embeddings.SourceText}); err != nil {
		s.log.Warn("automatic indexing failed", "cid", rootCID, "error", clipMessage(err.Error()))
		return []string{"content was added but not indexed: " + clipMessage(err.Error())}
	}
	return nil
}

// --- pin-aware deletion ----------------------------------------------------

// pinCovers reports whether any pin covers the CID: direct, recursive or
// indirect (pin ls --type=all). An error means the answer is unknown.
func (s *Server) pinCovers(ctx context.Context, c string) (bool, error) {
	var resp pinLsResponse
	err := s.daemon.Call(ctx, "pin/ls", []string{"/ipfs/" + c}, map[string]any{"type": "all"}, &resp)
	if err == nil {
		return len(resp.Keys) > 0, nil
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) && strings.Contains(rpcErr.Message, "is not pinned") {
		return false, nil
	}
	return false, err
}

// dropIfUnpinned deletes the vectors of one CID when it has any and nothing
// covers it: no pin (direct, recursive or indirect) and, when mfs is not nil,
// no file in the MFS. It never deletes on a guess: when the pin check, the MFS
// check or the store fails the vectors stay and the returned warning says so.
func (s *Server) dropIfUnpinned(ctx context.Context, svc *embeddings.Service, c string, mfs *mfsCoverage) (deleted bool, warning string) {
	n, err := svc.Store().CountCID(ctx, c)
	if err != nil {
		return false, fmt.Sprintf("index entries of %s were not checked: %s", c, clipMessage(err.Error()))
	}
	if n == 0 {
		return false, ""
	}
	if keep, w := s.keepEntries(ctx, c, mfs); keep {
		return false, w
	}
	if _, err := svc.Store().DeleteCID(ctx, c); err != nil {
		return false, fmt.Sprintf("index entries of %s were not deleted: %s", c, clipMessage(err.Error()))
	}
	return true, ""
}

// keepEntries reports whether the index entries of c must stay: a pin covers
// it, the MFS holds it, or either answer is unknown (the warning says which).
func (s *Server) keepEntries(ctx context.Context, c string, mfs *mfsCoverage) (keep bool, warning string) {
	pinned, err := s.pinCovers(ctx, c)
	if err != nil {
		return true, fmt.Sprintf("index entries of %s were kept because the pin check failed: %s", c, clipMessage(err.Error()))
	}
	if pinned {
		return true, ""
	}
	if mfs == nil {
		return false, ""
	}
	in, known := mfs.covers(ctx, c)
	switch {
	case in:
		return true, ""
	case !known:
		return true, fmt.Sprintf("index entries of %s were kept because the MFS could not be fully checked: %s", c, mfs.reason())
	}
	return false, ""
}

// afterUnpin removes the vectors that a confirmed pin_rm or pin_update has left
// uncovered. The roots it is given are the unpinned CIDs; a pinned directory
// has no vectors of its own, its files do (a bulk run indexes each file under
// its own CID), so the unpinned DAG is walked offline and every indexed file in
// it is checked too. Vectors stay while another pin or the MFS covers the CID.
// It returns warnings for the result's _meta.
func (s *Server) afterUnpin(ctx context.Context, roots []string) []string {
	svc := s.embeddingsExt
	if svc == nil || len(roots) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lifecycleTimeout)
	defer cancel()
	roots = slices.Compact(slices.Sorted(slices.Values(roots)))
	var warnings []string
	targets := slices.Clone(roots)
	indexed, err := s.loadIndexed(ctx)
	switch {
	case err != nil:
		warnings = append(warnings, "the files under the unpinned paths were not checked, run ipfs_index_reconcile: "+clipMessage(err.Error()))
	case len(indexed.byHash) > 0:
		for _, root := range roots {
			if _, own := indexed.lookup(root); own {
				continue // a file with entries of its own: nothing below it
			}
			complete, werr := s.walkFiles(ctx, root, indexed.see)
			if complete {
				continue
			}
			reason := "its walk reached a limit"
			if werr != nil {
				reason = clipMessage(werr.Error())
			}
			warnings = append(warnings, fmt.Sprintf("not every file under %s was checked (%s); run ipfs_index_reconcile to remove the entries of files no pin covers", root, reason))
		}
		targets = append(targets, indexed.found...)
	}
	return append(warnings, s.dropUncovered(ctx, svc, targets, &mfsCoverage{s: s})...)
}

// dropUncovered runs dropIfUnpinned over the CIDs, each once.
func (s *Server) dropUncovered(ctx context.Context, svc *embeddings.Service, cids []string, mfs *mfsCoverage) []string {
	var warnings []string
	for _, c := range slices.Compact(slices.Sorted(slices.Values(cids))) {
		if _, w := s.dropIfUnpinned(ctx, svc, c, mfs); w != "" {
			warnings = append(warnings, w)
		}
	}
	return warnings
}

// gcWatch finds, among the CIDs a garbage collection removed, those that are
// indexed. Garbage collection reports block keys, which carry the raw codec
// (and so differ in text from the dag-pb CID that was indexed), so CIDs are
// compared by multihash.
type gcWatch struct {
	byHash map[string]string
	found  []string
	seen   map[string]struct{}
}

// loadIndexed loads the indexed CIDs, by multihash. It fails when the store
// cannot be listed.
func (s *Server) loadIndexed(ctx context.Context) (*gcWatch, error) {
	w := &gcWatch{byHash: map[string]string{}, seen: map[string]struct{}{}}
	after := ""
	for {
		page, err := s.embeddingsExt.Store().ListCIDs(ctx, after, watchPageSize)
		if err != nil {
			return nil, err
		}
		for _, c := range page {
			if parsed, err := cid.Decode(c); err == nil {
				w.byHash[string(parsed.Hash())] = c
			}
		}
		if len(page) < watchPageSize {
			return w, nil
		}
		after = page[len(page)-1]
	}
}

// newGCWatch loads the indexed CIDs. It is nil when embeddings are off.
func (s *Server) newGCWatch(ctx context.Context) (*gcWatch, []string) {
	if s.embeddingsExt == nil {
		return nil, nil
	}
	w, err := s.loadIndexed(ctx)
	if err != nil {
		return nil, []string{"index entries were not checked against the removed blocks, run ipfs_index_reconcile: " + clipMessage(err.Error())}
	}
	return w, nil
}

// lookup returns the indexed form of a key that is an indexed CID.
func (w *gcWatch) lookup(key string) (string, bool) {
	parsed, err := cid.Decode(key)
	if err != nil {
		return "", false
	}
	c, ok := w.byHash[string(parsed.Hash())]
	return c, ok
}

// see records a key when it is an indexed CID.
func (w *gcWatch) see(key string) {
	if w == nil || len(w.byHash) == 0 {
		return
	}
	c, ok := w.lookup(key)
	if !ok {
		return
	}
	if _, dup := w.seen[c]; dup {
		return
	}
	w.seen[c] = struct{}{}
	w.found = append(w.found, c)
}

// afterGC deletes the vectors of indexed CIDs that garbage collection removed,
// after the same pin check as for an unpin.
func (s *Server) afterGC(ctx context.Context, w *gcWatch) []string {
	if w == nil || len(w.found) == 0 {
		return nil
	}
	svc := s.embeddingsExt
	// Garbage collection removes only blocks no pin and no MFS file reaches, and
	// each indexed file is itself among the removed keys, so no walk is needed.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lifecycleTimeout)
	defer cancel()
	return s.dropUncovered(ctx, svc, w.found, nil)
}

// --- ipfs_index_reconcile --------------------------------------------------

type indexReconcileInput struct {
	DryRun bool   `json:"dry_run"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type indexReconcileOutput struct {
	Checked  int      `json:"checked"`
	Deleted  []string `json:"deleted"`
	Kept     int      `json:"kept"`
	DryRun   bool     `json:"dry_run"`
	Page     pageInfo `json:"page"`
	Warnings []string `json:"warnings,omitempty"`
}

// indexReconcile deletes the vectors of indexed CIDs that no pin covers, one
// page of indexed CIDs per call, for content unpinned outside ipfs mcp. A CID
// that a pin or the MFS covers is kept, and so is one whose check fails.
func (s *Server) indexReconcile(ctx context.Context, raw json.RawMessage) (any, error) {
	var in indexReconcileInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	svc := s.embeddingsExt
	if svc == nil {
		return nil, notConfigured()
	}
	limit, err := pageLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	cur, err := decodeCursor(in.Cursor)
	if err != nil {
		return nil, err
	}
	// One more than the page says whether another page follows.
	cids, err := svc.Store().ListCIDs(ctx, cur.After, limit+1)
	if err != nil {
		return nil, indexErr(err)
	}
	out := indexReconcileOutput{Deleted: []string{}, DryRun: in.DryRun}
	if len(cids) > limit {
		cids = cids[:limit]
		out.Page = pageInfo{HasMore: true, NextCursor: encodeCursor(cursor{After: cids[limit-1]})}
	}
	out.Checked = len(cids)
	mfs := &mfsCoverage{s: s}
	for _, c := range cids {
		if keep, w := s.keepEntries(ctx, c, mfs); keep {
			out.Kept++
			if w != "" {
				out.Warnings = append(out.Warnings, w)
			}
			continue
		}
		if !in.DryRun {
			if _, err := svc.Store().DeleteCID(ctx, c); err != nil {
				out.Kept++
				out.Warnings = append(out.Warnings, fmt.Sprintf("index entries of %s were not deleted: %s", c, clipMessage(err.Error())))
				continue
			}
		}
		out.Deleted = append(out.Deleted, c)
	}
	return out, nil
}
