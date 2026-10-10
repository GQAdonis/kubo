//go:build !windows

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-embeddings-surrealdb with the real surreal binary on
// OS-chosen loopback ports. A test that needs surreal is skipped, with the
// reason, when no binary is found: IPFS_MCP_SURREAL_BIN, then PATH, then
// ~/.prometheus/bin/surreal. Every process a test starts is in its own process
// group and the group is killed on cleanup.

func surrealBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("IPFS_MCP_SURREAL_BIN"); p != "" {
		return p
	}
	if p, err := exec.LookPath("surreal"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".prometheus", "bin", "surreal")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no surreal binary: set IPFS_MCP_SURREAL_BIN to its absolute path")
	return ""
}

// psEntry is one line of the process table.
type psEntry struct {
	pid, ppid int
	args      string
}

func psTable(t *testing.T) []psEntry {
	t.Helper()
	out, err := exec.Command("ps", "-eo", "pid=,ppid=,args=").Output()
	require.NoError(t, err)
	var entries []psEntry
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		entries = append(entries, psEntry{pid, ppid, strings.Join(f[2:], " ")})
	}
	return entries
}

// processesMentioning lists processes whose command line contains s (a data
// dir unique to the test), excluding this test binary and ps itself.
func processesMentioning(t *testing.T, s string) []psEntry {
	t.Helper()
	var out []psEntry
	for _, e := range psTable(t) {
		if e.pid != os.Getpid() && strings.Contains(e.args, s) && !strings.HasPrefix(e.args, "ps ") {
			out = append(out, e)
		}
	}
	return out
}

func surrealProcess(t *testing.T, dataDir string) (psEntry, bool) {
	t.Helper()
	for _, e := range processesMentioning(t, filepath.Join(dataDir, "db")) {
		if strings.Contains(e.args, " start ") {
			return e, true
		}
	}
	return psEntry{}, false
}

func eventually(t *testing.T, d time.Duration, cond func() bool, msg string, args ...any) {
	t.Helper()
	require.Eventually(t, cond, d, 100*time.Millisecond, append([]any{msg}, args...)...)
}

// fakeEmbedServer is an OpenAI-compatible /v1/embeddings server whose vectors
// are one-hot on an axis chosen by the first word of the text.
func fakeEmbedServer(t *testing.T, dims int, axes map[string]int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		for i, text := range req.Input {
			v := make([]float32, dims)
			first, _, _ := strings.Cut(text, " ")
			v[axes[first]] = 1
			data = append(data, item{i, v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func coversOnly(hosts ...string) func(string) bool {
	return func(h string) bool {
		for _, x := range hosts {
			if x == h {
				return true
			}
		}
		return false
	}
}

var fakeAxes = map[string]int{"cats": 0, "dogs": 1, "fish": 2}

func requireSearchFinds(t *testing.T, svc *embeddings.Service, covers func(string) bool, query, wantCID string) {
	t.Helper()
	hits, err := svc.Search(t.Context(), covers, query, 2)
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	assert.Equal(t, wantCID, hits[0].CID)
	assert.InDelta(t, 0.0, hits[0].Distance, 0.001)
}

func indexThreeDocs(t *testing.T, svc *embeddings.Service, covers func(string) bool) {
	t.Helper()
	for cid, text := range map[string]string{"bafyCats": "cats purr", "bafyDogs": "dogs bark", "bafyFish": "fish swim"} {
		n, err := svc.Index(t.Context(), covers, embeddings.Document{CID: cid, Path: "/" + cid, MIME: "text/plain", Chunks: []string{text}})
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
}

// TestMCPEmbeddingsStoreEmbedded runs the supervised store in this process:
// lifecycle, loopback bind, credentials, insert and KNN, delete, and a restart
// on the same data dir (credential rotation, data kept).
func TestMCPEmbeddingsStoreEmbedded(t *testing.T) {
	bin := surrealBinary(t)
	embed := fakeEmbedServer(t, 8, fakeAxes)
	dataDir := filepath.Join(t.TempDir(), "idx")
	set := embeddings.Settings{
		Mode:     embeddings.ModeEmbedded,
		Embedded: &embeddings.EmbeddedSettings{Binary: bin, DataDir: dataDir, AvoidPorts: mcpserver.ReservedPorts},
		Embedder: embeddings.EmbedderSettings{BaseURL: embed.URL + "/v1", Model: "fake", Dimensions: 8},
	}
	covers := coversOnly(strings.TrimPrefix(embed.URL, "http://"))

	svc, err := embeddings.Open(t.Context(), set, embeddings.Options{})
	require.NoError(t, err)
	sup := svc.Supervisor()
	require.NotNil(t, sup)
	pid := sup.PID()
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = svc.Close()
		}
	})

	// Loopback only, ephemeral, never a reserved port.
	host, portStr, err := net.SplitHostPort(sup.Addr())
	require.NoError(t, err)
	port, _ := strconv.Atoi(portStr)
	assert.Equal(t, "127.0.0.1", host)
	assert.NotContains(t, mcpserver.ReservedPorts, port)
	assertNotReachableOffLoopback(t, port)

	// The process is the configured binary; the password is not on its command line.
	proc, ok := surrealProcess(t, dataDir)
	require.True(t, ok, "no surreal process for %s", dataDir)
	assert.Equal(t, pid, proc.pid)
	assert.Contains(t, proc.args, bin)
	assert.Contains(t, proc.args, "--bind 127.0.0.1:"+portStr)
	credPath := sup.CredentialsFile()
	raw, err := os.ReadFile(credPath)
	require.NoError(t, err)
	var cred struct {
		Password string `json:"password"`
	}
	require.NoError(t, json.Unmarshal(raw, &cred))
	require.NotEmpty(t, cred.Password)
	assert.NotContains(t, proc.args, cred.Password)
	fi, err := os.Stat(credPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	// Insert and KNN through the production store.
	indexThreeDocs(t, svc, covers)
	requireSearchFinds(t, svc, covers, "dogs", "bafyDogs")
	requireSearchFinds(t, svc, covers, "fish", "bafyFish")
	n, err := svc.Store().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	removed, err := svc.Store().DeleteCID(t.Context(), "bafyCats")
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	// An unauthenticated client gets no data out of the store.
	req, err := http.NewRequest(http.MethodPost, sup.Endpoint()+"/rpc", strings.NewReader(`{"id":1,"method":"query","params":["SELECT * FROM ipfs_chunk"]}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Surreal-NS", "ipfs_mcp")
	req.Header.Set("Surreal-DB", "index")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	t.Logf("unauthenticated read: HTTP %d %s", resp.StatusCode, body)
	assert.NotContains(t, string(body), "bafy")

	// Egress: an identity that acknowledges nothing cannot index or search.
	_, err = svc.Search(t.Context(), coversOnly(), "dogs", 1)
	require.ErrorIs(t, err, embeddings.ErrEgressNotAcknowledged)

	// Close stops the process.
	require.NoError(t, svc.Close())
	closed = true
	eventually(t, 15*time.Second, func() bool { return !pidAlive(pid) }, "surreal pid %d still alive after Close", pid)
	_, running := surrealProcess(t, dataDir)
	assert.False(t, running)
	_, err = os.Stat(filepath.Join(dataDir, "supervisor.json"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	// Restart on the same data dir: the data is still there and the old root
	// password is replaced, not reused.
	svc2, err := embeddings.Open(t.Context(), set, embeddings.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc2.Close() })
	n, err = svc2.Store().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, n, "indexed data survives a restart")
	raw2, err := os.ReadFile(credPath)
	require.NoError(t, err)
	assert.NotEqual(t, string(raw), string(raw2), "credentials are generated per start")
	requireSearchFinds(t, svc2, covers, "fish", "bafyFish")

	// A changed dimension is reported instead of silently reusing the index.
	require.NoError(t, svc2.Close())
	set16 := set
	set16.Embedder.Dimensions = 16
	_, err = embeddings.Open(t.Context(), set16, embeddings.Options{})
	require.ErrorIs(t, err, embeddings.ErrDimensionMismatch)
	_, running = surrealProcess(t, dataDir)
	assert.False(t, running, "a failed Open leaves nothing running")
}

// embeddingsFreePort returns a loopback port nothing listens on. It does not
// share the helper of the agent tests, which a noagent build leaves out.
func embeddingsFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	return port
}

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// assertNotReachableOffLoopback dials the port on every non-loopback address
// of this machine; none may answer.
func assertNotReachableOffLoopback(t *testing.T, port int) {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	require.NoError(t, err)
	tried := 0
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() || ipn.IP.IsLinkLocalUnicast() || ipn.IP.To4() == nil {
			continue
		}
		tried++
		c, err := net.DialTimeout("tcp", net.JoinHostPort(ipn.IP.String(), strconv.Itoa(port)), 2*time.Second)
		if err == nil {
			_ = c.Close()
			t.Fatalf("the embedded store answers on %s:%d: it is not loopback only", ipn.IP, port)
		}
	}
	t.Logf("checked %d non-loopback IPv4 addresses", tried)
}

// startSurrealRemote starts a standalone surreal (the "remote" server) on a
// free port in its own process group.
func startSurrealRemote(t *testing.T, bin, user, pass string) string {
	t.Helper()
	port := embeddingsFreePort(t)
	cmd := exec.Command(bin, "start", "--no-banner", "--bind", "127.0.0.1:"+port, "memory")
	cmd.Env = append(os.Environ(), "SURREAL_USER="+user, "SURREAL_PASS="+pass)
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	ownProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		killProcessGroup(cmd)
		_ = cmd.Wait()
	})
	base := "http://127.0.0.1:" + port
	eventually(t, 30*time.Second, func() bool {
		r, err := http.Get(base + "/health")
		if err != nil {
			return false
		}
		_ = r.Body.Close()
		return r.StatusCode == http.StatusOK
	}, "separate surreal did not start: %s", out.String())
	return base
}

// TestMCPEmbeddingsStoreRemote uses a separately started server, with the
// credential taken from an environment variable through the config path.
func TestMCPEmbeddingsStoreRemote(t *testing.T) {
	bin := surrealBinary(t)
	const user, pass = "root", "remote-test-password"
	base := startSurrealRemote(t, bin, user, pass)
	embed := fakeEmbedServer(t, 8, fakeAxes)
	covers := coversOnly(strings.TrimPrefix(embed.URL, "http://"), strings.TrimPrefix(base, "http://"))

	t.Setenv("CLI_MCP_TEST_SURREAL_CRED", user+":"+pass)
	cfgJSON := fmt.Sprintf(`{"embeddings":{"mode":"remote",
	  "remote":{"endpoint":%q,"namespace":"ipfs","database":"remote_idx","credential":{"env":"CLI_MCP_TEST_SURREAL_CRED"}},
	  "embedder":{"base_url":%q,"model":"fake","dimensions":8}}}`, base, embed.URL+"/v1")
	cfg, err := mcpserver.ParseConfig([]byte(cfgJSON))
	require.NoError(t, err)
	set, err := cfg.Embeddings.Settings()
	require.NoError(t, err)

	svc, err := embeddings.Open(t.Context(), set, embeddings.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Close() })
	assert.Nil(t, svc.Supervisor(), "remote mode starts no process")

	indexThreeDocs(t, svc, covers)
	requireSearchFinds(t, svc, covers, "cats", "bafyCats")
	n, err := svc.Store().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	// Indexing a CID again replaces its rows instead of adding to them.
	_, err = svc.Index(t.Context(), covers, embeddings.Document{CID: "bafyCats", Chunks: []string{"cats a", "cats b"}})
	require.NoError(t, err)
	n, err = svc.Store().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	// The remote store host must be acknowledged as well as the embedder.
	_, err = svc.Search(t.Context(), coversOnly(strings.TrimPrefix(embed.URL, "http://")), "cats", 1)
	var eg *embeddings.EgressError
	require.ErrorAs(t, err, &eg)
	assert.Equal(t, []string{strings.TrimPrefix(base, "http://")}, eg.Hosts)

	// Wrong credentials are reported without echoing them.
	t.Setenv("CLI_MCP_TEST_SURREAL_CRED", user+":wrong-password")
	bad, err := cfg.Embeddings.Settings()
	require.NoError(t, err)
	_, err = embeddings.Open(t.Context(), bad, embeddings.Options{})
	require.ErrorIs(t, err, embeddings.ErrAuth)
	assert.NotContains(t, err.Error(), "wrong-password")
}

func writeEmbeddingsConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mcp.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

var embeddedLine = regexp.MustCompile(`embeddings enabled: embedded store on 127\.0\.0\.1:(\d+)`)

func embeddedConfig(bin, dataDir, embedURL string) string {
	return fmt.Sprintf(`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":%q,"data_dir":%q},
	  "embedder":{"base_url":%q,"model":"fake","dimensions":8}}}`, bin, dataDir, embedURL)
}

// TestMCPEmbeddingsStoreDisabled: without an embeddings section no process is
// started, nothing is announced and no index tool is listed.
func TestMCPEmbeddingsStoreDisabled(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()

	for name, args := range map[string][]string{
		"no config":         nil,
		"config without it": {"--mcp-config", writeEmbeddingsConfig(t, `{}`)},
	} {
		t.Run(name, func(t *testing.T) {
			p := startMCP(t, node.IPFSBin, node.Dir, args...)
			p.initialize()
			for _, n := range p.toolNames() {
				assert.False(t, strings.HasPrefix(n, "ipfs_index_"), "index tool %s listed while embeddings are off", n)
			}
			for _, e := range psTable(t) {
				if e.ppid == p.cmd.Process.Pid {
					t.Errorf("ipfs mcp has a child process while embeddings are off: %s", e.args)
				}
			}
			assert.NotContains(t, p.stderr.String(), "embeddings")
			assert.Zero(t, p.closeAndWait())
		})
	}
}

// TestMCPEmbeddingsStoreConfigErrors: invalid sections stop ipfs mcp before it
// starts anything, naming the key.
func TestMCPEmbeddingsStoreConfigErrors(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	t.Cleanup(func() { node.StopDaemon() }) // parallel subtests outlive a defer
	embedder := `"embedder":{"base_url":"http://127.0.0.1:9/v1","model":"m","dimensions":8}`

	tests := map[string]struct{ cfg, want string }{
		"relative binary":  {`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"surreal","data_dir":"/tmp/x"},` + embedder + `}}`, "surreal_binary"},
		"remote no URL":    {`{"embeddings":{"mode":"remote","remote":{"namespace":"n","database":"d"},` + embedder + `}}`, "endpoint"},
		"remote no block":  {`{"embeddings":{"mode":"remote",` + embedder + `}}`, "remote"},
		"missing binary":   {`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"/nonexistent/surreal","data_dir":"` + t.TempDir() + `/d"},` + embedder + `}}`, "surreal_binary"},
		"unset credential": {`{"embeddings":{"mode":"remote","remote":{"endpoint":"http://127.0.0.1:9","namespace":"n","database":"d","credential":{"env":"CLI_MCP_UNSET_CRED"}},` + embedder + `}}`, "embeddings.remote.credential"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, code := runMCP(t, node.IPFSBin, node.Dir, "--mcp-config", writeEmbeddingsConfig(t, tt.cfg))
			assert.NotZero(t, code, "stdout: %s", stdout)
			assert.Contains(t, stderr, tt.want)
		})
	}
}

// TestMCPEmbeddingsStoreKillCleanup: killing ipfs mcp with SIGKILL leaves no
// surreal behind (the watchdog), and when the watchdog is also gone the next
// start reaps the orphan.
func TestMCPEmbeddingsStoreKillCleanup(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	embed := fakeEmbedServer(t, 8, fakeAxes)
	dataDir := filepath.Join(t.TempDir(), "idx")
	cfg := writeEmbeddingsConfig(t, embeddedConfig(bin, dataDir, embed.URL+"/v1"))

	start := func() (*mcpProc, int, int) {
		p := startMCP(t, node.IPFSBin, node.Dir, "--mcp-config", cfg)
		eventually(t, 60*time.Second, func() bool { return embeddedLine.MatchString(p.stderr.String()) },
			"embedded store did not start; stderr: %s", p.stderr.String())
		port, _ := strconv.Atoi(embeddedLine.FindStringSubmatch(p.stderr.String())[1])
		proc, ok := surrealProcess(t, dataDir)
		require.True(t, ok)
		assert.Equal(t, p.cmd.Process.Pid, proc.ppid, "surreal is a child of ipfs mcp")
		return p, proc.pid, port
	}

	// 1. SIGKILL: the watchdog stops the store.
	p, surrealPID, port := start()
	assert.NotContains(t, mcpserver.ReservedPorts, port)
	require.NoError(t, p.cmd.Process.Kill())
	_ = p.cmd.Wait()
	eventually(t, 30*time.Second, func() bool { return !pidAlive(surrealPID) },
		"surreal pid %d outlived a SIGKILLed ipfs mcp", surrealPID)

	// 2. SIGKILL with the watchdog removed too: the next start reaps the orphan.
	p, surrealPID, _ = start()
	for _, e := range processesMentioning(t, filepath.Join(dataDir, "db")) {
		if e.pid != surrealPID && strings.Contains(e.args, "kill -0") {
			// ESRCH: it was a transient child of the watchdog, already gone.
			if err := syscall.Kill(-e.pid, syscall.SIGKILL); err != nil {
				require.ErrorIs(t, err, syscall.ESRCH, "stop the watchdog %d", e.pid)
			}
		}
	}
	require.NoError(t, p.cmd.Process.Kill())
	_ = p.cmd.Wait()
	time.Sleep(2 * time.Second)
	require.True(t, pidAlive(surrealPID), "with no watchdog the orphan is expected to survive until the next start")

	p2, newPID, _ := start()
	assert.NotEqual(t, surrealPID, newPID)
	eventually(t, 15*time.Second, func() bool { return !pidAlive(surrealPID) }, "the orphan %d was not reaped", surrealPID)

	// 3. A clean exit (stdin closed) stops the store.
	assert.Zero(t, p2.closeAndWait())
	eventually(t, 15*time.Second, func() bool { return !pidAlive(newPID) }, "surreal %d outlived a clean exit", newPID)
	_, running := surrealProcess(t, dataDir)
	assert.False(t, running)
}
