package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server/confirm"
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
	testResolverDir     string
	testResolverBin     string
	// testResolverNoGo is set when `go` is not on PATH, the only reason a test
	// that needs the binary may skip. Any other failure fails the test.
	testResolverNoGo   bool
	testResolverBinErr error
)

// TestMain removes the test-resolver binary directory after the whole package
// has run: the binary is shared by several tests, so no single test owns it.
func TestMain(m *testing.M) {
	code := m.Run()
	if testResolverDir != "" {
		_ = os.RemoveAll(testResolverDir)
	}
	os.Exit(code)
}

func mcpTestResolverBin(t *testing.T) string {
	t.Helper()
	testResolverBinOnce.Do(func() {
		goBin, err := exec.LookPath("go")
		if err != nil {
			testResolverNoGo = true
			testResolverBinErr = err
			return
		}
		dir, err := os.MkdirTemp("", "ipfs-mcp-testresolver")
		if err != nil {
			testResolverBinErr = err
			return
		}
		testResolverDir = dir
		testResolverBin = filepath.Join(dir, "ipfs")
		root, _ := filepath.Abs("../..")
		cmd := exec.Command(goBin, "build", "-tags", "mcptestresolver", "-o", testResolverBin, "./cmd/ipfs")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			testResolverBinErr = &buildError{err: err, out: string(out)}
		}
	})
	if testResolverNoGo {
		t.Skipf("go is not on PATH, cannot build the test-resolver binary: %v", testResolverBinErr)
	}
	if testResolverBinErr != nil {
		t.Fatalf("cannot build the test-resolver binary: %v", testResolverBinErr)
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
	cs, _ := confirmClientStderr(t, bin, node, extraEnv, args...)
	return cs
}

// confirmClientStderr is confirmClient that also returns the server's stderr.
func confirmClientStderr(t *testing.T, bin string, node *harness.Node, extraEnv []string, args ...string) (*mcp.ClientSession, *syncBuffer) {
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
	return cs, stderr
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

// signV2 adds channel authentication version 2 headers, written from the spec
// in mcp/server/confirm/MECHANISMS.md and not from the server's code.
func signV2(req *http.Request, key, sock, path string, body []byte) {
	k := sha256.Sum256([]byte(key))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nb := make([]byte, 16)
	_, _ = rand.Read(nb)
	nonce := base64.RawURLEncoding.EncodeToString(nb)
	bh := sha256.Sum256(body)
	str := strings.Join([]string{"ipfs-mcp-confirm-v2", "request", filepath.Base(sock), req.Method, path, ts, nonce, hex.EncodeToString(bh[:])}, "\n")
	m := hmac.New(sha256.New, k[:])
	m.Write([]byte(str))
	req.Header.Set("X-IPFS-MCP-Auth-Version", "2")
	req.Header.Set("X-IPFS-MCP-Timestamp", ts)
	req.Header.Set("X-IPFS-MCP-Nonce", nonce)
	req.Header.Set("X-IPFS-MCP-Signature", base64.RawURLEncoding.EncodeToString(m.Sum(nil)))
}

// channelRequest sends one HTTP request to the confirmation socket.
func channelRequest(t *testing.T, sock, method, path, auth, cliMarker string, body any) (int, string) {
	t.Helper()
	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sock)
	}}, Timeout: 10 * time.Second}
	var rd io.Reader
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, "http://confirm"+path, rd)
	require.NoError(t, err)
	if key, ok := strings.CutPrefix(auth, "v2:"); ok {
		signV2(req, key, sock, path, raw)
	} else if auth != "" {
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

	// `ipfs mcp pair` refuses to run without a terminal on stdin, so an agent
	// or a script cannot obtain a key: no key is printed and nothing is stored.
	pair := exec.Command(bin, "mcp", "pair")
	pair.Env = append(mcpEnv(node.Dir), env...)
	pair.Stdin = nil
	keyOut, err := pair.Output()
	require.Error(t, err, "pair without a terminal must fail")
	assert.Empty(t, strings.TrimSpace(string(keyOut)))
	_, statErr := os.Stat(pairing)
	require.ErrorIs(t, statErr, os.ErrNotExist, "a refused pair stores nothing")

	// A person at a terminal pairs; confirm.Pair is what `ipfs mcp pair` runs
	// after its terminal and running-server checks. The key is returned once
	// and only its hash is stored.
	key, err := confirm.Pair(pairing)
	require.NoError(t, err)
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
	t.Run("bearer key, forged or real", func(t *testing.T) {
		for _, bearer := range []string{"Bearer forged", "Bearer " + key, "Bearer " + id} {
			status, body := channelRequest(t, sock, http.MethodPost, "/v1/resolve", bearer, "", resolveBody)
			assert.Equal(t, http.StatusUnauthorized, status, bearer)
			assert.Contains(t, body, "auth_version_unsupported")
		}
	})
	t.Run("forged signing key", func(t *testing.T) {
		for _, forged := range []string{"forged", key + "0", id} {
			status, body := channelRequest(t, sock, http.MethodPost, "/v1/resolve", "v2:"+forged, "", resolveBody)
			assert.Equal(t, http.StatusForbidden, status, forged)
			assert.Contains(t, body, "auth_invalid")
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
	status, body := channelRequest(t, sock, http.MethodPost, "/v1/resolve", "v2:"+key, "", map[string]any{"id": id2, "approve": true})
	require.Equal(t, http.StatusOK, status, body)
	eventuallyCode(t, cs, cid, "")
	assert.True(t, isPinned(node, cid))
}

// writeConfirmConfig writes an `ipfs mcp --mcp-config` file.
func writeConfirmConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(shortTempDir(t), "mcp.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

// TestMCPConfirmationConfigKeys covers confirmation.ttl, confirmation.cli and
// confirmation.resolvers from the config file (change native-confirmation-prompt,
// task 1.6). IPFS_MCP_CONFIRM is cleared so that the file decides the modes;
// the modes name desktop only, which keeps the real native prompt out of the
// test. Desktop is unpaired in most cases (so it builds no resolver); the
// resolvers case pairs it on purpose to show that narrowing, not the missing
// pairing, removes it.
func TestMCPConfirmationConfigKeys(t *testing.T) {
	t.Parallel()
	bin := mcpTestResolverBin(t)

	t.Run("ttl from the file replaces the 120 s lifetime", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		dir := shortTempDir(t)
		cfg := writeConfirmConfig(t, `{"confirmation": {"modes": ["desktop"], "ttl": "3s"}}`)
		cs := confirmClient(t, bin, node, []string{
			"IPFS_MCP_CONFIRM=",
			"IPFS_MCP_TEST_RESOLVER_DIR=" + dir,
			"IPFS_MCP_PAIRING_FILE=" + filepath.Join(dir, "none.json"),
			"IPFS_MCP_CONFIRM_DIR=" + filepath.Join(dir, "s"),
		}, "--mcp-config", cfg)
		cid := node.IPFSAddStr("config ttl "+t.Name(), "--pin=false", "--cid-version=1")

		code, id := confirmation(t, pinAdd(t, cs, cid))
		require.Equal(t, "confirmation_required", code)
		require.NotEmpty(t, id)
		time.Sleep(4 * time.Second) // past 3 s, far below the default 120 s
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+".approve"), nil, 0o600))
		eventuallyCode(t, cs, cid, "confirmation_expired")
		assert.False(t, isPinned(node, cid), "an approval after the configured deadline does not run the call")
	})

	t.Run("cli true opens the cli channel but never approves the stdio principal", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		dir := shortTempDir(t)
		sockDir := filepath.Join(dir, "s")
		cfg := writeConfirmConfig(t, `{"confirmation": {"modes": ["desktop"], "cli": true, "ttl": "30s"}}`)
		cs, stderr := confirmClientStderr(t, bin, node, []string{
			"IPFS_MCP_CONFIRM=",
			"IPFS_MCP_PAIRING_FILE=" + filepath.Join(dir, "none.json"),
			"IPFS_MCP_CONFIRM_DIR=" + sockDir,
		}, "--mcp-config", cfg)
		cid := node.IPFSAddStr("config cli "+t.Name(), "--pin=false", "--cid-version=1")

		// The cli resolver exists but never fits the stdio principal, which
		// cannot assert shell_capable: false: the call is refused, nothing is
		// listed or opened, and the server said so at startup.
		code, id := confirmation(t, pinAdd(t, cs, cid))
		assert.Equal(t, "confirmation_unavailable", code)
		assert.Empty(t, id)
		assert.Contains(t, stderr.String(), confirm.CLIStdioWarning)
		socks, err := filepath.Glob(filepath.Join(sockDir, "*.sock"))
		require.NoError(t, err)
		assert.Len(t, socks, 1, "the cli mode opened the confirmation channel")
		assert.False(t, isPinned(node, cid))
	})

	t.Run("without cli the same config opens no channel", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		dir := shortTempDir(t)
		sockDir := filepath.Join(dir, "s")
		cfg := writeConfirmConfig(t, `{"confirmation": {"modes": ["desktop"], "cli": false, "ttl": "30s"}}`)
		cs := confirmClient(t, bin, node, []string{
			"IPFS_MCP_CONFIRM=",
			"IPFS_MCP_PAIRING_FILE=" + filepath.Join(dir, "none.json"),
			"IPFS_MCP_CONFIRM_DIR=" + sockDir,
		}, "--mcp-config", cfg)
		cid := node.IPFSAddStr("config no cli "+t.Name(), "--pin=false", "--cid-version=1")

		code, _ := confirmation(t, pinAdd(t, cs, cid))
		assert.Equal(t, "confirmation_unavailable", code, "no resolver, no eliciting client: refused")
		socks, _ := filepath.Glob(filepath.Join(sockDir, "*.sock"))
		assert.Empty(t, socks)
	})

	t.Run("resolvers [] removes the paired desktop resolver and warns", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		dir := shortTempDir(t)
		sockDir := filepath.Join(dir, "s")
		pairing := filepath.Join(dir, "pairing.json")
		_, err := confirm.Pair(pairing)
		require.NoError(t, err)
		cfg := writeConfirmConfig(t, `{"confirmation": {"modes": ["desktop"], "resolvers": []}}`)
		cs, stderr := confirmClientStderr(t, bin, node, []string{
			"IPFS_MCP_CONFIRM=",
			"IPFS_MCP_PAIRING_FILE=" + pairing,
			"IPFS_MCP_CONFIRM_DIR=" + sockDir,
		}, "--mcp-config", cfg)
		cid := node.IPFSAddStr("config resolvers "+t.Name(), "--pin=false", "--cid-version=1")

		code, _ := confirmation(t, pinAdd(t, cs, cid))
		assert.Equal(t, "confirmation_unavailable", code)
		assert.False(t, isPinned(node, cid))
		assert.Contains(t, stderr.String(), confirm.NarrowedAwayWarning)
		socks, _ := filepath.Glob(filepath.Join(sockDir, "*.sock"))
		assert.Empty(t, socks, "narrowed away: the paired desktop channel is not opened")
	})

	t.Run("invalid values stop the server at startup and name the key", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon()
		defer node.StopDaemon()
		for name, tc := range map[string]struct{ body, want string }{
			"ttl above the bound":   {`{"confirmation": {"ttl": "11m"}}`, "confirmation.ttl"},
			"ttl zero":              {`{"confirmation": {"ttl": "0s"}}`, "confirmation.ttl"},
			"ttl below the minimum": {`{"confirmation": {"ttl": "500ms"}}`, "at least 1s"},
			"cli false vs modes":    {`{"confirmation": {"cli": false, "modes": ["cli"]}}`, "confirmation.cli"},
			"unknown resolver name": {`{"confirmation": {"resolvers": ["cli"]}}`, "resolvers"},
		} {
			cmd := exec.CommandContext(t.Context(), bin, "mcp", "--mcp-config", writeConfirmConfig(t, tc.body))
			cmd.Env = append(mcpEnv(node.Dir), "IPFS_MCP_CONFIRM=")
			cmd.Dir = node.Dir
			cmd.Stdin = strings.NewReader("")
			out, err := cmd.CombinedOutput()
			require.Error(t, err, name)
			assert.Contains(t, string(out), tc.want, name)
		}
	})
}
