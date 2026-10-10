package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Native confirmation prompt (change native-confirmation-prompt, tasks 1.4).
//
// These tests need a binary built with the test-only resolver:
//
//	go build -tags mcptestresolver ./cmd/ipfs
//
// The test builds it once into a temp directory. The tag adds, to `ipfs mcp`
// only, (a) a resolver whose decisions come from files the test writes, and
// (b) a short confirmation lifetime. Neither exists in a production build.
// No test here raises a real Touch ID or polkit prompt: the real-prompt case
// is the operator script mcp/server/confirm/hg1/attempt.sh.

var (
	testResolverBinOnce sync.Once
	testResolverBin     string
	testResolverBinErr  error
)

func mcpTestResolverBin(t *testing.T) string {
	t.Helper()
	testResolverBinOnce.Do(func() {
		goBin, err := exec.LookPath("go")
		if err != nil {
			testResolverBinErr = err
			return
		}
		dir, err := os.MkdirTemp("", "ipfs-mcp-testresolver")
		if err != nil {
			testResolverBinErr = err
			return
		}
		testResolverBin = filepath.Join(dir, "ipfs")
		root, _ := filepath.Abs("../..")
		cmd := exec.Command(goBin, "build", "-tags", "mcptestresolver", "-o", testResolverBin, "./cmd/ipfs")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			testResolverBinErr = &buildError{err: err, out: string(out)}
		}
	})
	if testResolverBinErr != nil {
		t.Skipf("cannot build the test-resolver binary: %v", testResolverBinErr)
	}
	return testResolverBin
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + ": " + e.out }

// shortTempDir returns a short directory (Unix socket paths are length
// limited).
func shortTempDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "mc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// confirmClient starts `bin mcp args...` with extra environment and connects
// a client that cannot elicit.
func confirmClient(t *testing.T, bin string, node *harness.Node, extraEnv []string, args ...string) *mcp.ClientSession {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"mcp"}, args...)...)
	cmd.Env = append(mcpEnv(node.Dir), extraEnv...)
	cmd.Dir = node.Dir
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	ctx, cancel := context.WithTimeout(t.Context(), mcpTimeout)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "kubo-cli-confirm-test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	require.NoError(t, err, "stderr: %s", stderr.String())
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// confirmation returns the error code and confirmation ID of a tool result.
func confirmation(t *testing.T, res *mcp.CallToolResult) (code, id string) {
	t.Helper()
	if !res.IsError {
		return "", ""
	}
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out struct {
		Error struct {
			Code         string `json:"code"`
			Confirmation struct {
				ID string `json:"confirmation_id"`
			} `json:"confirmation"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out.Error.Code, out.Error.Confirmation.ID
}

func pinAdd(t *testing.T, cs *mcp.ClientSession, cid string) *mcp.CallToolResult {
	t.Helper()
	return callMCP(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + cid})
}

// eventuallyCode repeats the call until it returns code (or success for "").
func eventuallyCode(t *testing.T, cs *mcp.ClientSession, cid, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		code, _ := confirmation(t, pinAdd(t, cs, cid))
		return code == want
	}, 20*time.Second, 100*time.Millisecond, "wanted %q", want)
}

func TestMCPConfirmTestResolver(t *testing.T) {
	t.Parallel()
	bin := mcpTestResolverBin(t)

	run := func(t *testing.T, ttl string) (*harness.Node, *mcp.ClientSession, string, string) {
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		t.Cleanup(func() { node.StopDaemon() })
		dir := shortTempDir(t)
		cs := confirmClient(t, bin, node, []string{
			"IPFS_MCP_TEST_RESOLVER_DIR=" + dir,
			"IPFS_MCP_TEST_CONFIRM_TTL=" + ttl,
		})
		cid := node.IPFSAddStr("confirm me "+t.Name(), "--pin=false", "--cid-version=1")
		require.False(t, isPinned(node, cid))
		return node, cs, dir, cid
	}

	t.Run("a non-eliciting write waits and runs once on approval", func(t *testing.T) {
		t.Parallel()
		node, cs, dir, cid := run(t, "30s")

		code, id := confirmation(t, pinAdd(t, cs, cid))
		require.Equal(t, "confirmation_required", code)
		require.NotEmpty(t, id)
		assert.False(t, isPinned(node, cid), "nothing runs before the human decides")

		code, again := confirmation(t, pinAdd(t, cs, cid))
		assert.Equal(t, "confirmation_pending", code, "repeating the call does not approve it")
		assert.Equal(t, id, again)
		assert.False(t, isPinned(node, cid))

		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".approve"), nil, 0o600))
		eventuallyCode(t, cs, cid, "")
		assert.True(t, isPinned(node, cid))

		// The approval was spent: the next call asks again.
		code, next := confirmation(t, pinAdd(t, cs, cid))
		assert.Equal(t, "confirmation_required", code)
		assert.NotEqual(t, id, next)
	})

	t.Run("deny never runs", func(t *testing.T) {
		t.Parallel()
		node, cs, dir, cid := run(t, "30s")
		_, id := confirmation(t, pinAdd(t, cs, cid))
		require.NotEmpty(t, id)
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".deny"), nil, 0o600))
		eventuallyCode(t, cs, cid, "confirmation_declined")
		assert.False(t, isPinned(node, cid))
	})

	t.Run("expiry is denial and a late approval is rejected", func(t *testing.T) {
		t.Parallel()
		node, cs, dir, cid := run(t, "3s")
		_, id := confirmation(t, pinAdd(t, cs, cid))
		require.NotEmpty(t, id)
		time.Sleep(4 * time.Second)
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".approve"), nil, 0o600))
		eventuallyCode(t, cs, cid, "confirmation_expired")
		assert.False(t, isPinned(node, cid), "an approval after the deadline does not run the call")
	})
}

// channelRequest sends one HTTP request to the confirmation socket.
func channelRequest(t *testing.T, sock, method, path, auth, cliMarker string, body any) (int, string) {
	t.Helper()
	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sock)
	}}, Timeout: 10 * time.Second}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, "http://confirm"+path, rd)
	require.NoError(t, err)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if cliMarker != "" {
		req.Header.Set("X-IPFS-MCP-Confirm", cliMarker)
	}
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestMCPConfirmHG1 is the automated HG-1 IPC case: a background process under
// the same user, with no TTY, tries to approve over the confirmation channel
// without the pairing key, with a forged key and through `ipfs mcp confirm`
// without --confirm=cli. Each attempt is rejected, the pin does not exist, and
// the confirmation stays pending until expiry, then is denied.
func TestMCPConfirmHG1(t *testing.T) {
	t.Parallel()
	bin := mcpTestResolverBin(t)

	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	sockDir := filepath.Join(shortTempDir(t), "s")
	pairing := filepath.Join(shortTempDir(t), "pairing.json")
	env := []string{
		"IPFS_MCP_CONFIRM_DIR=" + sockDir,
		"IPFS_MCP_PAIRING_FILE=" + pairing,
		"IPFS_MCP_TEST_CONFIRM_TTL=12s",
	}

	// Pair: the key is printed once and only its hash is stored.
	pair := exec.Command(bin, "mcp", "pair")
	pair.Env = append(mcpEnv(node.Dir), env...)
	keyOut, err := pair.Output()
	require.NoError(t, err)
	key := strings.TrimSpace(string(keyOut))
	require.NotEmpty(t, key)
	stored, err := os.ReadFile(pairing)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), key)

	cs := confirmClient(t, bin, node, env, "--confirm=desktop")
	cid := node.IPFSAddStr("hg1 ipc attempt", "--pin=false", "--cid-version=1")
	code, id := confirmation(t, pinAdd(t, cs, cid))
	require.Equal(t, "confirmation_required", code)
	require.NotEmpty(t, id)

	socks, err := filepath.Glob(filepath.Join(sockDir, "*.sock"))
	require.NoError(t, err)
	require.Len(t, socks, 1)
	sock := socks[0]
	resolveBody := map[string]any{"id": id, "approve": true}

	t.Run("no pairing key", func(t *testing.T) {
		status, _ := channelRequest(t, sock, http.MethodPost, "/v1/resolve", "", "", resolveBody)
		assert.Equal(t, http.StatusUnauthorized, status)
		status, _ = channelRequest(t, sock, http.MethodGet, "/v1/pending", "", "", nil)
		assert.Equal(t, http.StatusUnauthorized, status)
	})
	t.Run("forged key", func(t *testing.T) {
		for _, forged := range []string{"Bearer forged", "Bearer " + key + "0", "Bearer " + id} {
			status, _ := channelRequest(t, sock, http.MethodPost, "/v1/resolve", forged, "", resolveBody)
			assert.Equal(t, http.StatusForbidden, status, forged)
		}
	})
	t.Run("cli marker while the server was not started with --confirm=cli", func(t *testing.T) {
		status, body := channelRequest(t, sock, http.MethodPost, "/v1/resolve", "", "cli", resolveBody)
		assert.Equal(t, http.StatusForbidden, status)
		assert.Contains(t, body, "--confirm=cli")
	})
	t.Run("ipfs mcp confirm without --confirm=cli", func(t *testing.T) {
		cmd := exec.Command(bin, "mcp", "confirm", id)
		cmd.Env = append(mcpEnv(node.Dir), env...)
		cmd.Stdin = strings.NewReader("yes\n")
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "--confirm=cli")
	})
	t.Run("ipfs mcp confirm --confirm=cli without a TTY", func(t *testing.T) {
		cmd := exec.Command(bin, "mcp", "confirm", "--confirm=cli", id)
		cmd.Env = append(mcpEnv(node.Dir), env...)
		cmd.Stdin = strings.NewReader("yes\n") // a pipe, not a terminal
		out, err := cmd.CombinedOutput()
		require.Error(t, err)
		assert.Contains(t, string(out), "terminal")
	})

	// Every attempt failed: still pending, nothing pinned.
	code, still := confirmation(t, pinAdd(t, cs, cid))
	assert.Equal(t, "confirmation_pending", code)
	assert.Equal(t, id, still)
	assert.False(t, isPinned(node, cid))

	// Left alone it expires, and expiry is denial.
	eventuallyCode(t, cs, cid, "confirmation_expired")
	assert.False(t, isPinned(node, cid))

	// Positive control: the paired key does work, so the rejections above
	// were the key check and not a broken channel.
	_, id2 := confirmation(t, pinAdd(t, cs, cid))
	require.NotEmpty(t, id2)
	status, body := channelRequest(t, sock, http.MethodPost, "/v1/resolve", "Bearer "+key, "", map[string]any{"id": id2, "approve": true})
	require.Equal(t, http.StatusOK, status, body)
	eventuallyCode(t, cs, cid, "")
	assert.True(t, isPinned(node, cid))
}
