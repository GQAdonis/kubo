package confirm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeFactor is a presence factor whose outcome the test chooses. It stands in
// for the real prompt: unit tests never raise a Touch ID or polkit dialog.
type fakeFactor struct {
	resolver string
	checkErr error
	ok       bool
	authErr  error
	block    chan struct{} // when set, authenticate waits for it or ctx

	mu      sync.Mutex
	reasons []string
	active  int
	maxSeen int
}

func (f *fakeFactor) name() string                { return f.resolver }
func (f *fakeFactor) check(context.Context) error { return f.checkErr }
func (f *fakeFactor) authenticate(ctx context.Context, reason string) (bool, error) {
	f.mu.Lock()
	f.reasons = append(f.reasons, reason)
	f.active++
	f.maxSeen = max(f.maxSeen, f.active)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	return f.ok, f.authErr
}

type decision struct {
	approve bool
}

func collect() (server.ResolveFunc, chan decision) {
	ch := make(chan decision, 4)
	return func(approve bool) error { ch <- decision{approve}; return nil }, ch
}

func pending(text string) server.Pending {
	return server.Pending{ID: "cf_1", Summary: server.Summary{Tool: "pin_rm", Class: "destructive", Text: text}}
}

func wait(t *testing.T, ch chan decision) decision {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("no decision")
		return decision{}
	}
}

func TestNativeResolverApprovesOnlyWhenTheFactorAuthenticates(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    *fakeFactor
		want bool
	}{
		{"authenticated", &fakeFactor{resolver: server.ResolverMacOSLocalAuth, ok: true}, true},
		{"declined or cancelled", &fakeFactor{resolver: server.ResolverMacOSLocalAuth, ok: false}, false},
		{"prompt failed", &fakeFactor{resolver: server.ResolverMacOSLocalAuth, authErr: errors.New("boom")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolve, ch := collect()
			r := newNativeResolver(tc.f, quiet)
			require.NoError(t, r.Request(t.Context(), pending("Remove the pin"), resolve))
			assert.Equal(t, decision{tc.want}, wait(t, ch), "an error is a denial, never an approval")
		})
	}
}

func TestNativeResolverUnavailableRefusesTheRequest(t *testing.T) {
	f := &fakeFactor{resolver: server.ResolverLinuxPolkit, checkErr: ErrUnavailable, ok: true}
	resolve, ch := collect()
	err := newNativeResolver(f, quiet).Request(t.Context(), pending("x"), resolve)
	require.ErrorIs(t, err, ErrUnavailable)
	select {
	case d := <-ch:
		t.Fatalf("an unavailable mechanism resolved: %+v", d)
	case <-time.After(50 * time.Millisecond):
	}
	assert.Empty(t, f.reasons, "no prompt was shown")
}

func TestNativeResolverDisabledAfterFailedHG1(t *testing.T) {
	hg1Disabled[server.ResolverMacOSLocalAuth] = "synthetic click approved the prompt"
	t.Cleanup(func() { delete(hg1Disabled, server.ResolverMacOSLocalAuth) })
	f := &fakeFactor{resolver: server.ResolverMacOSLocalAuth, ok: true}
	resolve, _ := collect()
	err := newNativeResolver(f, quiet).Request(t.Context(), pending("x"), resolve)
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "HG-1")
	assert.Empty(t, f.reasons)
}

func TestNativeResolverExpiryDuringPromptDoesNotResolve(t *testing.T) {
	f := &fakeFactor{resolver: server.ResolverMacOSLocalAuth, block: make(chan struct{})}
	resolve, ch := collect()
	ctx, cancel := context.WithCancel(t.Context())
	require.NoError(t, newNativeResolver(f, quiet).Request(ctx, pending("x"), resolve))
	require.Eventually(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.active == 1 }, 5*time.Second, 5*time.Millisecond)
	cancel() // the confirmation expired: the prompt is dismissed
	select {
	case d := <-ch:
		t.Fatalf("an expired prompt resolved: %+v", d)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNativeResolverShowsOnePromptAtATime(t *testing.T) {
	f := &fakeFactor{resolver: server.ResolverMacOSLocalAuth, ok: true, block: make(chan struct{})}
	r := newNativeResolver(f, quiet)
	resolve, ch := collect()
	for range 3 {
		require.NoError(t, r.Request(t.Context(), pending("x"), resolve))
	}
	for range 3 {
		f.block <- struct{}{}
		wait(t, ch)
	}
	assert.Equal(t, 1, f.maxSeen)
}

func TestReasonNamesTheOperationAndDropsControlCharacters(t *testing.T) {
	got := reasonFor(pending("Remove the pin\n\x1b[31mbafy\u202e  at /ipfs/x"))
	assert.NotContains(t, got, "\n")
	assert.NotContains(t, got, "\x1b")
	assert.Contains(t, got, "Remove the pin")
	assert.Contains(t, got, "bafy")
	assert.NotContains(t, got, "\u202e", "bidi controls are dropped")
	assert.True(t, strings.HasPrefix(got, "Approve IPFS MCP operation: "))

	long := reasonFor(pending(strings.Repeat("a", 1000)))
	assert.LessOrEqual(t, len([]rune(long)), len("Approve IPFS MCP operation: ")+maxReasonRunes)
}

func TestRefusingFactorNeverAuthenticates(t *testing.T) {
	f := refusingFactor{resolver: server.ResolverDesktop, why: "no prompt here"}
	require.ErrorIs(t, f.check(t.Context()), ErrUnavailable)
	ok, err := f.authenticate(t.Context(), "x")
	assert.False(t, ok)
	assert.ErrorIs(t, err, ErrUnavailable)
	resolve, _ := collect()
	require.ErrorIs(t, newNativeResolver(f, quiet).Request(t.Context(), pending("x"), resolve), ErrUnavailable)
}

func TestParseModes(t *testing.T) {
	got, err := ParseModes(" Native, cli ,native")
	require.NoError(t, err)
	assert.Equal(t, []string{ModeNative, ModeCLI}, got)
	for _, none := range []string{"", "none", " , "} {
		got, err := ParseModes(none)
		require.NoError(t, err)
		assert.Empty(t, got, none)
	}
	_, err = ParseModes("native,telepathy")
	assert.Error(t, err)
}

func TestNoModesBuildsNoResolver(t *testing.T) {
	m, err := Build(Config{}, quiet)
	require.NoError(t, err)
	defer m.Close()
	assert.Empty(t, m.Resolvers(), "off by default: a non-eliciting client is refused")
}

func TestDesktopModeWithoutPairingBuildsNoResolver(t *testing.T) {
	cfg := Config{Modes: []string{ModeDesktop}, PairingFile: t.TempDir() + "/none.json", SocketDir: shortDir(t)}
	m, err := Build(cfg, quiet)
	require.NoError(t, err)
	defer m.Close()
	assert.Empty(t, m.Resolvers(), "unpaired Desktop refuses")
	assert.Nil(t, m.ch, "no channel without a resolver to serve")
}
