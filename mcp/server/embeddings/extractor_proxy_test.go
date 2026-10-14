package embeddings

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// proxyEnv is a proxy in front of the recording fakeXberg.
type proxyEnv struct {
	p *extractorProxy
	x *fakeXberg
}

func newProxyEnv(t *testing.T, maxFile int) *proxyEnv {
	t.Helper()
	x := newFakeXberg(t)
	p, err := startExtractorProxy(x.URL, maxFile)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return &proxyEnv{p: p, x: x}
}

// form builds a multipart body from name/value pairs; a name that starts with
// "file:" is a file part named by the rest.
func form(t *testing.T, fields [][2]string, file []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range fields {
		require.NoError(t, w.WriteField(f[0], f[1]))
	}
	if file != nil {
		part, err := w.CreateFormFile("files", "../../etc/doc.pdf")
		require.NoError(t, err)
		_, err = part.Write(file)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func (e *proxyEnv) do(t *testing.T, mutate func(*http.Request)) *http.Response {
	t.Helper()
	body, ct := form(t, [][2]string{{"output_format", "markdown"}}, []byte("%PDF-1.4"))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.p.Endpoint()+"/extract", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+e.p.Token().Reveal())
	if mutate != nil {
		mutate(req)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestProxyForwardsOnlyTheThreeFields(t *testing.T) {
	t.Parallel()
	e := newProxyEnv(t, 1<<20)
	body, ct := form(t, [][2]string{{"output_format", "markdown"}, {"config", `{"ocr":{"backend":"tesseract"}}`}}, []byte("%PDF-1.4 body"))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.p.Endpoint()+"/extract", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+e.p.Token().Reveal())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(got))
	require.Contains(t, string(got), "zanzibar quokka", "the answer is relayed")

	seen := e.x.seen()
	require.Len(t, seen, 1)
	r := seen[0]
	require.Empty(t, r.Auth, "the proxy's token is not passed on to xberg")
	require.Equal(t, map[string]string{"output_format": "markdown", "config": `{"ocr":{"backend":"tesseract"}}`}, r.Fields)
	require.Equal(t, "doc.pdf", r.Filename, "a directory in the name never travels")
	require.Equal(t, "%PDF-1.4 body", string(r.File))
}

func TestProxyRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"no token", func(r *http.Request) { r.Header.Del("Authorization") }, http.StatusUnauthorized},
		{"wrong token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, http.StatusUnauthorized},
		{"token without the scheme", func(r *http.Request) {
			r.Header.Set("Authorization", strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		}, http.StatusUnauthorized},
		{"another host (DNS rebinding)", func(r *http.Request) { r.Host = "rebind.example:80" }, http.StatusForbidden},
		{"another host with the token", func(r *http.Request) { r.Host = "localhost:" + strings.Split(r.URL.Host, ":")[1] }, http.StatusForbidden},
		{"a browser origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, http.StatusForbidden},
		{"another path", func(r *http.Request) { r.URL.Path = "/cache/clear"; r.Method = http.MethodDelete }, http.StatusNotFound},
		{"health", func(r *http.Request) { r.URL.Path = "/health"; r.Method = http.MethodGet }, http.StatusNotFound},
		{"a sub path", func(r *http.Request) { r.URL.Path = "/extract/x" }, http.StatusNotFound},
		{"a query", func(r *http.Request) { r.URL.RawQuery = "x=1" }, http.StatusNotFound},
		{"GET", func(r *http.Request) { r.Method = http.MethodGet; r.Body = nil; r.ContentLength = 0 }, http.StatusMethodNotAllowed},
		{"not multipart", func(r *http.Request) { r.Header.Set("Content-Type", "application/json") }, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newProxyEnv(t, 1<<20)
			resp := e.do(t, tt.mutate)
			require.Equal(t, tt.want, resp.StatusCode)
			require.Empty(t, e.x.seen(), "xberg received nothing")
		})
	}
}

func TestProxyNeverForwardsUrlsOrInputs(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"urls", "inputs", "uri", "file"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			e := newProxyEnv(t, 1<<20)
			body, ct := form(t, [][2]string{{"output_format", "markdown"}, {field, "http://169.254.169.254/latest/meta-data/"}}, []byte("%PDF-1.4"))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.p.Endpoint()+"/extract", body)
			require.NoError(t, err)
			req.Header.Set("Content-Type", ct)
			req.Header.Set("Authorization", "Bearer "+e.p.Token().Reveal())
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			msg, _ := io.ReadAll(resp.Body)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(msg))
			require.Contains(t, string(msg), "not forwarded")
			for _, r := range e.x.seen() {
				require.NotContains(t, r.Fields, field, "the field must not reach xberg")
			}
		})
	}
}

func TestProxyRefusesAMalformedForm(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		fields [][2]string
		file   []byte
		want   string
	}{
		{"no file", [][2]string{{"output_format", "markdown"}}, nil, "no files part"},
		{"other output format", [][2]string{{"output_format", "html"}}, []byte("x"), "must be markdown"},
		{"output format twice", [][2]string{{"output_format", "markdown"}, {"output_format", "markdown"}}, []byte("x"), "twice"},
		{"huge config", [][2]string{{"config", strings.Repeat("a", maxProxyField+1)}}, []byte("x"), "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newProxyEnv(t, 1<<20)
			body, ct := form(t, tt.fields, tt.file)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.p.Endpoint()+"/extract", body)
			require.NoError(t, err)
			req.Header.Set("Content-Type", ct)
			req.Header.Set("Authorization", "Bearer "+e.p.Token().Reveal())
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			msg, _ := io.ReadAll(resp.Body)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(msg))
			require.Contains(t, string(msg), tt.want)
		})
	}
}

func TestProxyBoundsTheRequestBody(t *testing.T) {
	t.Parallel()
	e := newProxyEnv(t, 100)
	body, ct := form(t, [][2]string{{"output_format", "markdown"}}, make([]byte, 2*proxyBodyOverhead))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.p.Endpoint()+"/extract", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Authorization", "Bearer "+e.p.Token().Reveal())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.GreaterOrEqual(t, resp.StatusCode, 400, "an oversize body is not relayed as a success")
	for _, r := range e.x.seen() {
		require.Less(t, len(r.File), 2*proxyBodyOverhead, "xberg did not get the whole oversize file")
	}
}

func TestProxyAnswersBadGatewayWhenXbergIsDown(t *testing.T) {
	t.Parallel()
	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close()
	p, err := startExtractorProxy(url, 1<<20)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	e := &proxyEnv{p: p, x: newFakeXberg(t)}
	resp := e.do(t, nil)
	require.Equal(t, http.StatusBadGateway, resp.StatusCode, "the client maps 502 to extractor_unavailable")
}

func TestProxyTokensAreFreshAndSecret(t *testing.T) {
	t.Parallel()
	a, b := newProxyEnv(t, 1), newProxyEnv(t, 1)
	require.NotEqual(t, a.p.Token(), b.p.Token())
	require.Len(t, a.p.Token().Reveal(), 64)
	require.NotContains(t, a.p.Token().String(), a.p.Token().Reveal())
}

func TestExtractorClientThroughTheProxyIsAuthenticated(t *testing.T) {
	t.Parallel()
	e := newProxyEnv(t, 1<<20)
	x, err := newExtractor(ExtractorSettings{Mode: ModeEmbedded}, e.p.Endpoint(), false, e.p.Token(), nil)
	require.NoError(t, err)
	got, err := x.Extract(t.Context(), nil, ExtractRequest{Filename: "a.pdf", MIME: "application/pdf", Data: []byte("%PDF-1.4")})
	require.NoError(t, err)
	require.Contains(t, got.Markdown, "zanzibar quokka")

	// The same client without the token is refused as a failed extraction.
	bad, err := newExtractor(ExtractorSettings{Mode: ModeEmbedded}, e.p.Endpoint(), false, "", nil)
	require.NoError(t, err)
	_, err = bad.Extract(t.Context(), nil, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractionFailed)
	require.ErrorContains(t, err, "HTTP 401")
}
