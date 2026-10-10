package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpTimeout = 60 * time.Second

// mcpCatalog is the contract catalog the expected tool sets derive from.
func mcpCatalog(t *testing.T) *contract.Catalog {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	return cat
}

// implementedMCPTools lists the tools the built binary registers when every
// group and the destructive selector are enabled. The catalog can define tools
// a build does not implement yet; expectations are limited to those it does.
func implementedMCPTools(t *testing.T, node *harness.Node) []string {
	t.Helper()
	cat := mcpCatalog(t)
	selectors := make([]string, 0, len(cat.Groups)+1)
	for _, g := range cat.Groups {
		selectors = append(selectors, g.Name)
	}
	selectors = append(selectors, cat.ToolSelection.DestructiveSelector)
	p := startMCP(t, node.IPFSBin, node.Dir, "--tools", strings.Join(selectors, ","))
	p.initialize()
	names := p.toolNames()
	assert.Equal(t, 0, p.closeAndWait())
	return names
}

// expectedMCPTools derives the tool set a selection offers from the contract:
// the catalog selection, limited to what this build implements.
func expectedMCPTools(t *testing.T, node *harness.Node, selectors []string, readOnly bool) []string {
	t.Helper()
	implemented := implementedMCPTools(t, node)
	return slices.DeleteFunc(mcpCatalog(t).SelectTools(selectors, readOnly), func(n string) bool {
		return !slices.Contains(implemented, n)
	})
}

// defaultMCPTools is the tool set of a default session: every default group
// with confirmation classes none and write, so no destructive tool.
func defaultMCPTools(t *testing.T, node *harness.Node) []string {
	t.Helper()
	return expectedMCPTools(t, node, mcpCatalog(t).ToolSelection.Default.Groups, false)
}

// requireNoDestructive fails if any named tool is destructive per the contract.
func requireNoDestructive(t *testing.T, names []string) {
	t.Helper()
	cat := mcpCatalog(t)
	for _, n := range names {
		def, ok := cat.Tool(n)
		require.True(t, ok, n)
		assert.NotEqual(t, contract.ClassDestructive, def.Confirmation, "%s is destructive and off by default", n)
	}
}

// syncBuffer is a bytes.Buffer safe for use by a process and a test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// mcpEnv returns the environment for an ipfs process that uses repo dir.
func mcpEnv(repoDir string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "IPFS_PATH=") && !strings.HasPrefix(kv, "IPFS_MCP_CONFIRM=") {
			env = append(env, kv)
		}
	}
	// The native confirmation prompt is on by default; tests must never raise
	// a Touch ID or polkit dialog, so they turn the resolvers off.
	return append(env, "IPFS_PATH="+repoDir, "IPFS_MCP_CONFIRM=none")
}

// mcpProc is a running `ipfs mcp` process spoken to over stdin and stdout.
type mcpProc struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *syncBuffer

	lines  chan []byte // one per stdout line, without the trailing newline
	rawMu  sync.Mutex
	raw    []byte // every byte read from stdout
	done   chan struct{}
	nextID int
}

// startMCP starts `ipfs mcp args...` with IPFS_PATH set to repoDir.
func startMCP(t *testing.T, bin, repoDir string, args ...string) *mcpProc {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"mcp"}, args...)...)
	cmd.Env = mcpEnv(repoDir)
	cmd.Dir = repoDir
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	p := &mcpProc{t: t, cmd: cmd, stdin: stdin, stderr: &syncBuffer{}, lines: make(chan []byte, 64), done: make(chan struct{})}
	cmd.Stderr = p.stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	go func() {
		defer close(p.lines)
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				p.rawMu.Lock()
				p.raw = append(p.raw, line...)
				p.rawMu.Unlock()
				p.lines <- bytes.TrimSuffix(line, []byte("\n"))
			}
			if err != nil {
				return
			}
		}
	}()
	return p
}

func (p *mcpProc) send(v any) {
	p.t.Helper()
	b, err := json.Marshal(v)
	require.NoError(p.t, err)
	_, err = p.stdin.Write(append(b, '\n'))
	require.NoError(p.t, err)
}

// rpc sends a request and returns the matching response.
func (p *mcpProc) rpc(method string, params any) map[string]any {
	p.t.Helper()
	p.nextID++
	id := p.nextID
	p.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	timeout := time.After(mcpTimeout)
	for {
		select {
		case line, ok := <-p.lines:
			require.True(p.t, ok, "stdout closed before the response to %s; stderr: %s", method, p.stderr.String())
			var msg map[string]any
			require.NoError(p.t, json.Unmarshal(line, &msg), "stdout line is not JSON: %q", line)
			if got, ok := msg["id"].(float64); ok && int(got) == id {
				return msg
			}
		case <-timeout:
			p.t.Fatalf("timed out waiting for the response to %s; stderr: %s", method, p.stderr.String())
		}
	}
}

// initialize performs the MCP handshake.
func (p *mcpProc) initialize() map[string]any {
	p.t.Helper()
	res := p.rpc("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "kubo-cli-test", "version": "0"},
	})
	p.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return res
}

func (p *mcpProc) toolNames() []string {
	p.t.Helper()
	res := p.rpc("tools/list", map[string]any{})
	result, ok := res["result"].(map[string]any)
	require.True(p.t, ok, "tools/list result: %v", res)
	tools, _ := result["tools"].([]any)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	return names
}

// callTool calls a tool and returns the result object.
func (p *mcpProc) callTool(name string, args map[string]any) map[string]any {
	p.t.Helper()
	res := p.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	result, ok := res["result"].(map[string]any)
	require.True(p.t, ok, "tools/call result: %v", res)
	return result
}

// closeAndWait closes stdin, waits for the process to exit and returns its
// exit code.
func (p *mcpProc) closeAndWait() int {
	p.t.Helper()
	require.NoError(p.t, p.stdin.Close())
	waited := make(chan error, 1)
	go func() { waited <- p.cmd.Wait() }()
	select {
	case <-waited:
	case <-time.After(mcpTimeout):
		p.t.Fatalf("ipfs mcp did not exit after stdin closed; stderr: %s", p.stderr.String())
	}
	for range p.lines { // drain
	}
	return p.cmd.ProcessState.ExitCode()
}

// assertCleanStdout requires that everything the process wrote to stdout is
// a sequence of newline-terminated JSON-RPC messages and nothing else.
func (p *mcpProc) assertCleanStdout() {
	p.t.Helper()
	p.rawMu.Lock()
	raw := slices.Clone(p.raw)
	p.rawMu.Unlock()
	require.NotEmpty(p.t, raw, "no output on stdout")
	require.Equal(p.t, byte('\n'), raw[len(raw)-1], "stdout must end with a newline")
	for _, line := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		var msg map[string]any
		require.NoError(p.t, json.Unmarshal(line, &msg), "stdout carries a non-JSON line: %q", line)
		require.Equal(p.t, "2.0", msg["jsonrpc"], "stdout line is not a JSON-RPC message: %q", line)
	}
}

// runMCP runs `ipfs mcp args...` to completion with empty stdin.
func runMCP(t *testing.T, bin, repoDir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), mcpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, append([]string{"mcp"}, args...)...)
	cmd.Env = mcpEnv(repoDir)
	cmd.Dir = repoDir
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr, "running ipfs mcp: %v", err)
	}
	return out.String(), errOut.String(), cmd.ProcessState.ExitCode()
}

// freeAddr returns a loopback address nothing listens on.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().(*net.TCPAddr)
	require.NoError(t, l.Close())
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", addr.Port)
}

// fakeRPCDaemon serves a minimal /api/v0 with the given version and command
// paths, standing in for a stock or old daemon.
func fakeRPCDaemon(t *testing.T, version string, commands []string) *httptest.Server {
	t.Helper()
	type node struct {
		Name        string
		Subcommands []node
	}
	root := node{Name: "ipfs"}
	for _, c := range commands {
		root.Subcommands = append(root.Subcommands, node{Name: c})
	}
	mux := http.NewServeMux()
	reply := func(v any) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(v)
		}
	}
	mux.HandleFunc("/api/v0/version", reply(map[string]string{"Version": version, "Commit": "fake", "Repo": "18"}))
	mux.HandleFunc("/api/v0/commands", reply(root))
	mux.HandleFunc("/api/v0/id", reply(map[string]any{
		"ID":           "12D3KooWRBy97UB99e3J6hiPesre1MZeuNQvfan4gBziswrRJsNK",
		"Addresses":    []string{"/ip4/127.0.0.1/tcp/4001"},
		"AgentVersion": "fake",
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMCP(t *testing.T) {
	t.Parallel()

	t.Run("stdio initialize, list and call against a daemon", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		p := startMCP(t, node.IPFSBin, node.Dir)
		initRes := p.initialize()
		result, ok := initRes["result"].(map[string]any)
		require.True(t, ok, "initialize result: %v", initRes)
		assert.Equal(t, "ipfs-mcp", result["serverInfo"].(map[string]any)["name"])

		got := p.toolNames()
		assert.ElementsMatch(t, defaultMCPTools(t, node), got, "the default set is derived from the contract")
		requireNoDestructive(t, got)

		info := p.callTool("ipfs_node_info", map[string]any{})
		assert.NotEqual(t, true, info["isError"], "ipfs_node_info: %v", info)
		structured := info["structuredContent"].(map[string]any)
		assert.Equal(t, node.PeerID().String(), structured["peer_id"])
		assert.NotEmpty(t, structured["version"])
		content := info["content"].([]any)
		require.Len(t, content, 1)
		var fromText map[string]any
		require.NoError(t, json.Unmarshal([]byte(content[0].(map[string]any)["text"].(string)), &fromText))
		assert.Equal(t, structured, fromText, "text fallback matches structuredContent")

		status := p.callTool("ipfs_node_status", map[string]any{})
		sc := status["structuredContent"].(map[string]any)
		assert.Equal(t, true, sc["daemon_reachable"])
		assert.Equal(t, true, sc["version_supported"])
		assert.Equal(t, "stdio", sc["transport"])
		assert.Equal(t, false, sc["read_only"])
		for _, u := range sc["unavailable_tools"].([]any) {
			assert.NotEqual(t, "missing_rpc_command", u.(map[string]any)["reason"], "a current daemon provides every implemented tool: %v", u)
		}

		assert.Equal(t, 0, p.closeAndWait(), "stderr: %s", p.stderr.String())
		p.assertCleanStdout()
	})

	t.Run("rejects invalid tool arguments as a tool error", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		p := startMCP(t, node.IPFSBin, node.Dir)
		p.initialize()
		res := p.callTool("ipfs_node_info", map[string]any{"bogus": 1})
		assert.Equal(t, true, res["isError"])
		errObj := res["structuredContent"].(map[string]any)["error"].(map[string]any)
		assert.Equal(t, "invalid_argument", errObj["code"])
		assert.Equal(t, 0, p.closeAndWait())
		p.assertCleanStdout()
	})

	t.Run("exits non-zero naming the api file when no daemon is running", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()

		stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir)
		assert.NotZero(t, code)
		assert.Empty(t, stdout, "nothing but JSON-RPC may reach stdout")
		assert.Contains(t, stderr, filepath.Join(node.Dir, "api"))

		// The repo lock was never taken: a daemon starts on the same repo.
		node.StartDaemon()
		defer node.StopDaemon()
		assert.True(t, node.IsAlive())
	})

	t.Run("exits non-zero naming the api address when it is unreachable", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()
		addr := freeAddr(t)

		stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--api", addr)
		assert.NotZero(t, code)
		assert.Empty(t, stdout)
		assert.Contains(t, stderr, addr)
		assert.Contains(t, stderr, "not reachable")
	})

	t.Run("does not hold repo.lock while it runs", func(t *testing.T) {
		t.Parallel()
		h := harness.NewT(t)
		serving := h.NewNode().Init().StartDaemon()
		defer serving.StopDaemon()
		// A second repo that no daemon runs yet. ipfs mcp is pointed at the
		// first node with --api while IPFS_PATH names the second repo.
		other := h.NewNode().Init()

		p := startMCP(t, other.IPFSBin, other.Dir, "--api", serving.APIAddr().String())
		p.initialize()
		info := p.callTool("ipfs_node_info", map[string]any{})
		assert.Equal(t, serving.PeerID().String(), info["structuredContent"].(map[string]any)["peer_id"],
			"--api wins over $IPFS_PATH/api")

		// With ipfs mcp running, a daemon can start on the repo it was given.
		other.StartDaemon()
		defer other.StopDaemon()
		assert.True(t, other.IsAlive())

		assert.Equal(t, 0, p.closeAndWait())
		p.assertCleanStdout()
	})

	t.Run("works without any repo when --api is given", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		emptyDir := t.TempDir()

		p := startMCP(t, node.IPFSBin, emptyDir, "--api", node.APIAddr().String())
		p.initialize()
		assert.ElementsMatch(t, defaultMCPTools(t, node), p.toolNames())
		assert.Equal(t, 0, p.closeAndWait())
		entries, err := os.ReadDir(emptyDir)
		require.NoError(t, err)
		assert.Empty(t, entries, "ipfs mcp must not create or lock a repo")
	})

	t.Run("accepts an http URL for --api", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		p := startMCP(t, node.IPFSBin, t.TempDir(), "--api", node.APIURL())
		p.initialize()
		info := p.callTool("ipfs_node_info", map[string]any{})
		assert.Equal(t, node.PeerID().String(), info["structuredContent"].(map[string]any)["peer_id"])
		assert.Equal(t, 0, p.closeAndWait())
	})

	t.Run("refuses a daemon older than the minimum version", func(t *testing.T) {
		t.Parallel()
		old := fakeRPCDaemon(t, "0.28.0", []string{"id", "version", "commands"})

		stdout, stderr, code := runMCP(t, harnessBin(t), t.TempDir(), "--api", old.URL)
		assert.Empty(t, stdout)
		assert.NotZero(t, code)
		assert.Contains(t, stderr, "0.28.0")
		assert.Contains(t, stderr, "0.29.0")
		assert.Contains(t, stderr, "older than")
	})

	t.Run("omits a tool whose RPC command the daemon lacks", func(t *testing.T) {
		t.Parallel()
		stock := fakeRPCDaemon(t, "0.35.0", []string{"version", "commands"}) // no "id"

		p := startMCP(t, harnessBin(t), t.TempDir(), "--api", stock.URL)
		p.initialize()
		assert.Equal(t, []string{"ipfs_node_status"}, p.toolNames())

		status := p.callTool("ipfs_node_status", map[string]any{})
		sc := status["structuredContent"].(map[string]any)
		var entry map[string]any
		for _, u := range sc["unavailable_tools"].([]any) {
			if u.(map[string]any)["name"] == "ipfs_node_info" {
				entry = u.(map[string]any)
			}
		}
		require.NotNil(t, entry, "ipfs_node_info is reported unavailable")
		assert.Equal(t, "missing_rpc_command", entry["reason"])
		assert.Equal(t, []any{"id"}, entry["missing_rpc"])
		assert.Equal(t, 0, p.closeAndWait())
	})

	t.Run("a disabled group is absent from tools/list", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		p := startMCP(t, node.IPFSBin, node.Dir, "--tools", "pin")
		p.initialize()
		assert.NotContains(t, p.toolNames(), "ipfs_node_info", "the node group is disabled")
		assert.Contains(t, p.toolNames(), "pin_ls")
		assert.Equal(t, 0, p.closeAndWait())

		p = startMCP(t, node.IPFSBin, node.Dir, "--tools", "node")
		p.initialize()
		assert.ElementsMatch(t, expectedMCPTools(t, node, []string{"node"}, false), p.toolNames())
		assert.Equal(t, 0, p.closeAndWait())
	})

	t.Run("read-only keeps tools that change nothing", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		p := startMCP(t, node.IPFSBin, node.Dir, "--read-only")
		p.initialize()
		got := p.toolNames()
		assert.ElementsMatch(t, expectedMCPTools(t, node, mcpCatalog(t).ToolSelection.Default.Groups, true), got)
		cat := mcpCatalog(t)
		for _, n := range got {
			def, _ := cat.Tool(n)
			assert.Equal(t, contract.ClassNone, def.Confirmation, "%s changes state but --read-only is on", n)
		}
		sc := p.callTool("ipfs_node_status", map[string]any{})["structuredContent"].(map[string]any)
		assert.Equal(t, true, sc["read_only"])
		assert.Equal(t, 0, p.closeAndWait())
	})

	t.Run("rejects an unknown tool group", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		_, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--tools", "bogus")
		assert.NotZero(t, code)
		assert.Contains(t, stderr, `unknown tool group "bogus"`)
	})

	t.Run("sends --api-auth to a daemon with a dedicated RPC secret", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()
		node.UpdateConfig(func(cfg *config.Config) {
			cfg.API.Authorizations = map[string]*config.RPCAuthScope{
				"starter": {AuthSecret: "bearer:starter-secret", AllowedPaths: []string{"/api/v0"}},
				"mcp": {
					AuthSecret:   "bearer:mcp-secret",
					AllowedPaths: []string{"/api/v0/id", "/api/v0/version", "/api/v0/commands"},
				},
			}
		})
		node.StartDaemonWithAuthorization("Bearer starter-secret")
		defer node.StopDaemon()

		_, stderr, code := runMCP(t, node.IPFSBin, node.Dir)
		assert.NotZero(t, code, "no secret: the daemon refuses")
		assert.NotContains(t, stderr, "mcp-secret")

		p := startMCP(t, node.IPFSBin, node.Dir, "--api-auth", "bearer:mcp-secret")
		p.initialize()
		info := p.callTool("ipfs_node_info", map[string]any{})
		assert.Equal(t, node.PeerID().String(), info["structuredContent"].(map[string]any)["peer_id"])
		assert.Equal(t, 0, p.closeAndWait())
		assert.NotContains(t, p.stderr.String(), "mcp-secret")
	})

	t.Run("ipfs mcp is a CLI-only command, not an RPC endpoint", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()

		resp := node.APIClient().Post("/api/v0/mcp", nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// harnessBin returns the ipfs binary the harness uses.
func harnessBin(t *testing.T) string {
	t.Helper()
	return harness.NewT(t).IPFSBin
}

func TestMCPHelp(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode()
	res := node.RunIPFS("mcp", "--help")
	require.NoError(t, res.Err)
	out := res.Stdout.String()
	assert.Contains(t, out, "ipfs mcp - Serve a running Kubo node to AI agents over MCP.")
	assert.Contains(t, out, "--tools")
	assert.Contains(t, out, "--read-only")
	assert.Contains(t, out, "GOTAGS=nomcp")
}

// TestMCPNoMCPBuild builds the binary with the nomcp tag and checks that
// `ipfs mcp` reports MCP as compiled out.
func TestMCPNoMCPBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the ipfs binary")
	}
	t.Parallel()

	root := filepath.Dir(harness.NewT(t).IPFSBin)
	root = filepath.Dir(filepath.Dir(root)) // cmd/ipfs -> repo root
	bin := filepath.Join(t.TempDir(), "ipfs-nomcp")

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-tags=nomcp", "-o", bin, "./cmd/ipfs")
	build.Dir = root
	out, err := build.CombinedOutput()
	require.NoError(t, err, "go build -tags=nomcp: %s", out)

	stdout, stderr, code := runMCP(t, bin, t.TempDir())
	assert.NotZero(t, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "MCP support is not included in this build")
}
