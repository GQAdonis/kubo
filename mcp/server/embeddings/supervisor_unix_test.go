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
