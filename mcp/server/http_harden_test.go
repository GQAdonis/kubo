package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedBuffer is a log sink that is safe to read while handlers write.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A rejected request is logged as a fixed category only: the path, the Host
// header and the Origin say what a caller asked for, and Kubo does not log that
// (R11 judge finding).
func TestRejectionLogsCarryOnlyFixedCategories(t *testing.T) {
	var logs lockedBuffer
	e := newHTTPEnv(t, func(o *HTTPOptions) {
		o.Logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})

	for _, r := range []rawReq{
		{token: tokReader, host: "victim-host.example.test:4242", body: initializeBody},
		{token: tokReader, body: initializeBody, header: map[string]string{"Origin": "http://origin-probe.example.test"}},
		{token: tokReader, body: initializeBody, header: map[string]string{"Sec-Fetch-Site": "cross-site-probe"}},
	} {
		resp := e.do(t, r)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	}

	got := logs.String()
	assert.Contains(t, got, "rejected request")
	for _, reason := range []string{"reason=host", "reason=origin", "reason=cross-site"} {
		assert.Contains(t, got, reason)
	}
	for _, leaked := range []string{"victim-host", "4242", "origin-probe", "cross-site-probe", RouteMCP, "path=", "host=", "origin="} {
		assert.NotContains(t, got, leaked, "the rejection log carries a caller-controlled value")
	}
}

// Anonymous access is for a client on this machine. A request that carries a
// forwarding header came through a proxy and needs a credential (R11-M5).
func TestAnonymousListenerRefusesForwardedRequests(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = nil })

	plain := e.do(t, rawReq{body: initializeBody})
	assert.Equal(t, http.StatusOK, plain.StatusCode, "a local client without a header is served")

	for _, h := range []map[string]string{
		{"X-Forwarded-For": "203.0.113.9"},
		{"X-Real-IP": "203.0.113.9"},
		{"Forwarded": "for=203.0.113.9;proto=https"},
	} {
		resp := e.do(t, rawReq{body: initializeBody, header: h})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "%v", h)
	}
}

func TestAuthenticatedListenerAcceptsForwardedRequests(t *testing.T) {
	e := newHTTPEnv(t, nil)
	resp := e.do(t, rawReq{token: tokReader, body: initializeBody, header: map[string]string{"X-Forwarded-For": "203.0.113.9"}})
	assert.Equal(t, http.StatusOK, resp.StatusCode, "behind a proxy a bearer token is the credential")
}

func TestIsForwarded(t *testing.T) {
	r, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	require.NoError(t, err)
	assert.False(t, isForwarded(r))
	r.Header.Set("x-real-ip", "1.2.3.4")
	assert.True(t, isForwarded(r))
}

// A body that arrives too slowly is cut off, so a slow upload cannot hold a
// connection forever.
func TestSlowRequestBodyIsCutOff(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.BodyReadTimeout = 300 * time.Millisecond })
	conn, err := net.Dial("tcp", strings.TrimPrefix(e.base, "http://"))
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: 127.0.0.1\r\nAuthorization: Bearer %s\r\n"+
		"Content-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: 1000\r\n\r\n{\"jsonrpc\"", RouteMCP, tokReader)
	require.NoError(t, err)

	start := time.Now()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = io.ReadAll(conn) // the server answers or closes; it must not wait for the rest
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatal("the server held a connection open for a stalled body")
	}
	assert.Less(t, time.Since(start), 4*time.Second)
}

// The body deadline is a read deadline that is lifted after each read: a session
// with a standalone event stream keeps working long after the timeout.
func TestEventStreamOutlivesTheBodyDeadline(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.BodyReadTimeout = 200 * time.Millisecond })
	cs := e.client(t, tokReader, nil)
	time.Sleep(700 * time.Millisecond)
	res := toolCall(t, cs, "ipfs_node_info", nil)
	require.False(t, res.IsError, "%+v", res.StructuredContent)
	time.Sleep(500 * time.Millisecond)
	res = toolCall(t, cs, "ipfs_node_info", nil)
	require.False(t, res.IsError)
}
