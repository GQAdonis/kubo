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
	// presenceTimeout bounds the offline check that a derived hit's content is
	// still on the node.
	presenceTimeout = 10 * time.Second
	// sniffPrefixBytes is how much of a file larger than the text limit is read
	// to tell a binary format from text before deciding how much of it to read.
	sniffPrefixBytes = 1024
	// maxDerivedChunks bounds the chunks one extracted document may yield: about
	// four times what a text file at the default policy.max_bytes yields, so one
	// confirmed ipfs_index_cid cannot cost thousands of embedder requests.
	maxDerivedChunks = 2000
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
	// Kind, Source and MIMEType describe how the chunk was produced. Entries
	// written before derived content existed carry neither in the store; they are
	// natively indexed text and are reported as such. MIMEType carries the value
	// of MIME.
	Kind     string `json:"kind,omitempty"`
	Source   string `json:"source,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
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
	present := map[string]bool{}
	for _, h := range hits {
		if h.Derived() {
			// Derived text (extracted, OCR, transcript) is not a slice of the
			// file's bytes; it was stored with the vector, so nothing is read.
			// It would outlive the content, though, so a hit is returned only
			// while the node still holds the file.
			src := cmpOr(h.Path, "/ipfs/"+h.CID)
			ok, seen := present[src]
			if !seen {
				ok = s.contentPresent(ctx, src)
				present[src] = ok
			}
			if !ok {
				continue
			}
			out.Results = append(out.Results, indexHit{
				CID: h.CID, Path: h.Path, MIME: h.MIME, ChunkIndex: h.ChunkIndex,
				Score: 1 - h.Distance, Text: h.Text, Kind: kindOrText(h.Kind), Source: sourceOrText(h.Source), MIMEType: h.MIME,
			})
			continue
		}
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
			Score: 1 - h.Distance, Text: chunks[h.ChunkIndex], Kind: kindOrText(h.Kind), Source: sourceOrText(h.Source), MIMEType: h.MIME,
		})
	}
	return out, nil
}

// kindOrText and sourceOrText report a row written before derived content
// existed (both empty) as the natively indexed text it is.
func kindOrText(k string) string {
	if k == "" {
		return kindText
	}
	return k
}

func sourceOrText(src string) string {
	if src == "" {
		return embeddings.SourceText
	}
	return src
}

// contentPresent reports whether the node still holds the file: it reads the
// first byte with an offline cat, the one RPC command ipfs_index_search is
// allowed, so nothing is fetched from the network. Content removed outside ipfs
// mcp (pin rm and repo gc by another client) is gone, and the derived text
// stored with its vector must go unseen with it. A failure to confirm counts as
// absent: text is not returned on a guess. The stored rows are left for
// ipfs_index_reconcile, which removes the vectors of unpinned content; a search
// stays read-only.
func (s *Server) contentPresent(ctx context.Context, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, presenceTimeout)
	defer cancel()
	err := s.rawCall(ctx, "cat", []string{path}, map[string]any{"length": int64(1), "offline": true}, nil, func(r io.Reader) error {
		_, err := readBounded(r, 1)
		return err
	})
	if err != nil {
		s.log.Warn("index search: derived hit dropped, its content is not on the node; run ipfs_index_reconcile to remove its entries",
			"path", path, "error", clipMessage(err.Error()))
		return false
	}
	return true
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
	ext := svc.Extractor()
	p := "/ipfs/" + in.CID
	st, err := s.stat(ctx, p)
	if err != nil {
		return nil, err
	}
	if st.Type != "file" {
		return skipped(in.CID, embeddings.SkipNotAFile), nil
	}
	limit, err := s.readLimit(ctx, p, st.Size, pol, ext)
	if err != nil {
		return nil, err
	}
	if st.Size > int64(limit) {
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
	err = s.rawCall(ctx, "cat", []string{p}, map[string]any{"length": int64(limit) + 1}, nil, func(r io.Reader) error {
		var err error
		data, err = readBounded(r, int64(limit)+1)
		return err
	})
	if err != nil {
		return nil, err
	}
	var out indexCIDOutput
	if ext == nil {
		out, err = s.indexNative(ctx, svc, covers, in.CID, p, data)
	} else {
		out, err = s.indexRouted(ctx, svc, ext, covers, in.CID, p, data)
	}
	if err != nil {
		return nil, indexErr(err)
	}
	return out, nil
}

// readLimit is how many bytes of the file at p may be read. Text keeps the
// policy's max_bytes whether or not an extractor is configured, as in G12. Only
// content with a binary signature (a PDF, an Office package, an image, a video),
// which only the extractor can use, may be larger, up to extractor.max_bytes; it
// is told apart by a small prefix, so a large log or data dump costs one small
// read, not tens of megabytes.
func (s *Server) readLimit(ctx context.Context, p string, size int64, pol embeddings.Policy, ext *embeddings.Extractor) (int, error) {
	if ext == nil || size <= int64(pol.MaxBytes) || ext.MaxBytes() <= pol.MaxBytes {
		return pol.MaxBytes, nil
	}
	var prefix []byte
	err := s.rawCall(ctx, "cat", []string{p}, map[string]any{"length": int64(sniffPrefixBytes)}, nil, func(r io.Reader) error {
		var err error
		prefix, err = readBounded(r, sniffPrefixBytes)
		return err
	})
	if err != nil {
		return 0, err
	}
	if sniffBinary(prefix) != "" {
		return ext.MaxBytes(), nil
	}
	return pol.MaxBytes, nil
}

// indexNative indexes UTF-8 text the policy allows by chunking its bytes: the
// G12 path. The chunk text is not stored; a search re-reads it.
func (s *Server) indexNative(ctx context.Context, svc *embeddings.Service, covers func(string) bool, c, p string, data []byte) (indexCIDOutput, error) {
	pol := svc.Policy()
	mime, skip := pol.CheckContent("", data)
	if skip != "" {
		return skipped(c, skip), nil
	}
	chunks := embeddings.Chunk(string(data), pol.ChunkChars)
	if len(chunks) == 0 {
		return skipped(c, embeddings.SkipEmpty), nil // blank content has nothing to embed
	}
	n, err := svc.Index(ctx, covers, embeddings.Document{CID: c, Path: p, MIME: mime, Chunks: chunks, Kind: kindText, Source: embeddings.SourceText})
	if err != nil {
		return indexCIDOutput{}, err
	}
	return indexCIDOutput{CID: c, ChunksIndexed: n, Model: svc.Embedder().Model()}, nil
}

// indexRouted routes content by kind when an extractor is configured. Text, and
// bytes of no listed kind, stay on the native path. Documents, code and data go
// to the extractor, and so do images (when OCR is on) and video (when
// transcription is on); the returned Markdown is chunked with the same chunker
// and stored with its source, kind and text.
func (s *Server) indexRouted(ctx context.Context, svc *embeddings.Service, ext *embeddings.Extractor, covers func(string) bool, c, p string, data []byte) (indexCIDOutput, error) {
	mime, kind := classifyContent("", data)
	if kind == "" || kind == kindText {
		return s.indexNative(ctx, svc, covers, c, p, data)
	}
	if !svc.Policy().AllowsMIME(mime) {
		if looksLikeText(data) && sniffBinary(data) == "" {
			// Text the allowlist does not list under its refined type (a JSON
			// or CSV file) is still plain text, as it was before the extractor.
			// A file with a binary signature is not, even if its bytes happen
			// to be ASCII.
			return s.indexNative(ctx, svc, covers, c, p, data)
		}
		return skipped(c, embeddings.SkipKindNotEnabled), nil
	}
	source, ocr := embeddings.SourceExtracted, false
	switch kind {
	case kindImage:
		if !ext.OCR() {
			return skipped(c, embeddings.SkipKindNotEnabled), nil
		}
		source, ocr = embeddings.SourceOCR, true
	case kindVideo:
		if !ext.Transcribe() {
			return skipped(c, embeddings.SkipKindNotEnabled), nil
		}
		source = embeddings.SourceTranscript
	case kindDocument:
		ocr = ext.OCR()
	}
	res, err := ext.Extract(ctx, covers, embeddings.ExtractRequest{Filename: "content" + extensionFor(mime), MIME: mime, Data: data, OCR: ocr})
	switch {
	case err == nil:
	case errors.Is(err, embeddings.ErrEgressNotAcknowledged):
		return indexCIDOutput{}, err
	case errors.Is(err, embeddings.ErrExtractTooLarge), errors.Is(err, embeddings.ErrExtractedTextTooLarge):
		return skipped(c, embeddings.SkipTooLarge), nil
	case errors.Is(err, embeddings.ErrExtractorUnavailable):
		s.log.Warn("index: extractor unavailable", "cid", c, "kind", kind, "error", clipMessage(err.Error()))
		return skipped(c, embeddings.SkipExtractorUnavailable), nil
	case errors.Is(err, embeddings.ErrExtractionFailed):
		s.log.Warn("index: extraction failed", "cid", c, "kind", kind, "error", clipMessage(err.Error()))
		return skipped(c, embeddings.SkipExtractionFailed), nil
	default:
		return indexCIDOutput{}, err
	}
	if kind == kindDocument && res.OCRUsed {
		source = embeddings.SourceOCR
	}
	chunks := embeddings.Chunk(res.Markdown, svc.Policy().ChunkChars)
	if len(chunks) == 0 {
		return skipped(c, embeddings.SkipEmpty), nil
	}
	if len(chunks) > maxDerivedChunks {
		// Embedding it would cost more requests than the operator confirmed one
		// document for; nothing is truncated silently.
		s.log.Warn("index: extracted text yields too many chunks", "cid", c, "kind", kind, "chunks", len(chunks), "limit", maxDerivedChunks)
		return skipped(c, embeddings.SkipTooLarge), nil
	}
	n, err := svc.Index(ctx, covers, embeddings.Document{CID: c, Path: p, MIME: mime, Chunks: chunks, Kind: kind, Source: source})
	if err != nil {
		return indexCIDOutput{}, err
	}
	return indexCIDOutput{CID: c, ChunksIndexed: n, Model: svc.Embedder().Model()}, nil
}

// settingHint names the configuration key of a host that blocked a request,
// when one key holds it, so the remedy points at the right host.
func settingHint(eg *embeddings.EgressError) string {
	if eg.Setting == "" {
		return ""
	}
	return "; the missing host is the one in " + eg.Setting + ", so add it to the egress_ack too"
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
				"use a principal whose egress_ack covers the embedder (and a remote store), or \"local\" for loopback" + settingHint(eg),
			Details: map[string]any{"reason": ReasonBackendHostNotAcked, "missing_hosts": eg.Hosts},
		}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	}
	return &ToolError{Code: CodeIndexUnavailable, Message: clipMessage(err.Error()), Retryable: true}
}
