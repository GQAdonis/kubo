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
	// maxSurrealResponse bounds one response from the store.
	maxSurrealResponse = 64 << 20
	// maxSnippet bounds the part of a store message kept in an error.
	maxSnippet = 300
	// surrealTimeout bounds one request to the store.
	surrealTimeout = 60 * time.Second
)

// ErrAuth is returned when the store rejects the credentials.
var ErrAuth = errors.New("surrealdb rejected the credentials")

// auth is how a client authenticates to SurrealDB.
type auth struct {
	user, pass string // HTTP basic, when user is set
	token      Secret // bearer, when user is empty
}

// parseCredential reads the credential form: "user:password" is a system user
// (HTTP basic); anything else is a bearer token; empty is no credential.
func parseCredential(c Secret) auth {
	raw := c.Reveal()
	if raw == "" {
		return auth{}
	}
	if user, pass, ok := strings.Cut(raw, ":"); ok && user != "" {
		return auth{user: user, pass: pass}
	}
	return auth{token: c}
}

// surrealClient speaks SurrealDB's HTTP JSON-RPC endpoint (POST /rpc, method
// "query") with the standard library. See SOURCES.md.
type surrealClient struct {
	endpoint  string // base URL without a trailing slash
	namespace string
	database  string
	auth      auth
	hc        *http.Client
	// gone, when set, is closed when the embedded store process has exited. The
	// client then sends nothing: its root credential must not go to whatever
	// process binds the freed port.
	gone <-chan struct{}
}

// ErrStoreExited: the embedded store process ended after it had started. The
// client fails closed instead of sending to its port; the next use of the
// store starts a new process.
var ErrStoreExited = errors.New("the embedded store process has exited")

func newSurrealClient(endpoint, ns, db string, a auth, hc *http.Client) *surrealClient {
	if hc == nil {
		hc = &http.Client{Timeout: surrealTimeout, Transport: directTransport()}
	}
	// The operator named one server; a redirect would carry the credentials
	// to another host.
	c := *hc
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &surrealClient{endpoint: strings.TrimRight(endpoint, "/"), namespace: ns, database: db, auth: a, hc: &c}
}

// rpcRequest is the JSON-RPC envelope of POST /rpc.
type rpcRequest struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

// rpcResponse is the envelope of a /rpc reply: either an error or one result
// per statement.
type rpcResponse struct {
	Result []stmtResult `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// stmtResult is the outcome of one statement of a query.
type stmtResult struct {
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
}

// query runs SurrealQL with bound variables and returns the result of each
// statement. A statement that failed is an error, with its message clipped.
func (c *surrealClient) query(ctx context.Context, surql string, vars map[string]any) ([]json.RawMessage, error) {
	params := []any{surql}
	if len(vars) > 0 {
		params = append(params, vars)
	}
	body, err := json.Marshal(rpcRequest{ID: 1, Method: "query", Params: params})
	if err != nil {
		return nil, fmt.Errorf("encode query: %w", err)
	}
	raw, status, err := c.do(ctx, http.MethodPost, "/rpc", body)
	if err != nil {
		return nil, err
	}
	var env rpcResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("surrealdb answered HTTP %d with a body that is not a query result: %s", status, clip(string(raw)))
	}
	if env.Error != nil {
		return nil, fmt.Errorf("surrealdb error %d: %s", env.Error.Code, clip(env.Error.Message))
	}
	out := make([]json.RawMessage, 0, len(env.Result))
	for i, r := range env.Result {
		if r.Status != "OK" {
			var msg string
			_ = json.Unmarshal(r.Result, &msg)
			return nil, fmt.Errorf("surrealdb statement %d failed: %s", i+1, clip(msg))
		}
		out = append(out, r.Result)
	}
	return out, nil
}

// do sends one request. It returns the body of a 2xx answer; 401 and 403 are
// ErrAuth; other statuses carry a clipped excerpt of the body.
func (c *surrealClient) do(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	select {
	case <-c.gone:
		return nil, 0, ErrStoreExited
	default:
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build surrealdb request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.namespace != "" {
		req.Header.Set("Surreal-NS", c.namespace)
	}
	if c.database != "" {
		req.Header.Set("Surreal-DB", c.database)
	}
	switch {
	case c.auth.user != "":
		req.SetBasicAuth(c.auth.user, c.auth.pass)
	case c.auth.token != "":
		req.Header.Set("Authorization", "Bearer "+c.auth.token.Reveal())
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("surrealdb request: %w", hostOnly(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSurrealResponse+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read surrealdb response: %w", err)
	}
	if len(raw) > maxSurrealResponse {
		return nil, resp.StatusCode, fmt.Errorf("surrealdb response exceeds %d bytes", maxSurrealResponse)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, resp.StatusCode, fmt.Errorf("%w (HTTP %d)", ErrAuth, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		// A JSON-RPC error rides a non-2xx status; let the caller parse it.
		if json.Valid(raw) {
			return raw, resp.StatusCode, nil
		}
		return nil, resp.StatusCode, fmt.Errorf("surrealdb answered HTTP %d: %s", resp.StatusCode, clip(string(raw)))
	}
	return raw, resp.StatusCode, nil
}

// health reports whether GET /health answers 200.
func (c *surrealClient) health(ctx context.Context) error {
	_, status, err := c.do(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("surrealdb /health answered HTTP %d", status)
	}
	return nil
}

// hostOnly drops the URL from a net/http error so a log line never carries
// more than the operator already configured; the cause is kept.
func hostOnly(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= maxSnippet {
		return s
	}
	return string(r[:maxSnippet]) + "..."
}
