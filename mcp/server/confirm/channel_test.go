package confirm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortDir returns a short private directory: Unix socket paths are limited
// to about 100 bytes, which t.TempDir often exceeds.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "cf")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

type chanEnv struct {
	m       *Manager
	dir     string
	key     string
	results chan decisionFrom
}

type decisionFrom struct {
	resolver string
	approve  bool
}

// newChanEnv builds a manager with the desktop (paired) and cli resolvers as
// requested, and opens one confirmation on each.
func newChanEnv(t *testing.T, modes ...string) *chanEnv {
	t.Helper()
	dir := shortDir(t)
	pairing := filepath.Join(dir, "pairing.json")
	key, err := Pair(pairing)
	require.NoError(t, err)
	m, err := Build(Config{Modes: modes, SocketDir: filepath.Join(dir, "s"), PairingFile: pairing}, quiet)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.NotNil(t, m.ch)
	e := &chanEnv{m: m, dir: dir, key: key, results: make(chan decisionFrom, 8)}
	for _, r := range m.Resolvers() {
		resolve := func(approve bool) error {
			e.results <- decisionFrom{r.Name(), approve}
			return nil
		}
		require.NoError(t, r.Request(t.Context(), pending("Remove the pin "+r.Name()), resolve))
	}
	return e
}

// do sends one request. auth "v2:<key>" signs it with channel authentication
// version 2 for that pairing key; any other non-empty auth is sent as the
// Authorization header; cli sets the cli marker.
func (e *chanEnv) do(t *testing.T, method, path, auth, cli string, body any) (int, map[string]any) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	h := http.Header{}
	if key, ok := strings.CutPrefix(auth, "v2:"); ok {
		h = e.sign(key, method, path, raw)
	} else if auth != "" {
		h.Set("Authorization", auth)
	}
	if cli != "" {
		h.Set(cliHeader, cli)
	}
	status, _, out := e.raw(t, method, path, h, raw)
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return status, m
}

// sign builds the version 2 headers for key, bound to this channel's socket.
func (e *chanEnv) sign(key, method, path string, body []byte) http.Header {
	k := sha256.Sum256([]byte(key))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := newNonce()
	h := http.Header{}
	h.Set(authVersionHeader, authVersion)
	h.Set(timestampHeader, ts)
	h.Set(nonceHeader, nonce)
	h.Set(signatureHeader, signRequest(k[:], e.m.ch.socketName(), method, path, ts, nonce, body))
	return h
}

// raw sends a request with exactly these headers and returns the status, the
// response headers and the raw body.
func (e *chanEnv) raw(t *testing.T, method, path string, h http.Header, body []byte) (int, http.Header, []byte) {
	t.Helper()
	sock := e.m.ch.sockPath
	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sock)
	}}}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, "http://confirm"+path, rd)
	require.NoError(t, err)
	for k, v := range h {
		req.Header[k] = v
	}
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, out
}

func (e *chanEnv) none(t *testing.T) {
	t.Helper()
	select {
	case d := <-e.results:
		t.Fatalf("a rejected request resolved a confirmation: %+v", d)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestChannelRejectsUnauthenticatedAndForgedClients(t *testing.T) {
	e := newChanEnv(t, ModeDesktop, ModeCLI)
	body := map[string]any{"id": "cf_1", "approve": true}

	// No pairing key.
	status, _ := e.do(t, http.MethodPost, resolvePath, "", "", body)
	assert.Equal(t, http.StatusUnauthorized, status)
	status, _ = e.do(t, http.MethodGet, pendingPath, "", "", nil)
	assert.Equal(t, http.StatusUnauthorized, status, "an unpaired client cannot even list")

	// A bearer key is retired, whether forged or the real one.
	for _, bearer := range []string{"Bearer forged", "Bearer " + e.key, "Basic " + e.key, e.key} {
		status, out := e.do(t, http.MethodPost, resolvePath, bearer, "", body)
		assert.Equal(t, http.StatusUnauthorized, status, bearer)
		assert.Equal(t, codeUnsupported, out["code"], bearer)
		status, _ = e.do(t, http.MethodGet, pendingPath, bearer, "", nil)
		assert.Equal(t, http.StatusUnauthorized, status, bearer)
	}

	// A signature made with a forged key, and near misses of the real one.
	for _, forged := range []string{"forged", e.key + "x", e.key[1:]} {
		status, out := e.do(t, http.MethodPost, resolvePath, "v2:"+forged, "", body)
		assert.Equal(t, http.StatusForbidden, status, forged)
		assert.Equal(t, codeInvalid, out["code"], forged)
	}

	// The CLI marker with a wrong value.
	status, _ = e.do(t, http.MethodPost, resolvePath, "", "yes", body)
	assert.Equal(t, http.StatusUnauthorized, status)
	e.none(t)
}

func TestChannelPairedDesktopResolves(t *testing.T) {
	e := newChanEnv(t, ModeDesktop)
	status, out := e.do(t, http.MethodGet, pendingPath, "v2:"+e.key, "", nil)
	require.Equal(t, http.StatusOK, status)
	list := out["pending"].([]any)
	require.Len(t, list, 1)
	assert.Contains(t, list[0].(map[string]any)["summary"], "Remove the pin")

	status, _ = e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{"id": "cf_1", "approve": true})
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, decisionFrom{server.ResolverDesktop, true}, <-e.results)

	status, _ = e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{"id": "cf_other", "approve": true})
	assert.Equal(t, http.StatusNotFound, status)
	status, _ = e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{"id": "cf_1", "approve": true, "extra": 1})
	assert.Equal(t, http.StatusBadRequest, status, "unknown fields are rejected")
}

func TestChannelCLIModeIsRefusedUnlessEnabled(t *testing.T) {
	e := newChanEnv(t, ModeDesktop) // paired, but no --confirm=cli
	status, out := e.do(t, http.MethodPost, resolvePath, "", cliValue, map[string]any{"id": "cf_1", "approve": true})
	assert.Equal(t, http.StatusForbidden, status)
	assert.Contains(t, out["error"], "--confirm=cli")
	e.none(t)

	// Even the paired desktop client cannot answer another client's item: the
	// cli table is empty here.
	status, _ = e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{"id": "cf_x", "approve": true})
	assert.Equal(t, http.StatusNotFound, status)
}

func TestChannelCLIEnabledResolvesOnlyItsOwnItems(t *testing.T) {
	e := newChanEnv(t, ModeCLI)
	status, _ := e.do(t, http.MethodPost, resolvePath, "", cliValue, map[string]any{"id": "cf_1", "approve": false})
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, decisionFrom{server.ResolverCLI, false}, <-e.results)
}

func TestChannelItemsDisappearWhenTheConfirmationExpires(t *testing.T) {
	dir := shortDir(t)
	m, err := Build(Config{Modes: []string{ModeCLI}, SocketDir: dir}, quiet)
	require.NoError(t, err)
	defer m.Close()
	ctx, cancel := context.WithCancel(t.Context())
	require.NoError(t, m.Resolvers()[0].Request(ctx, pending("x"), func(bool) error { return nil }))
	e := &chanEnv{m: m}
	status, out := e.do(t, http.MethodGet, pendingPath, "", cliValue, nil)
	require.Equal(t, http.StatusOK, status)
	require.Len(t, out["pending"], 1)
	cancel()
	require.Eventually(t, func() bool {
		_, out := e.do(t, http.MethodGet, pendingPath, "", cliValue, nil)
		return len(out["pending"].([]any)) == 0
	}, 5*time.Second, 10*time.Millisecond)
}

func TestChannelMapsBrokerErrors(t *testing.T) {
	dir := shortDir(t)
	m, err := Build(Config{Modes: []string{ModeCLI}, SocketDir: dir}, quiet)
	require.NoError(t, err)
	defer m.Close()
	require.NoError(t, m.Resolvers()[0].Request(t.Context(), pending("x"), func(bool) error { return server.ErrConfirmationExpired }))
	e := &chanEnv{m: m}
	status, _ := e.do(t, http.MethodPost, resolvePath, "", cliValue, map[string]any{"id": "cf_1", "approve": true})
	assert.Equal(t, http.StatusGone, status)
}

func TestSocketIsPrivate(t *testing.T) {
	e := newChanEnv(t, ModeCLI)
	fi, err := os.Stat(e.m.ch.sockPath)
	require.NoError(t, err)
	assert.Zero(t, fi.Mode().Perm()&0o077, "socket is owner-only")
	di, err := os.Stat(filepath.Dir(e.m.ch.sockPath))
	require.NoError(t, err)
	assert.Zero(t, di.Mode().Perm()&0o077)

	loose := shortDir(t)
	require.NoError(t, os.Chmod(loose, 0o755))
	_, err = Build(Config{Modes: []string{ModeCLI}, SocketDir: loose}, quiet)
	require.Error(t, err, "a directory other users can enter is refused")
}

func TestPairingStoresOnlyAHash(t *testing.T) {
	path := filepath.Join(shortDir(t), "p.json")
	key, err := Pair(path)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), key, "the key is never stored")
	v, err := loadVerifier(path)
	require.NoError(t, err)
	assert.True(t, keyMatches(v, key))
	assert.False(t, keyMatches(v, key+"a"))
	assert.False(t, keyMatches(nil, key))
	assert.False(t, keyMatches(v, ""))

	_, err = loadVerifier(filepath.Join(filepath.Dir(path), "missing.json"))
	assert.ErrorIs(t, err, ErrNotPaired)
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"verifier":"zz"}`), 0o600))
	_, err = loadVerifier(path)
	assert.Error(t, err)

	key2, err := Pair(path)
	require.NoError(t, err)
	v2, _ := loadVerifier(path)
	assert.NotEqual(t, key, key2)
	assert.False(t, keyMatches(v2, key), "pairing again replaces the old key")
}

func TestRunCLIRefusals(t *testing.T) {
	in, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer in.Close()
	tty := func(*os.File) bool { return true }
	noTTY := func(*os.File) bool { return false }

	// `ipfs mcp confirm` without --confirm=cli.
	err = RunCLI(t.Context(), CLIOptions{In: in, Out: io.Discard, IsTerminal: tty})
	require.ErrorIs(t, err, ErrCLINotEnabled)
	err = RunCLI(t.Context(), CLIOptions{Modes: []string{ModeNative}, In: in, Out: io.Discard, IsTerminal: tty})
	require.ErrorIs(t, err, ErrCLINotEnabled)

	// With the opt-in but stdin is not a terminal (a pipe, a script, an agent).
	err = RunCLI(t.Context(), CLIOptions{Modes: []string{ModeCLI}, In: in, Out: io.Discard, IsTerminal: noTTY})
	require.ErrorIs(t, err, ErrNoTTY)
	err = RunCLI(t.Context(), CLIOptions{Modes: []string{ModeCLI}, In: nil, Out: io.Discard})
	require.ErrorIs(t, err, ErrNoTTY)

	// Real terminal check on a non-terminal file.
	err = RunCLI(t.Context(), CLIOptions{Modes: []string{ModeCLI}, In: in, Out: io.Discard})
	require.ErrorIs(t, err, ErrNoTTY)

	// Opt-in and a terminal, but no server.
	err = RunCLI(t.Context(), CLIOptions{Modes: []string{ModeCLI}, SocketDir: shortDir(t), In: in, Out: io.Discard, IsTerminal: tty})
	require.ErrorIs(t, err, ErrNoServer)
}

func TestRunCLIApproveAndDeny(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   bool
	}{{"yes\n", true}, {"YES\n", true}, {"y\n", false}, {"no\n", false}, {"\n", false}} {
		t.Run(strings.TrimSpace(tc.answer), func(t *testing.T) {
			e := newChanEnv(t, ModeCLI)
			in := stdinWith(t, tc.answer)
			var out bytes.Buffer
			err := RunCLI(t.Context(), CLIOptions{
				Modes: []string{ModeCLI}, SocketDir: filepath.Dir(e.m.ch.sockPath), In: in, Out: &out,
				IsTerminal: func(*os.File) bool { return true },
			})
			require.NoError(t, err)
			assert.Contains(t, out.String(), "Remove the pin")
			assert.Equal(t, decisionFrom{server.ResolverCLI, tc.want}, <-e.results)
		})
	}
}

func TestRunCLINamedAndAmbiguous(t *testing.T) {
	e := newChanEnv(t, ModeCLI)
	dir := filepath.Dir(e.m.ch.sockPath)
	tty := func(*os.File) bool { return true }
	err := RunCLI(t.Context(), CLIOptions{Modes: []string{ModeCLI}, SocketDir: dir, ID: "cf_nope", In: stdinWith(t, "yes\n"), Out: io.Discard, IsTerminal: tty})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cf_nope")
	e.none(t)

	got, err := choose([]PendingView{{ID: "a"}, {ID: "b"}}, "")
	assert.Nil(t, got)
	assert.ErrorContains(t, err, "several")
}

// stdinWith returns a file whose content is s.
func stdinWith(t *testing.T, s string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(shortDir(t), "in")
	require.NoError(t, err)
	_, err = f.WriteString(s)
	require.NoError(t, err)
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestChannelResolvedItemLeavesEveryClientsList(t *testing.T) {
	e := newChanEnv(t, ModeDesktop)
	resolved := 0
	second := pending("second")
	second.ID = "cf_2"
	require.NoError(t, e.m.Resolvers()[0].Request(t.Context(), second, func(bool) error {
		resolved++
		if resolved > 1 {
			return server.ErrConfirmationResolved
		}
		return nil
	}))
	list := func() []any {
		status, out := e.do(t, http.MethodGet, pendingPath, "v2:"+e.key, "", nil)
		require.Equal(t, http.StatusOK, status)
		return out["pending"].([]any)
	}
	require.Len(t, list(), 2)

	// One client resolves; a second client listing afterwards no longer sees it.
	status, _ := e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{"id": "cf_2", "approve": false})
	require.Equal(t, http.StatusOK, status)
	remaining := list()
	require.Len(t, remaining, 1)
	assert.Equal(t, "cf_1", remaining[0].(map[string]any)["id"])

	// A repeat resolve still gets the broker's answer, not a vanished item.
	status, out := e.do(t, http.MethodPost, resolvePath, "v2:"+e.key, "", map[string]any{"id": "cf_2", "approve": true})
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, out["error"], "already resolved")
}
