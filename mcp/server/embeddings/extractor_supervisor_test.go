//go:build !windows

package embeddings

import (
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeXbergBinary writes an executable that runs the test binary as `xberg`.
func fakeXbergBinary(t *testing.T, version string) string {
	t.Helper()
	return fakeXbergWithEnv(t, version)
}

// fakeXbergWithEnv is fakeXbergBinary with more variables for the fake, such as
// FAKE_XBERG_HEALTH_VERSION=9.9.9. The supervisor starts xberg with a minimal
// environment, so they travel in the script.
func fakeXbergWithEnv(t *testing.T, version string, env ...string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "xberg")
	script := "#!/bin/sh\nexec env FAKE_XBERG=1 FAKE_XBERG_VERSION=" + version + " " + strings.Join(env, " ") + " " + exe + " \"$@\"\n"
	require.NoError(t, os.WriteFile(p, []byte(script), 0o755))
	return p
}

func readFakeRecord(t *testing.T, cacheDir string) fakeXbergRecord {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cacheDir, "fake-xberg.json"))
	require.NoError(t, err)
	var rec fakeXbergRecord
	require.NoError(t, json.Unmarshal(b, &rec))
	return rec
}

func TestStartExtractorSupervisorRefusals(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plain := filepath.Join(t.TempDir(), "xberg")
	require.NoError(t, os.WriteFile(plain, []byte("#!/bin/sh\necho 'xberg 1.3.10'\n"), 0o644))
	tests := []struct {
		name string
		cfg  ExtractorEmbedded
		want string
	}{
		{"relative path", ExtractorEmbedded{Binary: "xberg", CacheDir: dir}, "absolute path"},
		{"relative cache dir", ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: "cache"}, "cache_dir"},
		{"missing binary", ExtractorEmbedded{Binary: filepath.Join(dir, "nope"), CacheDir: dir}, "xberg_binary"},
		{"directory", ExtractorEmbedded{Binary: dir, CacheDir: dir}, "not a regular file"},
		{"not executable", ExtractorEmbedded{Binary: plain, CacheDir: dir}, "not executable"},
		{"old version", ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.9"), CacheDir: dir}, "older than the minimum 1.3.10"},
		{"older than a raised minimum", ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: dir, MinVersion: "1.4.0"}, "older than the minimum 1.4.0"},
		{"no version reported", ExtractorEmbedded{Binary: fakeXbergBinary(t, "hello"), CacheDir: dir}, "does not report a version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := StartExtractorSupervisor(t.Context(), tt.cfg, nil)
			require.Nil(t, s)
			require.ErrorContains(t, err, tt.want)
		})
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "a refused start must not touch the cache dir")
}

func TestExtractorSupervisorStartsOnLoopbackAndStopsOnClose(t *testing.T) {
	// Not parallel: it sets environment variables the supervisor must not pass on.
	t.Setenv("XBERG_API_ALLOW_LOCAL_URI_INPUTS", "1")
	t.Setenv("XBERG_LLM_API_KEY", "sk-should-not-travel")
	t.Setenv("OPENAI_API_KEY", "sk-should-not-travel-either")
	t.Setenv("HF_ENDPOINT", "https://hf-mirror.example")
	cache := t.TempDir()
	cfg := ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.4.2"), CacheDir: cache}

	x, err := OpenExtractor(t.Context(), ExtractorSettings{Mode: ModeEmbedded, Embedded: &cfg}, nil, Options{Logger: slog.New(slog.DiscardHandler)})
	require.NoError(t, err)
	sup := x.Supervisor()
	require.NotNil(t, sup)
	require.False(t, x.Remote(), "an embedded extractor needs no egress acknowledgement")

	host, port, err := net.SplitHostPort(sup.Addr())
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", host, "the bind host is loopback")
	require.NotEqual(t, "0", port)
	pid := sup.PID()
	require.Positive(t, pid)
	require.True(t, pidAlive(pid))

	rec := readFakeRecord(t, cache)
	require.Equal(t, []string{"serve", "--host", "127.0.0.1", "--port", port, "--config", filepath.Join(cache, xbergConfigFile)}, rec.Args)
	require.Equal(t, evalSymlinks(t, cache), evalSymlinks(t, rec.Cwd))
	env := strings.Join(rec.Env, "\n")
	require.Contains(t, env, "HF_HUB_OFFLINE=1", "downloads are off unless allowed")
	require.Contains(t, env, "XBERG_CACHE_DIR="+cache)
	require.Contains(t, env, "HF_HOME="+filepath.Join(cache, "huggingface"))
	require.Contains(t, env, "XBERG_CORS_ORIGINS=http://127.0.0.1:"+port)
	require.Contains(t, env, "DO_NOT_TRACK=1", "xberg is asked not to track")
	require.Contains(t, env, "HF_ENDPOINT=https://hf-mirror.example", "the operator's Hugging Face mirror is passed on")
	for _, banned := range []string{"XBERG_API_ALLOW_LOCAL_URI_INPUTS", "XBERG_LLM_API_KEY", "OPENAI_API_KEY", "sk-should-not"} {
		require.NotContains(t, env, banned, "the operator's environment must not reach xberg")
	}

	// The client does not talk to xberg's own port: it goes through the
	// authenticated proxy, whose token is the client's credential.
	require.NotEqual(t, sup.Endpoint(), x.endpoint[:len(x.endpoint)-len("/extract")])
	require.Equal(t, sup.ProxyEndpoint()+"/extract", x.endpoint)
	require.NotEmpty(t, x.cred)
	got, err := x.Extract(t.Context(), nil, ExtractRequest{Filename: "a.pdf", MIME: "application/pdf", Data: []byte("%PDF-1.4")})
	require.NoError(t, err)
	require.Equal(t, "from the fake xberg", got.Markdown)

	_, err = os.Stat(filepath.Join(cache, xbergStateFile))
	require.NoError(t, err, "the child is recorded for crash cleanup")

	require.NoError(t, x.Close())
	select {
	case <-sup.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("xberg did not stop on Close")
	}
	require.Eventually(t, func() bool { return !pidAlive(pid) }, 5*time.Second, 50*time.Millisecond, "the process is gone")
	_, err = os.Stat(filepath.Join(cache, xbergStateFile))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, x.Close(), "Close is idempotent")
	_, err = x.Extract(t.Context(), nil, ExtractRequest{Filename: "a.pdf", Data: []byte("x")})
	require.ErrorIs(t, err, ErrExtractorUnavailable, "nothing listens after Close")
}

func evalSymlinks(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return r
}

func TestExtractorSupervisorModelDownloadsOnlyWhenAllowed(t *testing.T) {
	cache := t.TempDir()
	cfg := ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: cache, AllowModelDownloads: true}
	sup, err := StartExtractorSupervisor(t.Context(), cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sup.Close() })
	require.NotContains(t, strings.Join(readFakeRecord(t, cache).Env, "\n"), "HF_HUB_OFFLINE")
}

// fixedPortListener is a net.Listener that only reports a port.
type fixedPortListener struct{ port int }

func (l fixedPortListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l fixedPortListener) Close() error              { return nil }
func (l fixedPortListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: l.port}
}

func TestPickPortSkipsListedPorts(t *testing.T) {
	t.Parallel()
	offered := []int{4001, 5001, 31337}
	next := 0
	got, err := pickPort([]int{4001, 5001}, func() (net.Listener, error) {
		l := fixedPortListener{offered[next]}
		next++
		return l, nil
	})
	require.NoError(t, err)
	require.Equal(t, 31337, got, "the two listed ports are passed over")
	require.Equal(t, 3, next)

	_, err = pickPort([]int{4001}, func() (net.Listener, error) { return fixedPortListener{4001}, nil })
	require.ErrorContains(t, err, "reserved ports", "an OS that only offers listed ports is an error, not a loop")
}

func TestExtractorSupervisorStartsWithAnAvoidList(t *testing.T) {
	cache := t.TempDir()
	cfg := ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: cache, AvoidPorts: []int{4001, 5001, 5050}}
	sup, err := StartExtractorSupervisor(t.Context(), cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sup.Close() })
	_, port, _ := net.SplitHostPort(sup.Addr())
	require.NotContains(t, []string{"4001", "5001", "5050"}, port)
}

func TestExtractorSupervisorRefusesAForeignResponder(t *testing.T) {
	cfg := ExtractorEmbedded{Binary: fakeXbergWithEnv(t, "1.3.10", "FAKE_XBERG_HEALTH_VERSION=9.9.9"), CacheDir: t.TempDir()}
	start := time.Now()
	sup, err := StartExtractorSupervisor(t.Context(), cfg, nil)
	require.Nil(t, sup)
	require.ErrorIs(t, err, errXbergForeign)
	require.ErrorContains(t, err, `"9.9.9"`)
	require.Less(t, time.Since(start), 20*time.Second, "a wrong version fails at once, not after the start timeout")
	_, err = os.Stat(filepath.Join(cfg.CacheDir, xbergStateFile))
	require.ErrorIs(t, err, os.ErrNotExist, "nothing is left recorded")
}

func TestExtractorSupervisorRefusesAnUnsafeCacheDir(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	require.NoError(t, os.Chmod(cache, 0o775))
	sup, err := StartExtractorSupervisor(t.Context(), ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: cache}, nil)
	require.Nil(t, sup)
	require.ErrorContains(t, err, "writable by its group or by everyone")
	require.ErrorContains(t, err, "cache_dir")

	require.NoError(t, os.Chmod(cache, 0o757))
	_, err = StartExtractorSupervisor(t.Context(), ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: cache}, nil)
	require.ErrorContains(t, err, "writable by its group or by everyone")

	require.NoError(t, os.Chmod(cache, 0o755))
	sup, err = StartExtractorSupervisor(t.Context(), ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: cache}, nil)
	require.NoError(t, err, "a directory others can read but not write is fine")
	require.NoError(t, sup.Close())
}

func TestSecondExtractorSupervisorDoesNotKillTheFirstsXberg(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	cfg := ExtractorEmbedded{Binary: fakeXbergBinary(t, "1.3.10"), CacheDir: cache}
	first, err := StartExtractorSupervisor(t.Context(), cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	pid := first.PID()

	second, err := StartExtractorSupervisor(t.Context(), cfg, nil)
	require.Nil(t, second)
	require.ErrorContains(t, err, "in use by another ipfs mcp")
	require.ErrorContains(t, err, "its own cache_dir")
	require.True(t, pidAlive(pid), "the first ipfs mcp's xberg is untouched")
	select {
	case <-first.Done():
		t.Fatal("the first xberg exited")
	default:
	}
	_, err = os.Stat(filepath.Join(cache, xbergStateFile))
	require.NoError(t, err, "the first's record of its child is kept")

	// Once the first is gone the directory is free again.
	require.NoError(t, first.Close())
	third, err := StartExtractorSupervisor(t.Context(), cfg, nil)
	require.NoError(t, err)
	require.NoError(t, third.Close())
}

func TestExtractorReapStaleStopsOnlyTheRecordedProcess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := &ExtractorSupervisor{cfg: ExtractorEmbedded{CacheDir: dir}, log: slog.New(slog.DiscardHandler)}

	leftover := exec.Command("sleep", "31341")
	require.NoError(t, leftover.Start())
	exited := make(chan struct{})
	go func() { _ = leftover.Wait(); close(exited) }()
	t.Cleanup(func() { _ = leftover.Process.Kill() })
	require.NoError(t, writeState(dir, state{PID: leftover.Process.Pid, KV: "sleep 31341"}))
	// writeState names the supervisor.json of the surreal store; the extractor
	// reads its own file name.
	require.NoError(t, os.Rename(filepath.Join(dir, stateFileName), filepath.Join(dir, xbergStateFile)))
	require.NoError(t, s.reapStale())
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		t.Fatal("leftover xberg was not stopped")
	}

	other := exec.Command("sleep", "31342")
	require.NoError(t, other.Start())
	t.Cleanup(func() { _ = other.Process.Kill(); _ = other.Wait() })
	require.NoError(t, writeState(dir, state{PID: other.Process.Pid, KV: "/not/this/process"}))
	require.NoError(t, os.Rename(filepath.Join(dir, stateFileName), filepath.Join(dir, xbergStateFile)))
	require.NoError(t, s.reapStale())
	require.True(t, pidAlive(other.Process.Pid), "a recycled pid is left alone")
}

func TestParseXbergVersion(t *testing.T) {
	t.Parallel()
	v, err := parseXbergVersion("xberg 1.3.10\n")
	require.NoError(t, err)
	require.Equal(t, version{1, 3, 10}, v)
	_, err = parseXbergVersion("xberg")
	require.Error(t, err)
}
