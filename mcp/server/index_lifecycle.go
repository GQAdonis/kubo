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
	if _, err := svc.Index(ictx, covers, embeddings.Document{CID: rootCID, Path: src, MIME: mime, Chunks: chunks}); err != nil {
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

// dropIfUnpinned deletes the vectors of one CID when it has any and no pin
// covers it. It never deletes on a guess: when the pin check or the store
// fails the vectors stay and the returned warning says so.
func (s *Server) dropIfUnpinned(ctx context.Context, svc *embeddings.Service, c string) (deleted bool, warning string) {
	n, err := svc.Store().CountCID(ctx, c)
	if err != nil {
		return false, fmt.Sprintf("index entries of %s were not checked: %s", c, clipMessage(err.Error()))
	}
	if n == 0 {
		return false, ""
	}
	pinned, err := s.pinCovers(ctx, c)
	if err != nil {
		return false, fmt.Sprintf("index entries of %s were kept because the pin check failed: %s", c, clipMessage(err.Error()))
	}
	if pinned {
		return false, ""
	}
	if _, err := svc.Store().DeleteCID(ctx, c); err != nil {
		return false, fmt.Sprintf("index entries of %s were not deleted: %s", c, clipMessage(err.Error()))
	}
	return true, ""
}

// afterUnpin removes the vectors of CIDs that a confirmed pin_rm or pin_update
// has just unpinned, unless another pin still covers them. It returns warnings
// for the result's _meta.
func (s *Server) afterUnpin(ctx context.Context, cids []string) []string {
	svc := s.embeddingsExt
	if svc == nil || len(cids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lifecycleTimeout)
	defer cancel()
	var warnings []string
	for _, c := range slices.Compact(slices.Sorted(slices.Values(cids))) {
		if _, w := s.dropIfUnpinned(ctx, svc, c); w != "" {
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
}

// newGCWatch loads the indexed CIDs. It is nil when embeddings are off.
func (s *Server) newGCWatch(ctx context.Context) (*gcWatch, []string) {
	svc := s.embeddingsExt
	if svc == nil {
		return nil, nil
	}
	w := &gcWatch{byHash: map[string]string{}}
	after := ""
	for {
		page, err := svc.Store().ListCIDs(ctx, after, watchPageSize)
		if err != nil {
			return nil, []string{"index entries were not checked against the removed blocks, run ipfs_index_reconcile: " + clipMessage(err.Error())}
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

// see records a removed key when it is an indexed CID.
func (w *gcWatch) see(key string) {
	if w == nil || len(w.byHash) == 0 {
		return
	}
	parsed, err := cid.Decode(key)
	if err != nil {
		return
	}
	if c, ok := w.byHash[string(parsed.Hash())]; ok && !slices.Contains(w.found, c) {
		w.found = append(w.found, c)
	}
}

// afterGC deletes the vectors of indexed CIDs that garbage collection removed,
// after the same pin check as for an unpin.
func (s *Server) afterGC(ctx context.Context, w *gcWatch) []string {
	if w == nil || len(w.found) == 0 {
		return nil
	}
	return s.afterUnpin(ctx, w.found)
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
// whose pin check fails is kept.
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
	for _, c := range cids {
		pinned, err := s.pinCovers(ctx, c)
		if err != nil {
			out.Kept++
			out.Warnings = append(out.Warnings, fmt.Sprintf("index entries of %s were kept because the pin check failed: %s", c, clipMessage(err.Error())))
			continue
		}
		if pinned {
			out.Kept++
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
