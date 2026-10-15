package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kindStore is an in-memory embeddings.Store that keeps the kind, source and
// text of a row, as the SurrealDB store does.
type kindStore struct {
	mu   sync.Mutex
	rows []embeddings.Row
	memRuns
}

func (k *kindStore) Bootstrap(context.Context) error { return nil }

func (k *kindStore) Replace(_ context.Context, cid string, rows []embeddings.Row) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.rows = slices.DeleteFunc(k.rows, func(r embeddings.Row) bool { return r.CID == cid })
	k.rows = append(k.rows, rows...)
	return nil
}

func (k *kindStore) hit(r embeddings.Row, v []float32) embeddings.Hit {
	return embeddings.Hit{
		CID: r.CID, Path: r.Path, MIME: r.MIME, ChunkIndex: r.ChunkIndex, Model: r.Model,
		Distance: cosineDistance(r.Vector, v), Kind: r.Kind, Source: r.Source, Text: r.Text, DescribedBy: r.DescribedBy,
	}
}

func (k *kindStore) nearest(v []float32, n int, keep func(embeddings.Row) bool) []embeddings.Hit {
	k.mu.Lock()
	defer k.mu.Unlock()
	var hits []embeddings.Hit
	for _, r := range k.rows {
		if keep(r) {
			hits = append(hits, k.hit(r, v))
		}
	}
	slices.SortStableFunc(hits, func(a, b embeddings.Hit) int {
		switch {
		case a.Distance < b.Distance:
			return -1
		case a.Distance > b.Distance:
			return 1
		}
		return 0
	})
	return hits[:min(n, len(hits))]
}

func (k *kindStore) Search(_ context.Context, v []float32, n int) ([]embeddings.Hit, error) {
	return k.nearest(v, n, func(embeddings.Row) bool { return true }), nil
}

func (k *kindStore) SearchCID(_ context.Context, v []float32, cid string, n int) ([]embeddings.Hit, error) {
	return k.nearest(v, n, func(r embeddings.Row) bool { return r.CID == cid }), nil
}

func (k *kindStore) DeleteCID(_ context.Context, cid string) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	before := len(k.rows)
	k.rows = slices.DeleteFunc(k.rows, func(r embeddings.Row) bool { return r.CID == cid })
	return before - len(k.rows), nil
}

func (k *kindStore) Count(context.Context) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.rows), nil
}

func (k *kindStore) CountCID(_ context.Context, cid string) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	n := 0
	for _, r := range k.rows {
		if r.CID == cid {
			n++
		}
	}
	return n, nil
}

func (k *kindStore) CountCIDs(context.Context) (int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	seen := map[string]struct{}{}
	for _, r := range k.rows {
		seen[r.CID] = struct{}{}
	}
	return len(seen), nil
}

func (k *kindStore) ListCIDs(_ context.Context, after string, limit int) ([]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	set := map[string]struct{}{}
	for _, r := range k.rows {
		if r.CID > after {
			set[r.CID] = struct{}{}
		}
	}
	cids := slices.Sorted(maps.Keys(set))
	return cids[:min(limit, len(cids))], nil
}

func (k *kindStore) snapshot() []embeddings.Row {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Clone(k.rows)
}

// extractorFake is an xberg stand-in that records what it receives. It
// answers every file with the Markdown reply says.
type extractorFake struct {
	*httptest.Server
	mu    sync.Mutex
	seen  []extractedFile
	reply func(f extractedFile) (status int, body string)
}

type extractedFile struct {
	Filename string
	PartType string
	Fields   map[string]string
	Size     int
}

func newExtractorFake(t *testing.T) *extractorFake {
	t.Helper()
	x := &extractorFake{reply: func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":"cats extracted by the fake","mime_type":"x","metadata":{"ocr_used":false}}]}`
	}}
	x.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := extractedFile{Fields: map[string]string{}}
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err == nil {
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				b, _ := io.ReadAll(p)
				if p.FormName() == "files" {
					f.Filename, f.PartType, f.Size = p.FileName(), p.Header.Get("Content-Type"), len(b)
				} else {
					f.Fields[p.FormName()] = string(b)
				}
			}
		}
		x.mu.Lock()
		x.seen = append(x.seen, f)
		reply := x.reply
		x.mu.Unlock()
		status, body := reply(f)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(x.Close)
	return x
}

// setReply replaces the answer under the lock the handler reads it with.
func (x *extractorFake) setReply(f func(extractedFile) (int, string)) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.reply = f
}

func (x *extractorFake) requests() []extractedFile {
	x.mu.Lock()
	defer x.mu.Unlock()
	return slices.Clone(x.seen)
}

func (x *extractorFake) host() string { return x.Server.Listener.Addr().String() }

// extEnv is an index environment whose service has a remote extractor.
type extEnv struct {
	*indexEnv
	x     *extractorFake
	store *kindStore
}

func newExtEnv(t *testing.T, policy embeddings.Policy, mut func(*embeddings.ExtractorSettings)) *extEnv {
	t.Helper()
	e := newIndexEnv(t, indexEnvOptions{noEmbeddings: true})
	x := newExtractorFake(t)
	ks := &kindStore{}
	set := embeddings.Settings{
		Mode:     embeddings.ModeEmbedded,
		Embedder: embeddings.EmbedderSettings{BaseURL: e.embed.URL + "/v1", Model: "fake-embed", Dimensions: indexTestDims},
		Policy:   policy,
	}
	emb, err := embeddings.NewEmbedder(set.Embedder, nil)
	require.NoError(t, err)
	xs := embeddings.ExtractorSettings{Mode: embeddings.ModeRemote, Remote: &embeddings.ExtractorRemote{BaseURL: x.URL}}
	if mut != nil {
		mut(&xs)
	}
	ext, err := embeddings.NewExtractor(xs, nil)
	require.NoError(t, err)
	e.s.attachEmbeddings(embeddings.NewService(set, ks, emb).WithExtractor(ext))
	return &extEnv{indexEnv: e, x: x, store: ks}
}

// principal acknowledges the embedder and the extractor.
func (e *extEnv) principal() Identity { return indexIdentity(e.embed.host(), e.x.host()) }

func (e *extEnv) index(id Identity, cid string) map[string]any {
	e.t.Helper()
	res := toolCall(e.t, e.connect(&id), "ipfs_index_cid", map[string]any{"cid": cid})
	require.False(e.t, res.IsError, "%+v", res.Content)
	out := structured(e.t, res)
	validateOutput(e.t, "ipfs_index_cid", out)
	return out
}

// --- fixtures -----------------------------------------------------------------

var (
	pdfBytes = []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")
	pngBytes = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 32)...)
	mp4Bytes = append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, 32)...)
)

func zipOf(t *testing.T, entries ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte("<x/>"))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

const (
	docxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	docxExt  = ".docx"
)

// allKinds is an allowlist of one type per kind the tests exercise.
var allKinds = []string{
	"text/plain", "application/pdf", docxMIME, "text/x-python", "application/json", "text/csv", "image/png", "video/mp4",
}

// --- 1.3: one test per kind ---------------------------------------------------

func TestExtractorRoutesEachKind(t *testing.T) {
	tests := []struct {
		name       string
		content    func(*testing.T) []byte
		wantMIME   string
		wantKind   string
		wantSource string
		wantExt    string
		ocrUsed    bool
	}{
		{"document pdf", func(*testing.T) []byte { return pdfBytes }, "application/pdf", "document", "extracted", ".pdf", false},
		{"document docx", func(t *testing.T) []byte { return zipOf(t, "[Content_Types].xml", "word/document.xml") }, docxMIME, "document", "extracted", docxExt, false},
		{"scanned document", func(*testing.T) []byte { return pdfBytes }, "application/pdf", "document", "ocr", ".pdf", true},
		{"code", func(*testing.T) []byte { return []byte("#!/usr/bin/env python\nprint('cats')\n") }, "text/x-python", "code", "extracted", ".py", false},
		{"data json", func(*testing.T) []byte { return []byte(`{"animal": "cats", "n": 3}`) }, "application/json", "data", "extracted", ".json", false},
		{"data csv", func(*testing.T) []byte { return []byte("animal,n\ncats,3\ndogs,4\n") }, "text/csv", "data", "extracted", ".csv", false},
		{"image", func(*testing.T) []byte { return pngBytes }, "image/png", "image", "ocr", ".png", true},
		{"video", func(*testing.T) []byte { return mp4Bytes }, "video/mp4", "video", "transcript", ".mp4", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds, ChunkChars: 200}, func(s *embeddings.ExtractorSettings) {
				s.OCR, s.Transcribe = true, true
			})
			e.x.setReply(func(extractedFile) (int, string) {
				used := "false"
				if tt.ocrUsed {
					used = "true"
				}
				return 200, `{"results":[{"content":"cats are in this ` + tt.wantKind + `","mime_type":"x","metadata":{"ocr_used":` + used + `}}]}`
			})
			e.files["/ipfs/bafyDoc"] = tt.content(t)

			out := e.index(e.principal(), "bafyDoc")
			assert.EqualValues(t, 1, out["chunks_indexed"])
			assert.NotContains(t, out, "skipped_reason")

			reqs := e.x.requests()
			require.Len(t, reqs, 1)
			assert.Equal(t, "content"+tt.wantExt, reqs[0].Filename, "the extension tells xberg the format")
			assert.Equal(t, tt.wantMIME, reqs[0].PartType)
			assert.Equal(t, "markdown", reqs[0].Fields["output_format"])

			rows := e.store.snapshot()
			require.Len(t, rows, 1)
			r := rows[0]
			assert.Equal(t, tt.wantKind, r.Kind)
			assert.Equal(t, tt.wantSource, r.Source)
			assert.Equal(t, tt.wantMIME, r.MIME)
			assert.Equal(t, "cats are in this "+tt.wantKind, r.Text, "derived chunk text is stored with the vector")
			assert.Equal(t, "/ipfs/bafyDoc", r.Path)
		})
	}
}

func TestTextKindStaysOnTheNativePath(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{}, nil)
	e.files["/ipfs/bafyText"] = []byte("cats purr when content")
	out := e.index(e.principal(), "bafyText")
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Empty(t, e.x.requests(), "text never goes to the extractor")
	rows := e.store.snapshot()
	require.Len(t, rows, 1)
	assert.Equal(t, "text", rows[0].Kind)
	assert.Equal(t, "text", rows[0].Source)
	assert.Empty(t, rows[0].Text, "native chunk text is re-read, not stored")
}

func TestPlainTextOutsideTheRefinedAllowlistEntryStaysNative(t *testing.T) {
	// JSON text with the default allowlist (text/plain, text/markdown) was
	// indexed as plain text before the extractor existed, and still is.
	e := newExtEnv(t, embeddings.Policy{}, nil)
	e.files["/ipfs/bafyJSON"] = []byte(`{"animal": "cats"}`)
	out := e.index(e.principal(), "bafyJSON")
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Empty(t, e.x.requests())
	assert.Equal(t, "text", e.store.snapshot()[0].Kind)
}

func TestSearchReturnsStoredTextForDerivedEntriesWithoutReading(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds}, nil)
	id := e.principal()
	e.files["/ipfs/bafyPDF"] = pdfBytes
	e.files["/ipfs/bafyNote"] = []byte("dogs bark at the mailman")
	require.EqualValues(t, 1, e.index(id, "bafyPDF")["chunks_indexed"])
	require.EqualValues(t, 1, e.index(id, "bafyNote")["chunks_indexed"])

	// A derived hit needs no re-read of its text; only an offline one-byte
	// presence check of the file.
	cats := e.d.count("cat")
	res := toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "cats", "top_k": 1})
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	validateOutput(t, "ipfs_index_search", out)
	results := out["results"].([]any)
	require.NotEmpty(t, results)
	first := results[0].(map[string]any)
	assert.Equal(t, "bafyPDF", first["cid"])
	assert.Equal(t, "cats extracted by the fake", first["text"])
	assert.Equal(t, "document", first["kind"])
	assert.Equal(t, "extracted", first["source"])
	assert.Equal(t, "application/pdf", first["mime_type"])
	assert.Equal(t, cats+1, e.d.count("cat"), "search read one byte, offline, to confirm the file is still here")
	assert.True(t, e.catOffline["/ipfs/bafyPDF"], "the presence check never fetches from the network")
	lastCat := e.catCalls()[len(e.catCalls())-1]
	assert.EqualValues(t, 1, lastCat.length)

	// A native text hit is still recovered by re-reading the file.
	res = toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "dogs", "cid": "bafyNote"})
	require.False(t, res.IsError)
	hit := structured(t, res)["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "dogs bark at the mailman", hit["text"])
	assert.Equal(t, "text", hit["source"])
	assert.Greater(t, e.d.count("cat"), cats+1, "native entries keep re-reading the file")
}

// catRead is one cat the fake node saw.
type catRead struct {
	path    string
	length  int64
	offline bool
}

func (e *indexEnv) catCalls() []catRead {
	e.d.mu.Lock()
	defer e.d.mu.Unlock()
	var out []catRead
	for _, c := range e.d.log {
		if c.Command != "cat" {
			continue
		}
		n, _ := c.Opts["length"].(int64)
		off, _ := c.Opts["offline"].(bool)
		out = append(out, catRead{path: c.Args[0], length: n, offline: off})
	}
	return out
}

func TestDerivedHitIsDroppedWhenItsContentIsGone(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds}, nil)
	id := e.principal()
	e.files["/ipfs/bafyPDF"] = pdfBytes
	require.EqualValues(t, 1, e.index(id, "bafyPDF")["chunks_indexed"])

	search := func() []any {
		res := toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "cats", "top_k": 5})
		require.False(t, res.IsError, "%+v", res.Content)
		out := structured(t, res)
		validateOutput(t, "ipfs_index_search", out)
		return out["results"].([]any)
	}
	require.Len(t, search(), 1, "the file is on the node")

	// pin rm and repo gc by another client: the node no longer has the file.
	delete(e.files, "/ipfs/bafyPDF")
	assert.Empty(t, search(), "the stored text must not outlive the content")
	assert.Len(t, e.store.snapshot(), 1, "a search stays read-only; ipfs_index_reconcile removes the rows")
	for _, c := range e.catCalls() {
		if c.path == "/ipfs/bafyPDF" && c.length == 1 {
			assert.True(t, c.offline, "the check never fetches from the network")
		}
	}
}

func TestRowsWithoutKindOrSourceAreReportedAsText(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{}, nil)
	id := e.principal()
	e.files["/ipfs/bafyOld"] = []byte("cats written before derived content existed")
	// A row as an earlier release stored it: no kind, no source, no text.
	require.NoError(t, e.store.Replace(t.Context(), "bafyOld", []embeddings.Row{{CID: "bafyOld", Path: "/ipfs/bafyOld", MIME: "text/plain", ChunkIndex: 0, Model: "fake-embed", Vector: axisVector(0)}}))
	res := toolCall(t, e.connect(&id), "ipfs_index_search", map[string]any{"query": "cats", "top_k": 1})
	require.False(t, res.IsError, "%+v", res.Content)
	hit := structured(t, res)["results"].([]any)[0].(map[string]any)
	assert.Equal(t, "text", hit["kind"])
	assert.Equal(t, "text", hit["source"])
	assert.Equal(t, "text/plain", hit["mime_type"])
}

func axisVector(axis int) []float32 {
	v := make([]float32, indexTestDims)
	v[axis] = 1
	return v
}

// --- bounds on derived text and on what is read --------------------------------

func TestExtractedTextOverTheCapIsTooLarge(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds}, func(s *embeddings.ExtractorSettings) { s.MaxTextBytes = 64 })
	e.files["/ipfs/bafyPDF"] = pdfBytes
	e.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":"` + strings.Repeat("cats ", 20) + `","mime_type":"x","metadata":{}}]}`
	})
	out := e.index(e.principal(), "bafyPDF")
	assert.Equal(t, "too_large", out["skipped_reason"])
	assert.EqualValues(t, 0, out["chunks_indexed"])
	assert.Zero(t, e.embed.requests.Load(), "nothing was embedded")
	assert.Empty(t, e.store.snapshot())

	e.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":"` + strings.Repeat("c", 64) + `","mime_type":"x","metadata":{}}]}`
	})
	assert.EqualValues(t, 1, e.index(e.principal(), "bafyPDF")["chunks_indexed"], "text at the cap is indexed")
}

func TestExtractedTextWithTooManyChunksIsTooLarge(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds, ChunkChars: embeddings.MinChunkChars}, nil)
	e.files["/ipfs/bafyPDF"] = pdfBytes
	// About 2300 chunks of 200 characters, well under the 2 MiB text cap.
	text := strings.Repeat("cats purr softly. ", 25000)
	require.Less(t, len(text), embeddings.DefaultExtractorMaxTextBytes)
	e.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":"` + text + `","mime_type":"x","metadata":{}}]}`
	})
	out := e.index(e.principal(), "bafyPDF")
	assert.Equal(t, "too_large", out["skipped_reason"])
	assert.Zero(t, e.embed.requests.Load(), "no embedder request is spent on a document over the chunk cap")
	assert.Empty(t, e.store.snapshot())
}

func TestTextKeepsPolicyMaxBytesEvenWithAnExtractor(t *testing.T) {
	pol := embeddings.Policy{MIMEAllowlist: allKinds, MaxBytes: 1024}
	e := newExtEnv(t, pol, func(s *embeddings.ExtractorSettings) { s.MaxBytes = 8192 })
	e.files["/ipfs/bafyLog"] = []byte(strings.Repeat("cats in the log\n", 300))       // 4800 bytes of text
	e.files["/ipfs/bafyPDF"] = append(slices.Clone(pdfBytes), make([]byte, 3000)...)  // a binary signature
	e.files["/ipfs/bafyHuge"] = append(slices.Clone(pdfBytes), make([]byte, 9000)...) // over extractor.max_bytes
	e.files["/ipfs/bafySmall"] = []byte("cats nap")

	assert.Equal(t, "too_large", e.index(e.principal(), "bafyLog")["skipped_reason"], "text is bounded by policy.max_bytes")
	for _, c := range e.catCalls() {
		assert.LessOrEqual(t, c.length, int64(sniffPrefixBytes), "a large text file costs a small prefix read, not a full read: %+v", c)
	}

	assert.EqualValues(t, 1, e.index(e.principal(), "bafyPDF")["chunks_indexed"], "binary content may use extractor.max_bytes")
	reads := e.catCalls()
	assert.EqualValues(t, 8192+1, reads[len(reads)-1].length, "the full read is bounded by extractor.max_bytes")

	assert.Equal(t, "too_large", e.index(e.principal(), "bafyHuge")["skipped_reason"])
	assert.EqualValues(t, 1, e.index(e.principal(), "bafySmall")["chunks_indexed"])
}

// --- 1.6: disabled states -----------------------------------------------------

func TestWithoutAnExtractorAPDFIsMimeNotAllowedAsInG12(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{policy: embeddings.Policy{MIMEAllowlist: allKinds}})
	e.files["/ipfs/bafyPDF"] = pdfBytes
	id := indexIdentity(e.embed.host())
	res := toolCall(t, e.connect(&id), "ipfs_index_cid", map[string]any{"cid": "bafyPDF"})
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	assert.Equal(t, "mime_not_allowed", out["skipped_reason"])
	assert.EqualValues(t, 0, out["chunks_indexed"])
	assert.Nil(t, e.s.embeddingsExt.Extractor(), "no extractor exists")
	assert.Zero(t, e.embed.requests.Load())
}

func TestExtractorWithAnUnlistedTypeIsKindNotEnabled(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{}, func(s *embeddings.ExtractorSettings) { s.OCR, s.Transcribe = true, true })
	e.files["/ipfs/bafyPDF"] = pdfBytes
	e.files["/ipfs/bafyPNG"] = pngBytes
	e.files["/ipfs/bafyMP4"] = mp4Bytes
	for _, c := range []string{"bafyPDF", "bafyPNG", "bafyMP4"} {
		out := e.index(e.principal(), c)
		assert.Equal(t, "kind_not_enabled", out["skipped_reason"], c)
	}
	assert.Empty(t, e.x.requests(), "nothing was sent to the extractor")
	assert.Empty(t, e.store.snapshot())
}

func TestImagesAndVideoNeedTheirStageOn(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds}, nil) // ocr and transcribe off
	e.files["/ipfs/bafyPNG"] = pngBytes
	e.files["/ipfs/bafyMP4"] = mp4Bytes
	assert.Equal(t, "describer_not_configured", e.index(e.principal(), "bafyPNG")["skipped_reason"], "OCR is off and no describer covers images")
	assert.Equal(t, "describer_not_configured", e.index(e.principal(), "bafyMP4")["skipped_reason"], "transcription is off and no describer covers video")
	assert.Empty(t, e.x.requests())
}

func TestExtractorFailuresAreSkippedReasons(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds}, func(s *embeddings.ExtractorSettings) { s.MaxBytes = 4096 })
	e.files["/ipfs/bafyPDF"] = pdfBytes
	e.files["/ipfs/bafyBig"] = append(slices.Clone(pdfBytes), make([]byte, 5000)...)

	e.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[],"errors":[{"index":0,"code":1008,"error_type":"plugin","message":"Hugging Face offline mode is enabled"}]}`
	})
	assert.Equal(t, "extraction_failed", e.index(e.principal(), "bafyPDF")["skipped_reason"])

	e.x.setReply(func(extractedFile) (int, string) { return 200, `not json` })
	assert.Equal(t, "extraction_failed", e.index(e.principal(), "bafyPDF")["skipped_reason"])

	e.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":" \n","mime_type":"x","metadata":{}}]}`
	})
	assert.Equal(t, "empty", e.index(e.principal(), "bafyPDF")["skipped_reason"])

	before := len(e.x.requests())
	assert.Equal(t, "too_large", e.index(e.principal(), "bafyBig")["skipped_reason"])
	assert.Len(t, e.x.requests(), before, "a file over extractor.max_bytes costs no request")

	e.x.Close()
	assert.Equal(t, "extractor_unavailable", e.index(e.principal(), "bafyPDF")["skipped_reason"])
	assert.Empty(t, e.store.snapshot())
}

// --- 1.4: egress --------------------------------------------------------------

func TestRemoteExtractorHostMustBeAcknowledged(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds}, nil)
	e.files["/ipfs/bafyPDF"] = pdfBytes
	e.files["/ipfs/bafyText"] = []byte("cats purr")

	embedderOnly := indexIdentity(e.embed.host())
	res := toolCall(t, e.connect(&embedderOnly), "ipfs_index_cid", map[string]any{"cid": "bafyPDF"})
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeEgressNotAcknowledged, te.Code)
	assert.Contains(t, te.Message, e.x.host(), "the result names the extractor host")
	assert.Equal(t, []any{e.x.host()}, te.Details["missing_hosts"])
	assert.Contains(t, te.Message, "embeddings.extractor.remote.base_url", "the remedy names the setting that holds the missing host")
	assert.Empty(t, e.x.requests(), "the extractor received nothing")
	assert.Empty(t, e.store.snapshot())

	// Other stages are unaffected: text does not need the extractor.
	out := e.index(embedderOnly, "bafyText")
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Empty(t, e.x.requests())

	// The loopback sentinel covers a loopback extractor.
	local := indexIdentity("local")
	assert.EqualValues(t, 1, e.index(local, "bafyPDF")["chunks_indexed"])
	assert.Len(t, e.x.requests(), 1)
}

func TestRemoteExtractorHostJoinsThePrincipalBackendHosts(t *testing.T) {
	var cfg Config
	require.NoError(t, json.Unmarshal([]byte(`{"embeddings":{"mode":"remote",
		"embedder":{"base_url":"http://127.0.0.1:11434/v1","model":"m","dimensions":4},
		"extractor":{"mode":"remote","remote":{"base_url":"https://xberg.example:8443"}}}}`), &cfg))
	assert.Contains(t, cfg.BackendHosts(), "xberg.example:8443")

	var embedded Config
	require.NoError(t, json.Unmarshal([]byte(`{"embeddings":{"mode":"remote",
		"embedder":{"base_url":"http://127.0.0.1:11434/v1","model":"m","dimensions":4},
		"extractor":{"mode":"embedded","embedded":{"xberg_binary":"/opt/xberg","cache_dir":"/var/xberg"}}}}`), &embedded))
	assert.Equal(t, []string{"127.0.0.1:11434"}, embedded.BackendHosts(), "an embedded xberg is a local child, not a host")
}

// --- 1.7: no CID injection ----------------------------------------------------

func TestExtractionAddsPinsAndAnnouncesNothing(t *testing.T) {
	e := newExtEnv(t, embeddings.Policy{MIMEAllowlist: allKinds}, nil)
	// xberg returns an image extracted from the document, as it would with
	// image extraction on; ipfs mcp must discard it.
	e.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":"cats and a picture","mime_type":"x","metadata":{},
			"images":[{"data":[137,80,78,71],"format":"png","image_index":0}]}]}`
	})
	e.files["/ipfs/bafyDocx"] = zipOf(t, "[Content_Types].xml", "word/document.xml", "word/media/image1.png")
	out := e.index(e.principal(), "bafyDocx")
	assert.EqualValues(t, 1, out["chunks_indexed"])

	banned := []string{"add", "block/put", "dag/put", "pin/add", "pin/update", "files/write", "files/cp", "routing/provide", "provide/once"}
	e.d.mu.Lock()
	defer e.d.mu.Unlock()
	for _, c := range e.d.log {
		assert.NotContains(t, banned, c.Command, "indexing a DOCX with an image must not call %s", c.Command)
	}
	assert.NotEmpty(t, e.d.log, "the file was read")
	assert.Equal(t, "cats and a picture", e.store.snapshot()[0].Text)
}
