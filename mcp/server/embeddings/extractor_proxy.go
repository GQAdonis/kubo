package embeddings

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"sync"
	"time"
)

// The embedded xberg answers any caller that can reach its port: it has no
// authentication option and, in 1.3.10, cannot listen on a Unix socket, so it
// must listen on loopback TCP. A web page can post a multipart form to such a
// port from the operator's browser (a CORS simple request; CORS only hides the
// answer) and DNS rebinding removes even that limit. extractorProxy is the only
// path ipfs mcp itself uses: a second loopback listener that insists on a
// per-start random bearer token and on its own Host, and that forwards
// nothing but POST /extract with the three fields ipfs mcp sends. It never
// forwards the fields that make xberg fetch a URL (urls) or read a server-side
// path (inputs).
//
// Residual risk, stated plainly: the proxy protects ipfs mcp's own requests
// and gives the extractor one audited entry point, but xberg's own port stays
// open on 127.0.0.1 for as long as ipfs mcp runs. Anything on this machine that
// finds that ephemeral port, including a browser tab that scans for it, can
// still call xberg directly. XBERG_CORS_ORIGINS and the minimal environment
// narrow what xberg does, not who may call it. Closing the gap needs a Unix
// socket or an authentication option in xberg itself.
const (
	// proxyBodyOverhead is added to the largest file for the multipart framing
	// and the small fields.
	proxyBodyOverhead = 1 << 20
	// maxProxyField bounds the value of a non-file multipart field.
	maxProxyField = 4 << 10
	// proxyHeaderTimeout bounds how long a client may take to send its headers.
	proxyHeaderTimeout = 10 * time.Second
)

// errProxyRefused marks a request the proxy refused after it had started to
// forward it; it is answered 400.
type errProxyRefused string

func (e errProxyRefused) Error() string { return string(e) }

type extractorProxy struct {
	ln       net.Listener
	srv      *http.Server
	host     string
	token    string
	upstream string
	maxBody  int64
	hc       *http.Client
	done     chan struct{}
	closeMu  sync.Once
}

// startExtractorProxy listens on an ephemeral 127.0.0.1 port (binding port 0
// itself, so no other process can take it between choosing and binding) and
// forwards to the xberg at upstream, a base URL.
func startExtractorProxy(upstream string, maxFile int) (*extractorProxy, error) {
	var tok [32]byte
	if _, err := rand.Read(tok[:]); err != nil {
		return nil, fmt.Errorf("extractor proxy token: %w", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("extractor proxy: %w", err)
	}
	p := &extractorProxy{
		ln:       ln,
		host:     ln.Addr().String(),
		token:    hex.EncodeToString(tok[:]),
		upstream: upstream,
		maxBody:  int64(maxFile) + proxyBodyOverhead,
		// No client timeout: the caller's request context bounds the exchange.
		hc:   &http.Client{Transport: directTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		done: make(chan struct{}),
	}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: proxyHeaderTimeout, MaxHeaderBytes: 16 << 10}
	go func() {
		defer close(p.done)
		_ = p.srv.Serve(ln)
	}()
	return p, nil
}

// Endpoint is the base URL the Extractor client uses.
func (p *extractorProxy) Endpoint() string { return "http://" + p.host }

// Token is the bearer token the proxy requires.
func (p *extractorProxy) Token() Secret { return Secret(p.token) }

// Close stops the listener and drops open connections. It is idempotent.
func (p *extractorProxy) Close() {
	p.closeMu.Do(func() {
		_ = p.srv.Close()
		<-p.done
	})
}

// ServeHTTP implements http.Handler.
func (p *extractorProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A browser always sends Origin on a cross-origin POST; ipfs mcp never does.
	if r.Host != p.host || r.Header.Get("Origin") != "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	want := []byte("Bearer " + p.token)
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path != "/extract" || r.URL.RawQuery != "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/form-data" || params["boundary"] == "" {
		http.Error(w, "a multipart/form-data body is required", http.StatusBadRequest)
		return
	}
	p.forward(w, r, multipart.NewReader(http.MaxBytesReader(w, r.Body, p.maxBody), params["boundary"]))
}

// forward streams the allowed parts of the incoming form to xberg through a
// pipe, so the file is never held a second time, and relays the answer.
func (p *extractorProxy) forward(w http.ResponseWriter, r *http.Request, in *multipart.Reader) {
	pr, pw := io.Pipe()
	out := multipart.NewWriter(pw)
	refused := make(chan error, 1)
	go func() {
		err := copyAllowedParts(in, out)
		if err == nil {
			err = out.Close()
		}
		refused <- err
		pw.CloseWithError(err)
	}()
	defer pr.Close()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, p.upstream+"/extract", pr)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", out.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	resp, err := p.hc.Do(req)
	if err != nil {
		// The pipe's error says whether the request itself was the problem.
		var rf errProxyRefused
		if perr := drainErr(refused); errors.As(perr, &rf) || errors.As(err, &rf) {
			http.Error(w, string(rf), http.StatusBadRequest)
			return
		}
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxExtractResponse+1))
}

// drainErr returns the goroutine's result if it is ready, else nil.
func drainErr(c <-chan error) error {
	select {
	case err := <-c:
		return err
	default:
		return nil
	}
}

// copyAllowedParts copies the files part, output_format and config to out and
// refuses everything else: urls and inputs make xberg fetch or read, and any
// other field is not one ipfs mcp sends.
func copyAllowedParts(in *multipart.Reader, out *multipart.Writer) error {
	seen := map[string]bool{}
	for {
		part, err := in.NextPart()
		if errors.Is(err, io.EOF) {
			if !seen["files"] {
				return errProxyRefused("the form has no files part")
			}
			return nil
		}
		if err != nil {
			return err
		}
		name := part.FormName()
		if seen[name] {
			return errProxyRefused("the field " + strconv.Quote(clipRunes(name, 32)) + " appears twice")
		}
		seen[name] = true
		switch name {
		case "files":
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename=%q`, safeFilename(part.FileName())))
			if ct := part.Header.Get("Content-Type"); ct != "" {
				h.Set("Content-Type", ct)
			}
			w, err := out.CreatePart(h)
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, part); err != nil {
				return err
			}
		case "output_format", "config":
			val, err := io.ReadAll(io.LimitReader(part, maxProxyField+1))
			if err != nil {
				return err
			}
			if len(val) > maxProxyField {
				return errProxyRefused("the field " + name + " is too large")
			}
			if name == "output_format" && string(val) != "markdown" {
				return errProxyRefused("output_format must be markdown")
			}
			if err := out.WriteField(name, string(val)); err != nil {
				return err
			}
		default:
			return errProxyRefused("the field " + strconv.Quote(clipRunes(name, 32)) + " is not forwarded")
		}
	}
}
