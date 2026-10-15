package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-agent-principal-enforcement. They run the built
// ipfs binary against a daemon with an isolated IPFS_PATH. Every listener
// (ipfs mcp and the fake UAR) binds an OS-chosen loopback port.

// The contract pins no UAR settings endpoint until a published UAR release
// documents one (ADR-0007), so the built binary never reads or probes UAR
// settings. The fake serves only the agent card and records every path it was
// asked for, which lets the tests prove that. The verdicts that depend on the
// settings content are covered by the unit tests in mcp/server.
const (
	mcpFakeUARAdminToken = "cli-uar-admin-secret-0123456789"
	mcpFakeUARAdminEnv   = "CLI_UAR_ADMIN_TOKEN"
)

// mcpFakeUAR is an httptest stand-in for a UAR instance.
type mcpFakeUAR struct {
	srv *httptest.Server

	mu      sync.Mutex
	version string
	paths   map[string]int
	hits    int
}

func newMCPFakeUAR(t *testing.T) *mcpFakeUAR {
	t.Helper()
	u := &mcpFakeUAR{version: "1.4.0", paths: map[string]int{}}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		defer u.mu.Unlock()
		u.hits++
		u.paths[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/agent-card.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "fake uar", "version": u.version})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *mcpFakeUAR) cardURL() string { return u.srv.URL + "/.well-known/agent-card.json" }

func (u *mcpFakeUAR) edit(fn func(u *mcpFakeUAR)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fn(u)
}

func (u *mcpFakeUAR) requests() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits
}

// onlyCardRead reports whether the only path ever requested is the agent card.
func (u *mcpFakeUAR) onlyCardRead() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	for p := range u.paths {
		if p != "/.well-known/agent-card.json" {
			return false
		}
	}
	return true
}

// pathHits reports how many requests named path.
func (u *mcpFakeUAR) pathHits(path string) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.paths[path]
}

// principalConfig writes an ipfs mcp config file and returns its path.
func principalConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp-principals.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// noAmbientConfig keeps a developer's IPFS_MCP_CONFIG out of the test.
var noAmbientConfig = []string{"IPFS_MCP_CONFIG="}

// runCheckUAR runs `ipfs mcp check-uar` with extra environment variables.
func runCheckUAR(t *testing.T, node *harness.Node, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), mcpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, node.IPFSBin, append([]string{"mcp", "check-uar"}, args...)...)
	cmd.Env = append(append(mcpEnv(node.Dir), noAmbientConfig...), env...)
	cmd.Dir = node.Dir
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr, "running ipfs mcp check-uar: %v", err)
	}
	return out.String(), errOut.String(), cmd.ProcessState.ExitCode()
}

// namedClient connects over HTTP as an MCP client with the given name.
func (p *mcpHTTPProc) namedClient(token, name string) (*mcp.ClientSession, error) {
	p.t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:   p.base + "/mcp",
		HTTPClient: &http.Client{Transport: bearerRoundTripper{token: token}},
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: name, Version: "0"}, nil).Connect(p.t.Context(), transport, nil)
	if err == nil {
		p.t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

func TestMCPPrincipalUnconfiguredGetsNothing(t *testing.T) {
	t.Parallel()

	t.Run("a token that maps to no configured principal gets 401 and no session", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		p := startMCPHTTP(t, node, noAmbientConfig)
		for _, token := range []string{"valid-format-but-unowned-token-0123", mcpHTTPPeerToken + "x", ""} {
			assert.Equal(t, http.StatusUnauthorized, p.status(mcpHTTPRequest{token: token, body: mcpHTTPInitialize}), "token %q", token)
		}
		_, err := p.client("valid-format-but-unowned-token-0123", nil)
		require.Error(t, err)
		// A configured principal is unaffected.
		assert.NotEmpty(t, mcpListedTools(t, p.mustClient(mcpHTTPReaderToken, nil)))
	})

	t.Run("without --mcp-config and IPFS_MCP_CONFIG no ui, a2a or uar principal exists and nothing is checked", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		uar := newMCPFakeUAR(t)
		p := startMCPHTTPArgs(t, node, noAmbientConfig)

		// No credential is configured, so there is no principal to log in as:
		// every caller is the anonymous read-only principal.
		assert.Equal(t, http.StatusOK, p.status(mcpHTTPRequest{token: "any-token-at-all-0123456789", body: mcpHTTPInitialize}))
		cs := p.mustClient("", nil)
		cat := mcpCatalog(t)
		listed := mcpListedTools(t, cs)
		require.NotEmpty(t, listed)
		for _, name := range listed {
			tool, ok := cat.Tool(name)
			require.True(t, ok, name)
			assert.Equal(t, "none", tool.Confirmation, "%s is not read-only", name)
		}
		// The agent routes of ui, a2a and uar principals are not mounted.
		for _, path := range []string{"/a2a", "/agui", "/.well-known/agent-card.json"} {
			assert.Equal(t, http.StatusNotFound, p.status(mcpHTTPRequest{path: path, token: mcpHTTPPeerToken, body: "{}"}), path)
		}
		assert.NotContains(t, p.stderr.String(), "check-uar")
		assert.Zero(t, uar.requests(), "no check-uar connection is made unless a uar principal is configured")
	})
}

func TestMCPPrincipalEgressAckIsRequiredForContentTools(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	cid := node.IPFSAddStr("egress acknowledgement content")

	cfg := principalConfig(t, `{"principals":[
	  {"name":"noack","kind":"human-mcp","credential":{"env":"CLI_MCP_READER_TOKEN"}},
	  {"name":"acked","kind":"human-mcp","credential":{"env":"CLI_MCP_ADMIN_TOKEN"},"egress_ack":["local"]}]}`)
	p := startMCPHTTPArgs(t, node, noAmbientConfig, "--mcp-config", cfg)

	noack := p.mustClient(mcpHTTPReaderToken, nil)
	listed := mcpListedTools(t, noack)
	var egress []string
	for _, name := range listed {
		if tool, ok := mcpCatalog(t).Tool(name); ok && tool.ContentEgress {
			egress = append(egress, name)
		}
	}
	require.Contains(t, egress, "ipfs_cat")
	require.Greater(t, len(egress), 4, "the catalog marks several tools contentEgress")
	for _, name := range egress {
		res := callMCP(t, noack, name, map[string]any{})
		assert.Equal(t, "egress_not_acknowledged", mcpToolErrorCode(t, res), name)
		requireMatchesCommonToolError(t, res)
	}

	acked := p.mustClient(mcpHTTPAdminToken, nil)
	res := callMCP(t, acked, "ipfs_cat", map[string]any{"path": "/ipfs/" + cid})
	require.False(t, res.IsError, "%+v", res.StructuredContent)
	assert.Contains(t, asString(res.StructuredContent), "egress acknowledgement content")
}

// requireMatchesCommonToolError checks that an error result carries the
// contract's ToolError shape.
func requireMatchesCommonToolError(t *testing.T, res *mcp.CallToolResult) {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.NotEmpty(t, out.Error.Code)
	assert.NotEmpty(t, out.Error.Message)
	assert.True(t, res.IsError)
	require.NotEmpty(t, res.Content)
}

func asString(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestMCPPrincipalCheckUAR(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init()
	env := []string{mcpFakeUARAdminEnv + "=" + mcpFakeUARAdminToken}

	t.Run("verify reads the pinned effective-settings path and refuses what it cannot read", func(t *testing.T) {
		t.Parallel()
		u := newMCPFakeUAR(t)
		stdout, stderr, code := runCheckUAR(t, node, env, u.cardURL(),
			"--admin-credential-env", mcpFakeUARAdminEnv, "--min-version", "1.4.0")
		assert.NotEqual(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
		assert.Contains(t, stdout, "[refuse] settings_unreadable")
		assert.Contains(t, stdout, "[refuse] auth_probe_inconclusive")
		assert.Contains(t, stdout, "verdict: refuse")
		assert.NotContains(t, stdout+stderr, mcpFakeUARAdminToken, "the admin credential is never printed")
		assert.False(t, u.onlyCardRead(), "the pinned effective-settings path is read (v1.1.0)")
		assert.Positive(t, u.pathHits("/api/uar/settings/effective"))
	})

	t.Run("assert records both as info and accepts a published version", func(t *testing.T) {
		t.Parallel()
		u := newMCPFakeUAR(t)
		// The base URL works as well as the card URL.
		stdout, stderr, code := runCheckUAR(t, node, env, u.srv.URL,
			"--min-version", "1.4.0", "--check", "assert", "--mcp-config", principalConfig(t, `{"http":{"listen":"127.0.0.1:5050"}}`))
		assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
		assert.Contains(t, stdout, "[info] settings_asserted")
		assert.Contains(t, stdout, "[info] auth_probe_inconclusive")
		assert.Contains(t, stdout, "check: assert")
		assert.Contains(t, stdout, "verdict: ok")
	})

	t.Run("the version check runs under assert", func(t *testing.T) {
		t.Parallel()
		u := newMCPFakeUAR(t)
		u.edit(func(u *mcpFakeUAR) { u.version = "1.3.0" })
		stdout, _, code := runCheckUAR(t, node, env, u.cardURL(), "--min-version", "1.4.0", "--check", "assert")
		assert.NotEqual(t, 0, code)
		assert.Contains(t, stdout, "version_too_old")

		// No --min-version: the contract's pinned minimum (1.1.0) applies. A card
		// older than it is refused, and min_version_not_set is gone.
		u.edit(func(u *mcpFakeUAR) { u.version = "1.0.0" })
		stdout, _, code = runCheckUAR(t, node, env, u.cardURL(), "--check", "assert")
		assert.NotEqual(t, 0, code)
		assert.Contains(t, stdout, "version_too_old")
		assert.NotContains(t, stdout, "min_version_not_set")
	})

	t.Run("an unpublished version is refused", func(t *testing.T) {
		t.Parallel()
		u := newMCPFakeUAR(t)
		u.edit(func(u *mcpFakeUAR) { u.version = "1.5.0-rc1" })
		stdout, _, code := runCheckUAR(t, node, env, u.cardURL(), "--min-version", "1.4.0", "--check", "assert")
		assert.NotEqual(t, 0, code)
		assert.Contains(t, stdout, "version_not_published_release")
	})

	t.Run("there is no default address", func(t *testing.T) {
		t.Parallel()
		u := newMCPFakeUAR(t)
		_, stderr, code := runCheckUAR(t, node, env)
		assert.NotEqual(t, 0, code)
		assert.Contains(t, stderr, "no default address")
		assert.Zero(t, u.requests(), "nothing was contacted")
	})
}

func TestMCPPrincipalUARSessionChecks(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	u := newMCPFakeUAR(t)
	cfg := principalConfig(t, `{"principals":[
	  {"name":"reader","kind":"human-mcp","credential":{"env":"CLI_MCP_READER_TOKEN"}},
	  {"name":"runtime","kind":"uar","credential":{"env":"CLI_MCP_OTHER_TOKEN"},
	   "write_groups":["pin"],"egress_ack":["local"],"check":"assert",
	   "uar":{"card_url":"`+u.cardURL()+`","admin_credential":{"env":"`+mcpFakeUARAdminEnv+`"},"min_version":"1.4.0"}}]}`)
	p := startMCPHTTPArgs(t, node, append([]string{mcpFakeUARAdminEnv + "=" + mcpFakeUARAdminToken}, noAmbientConfig...), "--mcp-config", cfg)

	// The startup check ran and printed its outcome, and the assertion the
	// operator made with check: assert.
	assert.Contains(t, p.stderr.String(), "principal runtime: check-uar ok")
	assert.Contains(t, p.stderr.String(), "principal runtime: check: assert is set")
	assert.Contains(t, p.stderr.String(), "the operator asserts")
	assert.NotContains(t, p.stderr.String(), mcpFakeUARAdminToken)

	// Under assert a published version is enough; settings are not read.
	cs, err := p.namedClient(mcpHTTPOtherToken, "uar")
	require.NoError(t, err, "stderr: %s", p.stderr.String())
	assert.Contains(t, mcpListedTools(t, cs), "pin_add")
	assert.Positive(t, u.pathHits("/api/uar/settings/effective"), "the pinned effective-settings path is read; under assert its failure is only a finding")

	// The instance is downgraded after startup: the next session initialize is
	// refused, for that principal only.
	u.edit(func(u *mcpFakeUAR) { u.version = "1.3.0" })
	_, err = p.namedClient(mcpHTTPOtherToken, "uar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version_too_old")
	assert.NotEmpty(t, mcpListedTools(t, p.mustClient(mcpHTTPReaderToken, nil)), "other principals keep running")
}

// TestMCPPrincipalUARVerifyIsRefusedWhenSettingsAreUnreadable: v1.1.0 pins the
// effective-settings path, so verify reads it; this instance serves nothing
// there, and the principal is refused (fail closed).
func TestMCPPrincipalUARVerifyIsRefusedWhenSettingsAreUnreadable(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	u := newMCPFakeUAR(t)
	cfg := principalConfig(t, `{"principals":[
	  {"name":"reader","kind":"human-mcp","credential":{"env":"CLI_MCP_READER_TOKEN"}},
	  {"name":"runtime","kind":"uar","credential":{"env":"CLI_MCP_OTHER_TOKEN"},"write_groups":["pin"],"egress_ack":["local"],
	   "uar":{"card_url":"`+u.cardURL()+`","min_version":"1.4.0"}}]}`)
	p := startMCPHTTPArgs(t, node, noAmbientConfig, "--mcp-config", cfg)

	assert.Contains(t, p.stderr.String(), "principal runtime: check-uar refuse")
	_, err := p.namedClient(mcpHTTPOtherToken, "uar")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "settings_unreadable")
	assert.Positive(t, u.pathHits("/api/uar/settings/effective"))
	assert.NotEmpty(t, mcpListedTools(t, p.mustClient(mcpHTTPReaderToken, nil)), "other principals keep running")
}

func TestMCPPrincipalStdioClientIdentifyingAsUARIsRefusedContentTools(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	cid := node.IPFSAddStr("stdio uar guard content")

	call := func(clientName string) *mcp.CallToolResult {
		t.Helper()
		cmd := exec.Command(node.IPFSBin, "mcp")
		cmd.Env = append(mcpEnv(node.Dir), noAmbientConfig...)
		cmd.Dir = node.Dir
		cs, err := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "0"}, nil).
			Connect(t.Context(), &mcp.CommandTransport{Command: cmd}, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = cs.Close() })
		return callMCP(t, cs, "ipfs_cat", map[string]any{"path": "/ipfs/" + cid})
	}
	res := call("uar")
	assert.Equal(t, "egress_not_acknowledged", mcpToolErrorCode(t, res))
	assert.NotContains(t, asString(res.StructuredContent), "stdio uar guard content")

	res = call("claude-code")
	require.False(t, res.IsError, "%+v", res.StructuredContent)
}

func TestMCPPrincipalShellCapableIsRequiredUnderConfirmCLI(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	cfg := principalConfig(t, `{"principals":[
	  {"name":"unasserted","kind":"human-mcp","credential":{"env":"CLI_MCP_READER_TOKEN"},"write_groups":["pin"]},
	  {"name":"asserted","kind":"human-mcp","credential":{"env":"CLI_MCP_ADMIN_TOKEN"},"write_groups":["pin"],"shell_capable":false}]}`)
	sockDir, err := os.MkdirTemp("", "ms")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	p := startMCPHTTPArgs(t, node, append([]string{"IPFS_MCP_CONFIRM_DIR=" + sockDir}, noAmbientConfig...),
		"--mcp-config", cfg, "--confirm=cli")

	assert.Contains(t, p.stderr.String(), "principal unasserted: write groups refused")
	assert.NotContains(t, mcpListedTools(t, p.mustClient(mcpHTTPReaderToken, nil)), "pin_add")
	assert.Contains(t, mcpListedTools(t, p.mustClient(mcpHTTPAdminToken, nil)), "pin_add")

	res := callMCP(t, p.mustClient(mcpHTTPReaderToken, nil), "pin_add", map[string]any{"path": "/ipfs/" + node.IPFSAddStr("shell capable")})
	assert.Equal(t, "permission_denied", mcpToolErrorCode(t, res))
	assert.Contains(t, strings.ToLower(asString(res.StructuredContent)), "shell_capable")
}

func TestMCPPrincipalOperatorTokenIsNotExemptUnderConfirmCLI(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	sockDir, err := os.MkdirTemp("", "ms")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	p := startMCPHTTPArgs(t, node, append([]string{"IPFS_MCP_CONFIRM_DIR=" + sockDir}, noAmbientConfig...),
		"--auth-token-env", "CLI_MCP_ADMIN_TOKEN", "--confirm=cli")

	// The startup message names the supported path.
	startup := p.stderr.String()
	assert.Contains(t, startup, "principal operator: write groups refused")
	assert.Contains(t, startup, "human-mcp")
	assert.Contains(t, startup, "write_groups")
	assert.Contains(t, startup, "shell_capable: false")

	cs := p.mustClient(mcpHTTPAdminToken, nil)
	assert.NotContains(t, mcpListedTools(t, cs), "pin_add")
	res := callMCP(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + node.IPFSAddStr("operator shell")})
	assert.Equal(t, "permission_denied", mcpToolErrorCode(t, res))
	refusal := asString(res.StructuredContent)
	assert.Contains(t, refusal, "human-mcp")
	assert.Contains(t, refusal, "write_groups")
}
