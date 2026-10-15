package embeddings

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Extractor defaults and limits, the contract's defaults of
// embeddings.extractor.
const (
	// DefaultExtractorTimeout bounds one extraction request.
	DefaultExtractorTimeout = 120 * time.Second
	// MaxExtractorTimeout is the contract's upper bound of timeout_seconds.
	MaxExtractorTimeout = time.Hour
	// DefaultExtractorMaxBytes is the largest file sent to the extractor.
	DefaultExtractorMaxBytes = 50 << 20
	// MaxExtractorMaxBytes is the contract's upper bound of max_bytes.
	MaxExtractorMaxBytes = 100 << 20
	// DefaultExtractorMaxTextBytes bounds the Markdown taken from one extraction
	// (embeddings.extractor.max_text_bytes). It sits near the ceiling of native
	// text (embeddings.policy.max_bytes, 1 MiB by default), so a document costs
	// about as many embedder requests as a text file of the same default limit.
	DefaultExtractorMaxTextBytes = 2 << 20
	// MaxExtractorMaxTextBytes is the upper bound of max_text_bytes.
	MaxExtractorMaxTextBytes = 32 << 20
	// DefaultExtractorMinVersion is the oldest xberg the embedded mode accepts
	// when the operator names none.
	DefaultExtractorMinVersion = "1.3.10"

	// maxExtractResponse bounds one extractor response body, whatever the
	// request size: it also bounds images or metadata the extractor adds.
	maxExtractResponse = 64 << 20
	// maxExtractError is how much of an extractor error message is kept.
	maxExtractError = 240
)

// Extractor failures. Callers match them with errors.Is to choose a
// skipped_reason: extractor_unavailable, extraction_failed or too_large.
var (
	// ErrExtractorUnavailable: the extractor could not be reached or did not
	// answer in time (connection refused, timeout, a gateway error).
	ErrExtractorUnavailable = errors.New("extractor unavailable")
	// ErrExtractionFailed: the extractor answered but produced no text, or its
	// answer was not a well-formed extraction response.
	ErrExtractionFailed = errors.New("extraction failed")
	// ErrExtractTooLarge: the file is larger than extractor.max_bytes. No
	// request was built.
	ErrExtractTooLarge = errors.New("file is larger than embeddings.extractor.max_bytes")
	// ErrExtractedTextTooLarge: the extracted text is larger than
	// extractor.max_text_bytes. The text is discarded, not truncated.
	ErrExtractedTextTooLarge = errors.New("extracted text is larger than embeddings.extractor.max_text_bytes")
)

// ExtractorSettings is a resolved embeddings.extractor configuration:
// credentials are values, read from their env or file reference by the caller.
type ExtractorSettings struct {
	Mode     Mode
	Embedded *ExtractorEmbedded
	Remote   *ExtractorRemote
	// Timeout bounds one request; 0 means DefaultExtractorTimeout.
	Timeout time.Duration
	// MaxBytes is the largest file sent; 0 means DefaultExtractorMaxBytes.
	MaxBytes int
	// MaxTextBytes is the largest Markdown taken from one extraction; 0 means
	// DefaultExtractorMaxTextBytes.
	MaxTextBytes int
	// OCR turns on OCR of images and scanned pages.
	OCR bool
	// Transcribe turns on transcription of audio tracks.
	Transcribe bool
	// InsecureHTTPHosts is embeddings.insecure_http_hosts: hosts a remote
	// extractor may be reached at over plain http.
	InsecureHTTPHosts []string
}

// ExtractorEmbedded configures the supervised xberg process.
type ExtractorEmbedded struct {
	// Binary is the absolute path of the xberg executable. It is never looked
	// up on PATH and never downloaded.
	Binary string
	// CacheDir is the absolute directory that holds xberg's cache and its
	// Hugging Face model cache.
	CacheDir string
	// MinVersion is the oldest accepted xberg; empty means
	// DefaultExtractorMinVersion.
	MinVersion string
	// AllowModelDownloads lets xberg fetch models from the Hugging Face Hub.
	// False runs xberg with HF_HUB_OFFLINE=1.
	AllowModelDownloads bool
	// AvoidPorts are ports the ephemeral bind must not use. Set by the caller.
	AvoidPorts []int
}

// ExtractorRemote configures the connection to an xberg server.
type ExtractorRemote struct {
	BaseURL string
	// Credential is sent as a bearer token to an authenticating proxy in front
	// of xberg; empty sends no Authorization header.
	Credential Secret
}

// Validate checks the settings without touching the network, the file system
// or any process. Errors name the configuration key.
func (s ExtractorSettings) Validate() error {
	var errs []error
	switch s.Mode {
	case ModeEmbedded:
		errs = append(errs, s.validateEmbedded()...)
	case ModeRemote:
		errs = append(errs, s.validateRemote()...)
	default:
		errs = append(errs, fmt.Errorf("embeddings.extractor.mode %q is not embedded or remote", s.Mode))
	}
	if s.Timeout < 0 || s.Timeout > MaxExtractorTimeout {
		errs = append(errs, fmt.Errorf("embeddings.extractor.timeout_seconds %d must be 0 (the default) or between 1 and %d", int(s.Timeout/time.Second), int(MaxExtractorTimeout/time.Second)))
	}
	if s.MaxBytes < 0 || s.MaxBytes > MaxExtractorMaxBytes {
		errs = append(errs, fmt.Errorf("embeddings.extractor.max_bytes %d must be 0 (the default) or between 1 and %d", s.MaxBytes, MaxExtractorMaxBytes))
	}
	if s.MaxTextBytes < 0 || s.MaxTextBytes > MaxExtractorMaxTextBytes {
		errs = append(errs, fmt.Errorf("embeddings.extractor.max_text_bytes %d must be 0 (the default) or between 1 and %d", s.MaxTextBytes, MaxExtractorMaxTextBytes))
	}
	return errors.Join(errs...)
}

func (s ExtractorSettings) validateEmbedded() []error {
	e := s.Embedded
	if e == nil {
		return []error{errors.New("embeddings.extractor.embedded is required when embeddings.extractor.mode is embedded")}
	}
	var errs []error
	if !filepath.IsAbs(e.Binary) {
		errs = append(errs, fmt.Errorf("embeddings.extractor.embedded.xberg_binary %q must be an absolute path (it is never looked up on PATH)", e.Binary))
	}
	if !filepath.IsAbs(e.CacheDir) {
		errs = append(errs, fmt.Errorf("embeddings.extractor.embedded.cache_dir %q must be an absolute path", e.CacheDir))
	}
	if e.MinVersion != "" {
		if _, err := parseVersion(e.MinVersion); err != nil {
			errs = append(errs, fmt.Errorf("embeddings.extractor.embedded.min_version: %w", err))
		}
	}
	if s.Remote != nil {
		errs = append(errs, errors.New("embeddings.extractor.remote is set but embeddings.extractor.mode is embedded"))
	}
	return errs
}

func (s ExtractorSettings) validateRemote() []error {
	r := s.Remote
	if r == nil {
		return []error{errors.New("embeddings.extractor.remote.base_url is required when embeddings.extractor.mode is remote: there is no default xberg URL")}
	}
	var errs []error
	if err := checkEndpoint(r.BaseURL); err != nil {
		errs = append(errs, fmt.Errorf("embeddings.extractor.remote.base_url: %w", err))
	} else if !httpsOrLoopback(r.BaseURL) && !insecureHTTPListed(r.BaseURL, s.InsecureHTTPHosts) {
		// Document content, not only a credential, crosses the network.
		errs = append(errs, errors.New("embeddings.extractor.remote.base_url: refusing plain http to a non-loopback host, which would send documents (and any credential) unencrypted; use https or list the host in embeddings.insecure_http_hosts"))
	}
	if s.Embedded != nil {
		errs = append(errs, errors.New("embeddings.extractor.embedded is set but embeddings.extractor.mode is remote"))
	}
	return errs
}

// Extractor is a client of an xberg server's POST /extract. It is built only
// from an operator's configuration and contacts nothing until Extract is
// called. xberg is a separate process or service; nothing of it is linked.
type Extractor struct {
	endpoint string
	host     string
	remote   bool
	cred     Secret
	hc       *http.Client
	timeout  time.Duration
	maxBytes int
	maxText  int
	ocr      bool
	trans    bool
	sup      *ExtractorSupervisor
}

// NewExtractor builds a client for remote-mode settings; the embedded mode is
// built by OpenExtractor, which starts the process first. It does no I/O.
func NewExtractor(s ExtractorSettings, hc *http.Client) (*Extractor, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Mode != ModeRemote {
		return nil, errors.New("embeddings.extractor: NewExtractor builds only the remote mode; use OpenExtractor for the embedded one")
	}
	return newExtractor(s, s.Remote.BaseURL, true, s.Remote.Credential, hc)
}

func newExtractor(s ExtractorSettings, baseURL string, remote bool, cred Secret, hc *http.Client) (*Extractor, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("embeddings.extractor base URL: %w", err)
	}
	e := &Extractor{
		endpoint: strings.TrimRight(baseURL, "/") + "/extract",
		host:     u.Host,
		remote:   remote,
		cred:     cred,
		timeout:  s.Timeout,
		maxBytes: s.MaxBytes,
		maxText:  s.MaxTextBytes,
		ocr:      s.OCR,
		trans:    s.Transcribe,
	}
	if e.timeout == 0 {
		e.timeout = DefaultExtractorTimeout
	}
	if e.maxBytes == 0 {
		e.maxBytes = DefaultExtractorMaxBytes
	}
	if e.maxText == 0 {
		e.maxText = DefaultExtractorMaxTextBytes
	}
	if hc == nil {
		hc = &http.Client{Transport: directTransport()}
	}
	// The per-request deadline is the context's; a redirect would carry the
	// document to a host the operator did not name and the principal did not
	// acknowledge.
	c := *hc
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	e.hc = &c
	return e, nil
}

// OpenExtractor builds the client for the settings: remote mode connects
// lazily, embedded mode starts the supervised xberg process and waits until it
// is healthy. A failure leaves nothing running.
func OpenExtractor(ctx context.Context, s ExtractorSettings, hc *http.Client, opts Options) (*Extractor, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Mode == ModeRemote {
		return NewExtractor(s, hc)
	}
	sup, err := StartExtractorSupervisor(ctx, *s.Embedded, opts.Logger)
	if err != nil {
		return nil, err
	}
	// Requests go to the authenticated loopback proxy, not to xberg's own port.
	if err := sup.startProxy(cmp.Or(s.MaxBytes, DefaultExtractorMaxBytes)); err != nil {
		_ = sup.Close()
		return nil, err
	}
	e, err := newExtractor(s, sup.ProxyEndpoint(), false, sup.ProxyToken(), hc)
	if err != nil {
		_ = sup.Close()
		return nil, err
	}
	e.sup = sup
	return e, nil
}

// Remote reports whether the extractor is a service the operator named by URL.
// Content sent to it leaves the supervised process boundary and needs an
// egress acknowledgement; the embedded one is a child on this machine.
func (e *Extractor) Remote() bool { return e.remote }

// Host is the host:port Extract sends file content to.
func (e *Extractor) Host() string { return e.host }

// MaxBytes is the largest file Extract accepts.
func (e *Extractor) MaxBytes() int { return e.maxBytes }

// MaxTextBytes is the largest Markdown Extract returns.
func (e *Extractor) MaxTextBytes() int { return e.maxText }

// OCR reports whether OCR of images and scanned pages is on.
func (e *Extractor) OCR() bool { return e.ocr }

// Transcribe reports whether transcription of audio tracks is on.
func (e *Extractor) Transcribe() bool { return e.trans }

// Supervisor is the supervised xberg process, nil in remote mode.
func (e *Extractor) Supervisor() *ExtractorSupervisor { return e.sup }

// Close stops the supervised xberg process, if any.
func (e *Extractor) Close() error {
	if e == nil || e.sup == nil {
		return nil
	}
	return e.sup.Close()
}

// ExtractRequest is one file for Extract.
type ExtractRequest struct {
	// Filename is the name sent with the file; xberg detects the format from
	// its extension and from the bytes. It carries no directory.
	Filename string
	// MIME is the content type the caller detected; it is sent as the part's
	// Content-Type.
	MIME string
	Data []byte
	// OCR asks xberg to run OCR (images, scanned pages). It is honoured only
	// when the extractor's ocr setting is on.
	OCR bool
}

// Extraction is a validated extraction result. Markdown is untrusted text.
type Extraction struct {
	Markdown string
	// MIME is the type xberg detected, informational only.
	MIME string
	// OCRUsed reports that xberg ran OCR for this file.
	OCRUsed bool
}

type extractResponse struct {
	Results []struct {
		Content  *string `json:"content"`
		MIMEType string  `json:"mime_type"`
		Metadata struct {
			OCRUsed bool `json:"ocr_used"`
		} `json:"metadata"`
	} `json:"results"`
	Errors []struct {
		Code      int    `json:"code"`
		ErrorType string `json:"error_type"`
		Message   string `json:"message"`
	} `json:"errors"`
	Message string `json:"message"`
}

// Extract sends one file to POST /extract and returns its Markdown.
//
// Nothing is built or sent when the file is larger than the extractor's
// max_bytes (ErrExtractTooLarge) or, in remote mode, when covers does not
// acknowledge the extractor's host (*EgressError). The response is untrusted:
// its size is bounded, it must be one well-formed result with UTF-8 text and
// no NUL byte, and any images in it are never read. A file xberg cannot read is
// ErrExtractionFailed; an extractor that cannot be reached or does not answer
// in time is ErrExtractorUnavailable.
func (e *Extractor) Extract(ctx context.Context, covers func(host string) bool, r ExtractRequest) (Extraction, error) {
	if len(r.Data) > e.maxBytes {
		return Extraction{}, fmt.Errorf("%w (%d bytes, limit %d)", ErrExtractTooLarge, len(r.Data), e.maxBytes)
	}
	if e.sup != nil {
		select {
		case <-e.sup.Done():
			return Extraction{}, fmt.Errorf("%w: the embedded xberg process has exited; restart ipfs mcp", ErrExtractorUnavailable)
		default:
		}
	}
	if e.remote && (covers == nil || !covers(e.host)) {
		return Extraction{}, &EgressError{Hosts: []string{e.host}, Setting: "embeddings.extractor.remote.base_url"}
	}
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	body, contentType := e.build(r)
	// Closing the read end unblocks the writer goroutine whatever happens below.
	defer body.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, body)
	if err != nil {
		return Extraction{}, fmt.Errorf("build extractor request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if e.cred != "" {
		req.Header.Set("Authorization", "Bearer "+e.cred.Reveal())
	}
	resp, err := e.hc.Do(req)
	if err != nil {
		return Extraction{}, fmt.Errorf("%w: %w", ErrExtractorUnavailable, hostOnly(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxExtractResponse+1))
	if err != nil {
		return Extraction{}, fmt.Errorf("%w: read extractor response: %w", ErrExtractorUnavailable, hostOnly(err))
	}
	if len(raw) > maxExtractResponse {
		return Extraction{}, fmt.Errorf("%w: the extractor response exceeds %d bytes", ErrExtractionFailed, maxExtractResponse)
	}
	return parseExtraction(resp.StatusCode, raw, e.maxText)
}

// build streams the multipart request through a pipe, so the file is not copied
// into a second buffer: output_format=markdown, a config object only when OCR is
// asked for, and the file as the single files part. It never writes a urls or
// inputs field, the two that make xberg fetch a URL or read a server-side path.
// A write error reaches the reader, and so the HTTP client, as the read error.
func (e *Extractor) build(r ExtractRequest) (io.ReadCloser, string) {
	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	go func() { pw.CloseWithError(e.writeForm(w, r)) }()
	return pr, w.FormDataContentType()
}

func (e *Extractor) writeForm(w *multipart.Writer, r ExtractRequest) error {
	if err := w.WriteField("output_format", "markdown"); err != nil {
		return fmt.Errorf("build extractor request: %w", err)
	}
	if r.OCR && e.ocr {
		if err := w.WriteField("config", `{"ocr":{"backend":"tesseract"}}`); err != nil {
			return fmt.Errorf("build extractor request: %w", err)
		}
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename=%q`, safeFilename(r.Filename)))
	if r.MIME != "" && !strings.ContainsAny(r.MIME, "\r\n") {
		h.Set("Content-Type", r.MIME)
	}
	part, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("build extractor request: %w", err)
	}
	if _, err := part.Write(r.Data); err != nil {
		return fmt.Errorf("build extractor request: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("build extractor request: %w", err)
	}
	return nil
}

// safeFilename keeps the base name, drops control characters and quotes, and
// falls back to "content" so a caller-supplied name never injects a header or a
// path.
func safeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		return "content"
	}
	return clipRunes(name, 128)
}

// parseExtraction validates an extractor response as untrusted input.
func parseExtraction(status int, raw []byte, maxText int) (Extraction, error) {
	var out extractResponse
	// encoding/json turns invalid UTF-8 into U+FFFD; the extractor's text is
	// refused rather than silently repaired.
	jsonErr := json.Unmarshal(raw, &out)
	if jsonErr == nil && !utf8.Valid(raw) {
		return Extraction{}, fmt.Errorf("%w: the extractor response is not valid UTF-8", ErrExtractionFailed)
	}
	if status < 200 || status > 299 {
		msg := ""
		if jsonErr == nil {
			msg = cleanMessage(out.Message)
		}
		base := ErrExtractionFailed
		if status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout || status == http.StatusTooManyRequests {
			base = ErrExtractorUnavailable
		}
		if msg != "" {
			return Extraction{}, fmt.Errorf("%w: the extractor answered HTTP %d: %s", base, status, msg)
		}
		return Extraction{}, fmt.Errorf("%w: the extractor answered HTTP %d", base, status)
	}
	if jsonErr != nil {
		return Extraction{}, fmt.Errorf("%w: the extractor answered HTTP %d with a body that is not an extraction response", ErrExtractionFailed, status)
	}
	// xberg reports a file it cannot read as HTTP 200 with an errors entry and
	// no result, so the entry decides, not the status line.
	if len(out.Errors) > 0 {
		e := out.Errors[0]
		return Extraction{}, fmt.Errorf("%w: %s: %s", ErrExtractionFailed, cleanMessage(e.ErrorType), cleanMessage(e.Message))
	}
	if len(out.Results) != 1 {
		return Extraction{}, fmt.Errorf("%w: the extractor returned %d results for one file", ErrExtractionFailed, len(out.Results))
	}
	res := out.Results[0]
	if res.Content == nil {
		return Extraction{}, fmt.Errorf("%w: the extraction result has no content", ErrExtractionFailed)
	}
	text := *res.Content
	switch {
	case len(text) > maxText:
		return Extraction{}, fmt.Errorf("%w (%d bytes, limit %d)", ErrExtractedTextTooLarge, len(text), maxText)
	case !utf8.ValidString(text):
		return Extraction{}, fmt.Errorf("%w: the extracted text is not valid UTF-8", ErrExtractionFailed)
	case strings.ContainsRune(text, 0):
		return Extraction{}, fmt.Errorf("%w: the extracted text contains a NUL byte", ErrExtractionFailed)
	}
	return Extraction{Markdown: text, MIME: cleanMessage(res.MIMEType), OCRUsed: res.Metadata.OCRUsed}, nil
}

// cleanMessage makes extractor-supplied text safe for an error or a log line:
// control characters become spaces and the length is bounded.
func cleanMessage(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	return clipRunes(strings.TrimSpace(s), maxExtractError)
}
