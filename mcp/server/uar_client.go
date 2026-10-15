//go:build !noagent

package server

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

	"github.com/ipfs/kubo/mcp/server/embeddings"
)

// Bounds of the UAR management client. A reply is a small JSON document; one
// that is larger is a broken or hostile peer.
const (
	// maxUARReply bounds one management reply.
	maxUARReply = 1 << 20
	// uarMgmtTimeout bounds one management request (agent, skill and card
	// calls). Runs are bounded by the run's wall clock instead.
	uarMgmtTimeout = 20 * time.Second
)

// uarCardPath is the public agent card; its "version" is the release version.
const uarCardPath = "/.well-known/agent-card.json"

// uarHTTPError is a UAR reply that was not a success. Its text names the
// operation and the status only: a reply body can carry conversation content
// or credentials, so it is never copied into an error.
type uarHTTPError struct {
	Op     string
	Status int
}

func (e *uarHTTPError) Error() string {
	return fmt.Sprintf("UAR %s answered HTTP %d", e.Op, e.Status)
}

// uarClient talks to one UAR instance with the operator's credential. It never
// follows a redirect (the credential would go with it) and ignores the proxy
// environment, like every client that carries operator-named credentials.
type uarClient struct {
	base string
	cred *SecretRef
	hc   *http.Client
}

// newUARClient builds the client for the instance at baseURL. hc replaces the
// default transport (tests).
func newUARClient(baseURL string, cred *SecretRef, hc *http.Client) *uarClient {
	if hc == nil {
		hc = &http.Client{Transport: embeddings.DirectTransport()}
	}
	cp := *hc
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &uarClient{base: strings.TrimRight(baseURL, "/"), cred: cred, hc: &cp}
}

// token reads the UAR credential. It is read at every request, so a rotated
// token (a short-lived JWT in a mounted file) takes effect without a restart.
// An unreadable reference sends no Authorization header; UAR then answers 401
// and the run reports it (CRITICAL-2: never fatal).
func (c *uarClient) token() string {
	if c.cred == nil {
		return ""
	}
	t, err := c.cred.Read()
	if err != nil {
		return ""
	}
	return t
}

// newRequest builds a request to path (which starts with "/") on the instance.
func (c *uarClient) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return nil, fmt.Errorf("build UAR request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t := c.token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	return req, nil
}

// uarReply is a fully read management reply.
type uarReply struct {
	Status int
	Header http.Header
	Body   []byte
}

// ok reports a 2xx status.
func (r uarReply) ok() bool { return r.Status >= 200 && r.Status < 300 }

// call performs one management request: JSON in (v marshalled when not nil),
// bounded body out. A transport failure is an error; an HTTP status is not,
// the caller reads it from the reply.
func (c *uarClient) call(ctx context.Context, method, path string, v any, hdr map[string]string) (uarReply, error) {
	var body []byte
	if v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			return uarReply{}, fmt.Errorf("encode UAR request: %w", err)
		}
		body = b
	}
	ctx, cancel := context.WithTimeout(ctx, uarMgmtTimeout)
	defer cancel()
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return uarReply{}, err
	}
	req.Header.Set("Accept", "application/json")
	for k, val := range hdr {
		req.Header.Set(k, val)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return uarReply{}, fmt.Errorf("UAR %s %s: %w", method, path, unwrapURLError(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxUARReply+1))
	if err != nil {
		return uarReply{}, fmt.Errorf("read UAR reply: %w", err)
	}
	if len(data) > maxUARReply {
		return uarReply{}, errors.New("UAR reply is too large")
	}
	return uarReply{Status: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// unwrapURLError drops the request URL from a transport error: a URL could
// carry credentials, and the operator knows which instance they configured.
func unwrapURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// card reads the public agent card and returns its version.
func (c *uarClient) card(ctx context.Context) (string, error) {
	r, err := c.call(ctx, http.MethodGet, uarCardPath, nil, nil)
	if err != nil {
		return "", err
	}
	if !r.ok() {
		return "", &uarHTTPError{Op: "agent card", Status: r.Status}
	}
	var card struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(r.Body, &card); err != nil || card.Version == "" {
		return "", errors.New("the UAR agent card has no version")
	}
	return card.Version, nil
}

// checkRelease reads the card and refuses an instance older than the pinned
// minimum, or one that is not a published release.
func (c *uarClient) checkRelease(ctx context.Context, minVersion string) (string, error) {
	version, err := c.card(ctx)
	if err != nil {
		return "", err
	}
	got, ok := parsePublishedVersion(version)
	if !ok {
		return version, fmt.Errorf("UAR version %q is not a published release", version)
	}
	min, ok := parsePublishedVersion(minVersion)
	if !ok {
		return version, fmt.Errorf("the pinned minimum UAR version %q is not a release version", minVersion)
	}
	if versionLess(got, min) {
		return version, fmt.Errorf("UAR version %s is older than the pinned minimum %s", version, minVersion)
	}
	return version, nil
}
