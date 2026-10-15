package embeddings

import (
	"net/http"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Policy defaults, the contract's defaults of embeddings.policy.
const (
	// DefaultMaxBytes is the largest content that is indexed.
	DefaultMaxBytes = 1 << 20
	// DefaultChunkChars is the longest chunk, in characters.
	DefaultChunkChars = 2000
	// MaxMaxBytes and the chunk bounds are the contract's limits.
	MaxMaxBytes   = 100 << 20
	MinChunkChars = 200
	MaxChunkChars = 20000
)

// DefaultMIMEAllowlist is the content types indexed when the operator names
// none.
var DefaultMIMEAllowlist = []string{"text/plain", "text/markdown"}

// Policy says what is indexed. The zero value indexes nothing automatically
// and uses the defaults for everything else; Effective fills them in.
type Policy struct {
	// Auto indexes content after a successful ipfs_add (embeddings.policy.auto).
	// It is off by default: only ipfs_index_cid indexes then.
	Auto bool
	// MIMEAllowlist lists the content types that are indexed. Nil means
	// DefaultMIMEAllowlist; an empty list means nothing is indexed.
	MIMEAllowlist []string
	// MaxBytes is the largest content that is indexed; 0 means DefaultMaxBytes.
	MaxBytes int
	// ChunkChars is the longest chunk in characters; 0 means DefaultChunkChars.
	ChunkChars int
}

// Effective returns the policy with defaults filled in.
func (p Policy) Effective() Policy {
	if p.MIMEAllowlist == nil {
		p.MIMEAllowlist = slices.Clone(DefaultMIMEAllowlist)
	}
	if p.MaxBytes == 0 {
		p.MaxBytes = DefaultMaxBytes
	}
	if p.ChunkChars == 0 {
		p.ChunkChars = DefaultChunkChars
	}
	return p
}

// AllowsMIME reports whether the content type (parameters ignored) is on the
// allowlist.
func (p Policy) AllowsMIME(mime string) bool {
	mime = baseMIME(mime)
	return slices.ContainsFunc(p.MIMEAllowlist, func(a string) bool { return baseMIME(a) == mime })
}

func baseMIME(m string) string {
	m, _, _ = strings.Cut(m, ";")
	return strings.ToLower(strings.TrimSpace(m))
}

// Skip reasons of CheckContent, the contract's skipped_reason values.
const (
	SkipMIME     = "mime_not_allowed"
	SkipTooLarge = "too_large"
	SkipNotAFile = "not_a_file"
	SkipEmpty    = "empty"
	// SkipKindNotEnabled: the content's kind is handled by a stage that is off,
	// or its type is not in the allowlist although an extractor is configured.
	SkipKindNotEnabled = "kind_not_enabled"
	// SkipExtractorUnavailable: the extractor could not be reached.
	SkipExtractorUnavailable = "extractor_unavailable"
	// SkipExtractionFailed: the extractor answered without usable text.
	SkipExtractionFailed = "extraction_failed"
	// SkipAlreadyIndexed: the CID has index entries and force was false.
	SkipAlreadyIndexed = "already_indexed"
	// SkipNotLocal: the file's blocks are not on the node; indexing never
	// fetches them.
	SkipNotLocal = "not_local"
	// SkipDescriberNotConfigured: the kind is covered only by the describer and
	// no describer block exists, its kinds omit the kind, or its limit is 0.
	SkipDescriberNotConfigured = "describer_not_configured"
)

// CheckContent decides whether content is indexed under the policy. It returns
// the content type and a skip reason, which is empty when the content passes.
// The type comes from the file name's extension when it has a known one, else
// from the bytes; content that is not UTF-8 text is never indexed.
func (p Policy) CheckContent(name string, data []byte) (mime, skip string) {
	if len(data) > p.MaxBytes {
		return "", SkipTooLarge
	}
	mime = DetectMIME(name, data)
	if mime == "" || !p.AllowsMIME(mime) {
		return mime, SkipMIME
	}
	return mime, ""
}

// DetectMIME names the content type of text content, or "" when the bytes are
// not UTF-8 text. A .md, .markdown or .txt extension decides; anything else is
// sniffed (plain text is text/plain, markup such as HTML keeps its own type).
func DetectMIME(name string, data []byte) string {
	if !utf8.Valid(data) {
		return ""
	}
	if slices.Contains(data, 0) {
		return ""
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	}
	sniffed := baseMIME(http.DetectContentType(data))
	if !strings.HasPrefix(sniffed, "text/") {
		return ""
	}
	return sniffed
}

// Chunk splits text into pieces of at most size characters, cutting at a
// paragraph break, else a line break, else a space when one lies in the second
// half of the piece. Pieces are trimmed and empty ones dropped. The result is
// a pure function of text and size: a search re-splits the content of a CID
// with the same size to recover the text of a chunk by its index.
func Chunk(text string, size int) []string {
	if size < 1 {
		size = DefaultChunkChars
	}
	runes := []rune(text)
	chunks := make([]string, 0, len(runes)/size+1)
	for pos := 0; pos < len(runes); {
		end := min(pos+size, len(runes))
		cut := end
		if end < len(runes) {
			cut = cutPoint(runes, pos, end)
		}
		if piece := strings.TrimSpace(string(runes[pos:cut])); piece != "" {
			chunks = append(chunks, piece)
		}
		pos = cut
	}
	return chunks
}

// cutPoint picks where the piece that starts at pos and may reach end ends.
func cutPoint(runes []rune, pos, end int) int {
	floor := pos + (end-pos)/2
	for i := end - 1; i > floor; i-- {
		if runes[i] == '\n' && runes[i-1] == '\n' {
			return i + 1
		}
	}
	for i := end - 1; i > floor; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}
	for i := end - 1; i > floor; i-- {
		if unicode.IsSpace(runes[i]) {
			return i + 1
		}
	}
	return end
}
