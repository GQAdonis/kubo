package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tokens given to the ipfs mcp process through its environment. They are test
// values only.
const (
	mcpHTTPReaderToken = "cli-reader-token-0123456789"
	mcpHTTPAdminToken  = "cli-admin-token-01234567890"
	mcpHTTPPeerToken   = "cli-a2a-peer-token-012345678"
	mcpHTTPOtherToken  = "cli-other-token-012345678901"
	// mcpHTTPSettingsToken is the token of an admin principal (kind admin,
	// scoped to the settings route). The process gets it only when the test
	// passes CLI_MCP_SETTINGS_TOKEN in extraEnv.
	mcpHTTPSettingsToken = "cli-settings-admin-token-0123"
)

// mcpHTTPConfig is the config file the tests pass with --mcp-config: a read-only
// principal, a principal granted write and destructive pin, and an A2A peer.
// Credentials are env references, never values.
const mcpHTTPConfig = `{
  "http": {"allowed_origins": ["http://app.example"]},
  "principals": [
    {"name": "reader", "kind": "human-mcp", "credential": {"env": "CLI_MCP_READER_TOKEN"}},
    {"name": "admin", "kind": "human-mcp", "credential": {"env": "CLI_MCP_ADMIN_TOKEN"},
     "write_groups": ["pin"], "destructive_groups": ["pin"]},
    {"name": "peer", "kind": "a2a", "credential": {"env": "CLI_MCP_PEER_TOKEN"}, "groups": ["node", "pin"]},
    {"name": "other", "kind": "human-mcp", "credential": {"env": "CLI_MCP_OTHER_TOKEN"}, "write_groups": ["pin"]}
  ]
}`

var mcpHTTPListening = regexp.MustCompile(`serving MCP over HTTP on (http://\S+)`)

// mcpHTTPProc is a running `ipfs mcp --http` process.
type mcpHTTPProc struct {
	t      *testing.T
	cmd    *exec.Cmd
	stderr *syncBuffer
	// base is http://host:port, without the route.
	base string
}

// startMCPHTTP starts `ipfs mcp --http=127.0.0.1:0 args...` for the repo of
// node. The OS chooses the port, and the process reports it on stderr, so no
// test binds a fixed port.
func startMCPHTTP(t *testing.T, node *harness.Node, extraEnv []string, args ...string) *mcpHTTPProc {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	require.NoError(t, os.WriteFile(cfg, []byte(mcpHTTPConfig), 0o600))
	return startMCPHTTPArgs(t, node, extraEnv, append([]string{"--mcp-config", cfg}, args...)...)
}

// startMCPHTTPArgs is startMCPHTTP without the shared config file.
func startMCPHTTPArgs(t *testing.T, node *harness.Node, extraEnv []string, args ...string) *mcpHTTPProc {
	t.Helper()
	return startMCPHTTPBin(t, node.IPFSBin, node, extraEnv, args...)
}

// startMCPHTTPBin is startMCPHTTPArgs for a given ipfs binary (for example the
// test-resolver build).
func startMCPHTTPBin(t *testing.T, bin string, node *harness.Node, extraEnv []string, args ...string) *mcpHTTPProc {
	t.Helper()
	full := append([]string{"mcp", "--http=127.0.0.1:0"}, args...)
	cmd := exec.Command(bin, full...)
	cmd.Env = append(mcpEnv(node.Dir),
		"CLI_MCP_READER_TOKEN="+mcpHTTPReaderToken,
		"CLI_MCP_ADMIN_TOKEN="+mcpHTTPAdminToken,
		"CLI_MCP_PEER_TOKEN="+mcpHTTPPeerToken,
		"CLI_MCP_OTHER_TOKEN="+mcpHTTPOtherToken,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Dir = node.Dir
	p := &mcpHTTPProc{t: t, cmd: cmd, stderr: &syncBuffer{}}
	cmd.Stderr = p.stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(mcpTimeout)
	for time.Now().Before(deadline) {
		if m := mcpHTTPListening.FindStringSubmatch(p.stderr.String()); m != nil {
			p.base = strings.TrimSuffix(m[1], "/mcp")
			return p
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ipfs mcp --http did not report its address; stderr: %s", p.stderr.String())
	return nil
}

// bearerRoundTripper adds a bearer token to every request.
type bearerRoundTripper struct{ token string }

func (b bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// client connects a go-sdk client over Streamable HTTP.
func (p *mcpHTTPProc) client(token string, co *mcp.ClientOptions) (*mcp.ClientSession, error) {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(p.t.Context(), mcpTimeout)
	p.t.Cleanup(cancel)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   p.base + "/mcp",
		HTTPClient: &http.Client{Transport: bearerRoundTripper{token: token}},
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "kubo-cli-http-test", Version: "0"}, co).Connect(ctx, transport, nil)
	if err == nil {
		p.t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

func (p *mcpHTTPProc) mustClient(token string, co *mcp.ClientOptions) *mcp.ClientSession {
	p.t.Helper()
	cs, err := p.client(token, co)
	require.NoError(p.t, err, "stderr: %s", p.stderr.String())
	return cs
}

// mcpHTTPRequest is one raw HTTP request.
type mcpHTTPRequest struct {
	method, path, token, host, body string
	header                          map[string]string
}

func (p *mcpHTTPProc) status(r mcpHTTPRequest) int {
	p.t.Helper()
	if r.method == "" {
		r.method = http.MethodPost
	}
	if r.path == "" {
		r.path = "/mcp"
	}
	req, err := http.NewRequestWithContext(p.t.Context(), r.method, p.base+r.path, strings.NewReader(r.body))
	require.NoError(p.t, err)
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
	require.NoError(p.t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

const mcpHTTPInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"raw","version":"0"}}}`

func mcpListedTools(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestMCPHTTP(t *testing.T) {
	t.Parallel()

	t.Run("rejects missing and wrong tokens, rebinding and cross-origin requests", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		p := startMCPHTTP(t, node, nil)

		for _, tc := range []struct {
			name string
			req  mcpHTTPRequest
			want int
		}{
			{"no token", mcpHTTPRequest{body: mcpHTTPInitialize}, http.StatusUnauthorized},
			{"wrong token", mcpHTTPRequest{token: "wrong-token-0123456789", body: mcpHTTPInitialize}, http.StatusUnauthorized},
			{"valid token", mcpHTTPRequest{token: mcpHTTPReaderToken, body: mcpHTTPInitialize}, http.StatusOK},
			{"token of another route's principal", mcpHTTPRequest{token: mcpHTTPPeerToken, body: mcpHTTPInitialize}, http.StatusForbidden},
			{"dns rebinding", mcpHTTPRequest{token: mcpHTTPReaderToken, host: "evil.example", body: mcpHTTPInitialize}, http.StatusForbidden},
			{"cross-origin page", mcpHTTPRequest{token: mcpHTTPReaderToken, header: map[string]string{"Origin": "https://evil.example"}, body: mcpHTTPInitialize}, http.StatusForbidden},
			{"cross-site fetch", mcpHTTPRequest{token: mcpHTTPReaderToken, header: map[string]string{"Sec-Fetch-Site": "cross-site"}, body: mcpHTTPInitialize}, http.StatusForbidden},
			{"allowlisted origin", mcpHTTPRequest{token: mcpHTTPReaderToken, header: map[string]string{"Origin": "http://app.example"}, body: mcpHTTPInitialize}, http.StatusOK},
			{"agent routes are not mounted", mcpHTTPRequest{path: "/a2a", token: mcpHTTPPeerToken, body: "{}"}, http.StatusNotFound},
			{"agent card is not mounted", mcpHTTPRequest{method: http.MethodGet, path: "/.well-known/agent-card.json"}, http.StatusNotFound},
		} {
			assert.Equal(t, tc.want, p.status(tc.req), tc.name)
		}

		_, err := p.client("", nil)
		require.Error(t, err, "a client without a token cannot start a session")
	})

	t.Run("a principal is read-only until a group is granted by name", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		cid := node.IPFSAddStr("http principal scopes")
		p := startMCPHTTP(t, node, nil, "--tools", "pin,destructive")

		reader := p.mustClient(mcpHTTPReaderToken, nil)
		assert.ElementsMatch(t, []string{"pin_ls", "pin_verify"}, filterPinTools(mcpListedTools(t, reader)))
		res := callMCP(t, reader, "pin_add", map[string]any{"path": "/ipfs/" + cid})
		assert.Equal(t, "permission_denied", mcpToolErrorCode(t, res))
		res = callMCP(t, reader, "pin_rm", map[string]any{"path": "/ipfs/" + cid})
		assert.Equal(t, "permission_denied", mcpToolErrorCode(t, res))
		assert.Contains(t, node.IPFS("pin", "ls", "--type=recursive", "-q").Stdout.String(), cid, "nothing was removed")

		admin := p.mustClient(mcpHTTPAdminToken, nil)
		assert.Contains(t, mcpListedTools(t, admin), "pin_rm", "destructive_groups lists the destructive tool")
		other := p.mustClient(mcpHTTPOtherToken, nil)
		assert.NotContains(t, mcpListedTools(t, other), "pin_rm", "write_groups alone does not")
		assert.Contains(t, mcpListedTools(t, other), "pin_add")
	})

	t.Run("lists the same tools as stdio for the same flags", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		tokenFile := filepath.Join(t.TempDir(), "operator.token")
		require.NoError(t, os.WriteFile(tokenFile, []byte(mcpHTTPAdminToken+"\n"), 0o600))
		// --auth-token-file grants the operator every group.
		p := startMCPHTTPArgs(t, node, nil, "--auth-token-file", tokenFile)

		stdio := startMCP(t, node.IPFSBin, node.Dir)
		stdio.initialize()
		want := stdio.toolNames()
		assert.Equal(t, 0, stdio.closeAndWait(), "stderr: %s", stdio.stderr.String())

		operator := p.mustClient(mcpHTTPAdminToken, nil)
		assert.ElementsMatch(t, want, mcpListedTools(t, operator))

		overHTTP, err := operator.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{}})
		require.NoError(t, err)
		require.False(t, overHTTP.IsError, "%+v", overHTTP.Content)
		structured, ok := overHTTP.StructuredContent.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, node.PeerID().String(), structured["peer_id"])
		status := callMCP(t, operator, "ipfs_node_status", map[string]any{})
		assert.Equal(t, "http", status.StructuredContent.(map[string]any)["transport"])
	})

	t.Run("a destructive call is confirmed through elicitation over HTTP", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		cid := node.IPFSAddStr("http destructive confirmation")
		p := startMCPHTTP(t, node, nil, "--tools", "pin,destructive")
		pinned := func() bool {
			return strings.Contains(node.IPFS("pin", "ls", "--type=recursive", "-q").Stdout.String(), cid)
		}
		require.True(t, pinned())

		// Declined: nothing changes.
		no := &elicitor{action: "decline"}
		cs := p.mustClient(mcpHTTPAdminToken, no.options())
		res := callMCP(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + cid})
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, res))
		require.Len(t, no.prompts, 1)
		assert.True(t, pinned(), "a declined confirmation leaves the pin")

		// A client that cannot elicit and has no out-of-band prompt is refused.
		silent := p.mustClient(mcpHTTPAdminToken, nil)
		res = callMCP(t, silent, "pin_rm", map[string]any{"path": "/ipfs/" + cid})
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, res))
		assert.True(t, pinned())

		// Accepted by the user: the pin is removed, once.
		yes := &elicitor{action: "accept"}
		cs = p.mustClient(mcpHTTPAdminToken, yes.options())
		res = callMCP(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + cid})
		require.False(t, res.IsError, "%+v", res.Content)
		require.Len(t, yes.prompts, 1)
		assert.Contains(t, yes.prompts[0], cid, "the prompt names what will be removed")
		assert.False(t, pinned(), "the confirmed call ran")
	})

	t.Run("refuses a non-loopback address without a token and reserved ports", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		_, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--http=0.0.0.0:0")
		assert.NotEqual(t, 0, code)
		assert.Contains(t, stderr, "without authentication")

		for _, addr := range []string{"127.0.0.1:4000", "127.0.0.1:5001", "/ip4/127.0.0.1/tcp/8080"} {
			_, stderr, code = runMCP(t, node.IPFSBin, node.Dir, "--http="+addr)
			assert.NotEqual(t, 0, code, addr)
			assert.Contains(t, stderr, "reserved", addr)
		}
	})

	t.Run("rejects a config with a literal credential", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg := filepath.Join(dir, "bad.json")
		require.NoError(t, os.WriteFile(cfg, []byte(`{"principals":[{"name":"ui","kind":"ui","credential":"s3cr3t-token-value"}]}`), 0o600))
		node := harness.NewT(t).NewNode().Init()
		_, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--http=127.0.0.1:0", "--mcp-config", cfg)
		assert.NotEqual(t, 0, code)
		assert.Contains(t, stderr, "credential")
		assert.NotContains(t, stderr, "s3cr3t-token-value", "the error never repeats a secret")
	})

	t.Run("prints least-privilege RPC paths", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()
		entry := func(args ...string) (paths []string) {
			stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir, append([]string{"--print-rpc-allowed-paths"}, args...)...)
			require.Equal(t, 0, code, "stderr: %s", stderr)
			var parsed struct{ AllowedPaths []string }
			require.NoError(t, json.Unmarshal([]byte(stdout), &parsed), "stdout: %s", stdout)
			return parsed.AllowedPaths
		}
		readOnly := entry("--read-only")
		assert.Contains(t, readOnly, "/api/v0/pin/ls")
		assert.NotContains(t, readOnly, "/api/v0/pin/add")
		assert.NotContains(t, readOnly, "/api/v0/pin/rm")
		assert.NotContains(t, readOnly, "/api/v0/add")

		all := entry()
		assert.Contains(t, all, "/api/v0/pin/add")
		assert.NotContains(t, all, "/api/v0/pin/rm", "destructive tools are off by default")
		assert.Contains(t, entry("--tools", "pin,destructive"), "/api/v0/pin/rm")
	})

	t.Run("does not open a listener without --http", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		p := startMCP(t, node.IPFSBin, node.Dir)
		p.initialize()
		assert.NotContains(t, p.stderr.String(), "serving MCP over HTTP")
		assert.Equal(t, 0, p.closeAndWait())
	})
}

// filterPinTools keeps the pin_* tool names.
func filterPinTools(names []string) []string {
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, "pin_") {
			out = append(out, n)
		}
	}
	return out
}
