package server

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tokens of the test principals. No test names a port: every listener binds
// 127.0.0.1:0 and the tests read the address back.
const (
	tokReader = "reader-token-0123456789"
	tokWriter = "writer-token-0123456789"
	tokAdmin  = "admin-token-01234567890"
	tokPeer   = "a2a-peer-token-01234567"
	tokChat   = "ui-chat-token-012345678"
	tokOther  = "other-token-01234567890"
)

func testVerifier(t *testing.T) *StaticVerifier {
	t.Helper()
	mk := func(id, kind string, groups, write, destructive []string) Identity {
		return NewIdentity(Principal{ID: id, Kind: kind}, groups, write, destructive)
	}
	v, err := NewStaticVerifier([]StaticCredential{
		{Identity: mk("reader", KindHumanMCP, []string{AllGroups}, nil, nil), Token: tokReader},
		{Identity: mk("writer", KindHumanMCP, []string{AllGroups}, []string{"pin"}, nil), Token: tokWriter},
		{Identity: mk("admin", KindHumanMCP, []string{AllGroups}, []string{"pin"}, []string{"pin"}), Token: tokAdmin},
		{Identity: mk("peer", KindA2A, []string{"node", "pin"}, nil, nil), Token: tokPeer},
		{Identity: mk("chat", KindUI, []string{AllGroups}, nil, nil), Token: tokChat},
		{Identity: mk("other", KindHumanMCP, []string{AllGroups}, []string{"pin"}, nil), Token: tokOther},
	})
	require.NoError(t, err)
	return v
}

// httpEnv is a running HTTP listener over a server with the node and pin tools.
type httpEnv struct {
	s        *Server
	hs       *HTTPServer
	base     string
	d        *fakeDaemon
	pins     *pinStore
	resolver *fakeResolver
	target   string
}

func newHTTPEnv(t *testing.T, mod func(*HTTPOptions)) *httpEnv {
	t.Helper()
	d := newFakeDaemon(t)
	target := testCID(t, 1)
	pins := installPins(d, map[string]fakePin{target: {Type: "recursive"}})
	res := newFakeResolver(ResolverLinuxPolkit)
	s := mustNew(t, d, Options{Tools: []string{"node", "pin", "destructive"}, Resolvers: []Resolver{res}, Transport: "http"})
	opts := HTTPOptions{Listen: "127.0.0.1:0", Verifier: testVerifier(t)}
	if mod != nil {
		mod(&opts)
	}
	hs, err := NewHTTPServer(s, opts)
	require.NoError(t, err)
	addr, err := hs.Listen()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- hs.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	return &httpEnv{s: s, hs: hs, base: "http://" + addr.String(), d: d, pins: pins, resolver: res, target: target}
}

// bearerTransport adds a bearer token to every request.
type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// client opens an MCP session over HTTP with the token.
func (e *httpEnv) client(t *testing.T, token string, co *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	cs, err := e.connect(t, token, co)
	require.NoError(t, err)
	return cs
}

func (e *httpEnv) connect(t *testing.T, token string, co *mcp.ClientOptions) (*mcp.ClientSession, error) {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             e.base + RouteMCP,
		HTTPClient:           &http.Client{Transport: bearerTransport{token: token}},
		DisableStandaloneSSE: false,
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "http-test", Version: "0"}, co).Connect(t.Context(), transport, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

// request sends one raw request. host overrides the Host header when set.
type rawReq struct {
	method, path, token, host, body string
	header                          map[string]string
}

func (e *httpEnv) do(t *testing.T, r rawReq) *http.Response {
	t.Helper()
	if r.method == "" {
		r.method = http.MethodPost
	}
	if r.path == "" {
		r.path = RouteMCP
	}
	req, err := http.NewRequestWithContext(t.Context(), r.method, e.base+r.path, strings.NewReader(r.body))
	require.NoError(t, err)
	if r.host != "" {
		req.Host = r.host
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range r.header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"raw","version":"0"}}}`

func TestHTTPRequestHardening(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) {
		o.AllowedOrigins = []string{"http://app.example"}
		o.MaxBodyBytes = 2048
	})
	cases := []struct {
		name string
		req  rawReq
		want int
	}{
		{"missing token", rawReq{body: initializeBody}, http.StatusUnauthorized},
		{"unknown token", rawReq{token: "not-a-configured-token", body: initializeBody}, http.StatusUnauthorized},
		{"basic scheme", rawReq{header: map[string]string{"Authorization": "Basic " + tokReader}, body: initializeBody}, http.StatusUnauthorized},
		{"valid token", rawReq{token: tokReader, body: initializeBody}, http.StatusOK},
		{"dns rebinding with a valid token", rawReq{token: tokReader, host: "evil.example", body: initializeBody}, http.StatusForbidden},
		{"dns rebinding without a token", rawReq{host: "evil.example", body: initializeBody}, http.StatusForbidden},
		{"rebinding to a public IP", rawReq{token: tokReader, host: "203.0.113.7:80", body: initializeBody}, http.StatusForbidden},
		{"cross-origin page", rawReq{token: tokReader, header: map[string]string{"Origin": "https://evil.example"}, body: initializeBody}, http.StatusForbidden},
		{"null origin", rawReq{token: tokReader, header: map[string]string{"Origin": "null"}, body: initializeBody}, http.StatusForbidden},
		{"cross-site fetch metadata", rawReq{token: tokReader, header: map[string]string{"Sec-Fetch-Site": "cross-site"}, body: initializeBody}, http.StatusForbidden},
		{"cross-origin GET", rawReq{method: http.MethodGet, token: tokReader, header: map[string]string{"Origin": "https://evil.example"}}, http.StatusForbidden},
		{"allowed origin", rawReq{token: tokReader, header: map[string]string{"Origin": "http://app.example"}, body: initializeBody}, http.StatusOK},
		{"preflight from allowed origin needs no token", rawReq{method: http.MethodOptions, header: map[string]string{"Origin": "http://app.example", "Access-Control-Request-Method": "POST"}}, http.StatusNoContent},
		{"preflight from other origin", rawReq{method: http.MethodOptions, header: map[string]string{"Origin": "http://other.example", "Access-Control-Request-Method": "POST"}}, http.StatusForbidden},
		{"oversized body", rawReq{token: tokReader, body: `{"jsonrpc":"2.0","id":1,"method":"` + strings.Repeat("x", 8192) + `"}`}, http.StatusBadRequest},
		{"unknown route", rawReq{path: "/nope", token: tokReader}, http.StatusNotFound},
		{"agent routes are not mounted yet", rawReq{path: RouteA2A, token: tokPeer, body: "{}"}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := e.do(t, tc.req)
			if tc.want == http.StatusBadRequest {
				assert.Contains(t, []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge}, resp.StatusCode)
			} else {
				assert.Equal(t, tc.want, resp.StatusCode)
			}
			if resp.StatusCode == http.StatusUnauthorized {
				assert.Contains(t, resp.Header.Get("WWW-Authenticate"), "Bearer")
			}
		})
	}
	assert.Zero(t, e.d.count("pin/add"), "no rejected request ran a tool")
}

func TestHTTPAllowedOriginGetsCORSHeaders(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.AllowedOrigins = []string{"http://app.example"} })
	resp := e.do(t, rawReq{method: http.MethodOptions, header: map[string]string{"Origin": "http://app.example"}})
	assert.Equal(t, "http://app.example", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Contains(t, resp.Header.Get("Access-Control-Expose-Headers"), "Mcp-Session-Id")

	other := e.do(t, rawReq{token: tokReader, body: initializeBody})
	assert.Empty(t, other.Header.Get("Access-Control-Allow-Origin"), "a request without an Origin gets no CORS grant")
}

func TestHTTPParityWithStdio(t *testing.T) {
	e := newHTTPEnv(t, nil)
	stdio := toolNames(t, connect(t, e.s))
	require.NotEmpty(t, stdio)

	// A principal with every scope sees the same tools as stdio for the same flags.
	admin := e.client(t, tokAdmin, nil)
	assert.ElementsMatch(t, stdio, httpToolNames(t, admin))

	// A call returns the same structured result over both transports.
	overHTTP := toolCall(t, admin, "ipfs_node_info", nil)
	overStdio := toolCall(t, connect(t, e.s), "ipfs_node_info", nil)
	require.False(t, overHTTP.IsError, "%+v", overHTTP.Content)
	assert.Equal(t, asJSON(t, overStdio.StructuredContent), asJSON(t, overHTTP.StructuredContent))
}

func httpToolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestHTTPScopesNarrowTheToolList(t *testing.T) {
	e := newHTTPEnv(t, nil)
	cat, err := contract.Load()
	require.NoError(t, err)
	withClass := func(classes ...string) []string {
		var out []string
		for _, n := range e.s.ToolNames() {
			if tool, _ := cat.Tool(n); slices.Contains(classes, tool.Confirmation) {
				out = append(out, n)
			}
		}
		return out
	}
	require.NotEmpty(t, withClass(contract.ClassDestructive), "the env registers destructive tools")

	assert.ElementsMatch(t, withClass(contract.ClassNone), httpToolNames(t, e.client(t, tokReader, nil)), "read-only by default")
	assert.ElementsMatch(t, withClass(contract.ClassNone, contract.ClassWrite), httpToolNames(t, e.client(t, tokWriter, nil)), "write is granted by group name; destructive stays off")
	assert.ElementsMatch(t, withClass(contract.ClassNone, contract.ClassWrite, contract.ClassDestructive), httpToolNames(t, e.client(t, tokAdmin, nil)))
	assert.ElementsMatch(t, withClass(contract.ClassNone), httpToolNames(t, e.client(t, tokChat, nil)), "the chat island is read-only too")
	_, err = e.connect(t, tokPeer, nil)
	require.Error(t, err, "an a2a principal does not use the MCP route")
}

func TestHTTPToolOutsideGrantsIsRefusedWithoutRunning(t *testing.T) {
	e := newHTTPEnv(t, nil)
	reader := e.client(t, tokReader, nil)
	path := "/ipfs/" + testCID(t, 5)

	res := toolCall(t, reader, "pin_add", map[string]any{"path": path})
	te := confirmationOf(t, res, CodePermissionDenied)
	assert.Contains(t, te.Message, "pin_add")
	assert.Equal(t, ReasonPrincipalNotGranted, te.Details["reason"])

	// A writer without the destructive grant cannot remove, even though the tool is registered.
	writer := e.client(t, tokWriter, nil)
	confirmationOf(t, toolCall(t, writer, "pin_rm", map[string]any{"path": "/ipfs/" + e.target}), CodePermissionDenied)

	assert.Zero(t, e.d.count("pin/add"))
	assert.Zero(t, e.d.count("pin/rm"))
	assert.True(t, e.pins.has(e.target))
}

func TestHTTPPrincipalKindsAreScopedToRoutes(t *testing.T) {
	e := newHTTPEnv(t, nil)
	// The A2A peer's token is valid, but not for the MCP route (G9-4).
	resp := e.do(t, rawReq{token: tokPeer, body: initializeBody})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "not accepted")
	// The chat island's token is.
	assert.Equal(t, http.StatusOK, e.do(t, rawReq{token: tokChat, body: initializeBody}).StatusCode)
}

func TestHTTPSessionCannotBeUsedByAnotherPrincipal(t *testing.T) {
	e := newHTTPEnv(t, nil)
	first := e.do(t, rawReq{token: tokReader, body: initializeBody})
	require.Equal(t, http.StatusOK, first.StatusCode)
	sid := first.Header.Get("Mcp-Session-Id")
	require.NotEmpty(t, sid)

	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	hdr := map[string]string{"Mcp-Session-Id": sid, "MCP-Protocol-Version": "2025-06-18"}
	hijack := e.do(t, rawReq{token: tokOther, body: listBody, header: hdr})
	assert.Equal(t, http.StatusForbidden, hijack.StatusCode)
	own := e.do(t, rawReq{token: tokReader, body: listBody, header: hdr})
	assert.Equal(t, http.StatusOK, own.StatusCode)
}

// TestHTTPConfirmTokenIsBoundToSessionAndPrincipal is task 1.3 end to end: a
// confirm token minted for a call in one HTTP session is refused in another
// session of the same principal, and for another principal, and still works in
// the session that asked.
func TestHTTPConfirmTokenIsBoundToSessionAndPrincipal(t *testing.T) {
	e := newHTTPEnv(t, nil)
	args := map[string]any{"path": "/ipfs/" + testCID(t, 7)}
	sessionA := e.client(t, tokWriter, nil)
	sessionB := e.client(t, tokWriter, nil)
	otherPrincipal := e.client(t, tokOther, nil)

	required := confirmationOf(t, toolCall(t, sessionA, "pin_add", args), CodeConfirmRequired)
	require.NotNil(t, required.Confirmation)
	token := required.Confirmation.ConfirmToken
	require.NotEmpty(t, token, "a human-mcp principal gets a binding handle")
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))

	withToken := func(cs *mcp.ClientSession) *mcp.CallToolResult {
		a := map[string]any{"confirm_token": token}
		for k, v := range args {
			a[k] = v
		}
		return toolCall(t, cs, "pin_add", a)
	}
	inB := confirmationOf(t, withToken(sessionB), CodeConfirmInvalid)
	assert.Contains(t, inB.Message, "another session")
	inOther := confirmationOf(t, withToken(otherPrincipal), CodeConfirmInvalid)
	assert.True(t, strings.Contains(inOther.Message, "another session") || strings.Contains(inOther.Message, "another principal"), inOther.Message)
	assert.Zero(t, e.d.count("pin/add"), "a refused token ran nothing")

	ok := withToken(sessionA)
	require.False(t, ok.IsError, "%+v", ok.Content)
	assert.Equal(t, 1, e.d.count("pin/add"))
}

func TestHTTPAnonymousLoopbackIsReadOnly(t *testing.T) {
	e := newHTTPEnv(t, func(o *HTTPOptions) { o.Verifier = nil })
	cs := e.client(t, "", nil)
	cat, err := contract.Load()
	require.NoError(t, err)
	for _, n := range httpToolNames(t, cs) {
		tool, _ := cat.Tool(n)
		assert.Equal(t, contract.ClassNone, tool.Confirmation, n)
	}
	confirmationOf(t, toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 9)}), CodePermissionDenied)
	assert.Zero(t, e.d.count("pin/add"))
}

func TestHTTPListenerRefusals(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{})
	for _, listen := range []string{"0.0.0.0:0", "192.0.2.10:0", "[::]:0", "example.com:0", "/ip4/0.0.0.0/tcp/0"} {
		t.Run("no verifier "+listen, func(t *testing.T) {
			_, err := NewHTTPServer(s, HTTPOptions{Listen: listen})
			require.ErrorIs(t, err, errNonLoopbackWithoutAuth)
		})
	}
	// With a verifier a non-loopback bind is allowed (it is not opened here).
	_, err := NewHTTPServer(s, HTTPOptions{Listen: "0.0.0.0:0", Verifier: testVerifier(t)})
	require.NoError(t, err)

	for _, listen := range []string{"127.0.0.1:4000", "127.0.0.1:5001", "localhost:8080", "/ip4/127.0.0.1/tcp/11434", "127.0.0.1:99999", "127.0.0.1", ":0"} {
		_, err := NewHTTPServer(s, HTTPOptions{Listen: listen})
		require.Error(t, err, listen)
	}
	_, err = NewHTTPServer(s, HTTPOptions{Listen: "127.0.0.1:0", AllowedOrigins: []string{"evil.example"}})
	require.Error(t, err, "an origin needs a scheme")
	_, err = NewHTTPServer(s, HTTPOptions{Listen: "127.0.0.1:0", MaxBodyBytes: -1})
	require.Error(t, err)
}

func TestHTTPDefaultsMatchTheContract(t *testing.T) {
	l, err := ParseListen("")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:5050", l.String())
	assert.Equal(t, DefaultHTTPListen, l.String())
	assert.True(t, l.IsLoopback())
	assert.EqualValues(t, 4194304, DefaultMaxBodyBytes)
	assert.Equal(t, "30m0s", DefaultSessionTimeout.String())
}

func TestHTTPDisabledToolIsNotOffered(t *testing.T) {
	d := newFakeDaemon(t)
	s := mustNew(t, d, Options{Tools: []string{"pin"}, Disabled: []string{"pin_ls"}})
	assert.NotContains(t, s.ToolNames(), "pin_ls")
	assert.Contains(t, s.ToolNames(), "pin_add")
}
