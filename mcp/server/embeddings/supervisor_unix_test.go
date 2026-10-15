//go:build !windows

package embeddings

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReapStaleStopsOnlyTheRecordedStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := &Supervisor{cfg: EmbeddedSettings{DataDir: dir}, log: slog.New(slog.DiscardHandler)}

	// A process whose command line carries the recorded data path is reaped.
	store := exec.Command("sleep", "31337")
	require.NoError(t, store.Start())
	exited := make(chan struct{})
	go func() { _ = store.Wait(); close(exited) }()
	t.Cleanup(func() { _ = store.Process.Kill() })
	require.NoError(t, writeState(dir, state{PID: store.Process.Pid, KV: "sleep 31337"}))
	require.NoError(t, s.reapStale())
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		t.Fatal("leftover store was not stopped")
	}
	_, err := os.Stat(filepath.Join(dir, stateFileName))
	require.ErrorIs(t, err, os.ErrNotExist)

	// A live process that does not match (a recycled pid) is left alone.
	other := exec.Command("sleep", "31338")
	require.NoError(t, other.Start())
	t.Cleanup(func() { _ = other.Process.Kill(); _ = other.Wait() })
	require.NoError(t, writeState(dir, state{PID: other.Process.Pid, KV: "/some/other/db"}))
	require.NoError(t, s.reapStale())
	require.True(t, pidAlive(other.Process.Pid))
}

func TestReapStaleIgnoresMissingAndCorruptState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := &Supervisor{cfg: EmbeddedSettings{DataDir: dir}, log: slog.New(slog.DiscardHandler)}
	require.NoError(t, s.reapStale())
	require.NoError(t, os.WriteFile(filepath.Join(dir, stateFileName), []byte("{not json"), 0o600))
	require.NoError(t, s.reapStale())
	_, err := os.Stat(filepath.Join(dir, stateFileName))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestChildrenOfTheSupervisorDoNotInheritSecrets: the surreal version check and
// the sh watchdog run with the minimal environment, so a secret in the parent's
// environment reaches neither.
func TestChildrenOfTheSupervisorDoNotInheritSecrets(t *testing.T) {
	t.Setenv("MCP_TEST_SECRET", "sk-parent-secret")
	t.Setenv("OPENAI_API_KEY", "sk-parent-secret-2")

	dump := filepath.Join(t.TempDir(), "env.txt")
	bin := filepath.Join(t.TempDir(), "surreal")
	script := "#!/bin/sh\nenv > '" + dump + "'\necho '3.3.0 for linux on x86_64'\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	require.NoError(t, checkBinary(t.Context(), EmbeddedSettings{Binary: bin}))
	got, err := os.ReadFile(dump)
	require.NoError(t, err)
	require.NotContains(t, string(got), "sk-parent-secret")
	require.NotContains(t, string(got), "MCP_TEST_SECRET")
	require.Contains(t, string(got), "PATH=", "the minimal variables are still passed")

	wd := startWatchdog(os.Getpid(), "kv-path")
	require.NotNil(t, wd, "sh is available on this platform")
	t.Cleanup(func() { stopWatchdog(wd) })
	require.NotNil(t, wd.Env, "an explicit environment, not the parent's")
	for _, kv := range wd.Env {
		require.NotContains(t, kv, "sk-parent-secret")
	}
}
