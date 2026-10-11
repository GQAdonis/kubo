package confirm

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- polkit (R11-H3) --------------------------------------------------------

func TestPolkitUsesAbsoluteSystemPathsEvenWithAPoisonedPATH(t *testing.T) {
	fakeBin := t.TempDir()
	for _, name := range []string{"pkcheck", "pkaction"} {
		require.NoError(t, os.WriteFile(filepath.Join(fakeBin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	run, ran := scripted(pkAuthorized)
	f := newPolkitFactor(run, func(string) error { return nil })
	ok, err := f.authenticate(t.Context(), "Approve IPFS MCP operation: Remove a pin")
	require.NoError(t, err)
	assert.True(t, ok)
	require.Len(t, *ran, 1)
	assert.Equal(t, "/usr/bin/pkcheck", (*ran)[0].name, "the helper is never looked up on PATH")

	require.NoError(t, f.check(t.Context()))
	assert.Equal(t, "/usr/bin/pkaction", (*ran)[1].name)
}

func TestPolkitRefusesAnUntrustedHelper(t *testing.T) {
	untrusted := func(path string) error { return assert.AnError }
	run, ran := scripted(pkAuthorized)
	f := newPolkitFactor(run, untrusted)

	require.ErrorIs(t, f.check(t.Context()), ErrUnavailable)
	ok, err := f.authenticate(t.Context(), "x")
	assert.False(t, ok, "a helper that cannot be trusted never approves")
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Empty(t, *ran, "an untrusted helper is not run at all")
}

func TestTrustedBinaryRequiresARootOwnedFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("the files this test creates would be root-owned")
	}
	bin := filepath.Join(t.TempDir(), "pkcheck")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	err := trustedBinary(bin)
	require.Error(t, err, "a file owned by an ordinary user is not trusted")
	assert.Contains(t, err.Error(), "root")

	require.Error(t, trustedBinary(filepath.Join(t.TempDir(), "missing")))
}

func TestTrustedBinaryRefusesAWritableHelper(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("needs root to create a root-owned, group-writable file")
	}
	bin := filepath.Join(t.TempDir(), "pkcheck")
	require.NoError(t, os.WriteFile(bin, []byte("x"), 0o755))
	require.NoError(t, os.Chmod(bin, 0o775))
	require.ErrorContains(t, trustedBinary(bin), "writable")
}

// --- environment (R11-M1) ---------------------------------------------------

func TestSafeEnvKeepsOnlyAllowlistedVariables(t *testing.T) {
	inherited := []string{
		"PATH=/home/me/evil/bin",
		"HOME=/home/me",
		"DEVELOPER_DIR=/tmp/fake-xcode",
		"TOOLCHAINS=evil",
		"SDKROOT=/tmp/sdk",
		"DYLD_INSERT_LIBRARIES=/tmp/x.dylib",
		"DYLD_FALLBACK_LIBRARY_PATH=/tmp",
		"LD_PRELOAD=/tmp/x.so",
		"LANG=en_US.UTF-8",
		"SECRET_TOKEN=hunter2",
	}
	got := safeEnv(inherited, []string{envLAProbe + "=1"})
	assert.Contains(t, got, "PATH="+trustedPath)
	assert.Contains(t, got, "HOME=/home/me")
	assert.Contains(t, got, "LANG=en_US.UTF-8")
	assert.Contains(t, got, envLAProbe+"=1", "the helper's own parameters are appended")
	joined := strings.Join(got, "\n")
	for _, bad := range []string{"DEVELOPER_DIR", "TOOLCHAINS", "SDKROOT", "DYLD_", "LD_PRELOAD", "SECRET_TOKEN", "/home/me/evil"} {
		assert.NotContains(t, joined, bad)
	}
}

func TestExecRunnerStripsToolchainVariables(t *testing.T) {
	t.Setenv("DEVELOPER_DIR", "/tmp/fake-xcode")
	t.Setenv("TOOLCHAINS", "evil")
	t.Setenv("SDKROOT", "/tmp/sdk")
	t.Setenv("DYLD_FALLBACK_LIBRARY_PATH", "/tmp/evil")
	t.Setenv("PATH", "/tmp/evil-bin:"+os.Getenv("PATH"))
	// sh exits 0 only when none of the variables reached it and PATH is fixed.
	script := `test -z "$DEVELOPER_DIR" && test -z "$TOOLCHAINS" && test -z "$SDKROOT" && ` +
		`test -z "$DYLD_FALLBACK_LIBRARY_PATH" && test "$PATH" = "` + trustedPath + `" && test "$EXTRA" = "1"`
	code, err := execRunner(t.Context(), "/bin/sh", []string{"-c", script}, []string{"EXTRA=1"}, "")
	require.NoError(t, err)
	assert.Zero(t, code, "the helper saw an inherited toolchain variable")
}

// --- macOS probe (judge: unbounded probe) -----------------------------------

func TestMacProbeIsBoundedAndIndependentOfTheRequest(t *testing.T) {
	var started int
	stalled := func(ctx context.Context, _ string, _, _ []string, _ string) (int, error) {
		started++
		<-ctx.Done() // a swift that never starts
		return -1, ctx.Err()
	}
	f := newMacFactor(stalled)
	f.swift = "/bin/sh"
	f.probeTimeout = 50 * time.Millisecond

	begin := time.Now()
	// The request context is never canceled: only the probe's own bound ends it.
	err := f.check(context.Background())
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "did not finish")
	assert.Less(t, time.Since(begin), 5*time.Second)

	require.ErrorIs(t, f.check(context.Background()), ErrUnavailable)
	assert.Equal(t, 1, started, "the failed probe is remembered, later requests do not wait for it again")
}

// --- CLI sanitizing (judge: terminal injection) -----------------------------

func TestRenderPendingStripsTerminalControlSequences(t *testing.T) {
	hostile := PendingView{
		ID:        "cf_1\x1b[2K",
		Tool:      "pin_rm\rpin_add",
		Class:     "destructive\x1b]0;owned\x07",
		Summary:   "Remove the pin\r\x1b[2KAdd a harmless file\u202e\u200b \x08\x7f",
		ExpiresAt: time.Now().Add(time.Minute),
	}
	got := renderPending(hostile)
	for _, r := range got {
		if r == '\n' {
			continue
		}
		assert.False(t, r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0), "control character %q reached the terminal", r)
	}
	assert.NotContains(t, got, "\u202e", "bidirectional override")
	assert.NotContains(t, got, "\u200b", "zero-width space")
	assert.Equal(t, 3, strings.Count(got, "\n"), "only the three lines the renderer writes")
	assert.Contains(t, got, "Remove the pin")
	assert.Contains(t, got, "Add a harmless file", "the text stays visible, it is only defused")
}

func TestRunCLIPrintsOnlySanitizedText(t *testing.T) {
	dir := shortDir(t)
	m, err := Build(Config{Modes: []string{ModeCLI}, SocketDir: dir}, quiet)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	p := pending("Remove the pin\r\x1b[2K(this line was overwritten)")
	require.NoError(t, m.Resolvers()[0].Request(t.Context(), p, func(bool) error { return nil }))

	var out bytes.Buffer
	err = RunCLI(t.Context(), CLIOptions{
		Modes: []string{ModeCLI}, SocketDir: dir, In: stdinWith(t, "no\n"), Out: &out,
		IsTerminal: func(*os.File) bool { return true },
	})
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "\x1b")
	assert.NotContains(t, out.String(), "\r")
	assert.Contains(t, out.String(), "Remove the pin")
}

// --- pairing (R11-H4) -------------------------------------------------------

func TestReplacedPairingRevokesTheOldKeyOnARunningChannel(t *testing.T) {
	e := newChanEnv(t, ModeDesktop)
	pairing := filepath.Join(e.dir, "pairing.json")

	status, _ := e.do(t, http.MethodGet, pendingPath, "Bearer "+e.key, "", nil)
	require.Equal(t, http.StatusOK, status, "the first key works")

	newKey, err := Pair(pairing)
	require.NoError(t, err)

	status, _ = e.do(t, http.MethodGet, pendingPath, "Bearer "+e.key, "", nil)
	assert.Equal(t, http.StatusForbidden, status, "the replaced key stops working without a restart")
	status, _ = e.do(t, http.MethodGet, pendingPath, "Bearer "+newKey, "", nil)
	assert.Equal(t, http.StatusOK, status, "the new key works on the running server")

	require.NoError(t, os.Remove(pairing))
	status, _ = e.do(t, http.MethodGet, pendingPath, "Bearer "+newKey, "", nil)
	assert.Equal(t, http.StatusForbidden, status, "a removed pairing revokes every key")
}

func TestPairWritesTheVerifierAtomically(t *testing.T) {
	path := filepath.Join(shortDir(t), "sub", "p.json")
	_, err := Pair(path)
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file is left behind")
	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Zero(t, fi.Mode().Perm()&0o077)
}

func TestRunPairNeedsATerminal(t *testing.T) {
	pairing := filepath.Join(shortDir(t), "p.json")
	in, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer in.Close()

	_, err = RunPair(PairOptions{PairingFile: pairing, SocketDir: shortDir(t), In: in, IsTerminal: func(*os.File) bool { return false }})
	require.ErrorIs(t, err, ErrPairNoTTY)
	_, err = RunPair(PairOptions{PairingFile: pairing, SocketDir: shortDir(t), In: in})
	require.ErrorIs(t, err, ErrPairNoTTY, "the real check refuses a non-terminal file")
	_, statErr := os.Stat(pairing)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "nothing was written")

	key, err := RunPair(PairOptions{PairingFile: pairing, SocketDir: shortDir(t), In: in, IsTerminal: func(*os.File) bool { return true }})
	require.NoError(t, err)
	assert.NotEmpty(t, key)
}

func TestRunPairRefusesWhileAServerHoldsTheSocket(t *testing.T) {
	e := newChanEnv(t, ModeDesktop)
	pairing := filepath.Join(e.dir, "pairing.json")
	before, err := os.ReadFile(pairing)
	require.NoError(t, err)

	_, err = RunPair(PairOptions{
		PairingFile: pairing, SocketDir: filepath.Dir(e.m.ch.sockPath),
		In: os.Stdin, IsTerminal: func(*os.File) bool { return true },
	})
	require.ErrorIs(t, err, ErrPairServerRunning)
	after, err := os.ReadFile(pairing)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the pairing is untouched")
}

func TestRunPairIgnoresAStaleSocket(t *testing.T) {
	dir := shortDir(t)
	// A socket file that nothing listens on: bind and close without removing.
	sock := filepath.Join(dir, "999999.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	ul := ln.(*net.UnixListener)
	ul.SetUnlinkOnClose(false)
	require.NoError(t, ln.Close())
	_, statErr := os.Stat(sock)
	require.NoError(t, statErr, "the stale socket file exists")

	_, err = RunPair(PairOptions{
		PairingFile: filepath.Join(dir, "p.json"), SocketDir: dir,
		In: os.Stdin, IsTerminal: func(*os.File) bool { return true },
	})
	require.NoError(t, err)
}

// --- private directory owner (R11-L4) ---------------------------------------

func TestDirOwnedByRefusesAnotherUser(t *testing.T) {
	dir := t.TempDir()
	fi, err := os.Stat(dir)
	require.NoError(t, err)
	owner, known := fileOwner(fi)
	if !known {
		t.Skip("this platform reports no file owner")
	}
	require.NoError(t, dirOwnedBy(fi, dir, owner))
	err = dirOwnedBy(fi, dir, owner+1)
	require.Error(t, err, "a directory another user owns is refused")
	assert.Contains(t, err.Error(), "owned by uid")
}

// --- the presence resolvers (R11-H4) ----------------------------------------

func TestDesktopIsNotAPresenceResolver(t *testing.T) {
	assert.False(t, server.IsPresenceResolver(server.ResolverDesktop), "no Desktop window consumes the pairing key yet")
	assert.True(t, server.IsPresenceResolver(server.ResolverMacOSLocalAuth))
	assert.True(t, server.IsPresenceResolver(server.ResolverLinuxPolkit))
}

// --- channel lifecycle and CLI read (R11 round 2) ----------------------------

func TestTwoChannelsInOneProcessCoexist(t *testing.T) {
	dir := shortDir(t)
	cfg := Config{Modes: []string{ModeCLI}, SocketDir: dir}
	first, err := Build(cfg, quiet)
	require.NoError(t, err)
	t.Cleanup(first.Close)
	second, err := Build(cfg, quiet)
	require.NoError(t, err)
	t.Cleanup(second.Close)
	require.NotEqual(t, first.ch.sockPath, second.ch.sockPath, "each channel owns its own socket")

	alive := func(m *Manager) bool {
		e := &chanEnv{m: m}
		status, _ := e.do(t, http.MethodGet, pendingPath, "", cliValue, nil)
		return status == http.StatusOK
	}
	assert.True(t, alive(first), "creating the second channel did not disconnect the first")
	assert.True(t, alive(second))

	first.Close()
	_, err = os.Stat(first.ch.sockPath)
	assert.ErrorIs(t, err, os.ErrNotExist, "a closed channel removes its socket")
	_, err = os.Stat(second.ch.sockPath)
	require.NoError(t, err, "closing the first channel did not remove the second one's socket")
	assert.True(t, alive(second))
}

func TestReadAnswerReturnsWhenTheContextIsCanceled(t *testing.T) {
	pr, pw := io.Pipe() // nothing is ever written: the read never returns
	t.Cleanup(func() { _ = pw.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := readAnswer(ctx, pr, time.Now().Add(time.Hour))
		done <- err
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
		assert.Contains(t, err.Error(), "nothing was approved")
	case <-time.After(5 * time.Second):
		t.Fatal("readAnswer stayed blocked on the terminal after the context ended")
	}
}

func TestReadAnswerReturnsWhenTheConfirmationExpires(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	_, err := readAnswer(t.Context(), pr, time.Now().Add(50*time.Millisecond))
	require.ErrorIs(t, err, ErrAnswerExpired)
}

func TestReadAnswerReturnsALine(t *testing.T) {
	line, err := readAnswer(t.Context(), strings.NewReader("yes\n"), time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, "yes\n", line)
	_, err = readAnswer(t.Context(), strings.NewReader(""), time.Time{})
	require.Error(t, err, "end of input with no answer is an error, not a denial")
}

// failingWriter fails every write, as a closed terminal does.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCLIDoesNotApproveWhenTheOperationCannotBeShown(t *testing.T) {
	e := newChanEnv(t, ModeCLI)
	err := RunCLI(t.Context(), CLIOptions{
		Modes: []string{ModeCLI}, SocketDir: filepath.Dir(e.m.ch.sockPath),
		In: stdinWith(t, "yes\n"), Out: failingWriter{},
		IsTerminal: func(*os.File) bool { return true },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was approved")
	select {
	case d := <-e.results:
		t.Fatalf("a decision was sent although nothing was shown: %+v", d)
	case <-time.After(200 * time.Millisecond):
	}
}
