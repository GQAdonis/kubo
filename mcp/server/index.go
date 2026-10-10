package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/ipfs/kubo/mcp/server/embeddings"
)

// This file is built with and without the noagent tag, like embeddings.go.

func init() { registerGroup(indexTools) }

// indexGroup is the contract group of the index tools.
const indexGroup = "index"

const (
	// statusProbeTimeout bounds the store reachability probe of
	// ipfs_index_status.
	statusProbeTimeout = 10 * time.Second
	// chunkReadTimeout bounds reading one document back to cut out the text
	// of a search hit.
	chunkReadTimeout = 20 * time.Second
	// defaultTopK is ipfs_index_search's default top_k.
	defaultTopK = 5
)

func indexTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "ipfs_index_search", Call: s.indexSearch},
		{Name: "ipfs_index_cid", Call: s.indexCID},
		{Name: "ipfs_index_status", Call: s.indexStatus},
		{Name: "ipfs_index_reconcile", Call: s.indexReconcile},
	}
}

// attachEmbeddings makes the embeddings service the server's and offers the
// index tools that were waiting for it. Until it runs, the index tools are
// unavailable with the reason not_configured. It must run before the server
// is used by a transport or the agent loop, which read the tool set.
func (s *Server) attachEmbeddings(svc *embeddings.Service) {
	s.embeddingsExt = svc
	waiting := s.indexImpls
	s.indexImpls = nil
	s.unavailable = slices.DeleteFunc(s.unavailable, func(u Unavailable) bool {
		return slices.ContainsFunc(waiting, func(i toolImpl) bool { return i.Name == u.Name })
	})
	for _, impl := range waiting {
		if err := s.place(impl); err != nil {
			s.log.Error("offer index tool", "tool", impl.Name, "error", err)
		}
	}
	slices.SortFunc(s.registered, func(a, b string) int { return indexOf(s.cat, a) - indexOf(s.cat, b) })
}

// callerCovers is the egress check of the identity behind ctx. Calls that
// carry no identity (stdio, in-process) are the operator's, whose empty
// egress_ack covers nothing.
func callerCovers(ctx context.Context) func(host string) bool {
	id, _ := IdentityFromContext(ctx)
	return EgressCovers(id)
}

// --- ipfs_index_status ----------------------------------------------------

type indexStatusInput struct {
	CID string `json:"cid"`
}

type indexStatusOutput struct {
	Enabled        bool   `json:"enabled"`
	Mode           string `json:"mode"`
	StoreReachable bool   `json:"store_reachable"`
	Auto           bool   `json:"auto"`
	Model          string `json:"model,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
	IndexedCIDs    *int   `json:"indexed_cids,omitempty"`
	ChunkCount     *int   `json:"chunk_count,omitempty"`
	CIDIndexed     *bool  `json:"cid_indexed,omitempty"`
}

func (s *Server) indexStatus(ctx context.Context, raw json.RawMessage) (any, error) {
	var in indexStatusInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	svc := s.embeddingsExt
	if svc == nil {
		return indexStatusOutput{Mode: "disabled"}, nil
	}
	out := indexStatusOutput{
		Enabled:    true,
		Mode:       string(svc.Mode()),
		Auto:       svc.Policy().Auto,
		Model:      svc.Embedder().Model(),
		Dimensions: svc.Embedder().Dimensions(),
	}
	ctx, cancel := context.WithTimeout(ctx, statusProbeTimeout)
	defer cancel()
	chunks, err := svc.Store().Count(ctx)
	if err != nil {
		s.log.Debug("index status: store unreachable", "error", err)
		return out, nil
	}
	out.StoreReachable = true
	out.ChunkCount = &chunks
	cids, err := svc.Store().CountCIDs(ctx)
	if err != nil {
		return nil, indexErr(fmt.Errorf("index status: %w", err))
	}
	out.IndexedCIDs = &cids
	if in.CID != "" {
		n, err := svc.Store().CountCID(ctx, in.CID)
		if err != nil {
			return nil, indexErr(fmt.Errorf("index status: %w", err))
		}
		indexed := n > 0
		out.CIDIndexed = &indexed
	}
	return out, nil
}

// --- ipfs_index_search ----------------------------------------------------

type indexSearchInput struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
	CID   string `json:"cid"`
}

type indexHit struct {
	CID        string  `json:"cid"`
	Path       string  `json:"path,omitempty"`
	MIME       string  `json:"mime,omitempty"`
	ChunkIndex int     `json:"chunk_index"`
	Score      float64 `json:"score"`
	Text       string  `json:"text"`
}

type indexSearchOutput struct {
	Model   string     `json:"model"`
	Results []indexHit `json:"results"`
}

// indexSearch embeds the query (which sends it to the embedder, so the
// caller's egress_ack must cover it), finds the nearest chunks and reads each
// chunk's text back from the node: the store keeps vectors, not text.
func (s *Server) indexSearch(ctx context.Context, raw json.RawMessage) (any, error) {
	var in indexSearchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	svc := s.embeddingsExt
	if svc == nil {
		return nil, notConfigured()
	}
	k := in.TopK
	if k == 0 {
		k = defaultTopK
	}
	covers := callerCovers(ctx)
	var hits []embeddings.Hit
	var err error
	if in.CID != "" {
		hits, err = svc.SearchCID(ctx, covers, in.Query, in.CID, k)
	} else {
		hits, err = svc.Search(ctx, covers, in.Query, k)
	}
	if err != nil {
		return nil, indexErr(err)
	}
	out := indexSearchOutput{Model: svc.Embedder().Model(), Results: make([]indexHit, 0, len(hits))}
	pol := svc.Policy()
	texts := map[string][]string{}
	for _, h := range hits {
		src := cmpOr(h.Path, "/ipfs/"+h.CID)
		chunks, ok := texts[src]
		if !ok {
			chunks = s.readChunks(ctx, src, pol)
			texts[src] = chunks
		}
		if h.ChunkIndex < 0 || h.ChunkIndex >= len(chunks) {
			// The content is gone from this node or was cut differently when
			// it was indexed. Reconcile or ipfs_index_cid with force repairs
			// the index.
			s.log.Debug("index search: chunk text unavailable", "cid", h.CID, "chunk", h.ChunkIndex)
			continue
		}
		out.Results = append(out.Results, indexHit{
			CID: h.CID, Path: h.Path, MIME: h.MIME, ChunkIndex: h.ChunkIndex,
			Score: 1 - h.Distance, Text: chunks[h.ChunkIndex],
		})
	}
	return out, nil
}

// readChunks reads the content at path from the node and cuts it with the
// policy that indexed it. It returns nil when the content cannot be read.
// The read is offline: a hit whose blocks were garbage collected is dropped
// at once instead of fetched from the network.
func (s *Server) readChunks(ctx context.Context, path string, pol embeddings.Policy) []string {
	ctx, cancel := context.WithTimeout(ctx, chunkReadTimeout)
	defer cancel()
	var data []byte
	err := s.rawCall(ctx, "cat", []string{path}, map[string]any{"length": int64(pol.MaxBytes) + 1, "offline": true}, nil, func(r io.Reader) error {
		var err error
		data, err = readBounded(r, int64(pol.MaxBytes)+1)
		return err
	})
	if err != nil || len(data) > pol.MaxBytes {
		s.log.Debug("index search: cannot read content", "path", path, "error", err)
		return nil
	}
	return embeddings.Chunk(string(data), pol.ChunkChars)
}

func notConfigured() error {
	return &ToolError{Code: CodeToolUnavailable, Message: "embeddings are not configured: add an embeddings section to the ipfs mcp config"}
}

// --- ipfs_index_cid -------------------------------------------------------

type indexCIDInput struct {
	CID   string `json:"cid"`
	Force bool   `json:"force"`
}

type indexCIDOutput struct {
	CID           string `json:"cid"`
	ChunksIndexed int    `json:"chunks_indexed"`
	Model         string `json:"model,omitempty"`
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// skipped is the result for content the policy leaves out.
func skipped(cid, reason string) indexCIDOutput {
	return indexCIDOutput{CID: cid, SkippedReason: reason}
}

// indexCID reads one file from the node and indexes it. It reads content the
// operator already has; it never adds, pins or announces anything.
func (s *Server) indexCID(ctx context.Context, raw json.RawMessage) (any, error) {
	var in indexCIDInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	svc := s.embeddingsExt
	if svc == nil {
		return nil, notConfigured()
	}
	covers := callerCovers(ctx)
	// Refuse before reading anything when the content could not be embedded.
	if err := svc.CheckEgress(covers); err != nil {
		return nil, indexErr(err)
	}
	pol := svc.Policy()
	p := "/ipfs/" + in.CID
	st, err := s.stat(ctx, p)
	if err != nil {
		return nil, err
	}
	if st.Type != "file" {
		return skipped(in.CID, embeddings.SkipNotAFile), nil
	}
	if st.Size > int64(pol.MaxBytes) {
		return skipped(in.CID, embeddings.SkipTooLarge), nil
	}
	if !in.Force {
		n, err := svc.Store().CountCID(ctx, in.CID)
		if err != nil {
			return nil, indexErr(fmt.Errorf("index %s: %w", in.CID, err))
		}
		if n > 0 {
			return skipped(in.CID, "already_indexed"), nil
		}
	}
	var data []byte
	err = s.rawCall(ctx, "cat", []string{p}, map[string]any{"length": int64(pol.MaxBytes) + 1}, nil, func(r io.Reader) error {
		var err error
		data, err = readBounded(r, int64(pol.MaxBytes)+1)
		return err
	})
	if err != nil {
		return nil, err
	}
	mime, skip := pol.CheckContent("", data)
	if skip != "" {
		return skipped(in.CID, skip), nil
	}
	chunks := embeddings.Chunk(string(data), pol.ChunkChars)
	if len(chunks) == 0 {
		return skipped(in.CID, embeddings.SkipEmpty), nil // blank content has nothing to embed
	}
	n, err := svc.Index(ctx, covers, embeddings.Document{CID: in.CID, Path: p, MIME: mime, Chunks: chunks})
	if err != nil {
		return nil, indexErr(err)
	}
	return indexCIDOutput{CID: in.CID, ChunksIndexed: n, Model: svc.Embedder().Model()}, nil
}

// indexErr maps a failure of the embedder or the vector store to the contract
// error model: a missing egress acknowledgement is egress_not_acknowledged and
// names the hosts; anything else that is not a cancellation is
// index_unavailable, which is retryable.
func indexErr(err error) error {
	var eg *embeddings.EgressError
	switch {
	case errors.As(err, &eg):
		return &ToolError{
			Code: CodeEgressNotAcknowledged,
			Message: "indexing and searching send text to " + strings.Join(eg.Hosts, ", ") + ", which is not in the egress_ack of the calling identity; " +
				"use a principal whose egress_ack covers the embedder (and a remote store), or \"local\" for loopback",
			Details: map[string]any{"reason": ReasonBackendHostNotAcked, "missing_hosts": eg.Hosts},
		}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	}
	return &ToolError{Code: CodeIndexUnavailable, Message: clipMessage(err.Error()), Retryable: true}
}
