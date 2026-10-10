package embeddings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func fakeBinary(t *testing.T, versionOutput string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "surreal")
	script := "#!/bin/sh\nif [ \"$1\" = version ]; then echo '" + versionOutput + "'; exit 0; fi\nexit 1\n"
	require.NoError(t, os.WriteFile(p, []byte(script), mode))
	return p
}

func TestStartSupervisorRefusals(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := []struct {
		name string
		cfg  EmbeddedSettings
		want string
	}{
		{"relative path", EmbeddedSettings{Binary: "surreal", DataDir: dir}, "absolute path"},
		{"missing binary", EmbeddedSettings{Binary: filepath.Join(dir, "nope"), DataDir: dir}, "surreal_binary"},
		{"directory", EmbeddedSettings{Binary: dir, DataDir: dir}, "not a regular file"},
		{"not executable", EmbeddedSettings{Binary: fakeBinary(t, "3.3.0 for macos on aarch64", 0o644), DataDir: dir}, "not executable"},
		{"old version", EmbeddedSettings{Binary: fakeBinary(t, "2.9.1 for macos on aarch64", 0o755), DataDir: dir}, "older than the minimum 3.0.0"},
		{"older than a raised minimum", EmbeddedSettings{Binary: fakeBinary(t, "3.3.0 for linux on x86_64", 0o755), DataDir: dir, MinVersion: "3.4.0"}, "older than the minimum 3.4.0"},
		{"no version reported", EmbeddedSettings{Binary: fakeBinary(t, "hello", 0o755), DataDir: dir}, "does not report a version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := StartSupervisor(t.Context(), tt.cfg, nil)
			require.Nil(t, s)
			require.ErrorContains(t, err, tt.want)
		})
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "a refused start must not touch the data dir")
}

func TestParseVersion(t *testing.T) {
	t.Parallel()
	v, err := parseVersion("3.3.0 for macos on aarch64\n")
	require.NoError(t, err)
	require.Equal(t, version{3, 3, 0}, v)
	v, err = parseVersion("v3.10.2-beta.1")
	require.NoError(t, err)
	require.Equal(t, version{3, 10, 2}, v)
	for _, bad := range []string{"", "3.3", "a.b.c", "3.x.1"} {
		_, err := parseVersion(bad)
		require.Error(t, err, bad)
	}
	require.Negative(t, compareVersions(version{2, 9, 9}, version{3, 0, 0}))
	require.Zero(t, compareVersions(version{3, 0, 0}, version{3, 0, 0}))
	require.Positive(t, compareVersions(version{3, 0, 1}, version{3, 0, 0}))
}

func TestCredentialsFileIsPrivate(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "credentials.json")
	require.NoError(t, writeCredentials(p, "abc123"))
	fi, err := os.Stat(p)
	require.NoError(t, err)
	if fi.Mode().Perm() != 0o600 {
		t.Skipf("file modes are not enforced on this platform: %v", fi.Mode().Perm())
	}
	got, err := readCredentials(p)
	require.NoError(t, err)
	require.Equal(t, "abc123", got)
}

func TestPickPortAvoidsListedPorts(t *testing.T) {
	t.Parallel()
	s := &Supervisor{}
	p, err := s.pickPort()
	require.NoError(t, err)
	require.Positive(t, p)
	s.cfg.AvoidPorts = []int{p}
	for range 5 {
		q, err := s.pickPort()
		require.NoError(t, err)
		require.NotEqual(t, p, q)
	}
}
