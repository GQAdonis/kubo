package embeddings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeXberg is an httptest stand-in for `xberg serve`: it records every
// /extract request and answers with the next prepared body.
type fakeXberg struct {
	*httptest.Server
	mu       sync.Mutex
	requests []fakeExtractRequest
	status   int
	body     string
	delay    time.Duration
}

type fakeExtractRequest struct {
	Auth        string
	ContentType string
	Fields      map[string]string
	Filename    string
	PartType    string
	File        []byte
}

const okExtractBody = `{"results":[{"content":"# Title\n\nThe zanzibar quokka.\n","mime_type":"application/pdf","metadata":{"ocr_used":false},"tables":[]}],"summary":{"inputs":1,"results":1,"errors":0}}`

func newFakeXberg(t *testing.T) *fakeXberg {
	t.Helper()
	f := &fakeXberg{status: http.StatusOK, body: okExtractBody}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := fakeExtractRequest{Auth: r.Header.Get("Authorization"), ContentType: r.Header.Get("Content-Type"), Fields: map[string]string{}}
		if r.URL.Path == "/extract" && r.Method == http.MethodPost {
			if mt, params, err := mime.ParseMediaType(rec.ContentType); err == nil && mt == "multipart/form-data" {
				mr := multipart.NewReader(r.Body, params["boundary"])
				for {
					p, err := mr.NextPart()
					if err != nil {
						break
					}
					b, _ := io.ReadAll(p)
					if p.FormName() == "files" {
						rec.Filename, rec.PartType, rec.File = p.FileName(), p.Header.Get("Content-Type"), b
					} else {
						rec.Fields[p.FormName()] = string(b)
					}
				}
			}
		}
		f.mu.Lock()
		f.requests = append(f.requests, rec)
		status, body, delay := f.status, f.body, f.delay
		f.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeXberg) seen() []fakeExtractRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeExtractRequest(nil), f.requests...)
}

func (f *fakeXberg) reply(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

func (f *fakeXberg) setDelay(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delay = d
}

func remoteExtractor(t *testing.T, f *fakeXberg, mut func(*ExtractorSettings)) *Extractor {
	t.Helper()
	s := ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: f.URL}}
	if mut != nil {
		mut(&s)
	}
	x, err := NewExtractor(s, nil)
	require.NoError(t, err)
	return x
}

func coverAll(string) bool { return true }

func TestExtractSendsOneFilePartAndMarkdownFormat(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	x := remoteExtractor(t, f, func(s *ExtractorSettings) { s.Remote.Credential = "tok-123" })
	got, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "../../etc/report.pdf", MIME: "application/pdf", Data: []byte("%PDF-1.4 body")})
	require.NoError(t, err)
	require.Equal(t, "# Title\n\nThe zanzibar quokka.\n", got.Markdown)
	require.False(t, got.OCRUsed)

	reqs := f.seen()
	require.Len(t, reqs, 1)
	r := reqs[0]
	require.Equal(t, "Bearer tok-123", r.Auth)
	require.Equal(t, "markdown", r.Fields["output_format"])
	require.Equal(t, "report.pdf", r.Filename, "a directory in the name never travels")
	require.Equal(t, "application/pdf", r.PartType)
	require.Equal(t, "%PDF-1.4 body", string(r.File))
	// The two fields that make xberg fetch a URL or read a server-side path.
	for _, banned := range []string{"urls", "inputs", "file", "uri"} {
		require.NotContains(t, r.Fields, banned)
	}
	require.NotContains(t, r.Fields, "config", "no config unless OCR is asked for")
}

func TestExtractOCRSendsConfigOnlyWhenOn(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	off := remoteExtractor(t, f, nil)
	_, err := off.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.png", Data: []byte("x"), OCR: true})
	require.NoError(t, err)
	on := remoteExtractor(t, f, func(s *ExtractorSettings) { s.OCR = true })
	_, err = on.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.png", Data: []byte("x"), OCR: true})
	require.NoError(t, err)
	_, err = on.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.NoError(t, err)
	reqs := f.seen()
	require.Len(t, reqs, 3)
	require.NotContains(t, reqs[0].Fields, "config", "the setting is off")
	require.JSONEq(t, `{"ocr":{"backend":"tesseract"}}`, reqs[1].Fields["config"])
	require.NotContains(t, reqs[2].Fields, "config", "this file did not ask for OCR")
}

func TestExtractRefusesOversizeBodyBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	x := remoteExtractor(t, f, func(s *ExtractorSettings) { s.MaxBytes = 10 })
	_, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: make([]byte, 11)})
	require.ErrorIs(t, err, ErrExtractTooLarge)
	require.Empty(t, f.seen(), "an oversize file must not cost a request")
	_, err = x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: make([]byte, 10)})
	require.NoError(t, err)
	require.Len(t, f.seen(), 1)
}

func TestExtractRefusesUnacknowledgedRemoteHostBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	x := remoteExtractor(t, f, nil)
	for _, covers := range []func(string) bool{nil, func(string) bool { return false }} {
		_, err := x.Extract(t.Context(), covers, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
		var eg *EgressError
		require.ErrorAs(t, err, &eg)
		require.ErrorIs(t, err, ErrEgressNotAcknowledged)
		require.Equal(t, []string{x.Host()}, eg.Hosts)
	}
	require.Empty(t, f.seen())
}

func TestExtractMalformedResponsesAreExtractionFailed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"not json", 200, `<html>nope</html>`, "not an extraction response"},
		{"no results", 200, `{"results":[],"summary":{}}`, "0 results"},
		{"two results", 200, `{"results":[{"content":"a","mime_type":"x"},{"content":"b","mime_type":"x"}]}`, "2 results"},
		{"missing content", 200, `{"results":[{"mime_type":"x"}]}`, "no content"},
		{"errors entry", 200, `{"results":[],"errors":[{"index":0,"code":1001,"error_type":"parsing","message":"Parsing error: Invalid cross-reference table"}]}`, "parsing: Parsing error"},
		{"invalid utf-8", 200, "{\"results\":[{\"content\":\"a\xffb\",\"mime_type\":\"x\"}]}", "valid UTF-8"},
		{"nul byte", 200, `{"results":[{"content":"a\u0000b","mime_type":"x"}]}`, "NUL"},
		{"bad request", 400, `{"error_type":"ValidationError","message":"Unknown multipart field"}`, "HTTP 400"},
		{"server error", 500, `{"message":"boom"}`, "HTTP 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeXberg(t)
			f.reply(tt.status, tt.body)
			x := remoteExtractor(t, f, nil)
			_, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
			require.ErrorIs(t, err, ErrExtractionFailed)
			require.NotErrorIs(t, err, ErrExtractorUnavailable)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestExtractOversizeResponseIsExtractionFailed(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	f.reply(200, `{"results":[{"content":"`+strings.Repeat("a", maxExtractResponse)+`","mime_type":"x"}]}`)
	x := remoteExtractor(t, f, nil)
	_, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractionFailed)
	require.ErrorContains(t, err, "exceeds")
}

func TestExtractErrorTextIsCleaned(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	f.reply(200, `{"results":[],"errors":[{"error_type":"parsing","message":"line1\u001b[31m\nline2 `+strings.Repeat("z", 1000)+`"}]}`)
	x := remoteExtractor(t, f, nil)
	_, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractionFailed)
	require.NotContains(t, err.Error(), "\x1b")
	require.NotContains(t, err.Error(), "\n")
	require.Less(t, len(err.Error()), 400)
}

func TestExtractTimeoutAndUnreachableAreUnavailable(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	f.setDelay(5 * time.Second)
	x := remoteExtractor(t, f, func(s *ExtractorSettings) { s.Timeout = 150 * time.Millisecond })
	start := time.Now()
	_, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractorUnavailable)
	require.Less(t, time.Since(start), 3*time.Second, "the timeout bounds the request")

	down := newFakeXberg(t)
	url := down.URL
	down.Close()
	x2, err := NewExtractor(ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: url}}, nil)
	require.NoError(t, err)
	_, err = x2.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractorUnavailable)
	require.NotContains(t, err.Error(), url, "the error does not echo the request URL")

	for _, status := range []int{502, 503, 504} {
		f.setDelay(0)
		f.reply(status, `{}`)
		_, err := remoteExtractor(t, f, nil).Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
		require.ErrorIs(t, err, ErrExtractorUnavailable, "HTTP %d", status)
	}
}

func TestExtractDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()
	var hit atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit.Add(1) }))
	t.Cleanup(other.Close)
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redir.Close)
	x, err := NewExtractor(ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: redir.URL}}, nil)
	require.NoError(t, err)
	_, err = x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractionFailed)
	require.Zero(t, hit.Load(), "the document must not follow a redirect to another host")
}

func TestExtractReportsOCRUsedAndEmptyText(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	f.reply(200, `{"results":[{"content":"  \n","mime_type":"image/png","metadata":{"ocr_used":true}}]}`)
	got, err := remoteExtractor(t, f, nil).Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.png", Data: []byte("x")})
	require.NoError(t, err)
	require.True(t, got.OCRUsed)
	require.Equal(t, "image/png", got.MIME)
	require.Empty(t, strings.TrimSpace(got.Markdown))
}

func TestExtractIgnoresImagesTheExtractorReturns(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	f.reply(200, `{"results":[{"content":"text","mime_type":"x","images":[{"data":[1,2,3],"format":"png","image_index":0}]}]}`)
	got, err := remoteExtractor(t, f, nil).Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.docx", Data: []byte("x")})
	require.NoError(t, err)
	b, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(b), "image")
	require.Equal(t, "text", got.Markdown)
}

func TestExtractorSettingsValidate(t *testing.T) {
	t.Parallel()
	good := func() ExtractorSettings {
		return ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: "https://xberg.example:8000"}}
	}
	require.NoError(t, good().Validate())
	for _, ok := range []string{"http://127.0.0.1:8000", "http://localhost:8000", "http://[::1]:8000", "https://xberg.example"} {
		s := good()
		s.Remote.BaseURL = ok
		require.NoError(t, s.Validate(), ok)
	}
	tests := []struct {
		name string
		mut  func(*ExtractorSettings)
		want string
	}{
		{"bad mode", func(s *ExtractorSettings) { s.Mode = "x" }, "embedded or remote"},
		{"remote without block", func(s *ExtractorSettings) { s.Remote = nil }, "no default xberg URL"},
		{"credentials in url", func(s *ExtractorSettings) { s.Remote.BaseURL = "https://u:p@h/" }, "credentials"},
		{"credential over http", func(s *ExtractorSettings) {
			s.Remote.BaseURL = "http://xberg.example"
			s.Remote.Credential = "tok"
		}, "plain http"},
		{"no credential over http", func(s *ExtractorSettings) { s.Remote.BaseURL = "http://xberg.example:8000" }, "refusing plain http to a non-loopback host"},
		{"max_text_bytes too large", func(s *ExtractorSettings) { s.MaxTextBytes = MaxExtractorMaxTextBytes + 1 }, "max_text_bytes"},
		{"max_text_bytes negative", func(s *ExtractorSettings) { s.MaxTextBytes = -1 }, "max_text_bytes"},
		{"embedded set in remote mode", func(s *ExtractorSettings) { s.Embedded = &ExtractorEmbedded{Binary: "/b", CacheDir: "/c"} }, "mode is remote"},
		{"timeout too large", func(s *ExtractorSettings) { s.Timeout = 2 * time.Hour }, "timeout_seconds"},
		{"max_bytes too large", func(s *ExtractorSettings) { s.MaxBytes = MaxExtractorMaxBytes + 1 }, "max_bytes"},
		{"embedded without block", func(s *ExtractorSettings) { s.Mode, s.Remote = ModeEmbedded, nil }, "embeddings.extractor.embedded is required"},
		{"relative binary", func(s *ExtractorSettings) {
			s.Mode, s.Remote = ModeEmbedded, nil
			s.Embedded = &ExtractorEmbedded{Binary: "xberg", CacheDir: "/c"}
		}, "absolute path"},
		{"relative cache dir", func(s *ExtractorSettings) {
			s.Mode, s.Remote = ModeEmbedded, nil
			s.Embedded = &ExtractorEmbedded{Binary: "/b/xberg", CacheDir: "c"}
		}, "cache_dir"},
		{"bad min version", func(s *ExtractorSettings) {
			s.Mode, s.Remote = ModeEmbedded, nil
			s.Embedded = &ExtractorEmbedded{Binary: "/b/xberg", CacheDir: "/c", MinVersion: "1.3"}
		}, "min_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := good()
			tt.mut(&s)
			require.ErrorContains(t, s.Validate(), tt.want)
		})
	}
}

func TestSecretIsNotPrintedByExtractorSettings(t *testing.T) {
	t.Parallel()
	s := ExtractorSettings{Mode: ModeRemote, Remote: &ExtractorRemote{BaseURL: "https://h", Credential: "super-secret"}}
	require.NotContains(t, fmt.Sprintf("%+v %#v", *s.Remote, s.Remote.Credential), "super-secret")
}

func TestExtractedTextOverMaxTextBytesIsRefused(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	f.reply(200, `{"results":[{"content":"`+strings.Repeat("a", 101)+`","mime_type":"x"}]}`)
	x := remoteExtractor(t, f, func(s *ExtractorSettings) { s.MaxTextBytes = 100 })
	require.Equal(t, 100, x.MaxTextBytes())
	_, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractedTextTooLarge)
	require.NotErrorIs(t, err, ErrExtractionFailed, "the caller reports it as too_large, not as a failed extraction")

	f.reply(200, `{"results":[{"content":"`+strings.Repeat("a", 100)+`","mime_type":"x"}]}`)
	got, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.NoError(t, err)
	require.Len(t, got.Markdown, 100, "the limit itself is allowed")

	def := remoteExtractor(t, f, nil)
	require.Equal(t, DefaultExtractorMaxTextBytes, def.MaxTextBytes())
}

func TestExtractStreamsTheFileWithoutBufferingAForm(t *testing.T) {
	t.Parallel()
	f := newFakeXberg(t)
	data := bytes.Repeat([]byte("0123456789abcdef"), 1<<16) // 1 MiB
	x := remoteExtractor(t, f, nil)
	_, err := x.Extract(t.Context(), coverAll, ExtractRequest{Filename: "big.pdf", MIME: "application/pdf", Data: data})
	require.NoError(t, err)
	reqs := f.seen()
	require.Len(t, reqs, 1)
	require.Equal(t, data, reqs[0].File, "the whole file arrives through the pipe")
}
