package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// maxEmbedResponse bounds one embeddings response.
	maxEmbedResponse = 64 << 20
	// embedTimeout bounds one embeddings request.
	embedTimeout = 120 * time.Second
	// maxEmbedError is how much of a backend error message is kept.
	maxEmbedError = 200
)

// ErrEgressNotAcknowledged is returned, before anything is sent, when the
// calling identity's egress_ack does not cover a host that content would reach.
var ErrEgressNotAcknowledged = errors.New("host is not in the egress_ack of the calling identity")

// EgressError names the hosts that blocked a request. It matches
// ErrEgressNotAcknowledged with errors.Is.
type EgressError struct {
	Hosts []string
	// Setting names the configuration key that holds the host, when one key
	// does, so the message can say where the host comes from.
	Setting string
}

// Error implements error.
func (e *EgressError) Error() string {
	return "refused: content would be sent to " + strings.Join(e.Hosts, ", ") +
		", which is not in the egress_ack of the calling identity"
}

// Is implements errors.Is.
func (e *EgressError) Is(target error) bool { return target == ErrEgressNotAcknowledged }

// EmbedError is a failure reported by the embeddings endpoint. It carries the
// status and a short excerpt of the endpoint's message, never the request.
type EmbedError struct {
	Status  int
	Message string
}

// Error implements error.
func (e *EmbedError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("embedder answered HTTP %d", e.Status)
	}
	return fmt.Sprintf("embedder answered HTTP %d: %s", e.Status, e.Message)
}

// Embedder is a client of an OpenAI-compatible POST {base_url}/embeddings
// endpoint. It is built only from an operator's configuration and contacts
// nothing until Embed is called.
type Embedder struct {
	endpoint string
	host     string
	model    string
	dims     int
	key      Secret
	hc       *http.Client
}

// NewEmbedder builds a client for the settings. It does no I/O.
func NewEmbedder(s EmbedderSettings, hc *http.Client) (*Embedder, error) {
	if errs := s.validate(); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("embeddings.embedder.base_url: %w", err)
	}
	if hc == nil {
		hc = &http.Client{Timeout: embedTimeout, Transport: directTransport()}
	}
	// A redirect would carry the document text and the key to a host the
	// operator did not name and the principal did not acknowledge.
	c := *hc
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Embedder{
		endpoint: strings.TrimRight(s.BaseURL, "/") + "/embeddings",
		host:     u.Host,
		model:    s.Model,
		dims:     s.Dimensions,
		key:      s.APIKey,
		hc:       &c,
	}, nil
}

// Host is the host:port that Embed sends text to.
func (e *Embedder) Host() string { return e.host }

// Model is the configured model name.
func (e *Embedder) Model() string { return e.model }

// Dimensions is the configured vector length.
func (e *Embedder) Dimensions() int { return e.dims }

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Index     *int      `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed returns one vector per text, in order. covers reports whether the
// calling identity's egress_ack covers a host; a nil covers refuses the call.
// Nothing is sent when the embedder's host is not covered.
func (e *Embedder) Embed(ctx context.Context, covers func(host string) bool, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if covers == nil || !covers(e.host) {
		return nil, &EgressError{Hosts: []string{e.host}}
	}
	body, err := json.Marshal(embedRequest{Model: e.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("encode embeddings request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build embeddings request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if e.key != "" {
		req.Header.Set("Authorization", "Bearer "+e.key.Reveal())
	}
	resp, err := e.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedder request: %w", hostOnly(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxEmbedResponse+1))
	if err != nil {
		return nil, fmt.Errorf("read embedder response: %w", err)
	}
	if len(raw) > maxEmbedResponse {
		return nil, fmt.Errorf("embedder response exceeds %d bytes", maxEmbedResponse)
	}
	var out embedResponse
	jsonErr := json.Unmarshal(raw, &out)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := ""
		if jsonErr == nil && out.Error != nil {
			msg = clipRunes(strings.TrimSpace(out.Error.Message), maxEmbedError)
		}
		return nil, &EmbedError{Status: resp.StatusCode, Message: msg}
	}
	if jsonErr != nil {
		return nil, fmt.Errorf("embedder answered HTTP %d with a body that is not an embeddings response", resp.StatusCode)
	}
	return e.collect(out, len(texts))
}

// collect orders the vectors by their index and checks count and dimension.
func (e *Embedder) collect(out embedResponse, n int) ([][]float32, error) {
	if len(out.Data) != n {
		return nil, fmt.Errorf("embedder returned %d vectors for %d texts", len(out.Data), n)
	}
	vecs := make([][]float32, n)
	for i, d := range out.Data {
		// Some servers omit index; their data is in input order.
		idx := i
		if d.Index != nil {
			idx = *d.Index
		}
		if idx < 0 || idx >= n || vecs[idx] != nil {
			return nil, fmt.Errorf("embedder returned an unusable index %d", idx)
		}
		if len(d.Embedding) != e.dims {
			return nil, fmt.Errorf("embedder returned a %d-dimensional vector but embeddings.embedder.dimensions is %d", len(d.Embedding), e.dims)
		}
		vecs[idx] = d.Embedding
	}
	return vecs, nil
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
