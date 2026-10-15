package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ipfs/go-cid"

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
	// rpcReadTimeout bounds one cat of the index path, so a read that stalls ends
	// instead of holding a bulk run or a tool call.
	rpcReadTimeout = 2 * time.Minute
	// searchReadBudget bounds the bytes one ipfs_index_search call reads back
	// from the node to cut chunk text, whatever top_k and policy.max_bytes are.
	searchReadBudget = 8 << 20
)

func indexTools(s *Server) []toolImpl {
	return append([]toolImpl{
		{Name: "ipfs_index_search", Call: s.indexSearch},
		{Name: "ipfs_index_cid", Call: s.indexCID},
		{Name: "ipfs_index_status", Call: s.indexStatus},
		{Name: "ipfs_index_reconcile", Call: s.indexReconcile},
	}, bulkImpls(s)...)
}

// attachEmbeddings makes the embeddings service the server's and offers the
// index tools that were waiting for it. Until it runs, the index tools are
// unavailable with the reason not_configured. It must run before the server
// is used by a transport or the agent loop, which read the tool set.
func (s *Server) attachEmbeddings(svc *embeddings.Service) {
	s.embeddingsExt = svc
	// A bulk start the calling principal cannot make is refused before anyone is
	// asked to confirm it, and its confirmation names the run's real limits and
	// hosts.
	s.preflights = map[string]func(context.Context, json.RawMessage) error{"ipfs_index_bulk_start": s.bulkPreflight}
	s.gate.summaryArgs = s.summaryArgs
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
	// ByKind counts distinct indexed files per content kind.
	ByKind map[string]int `json:"by_kind,omitempty"`
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
	byKind, err := svc.Store().CountCIDsByKind(ctx)
	if err != nil {
		return nil, indexErr(fmt.Errorf("index status: %w", err))
	}
	out.ByKind = byKind
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
	// Kinds and Sources keep only hits of those content kinds and chunk
	// sources; empty means every one.
	Kinds   []string `json:"kinds"`
	Sources []string `json:"sources"`
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
	// DescribedBy is the describer model; present only when Source is
	// "description", whose text is untrusted model output.
	DescribedBy string `json:"described_by,omitempty"`
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
	hits, err := svc.SearchFiltered(ctx, covers, in.Query, in.CID, k, embeddings.Filter{Kinds: in.Kinds, Sources: in.Sources})
	if err != nil {
		return nil, indexErr(err)
	}
	out := indexSearchOutput{Model: svc.Embedder().Model(), Results: make([]indexHit, 0, len(hits))}
	pol := svc.Policy()
	texts := map[string][]string{}
	present := map[string]bool{}
	// Native chunks are cut from a prefix of the file: the last chunk any hit
	// of a source needs says how much of it to read.
	deepest := map[string]int{}
	for _, h := range hits {
		if !h.Derived() {
			src := indexSource(h)
			deepest[src] = max(deepest[src], h.ChunkIndex)
		}
	}
	budget := searchReadBudget
	for _, h := range hits {
		if h.Derived() {
			// Derived text (extracted, OCR, transcript) is not a slice of the
			// file's bytes; it was stored with the vector, so nothing is read.
			// It would outlive the content, though, so a hit is returned only
			// while the node still holds the file.
			src := indexSource(h)
			ok, seen := present[src]
			if !seen {
				ok = src != "" && s.contentPresent(ctx, src)
				present[src] = ok
			}
			if !ok {
				continue
			}
			out.Results = append(out.Results, indexHit{
				CID: h.CID, Path: h.Path, MIME: h.MIME, ChunkIndex: h.ChunkIndex,
				Score: 1 - h.Distance, Text: h.Text, Kind: kindOrText(h.Kind), Source: sourceOrText(h.Source), MIMEType: h.MIME,
				DescribedBy: describedBy(h),
			})
			continue
		}
		src := indexSource(h)
		chunks, ok := texts[src]
		if !ok {
			if src != "" {
				chunks = s.readChunks(ctx, src, pol, deepest[src], &budget)
			}
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

// describedBy names the describer model of a description hit, and nothing for
// any other source: the field's presence says the text is model output.
func describedBy(h embeddings.Hit) string {
	if h.Source != embeddings.SourceDescription {
		return ""
	}
	return cmpOr(h.DescribedBy, "unknown")
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

// readChunks reads the start of the content at path from the node and cuts it
// with the policy that indexed it, returning at least the chunks up to index
// upto. It returns nil when the content cannot be read, or when reading it
// would take the call past budget, the bytes one search may still read. The
// read is offline: a hit whose blocks were garbage collected is dropped at once
// instead of fetched from the network.
//
// Chunk is prefix-stable: a chunk is decided by the text up to the end of its
// window, so reading the characters of chunks 0..upto plus one more window
// (at most four bytes each) gives the same chunks 0..upto as reading the whole
// file. That keeps the memory of one search at the budget, not at top_k times
// policy.max_bytes.
func (s *Server) readChunks(ctx context.Context, path string, pol embeddings.Policy, upto int, budget *int) []string {
	ctx, cancel := context.WithTimeout(ctx, chunkReadTimeout)
	defer cancel()
	want := int64(pol.MaxBytes) + 1
	if need := (int64(upto) + 2) * int64(pol.ChunkChars) * utf8.UTFMax; need < want {
		want = need
	}
	if want > int64(*budget) {
		s.log.Debug("index search: read budget exhausted, hit dropped", "path", path)
		return nil
	}
	*budget -= int(want)
	var data []byte
	err := s.rawCall(ctx, "cat", []string{path}, map[string]any{"length": want, "offline": true}, nil, func(r io.Reader) error {
		var err error
		data, err = readBounded(r, want)
		return err
	})
	if err != nil || len(data) > pol.MaxBytes {
		s.log.Debug("index search: cannot read content", "path", path, "error", err)
		return nil
	}
	return embeddings.Chunk(string(data), pol.ChunkChars)
}

// indexSource is the path a hit's content is read from. The stored path is not
// trusted: a store that could be steered (a compromised remote one) must not
// make the node read an arbitrary /ipfs or /ipns path, so the path is rebuilt
// from the parsed CID, keeping only the in-DAG file name that follows it.
func indexSource(h embeddings.Hit) string {
	return safeIndexPath(h.Path, h.CID)
}

// safeIndexPath builds "/ipfs/<cid>", or "/ipfs/<cid>/<name>" when the stored
// path is that CID followed by one plain file name. Anything else (another
// CID, an /ipns path, a name with a separator or a control character) is
// replaced by "/ipfs/<cid>", and a CID that does not parse gives "": the hit is
// then unreadable and dropped.
func safeIndexPath(stored, c string) string {
	parsed, err := cid.Decode(c)
	if err != nil {
		return ""
	}
	base := "/ipfs/" + parsed.String()
	rest, ok := strings.CutPrefix(stored, "/ipfs/")
	if !ok {
		return base
	}
	root, name, hasName := strings.Cut(rest, "/")
	rootCID, err := cid.Decode(root)
	if err != nil || !hasName || !rootCID.Equals(parsed) || !safeEntryName(name) {
		return base
	}
	return base + "/" + name
}

func notConfigured() error {
	return &ToolError{Code: CodeToolUnavailable, Message: "embeddings are not configured: add an embeddings section to the ipfs mcp config"}
}
