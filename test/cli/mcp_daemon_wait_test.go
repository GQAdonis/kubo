package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// daemonWaitConfig writes an ipfs mcp config with daemon.wait set.
func daemonWaitConfig(t *testing.T, wait string) string {
	t.Helper()
	return principalConfig(t, fmt.Sprintf(`{"daemon":{"wait":%q}}`, wait))
}

// A sidecar starts before the daemon container has its API up. With the wait on
// (the default), ipfs mcp keeps trying and serves once the daemon answers.
func TestMCPDaemonWaitServesOnceTheDaemonComesUp(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init()
	api := freeAddr(t)
	node.UpdateConfig(func(cfg *config.Config) { cfg.Addresses.API = []string{api} })

	p := startMCP(t, node.IPFSBin, node.Dir, "--api", api, "--mcp-config", daemonWaitConfig(t, "45s"))
	// The daemon is not running yet; give the first attempts time to be refused.
	time.Sleep(2500 * time.Millisecond)
	node.StartDaemon()
	defer node.StopDaemon()

	res := p.initialize()
	require.Contains(t, res, "result", "initialize after the daemon came up: %v; stderr: %s", res, p.stderr.String())
	assert.NotEmpty(t, p.toolNames())
	assert.Zero(t, p.closeAndWait())
}

// With no daemon, startup fails with the usual error once the bound passes.
func TestMCPDaemonWaitFailsAfterTheBound(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init()
	addr := freeAddr(t)

	start := time.Now()
	stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--api", addr, "--mcp-config", daemonWaitConfig(t, "3s"))
	elapsed := time.Since(start)

	assert.NotZero(t, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, addr)
	assert.Contains(t, stderr, "cannot use the ipfs daemon API")
	assert.Contains(t, stderr, "not reachable")
	assert.GreaterOrEqual(t, elapsed, 3*time.Second, "the wait ran its full bound")
	assert.Less(t, elapsed, 30*time.Second)
}

// A daemon that refuses the call (API.Authorizations) is not a startup race and
// fails at once, whatever the wait.
func TestMCPDaemonWaitDoesNotRetryAnAuthRefusal(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init()
	node.UpdateConfig(func(cfg *config.Config) {
		cfg.API.Authorizations = map[string]*config.RPCAuthScope{
			"starter": {AuthSecret: "bearer:starter-secret", AllowedPaths: []string{"/api/v0"}},
		}
	})
	node.StartDaemonWithAuthorization("Bearer starter-secret")
	defer node.StopDaemon()

	start := time.Now()
	_, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--api-auth", "bearer:wrong-secret-value", "--mcp-config", daemonWaitConfig(t, "45s"))
	assert.NotZero(t, code)
	assert.Contains(t, stderr, "cannot use the ipfs daemon API")
	assert.Less(t, time.Since(start), 20*time.Second, "an auth refusal does not wait")
}

// Without --api, discovery reads $IPFS_PATH/api. With no api file yet, the wait
// covers it: the file is read again on each attempt, so a daemon that starts
// about two seconds after ipfs mcp is found.
func TestMCPDaemonWaitFindsAnAPIFileThatAppearsLater(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init()
	_, err := os.Stat(node.Dir + "/api")
	require.ErrorIs(t, err, os.ErrNotExist, "no api file before the daemon starts")

	p := startMCP(t, node.IPFSBin, node.Dir, "--mcp-config", daemonWaitConfig(t, "45s"))
	time.Sleep(2 * time.Second)
	node.StartDaemon()
	defer node.StopDaemon()

	res := p.initialize()
	require.Contains(t, res, "result", "initialize after the daemon came up: %v; stderr: %s", res, p.stderr.String())
	assert.NotEmpty(t, p.toolNames())
	assert.Zero(t, p.closeAndWait())
}

// Plain stdio ipfs mcp defaults to no wait: an unreachable daemon fails at once,
// as before the wait existed. An MCP host has its own startup timeout.
func TestMCPDaemonWaitStdioDefaultsToFailingAtOnce(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init()
	addr := freeAddr(t)

	start := time.Now()
	stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--api", addr)
	assert.NotZero(t, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "not reachable")
	assert.Less(t, time.Since(start), 10*time.Second, "no default wait on stdio")
}

// With --http the default is 60 s: the sidecar keeps trying, and the first
// retry is logged at Warn, which the default log level shows.
func TestMCPDaemonWaitHTTPDefaultWaitsAndSaysSo(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init()

	cmd := exec.Command(node.IPFSBin, "mcp", "--http=127.0.0.1:0")
	cmd.Env = append(mcpEnv(node.Dir), noAmbientConfig...)
	cmd.Dir = node.Dir
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	ownProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		killProcessGroup(cmd)
		<-exited
	})

	require.Eventually(t, func() bool { return strings.Contains(stderr.String(), "not reachable yet") },
		10*time.Second, 100*time.Millisecond, "the first retry is logged at the default level; stderr: %s", stderr.String())
	assert.Contains(t, stderr.String(), "level=WARN")
	select {
	case err := <-exited:
		t.Fatalf("ipfs mcp --http gave up inside its default wait: %v; stderr: %s", err, stderr.String())
	case <-time.After(3 * time.Second):
	}
}
