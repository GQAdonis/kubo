package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file is built with and without the noagent tag, like index.go. It is the
// per-file index path: ipfs_index_cid calls it for one CID, and a bulk run calls
// it for each file it walks, with the file's name and offline reads.

// stageWarningsMetaKey is the _meta key of a result that lists the stages of
// the file that failed or were refused while another stage still produced text.
// The output schema of ipfs_index_cid has no field for them.
const stageWarningsMetaKey = contract.StageWarningsMetaKey

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

	// warnings name the stages that failed while another stage produced text,
	// each as "stage:reason" with a fixed reason code and no endpoint text. read
	// is how many content bytes the file cost. Neither is part of the contract's
	// output; warnings ride in _meta.
	warnings []string
	read     int64
}

func (o indexCIDOutput) decorateResult(r *mcp.CallToolResult) {
	if len(o.warnings) == 0 {
		return
	}
	if r.Meta == nil {
		r.Meta = mcp.Meta{}
	}
	r.Meta[stageWarningsMetaKey] = o.warnings
}

// skipped is the result for content the policy leaves out.
func skipped(cid, reason string) indexCIDOutput {
	return indexCIDOutput{CID: cid, SkippedReason: reason}
}

// indexTarget is one file the index path works on.
type indexTarget struct {
	CID  string
	Path string
	// Name is the file's name when the walk knows it (a directory entry, an MFS
	// path); it settles the media type of text that has no signature. Empty for
	// ipfs_index_cid, which is CID-only.
	Name  string
	Force bool
	// Offline makes every cat and ls carry offline: true, so a block that is not
	// on the node is a skip and never a fetch.
	Offline bool
	// Checked means the caller already asked the store whether the CID is
	// indexed, so the file is not asked about again.
	Checked bool
	// memo keeps what a stage already produced for this file, so a retry after
	// the describer's hourly limit does not send the file to the extractor again.
	memo *stageMemo
}

// stageMemo is the extractor output of one file across retries.
type stageMemo struct {
	ext *embeddings.Extraction
}

// rpcOptions adds offline: true to the options of a read when the target asks.
func (t indexTarget) rpcOptions(opts map[string]any) map[string]any {
	if t.Offline {
		opts["offline"] = true
	}
	return opts
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
	p := "/ipfs/" + in.CID
	st, err := s.stat(ctx, p)
	if err != nil {
		return nil, err
	}
	out, err := s.indexResolved(ctx, svc, covers, indexTarget{CID: in.CID, Path: p, Force: in.Force}, st)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// indexResolved indexes one file whose stat the caller has. A daemon failure
// is returned as it is; a failure of the embedder, store, extractor or
// describer is already mapped by indexErr.
func (s *Server) indexResolved(ctx context.Context, svc *embeddings.Service, covers func(string) bool, t indexTarget, st statResponse) (indexCIDOutput, error) {
	if st.Type != "file" {
		return skipped(t.CID, embeddings.SkipNotAFile), nil
	}
	limit, err := s.readLimit(ctx, svc, t, st.Size)
	if err != nil {
		if t.Offline && notLocal(err) {
			return skipped(t.CID, embeddings.SkipNotLocal), nil
		}
		return indexCIDOutput{}, err
	}
	if st.Size > int64(limit) {
		return skipped(t.CID, embeddings.SkipTooLarge), nil
	}
	if !t.Force && !t.Checked {
		n, err := svc.Store().CountCID(ctx, t.CID)
		if err != nil {
			return indexCIDOutput{}, indexErr(fmt.Errorf("index %s: %w", t.CID, err))
		}
		if n > 0 {
			return skipped(t.CID, embeddings.SkipAlreadyIndexed), nil
		}
	}
	var data []byte
	catCtx, cancel := context.WithTimeout(ctx, rpcReadTimeout)
	defer cancel()
	err = s.rawCall(catCtx, "cat", []string{t.Path}, t.rpcOptions(map[string]any{"length": int64(limit) + 1}), nil, func(r io.Reader) error {
		var err error
		data, err = readBounded(r, int64(limit)+1)
		return err
	})
	if err != nil {
		if t.Offline && notLocal(err) {
			return skipped(t.CID, embeddings.SkipNotLocal), nil
		}
		return indexCIDOutput{}, err
	}
	out, err := s.indexFile(ctx, svc, covers, t, data)
	if err != nil {
		return indexCIDOutput{}, indexErr(err)
	}
	out.read = int64(len(data))
	return out, nil
}

// notLocal reports whether an error says a block is not on the node: what an
// offline read answers for a block it does not hold. A missing root block is a
// daemon error ("block was not found locally (offline)"); a file whose data
// blocks are missing fails in the middle of the stream ("failed to fetch all
// nodes"), so the text is read whatever the error type.
func notLocal(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	m := strings.ToLower(err.Error())
	for _, phrase := range []string{"not found", "could not find", "failed to fetch all nodes", "not found locally"} {
		if strings.Contains(m, phrase) {
			return true
		}
	}
	return false
}

// readLimit is how many bytes of the file at t.Path may be read. Text keeps the
// policy's max_bytes whether or not an extractor or describer is configured, as
// in G12. Only content with a binary signature (a PDF, an Office package, an
// image, a video), which only a stage can use, may be larger, up to that
// stage's limit; it is told apart by a small prefix, so a large log or data
// dump costs one small read, not tens of megabytes.
func (s *Server) readLimit(ctx context.Context, svc *embeddings.Service, t indexTarget, size int64) (int, error) {
	pol := svc.Policy()
	if size <= int64(pol.MaxBytes) || binaryReadMax(svc) <= pol.MaxBytes {
		return pol.MaxBytes, nil
	}
	var prefix []byte
	catCtx, cancel := context.WithTimeout(ctx, rpcReadTimeout)
	defer cancel()
	err := s.rawCall(catCtx, "cat", []string{t.Path}, t.rpcOptions(map[string]any{"length": int64(sniffPrefixBytes)}), nil, func(r io.Reader) error {
		var err error
		prefix, err = readBounded(r, sniffPrefixBytes)
		return err
	})
	if err != nil {
		return 0, err
	}
	mime := sniffBinary(prefix)
	if mime == "" {
		return pol.MaxBytes, nil
	}
	return max(stageLimit(svc, kindOfMIME(mime)), pol.MaxBytes), nil
}

// binaryReadMax is the largest file any configured stage accepts.
func binaryReadMax(svc *embeddings.Service) int {
	m := 0
	if ext := svc.Extractor(); ext != nil {
		m = ext.MaxBytes()
	}
	if d := svc.Describer(); d.Enabled() {
		m = max(m, d.MaxBytes(embeddings.KindImage), d.MaxBytes(embeddings.KindVideo))
	}
	return m
}

// stageLimit is how many bytes the stages of a kind accept.
func stageLimit(svc *embeddings.Service, kind string) int {
	m := 0
	if ext := svc.Extractor(); ext != nil {
		m = ext.MaxBytes()
	}
	if d := svc.Describer(); (kind == kindImage || kind == kindVideo) && d.Describes(kind) {
		m = max(m, d.MaxBytes(kind))
	}
	return m
}

// indexFile routes the file's bytes by kind and indexes what the stages of that
// kind produce. With neither an extractor nor a describer it is the G12 path,
// except for an allowlisted image or video, whose only possible stage is the
// missing describer.
func (s *Server) indexFile(ctx context.Context, svc *embeddings.Service, covers func(string) bool, t indexTarget, data []byte) (indexCIDOutput, error) {
	ext, desc := svc.Extractor(), svc.Describer()
	mime, kind := classifyContent(t.Name, data)
	media := kind == kindImage || kind == kindVideo
	pol := svc.Policy()
	switch {
	case ext == nil && desc == nil:
		if media && pol.AllowsMIME(mime) {
			return skipped(t.CID, embeddings.SkipDescriberNotConfigured), nil
		}
		return s.indexNative(ctx, svc, covers, t, data)
	case kind == "" || kind == kindText, !media && ext == nil:
		return s.indexNative(ctx, svc, covers, t, data)
	case !pol.AllowsMIME(mime):
		if looksLikeText(data) && sniffBinary(data) == "" {
			// Text the allowlist does not list under its refined type (a JSON
			// or CSV file) is still plain text, as it was before the extractor.
			// A file with a binary signature is not, even if its bytes happen
			// to be ASCII.
			return s.indexNative(ctx, svc, covers, t, data)
		}
		return skipped(t.CID, embeddings.SkipKindNotEnabled), nil
	case media:
		return s.indexMedia(ctx, svc, covers, t, mime, kind, data)
	}
	return s.indexDocument(ctx, svc, covers, t, mime, kind, data)
}

// indexNative indexes UTF-8 text the policy allows by chunking its bytes: the
// G12 path. The chunk text is not stored; a search re-reads it.
func (s *Server) indexNative(ctx context.Context, svc *embeddings.Service, covers func(string) bool, t indexTarget, data []byte) (indexCIDOutput, error) {
	pol := svc.Policy()
	mime, skip := pol.CheckContent(t.Name, data)
	if skip != "" {
		return skipped(t.CID, skip), nil
	}
	chunks := embeddings.Chunk(string(data), pol.ChunkChars)
	if len(chunks) == 0 {
		return skipped(t.CID, embeddings.SkipEmpty), nil // blank content has nothing to embed
	}
	n, err := svc.Index(ctx, covers, embeddings.Document{CID: t.CID, Path: t.Path, MIME: mime, Chunks: chunks, Kind: kindText, Source: embeddings.SourceText})
	if err != nil {
		return indexCIDOutput{}, err
	}
	return indexCIDOutput{CID: t.CID, ChunksIndexed: n, Model: svc.Embedder().Model()}, nil
}

// stageFailure is one stage that produced no text: either a reason to skip the
// file or a hard error that stops it.
type stageFailure struct {
	stage string
	// skip is the skipped_reason when the stage's failure only skips the file.
	skip string
	// reason is the fixed code a warning carries; the error's own text can hold
	// what the endpoint said and stays in the log.
	reason string
	// err is the failure; hard means the call fails with it instead of skipping.
	err  error
	hard bool
}

// Fixed reason codes of a stage warning that are not skipped_reason values.
const (
	warnEgressNotAcknowledged = "egress_not_acknowledged"
	warnRateLimited           = "rate_limited"
	warnFailed                = "failed"
)

// message is the stage warning: the stage and the fixed reason code, never text
// that came from the endpoint.
func (f stageFailure) message() string { return f.stage + ":" + cmp.Or(f.skip, f.reason, warnFailed) }

// extractStage sends the file to the extractor and maps its failures. A refused
// egress is a hard failure: the calling identity cannot use this host at all.
func (s *Server) extractStage(ctx context.Context, ext *embeddings.Extractor, covers func(string) bool, t indexTarget, mime, kind string, data []byte, ocr bool) (embeddings.Extraction, *stageFailure) {
	res, err := ext.Extract(ctx, covers, embeddings.ExtractRequest{Filename: "content" + extensionFor(mime), MIME: mime, Data: data, OCR: ocr})
	switch {
	case err == nil:
		return res, nil
	case errors.Is(err, embeddings.ErrEgressNotAcknowledged):
		return res, &stageFailure{stage: "extractor", reason: warnEgressNotAcknowledged, err: err, hard: true}
	case errors.Is(err, embeddings.ErrExtractTooLarge), errors.Is(err, embeddings.ErrExtractedTextTooLarge):
		return res, &stageFailure{stage: "extractor", skip: embeddings.SkipTooLarge, err: err}
	case errors.Is(err, embeddings.ErrExtractorUnavailable):
		s.log.Warn("index: extractor unavailable", "cid", t.CID, "kind", kind, "error", clipMessage(err.Error()))
		return res, &stageFailure{stage: "extractor", skip: embeddings.SkipExtractorUnavailable, err: err}
	case errors.Is(err, embeddings.ErrExtractionFailed):
		s.log.Warn("index: extraction failed", "cid", t.CID, "kind", kind, "error", clipMessage(err.Error()))
		return res, &stageFailure{stage: "extractor", skip: embeddings.SkipExtractionFailed, err: err}
	}
	return res, &stageFailure{stage: "extractor", err: err, hard: true}
}

// describeStage sends an image or video to the describer and maps its
// failures. A request refused by the hourly limit is a hard failure so a retry
// can complete the file; indexing without the description would leave it out
// for good, since the file then counts as indexed.
func (s *Server) describeStage(ctx context.Context, desc *embeddings.Describer, covers func(string) bool, t indexTarget, mime, kind string, data []byte) (embeddings.Description, *stageFailure) {
	res, err := desc.Describe(ctx, covers, embeddings.DescribeRequest{Kind: kind, MIME: mime, Data: data})
	switch {
	case err == nil:
		return res, nil
	case errors.Is(err, embeddings.ErrEgressNotAcknowledged):
		return res, &stageFailure{stage: "describer", reason: warnEgressNotAcknowledged, err: err, hard: true}
	case errors.Is(err, embeddings.ErrDescribeRateLimited):
		return res, &stageFailure{stage: "describer", reason: warnRateLimited, err: err, hard: true}
	case errors.Is(err, embeddings.ErrDescriberDisabled):
		return res, &stageFailure{stage: "describer", skip: embeddings.SkipDescriberNotConfigured, err: err}
	case errors.Is(err, embeddings.ErrDescribeTooLarge):
		return res, &stageFailure{stage: "describer", skip: embeddings.SkipTooLarge, err: err}
	case errors.Is(err, embeddings.ErrDescriberUnavailable), errors.Is(err, embeddings.ErrDescribeFailed):
		// The contract has no describer-specific reason: extraction_failed is
		// the nearest, "no stage could produce text for this file".
		s.log.Warn("index: description failed", "cid", t.CID, "kind", kind, "error", clipMessage(err.Error()))
		return res, &stageFailure{stage: "describer", skip: embeddings.SkipExtractionFailed, err: err}
	}
	return res, &stageFailure{stage: "describer", err: err, hard: true}
}

// extractMedia is the extractor stage of an image or video, once per file: a
// bulk run that retries the file after the describer's hourly limit reuses the
// text the first attempt got.
func (s *Server) extractMedia(ctx context.Context, ext *embeddings.Extractor, covers func(string) bool, t indexTarget, mime, kind string, data []byte) (embeddings.Extraction, *stageFailure) {
	if t.memo != nil && t.memo.ext != nil {
		return *t.memo.ext, nil
	}
	res, f := s.extractStage(ctx, ext, covers, t, mime, kind, data, kind == kindImage)
	if f == nil && t.memo != nil {
		t.memo.ext = &res
	}
	return res, f
}

// indexMedia runs the stages of an image or a video: OCR or transcription
// through the extractor when on, and the describer when it covers the kind. The
// stages are independent: one that fails or is refused is a warning on the file
// as long as another produced text, and all produced text is stored in one
// replacement, so a later stage never erases an earlier one.
func (s *Server) indexMedia(ctx context.Context, svc *embeddings.Service, covers func(string) bool, t indexTarget, mime, kind string, data []byte) (indexCIDOutput, error) {
	ext, desc := svc.Extractor(), svc.Describer()
	pol := svc.Policy()
	var segs []embeddings.Segment
	var fails []stageFailure
	ran := false
	if ext != nil && (kind == kindImage && ext.OCR() || kind == kindVideo && ext.Transcribe()) {
		ran = true
		source := embeddings.SourceOCR
		if kind == kindVideo {
			source = embeddings.SourceTranscript
		}
		res, f := s.extractMedia(ctx, ext, covers, t, mime, kind, data)
		if f != nil {
			fails = append(fails, *f)
		} else if chunks := embeddings.Chunk(res.Markdown, pol.ChunkChars); len(chunks) > 0 {
			segs = append(segs, embeddings.Segment{Chunks: chunks, Kind: kind, Source: source})
		}
	}
	if desc.Describes(kind) {
		ran = true
		res, f := s.describeStage(ctx, desc, covers, t, mime, kind, data)
		if f != nil {
			fails = append(fails, *f)
		} else if chunks := embeddings.Chunk(res.Text, pol.ChunkChars); len(chunks) > 0 {
			segs = append(segs, embeddings.Segment{Chunks: chunks, Kind: kind, Source: embeddings.SourceDescription, DescribedBy: res.Model})
		}
	}
	// A hard failure stops the file: a used-up hourly limit is retried, not
	// indexed around. A stage that was refused for egress is only a warning
	// while another stage produced text; with none, the refusal is the answer,
	// naming the host.
	for _, f := range fails {
		if f.hard && !errors.Is(f.err, embeddings.ErrEgressNotAcknowledged) {
			return indexCIDOutput{}, f.err
		}
	}
	if len(segs) == 0 {
		for _, f := range fails {
			if f.hard {
				return indexCIDOutput{}, f.err
			}
		}
		return s.noMediaText(t, ran, fails), nil
	}
	total := 0
	for _, seg := range segs {
		total += len(seg.Chunks)
	}
	if total > maxDerivedChunks {
		// Embedding it would cost more requests than the operator confirmed one
		// file for; nothing is truncated silently.
		s.log.Warn("index: derived text yields too many chunks", "cid", t.CID, "kind", kind, "chunks", total, "limit", maxDerivedChunks)
		return skipped(t.CID, embeddings.SkipTooLarge), nil
	}
	n, err := svc.Index(ctx, covers, embeddings.Document{CID: t.CID, Path: t.Path, MIME: mime, Segments: segs})
	if err != nil {
		return indexCIDOutput{}, err
	}
	out := indexCIDOutput{CID: t.CID, ChunksIndexed: n, Model: svc.Embedder().Model()}
	for _, f := range fails {
		s.log.Warn("index: a stage failed while another produced text", "cid", t.CID, "kind", kind, "stage", f.stage, "error", clipMessage(f.err.Error()))
		out.warnings = append(out.warnings, f.message())
	}
	return out, nil
}

// noMediaText is the result for an image or video that no stage produced text
// for: the reason of the first stage that failed or, when no stage covers the
// kind, describer_not_configured (the allowlist already admitted the type, so
// the missing piece is the stage that would read it).
func (s *Server) noMediaText(t indexTarget, ran bool, fails []stageFailure) indexCIDOutput {
	switch {
	case len(fails) > 0:
		out := skipped(t.CID, fails[0].skip)
		for _, f := range fails {
			out.warnings = append(out.warnings, f.message())
		}
		return out
	case ran:
		// A stage ran and found no text (an image with no words, a blank answer).
		return skipped(t.CID, embeddings.SkipEmpty)
	}
	return skipped(t.CID, embeddings.SkipDescriberNotConfigured)
}

// indexDocument routes content by kind to the extractor: documents, code and
// data. The returned Markdown is chunked with the same chunker and stored with
// its source, kind and text.
func (s *Server) indexDocument(ctx context.Context, svc *embeddings.Service, covers func(string) bool, t indexTarget, mime, kind string, data []byte) (indexCIDOutput, error) {
	ext := svc.Extractor()
	ocr := kind == kindDocument && ext.OCR()
	res, f := s.extractStage(ctx, ext, covers, t, mime, kind, data, ocr)
	if f != nil {
		if f.hard {
			return indexCIDOutput{}, f.err
		}
		return skipped(t.CID, f.skip), nil
	}
	source := embeddings.SourceExtracted
	if kind == kindDocument && res.OCRUsed {
		source = embeddings.SourceOCR
	}
	chunks := embeddings.Chunk(res.Markdown, svc.Policy().ChunkChars)
	if len(chunks) == 0 {
		return skipped(t.CID, embeddings.SkipEmpty), nil
	}
	if len(chunks) > maxDerivedChunks {
		// Embedding it would cost more requests than the operator confirmed one
		// document for; nothing is truncated silently.
		s.log.Warn("index: extracted text yields too many chunks", "cid", t.CID, "kind", kind, "chunks", len(chunks), "limit", maxDerivedChunks)
		return skipped(t.CID, embeddings.SkipTooLarge), nil
	}
	n, err := svc.Index(ctx, covers, embeddings.Document{CID: t.CID, Path: t.Path, MIME: mime, Chunks: chunks, Kind: kind, Source: source})
	if err != nil {
		return indexCIDOutput{}, err
	}
	return indexCIDOutput{CID: t.CID, ChunksIndexed: n, Model: svc.Embedder().Model()}, nil
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
// names the hosts; a used-up describer limit is resource_exhausted; anything
// else that is not a cancellation is index_unavailable, which is retryable.
func indexErr(err error) error {
	var eg *embeddings.EgressError
	var te *ToolError
	switch {
	case errors.As(err, &te):
		return err
	case errors.As(err, &eg):
		return &ToolError{
			Code: CodeEgressNotAcknowledged,
			Message: "indexing and searching send text to " + strings.Join(eg.Hosts, ", ") + ", which is not in the egress_ack of the calling identity; " +
				"use a principal whose egress_ack covers the embedder (and a remote store), or \"local\" for loopback" + settingHint(eg),
			Details: map[string]any{"reason": ReasonBackendHostNotAcked, "missing_hosts": eg.Hosts},
		}
	case errors.Is(err, embeddings.ErrDescribeRateLimited):
		return &ToolError{Code: CodeResourceExhausted, Message: "embeddings.describer.max_requests_per_hour is reached; try again later", Retryable: true}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	}
	return &ToolError{Code: CodeIndexUnavailable, Message: clipMessage(err.Error()), Retryable: true}
}
