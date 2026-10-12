package confirm

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/ipfs/kubo/mcp/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// confirmation.resolvers and confirmation.cli (change native-confirmation-prompt,
// task 1.6).

func TestFileModes(t *testing.T) {
	list := func(m ...string) *[]string { return &m }
	yes, no := true, false
	for _, tc := range []struct {
		name         string
		modes        *[]string
		cli          *bool
		want         []string
		wantExplicit bool
	}{
		{name: "nothing set: default applies"},
		{name: "cli false alone changes nothing", cli: &no},
		{name: "modes only", modes: list(ModeNative), want: []string{ModeNative}, wantExplicit: true},
		{name: "empty modes", modes: list(), want: []string{}, wantExplicit: true},
		{name: "cli true alone extends the default", cli: &yes, want: []string{ModeNative, ModeCLI}, wantExplicit: true},
		{name: "cli true extends modes", modes: list(ModeNative), cli: &yes, want: []string{ModeNative, ModeCLI}, wantExplicit: true},
		{name: "cli true with empty modes", modes: list(), cli: &yes, want: []string{ModeCLI}, wantExplicit: true},
		{name: "cli true with cli already listed", modes: list(ModeCLI), cli: &yes, want: []string{ModeCLI}, wantExplicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, explicit := FileModes(tc.modes, tc.cli)
			assert.Equal(t, tc.wantExplicit, explicit)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("the default and the input are not aliased", func(t *testing.T) {
		yes := true
		got, _ := FileModes(nil, &yes)
		got[0] = "mutated"
		assert.Equal(t, []string{ModeNative}, DefaultModes)
		in := []string{ModeNative}
		got, _ = FileModes(&in, &yes)
		got[0] = "mutated"
		assert.Equal(t, []string{ModeNative}, in)
	})
}

// logRecorder captures what Build logs.
func logRecorder() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})), &buf
}

func pairedConfig(t *testing.T, modes []string, resolvers []string) Config {
	t.Helper()
	dir := shortDir(t)
	pairing := filepath.Join(dir, "pairing.json")
	_, err := Pair(pairing)
	require.NoError(t, err)
	return Config{Modes: modes, Resolvers: resolvers, SocketDir: filepath.Join(dir, "s"), PairingFile: pairing}
}

func names(m *Manager) []string {
	out := make([]string, 0, len(m.Resolvers()))
	for _, r := range m.Resolvers() {
		out = append(out, r.Name())
	}
	return out
}

func TestResolversNarrowTheModes(t *testing.T) {
	native := platformFactor().name()
	require.Contains(t, []string{server.ResolverMacOSLocalAuth, server.ResolverLinuxPolkit, server.ResolverDesktop}, native)

	t.Run("absent: every resolver the modes enable", func(t *testing.T) {
		log, buf := logRecorder()
		m, err := Build(pairedConfig(t, []string{ModeNative, ModeDesktop}, nil), log)
		require.NoError(t, err)
		defer m.Close()
		assert.Contains(t, names(m), native)
		assert.Contains(t, names(m), server.ResolverDesktop)
		assert.Empty(t, buf.String(), "no warning when nothing was narrowed")
	})

	t.Run("desktop only drops native", func(t *testing.T) {
		if native == server.ResolverDesktop {
			t.Skip("this platform's native resolver is the desktop channel")
		}
		log, buf := logRecorder()
		m, err := Build(pairedConfig(t, []string{ModeNative, ModeDesktop}, []string{server.ResolverDesktop}), log)
		require.NoError(t, err)
		defer m.Close()
		assert.Equal(t, []string{server.ResolverDesktop}, names(m))
		assert.Empty(t, buf.String(), "one resolver is left: no warning")
	})

	t.Run("the platform resolver only drops desktop", func(t *testing.T) {
		if native == server.ResolverDesktop {
			t.Skip("this platform's native resolver is the desktop channel")
		}
		log, buf := logRecorder()
		m, err := Build(pairedConfig(t, []string{ModeNative, ModeDesktop}, []string{native}), log)
		require.NoError(t, err)
		defer m.Close()
		assert.Equal(t, []string{native}, names(m))
		assert.Nil(t, m.ch, "no channel without desktop or cli")
		assert.Empty(t, buf.String())
	})

	t.Run("the other platform's resolver name removes native", func(t *testing.T) {
		other := server.ResolverLinuxPolkit
		if native == server.ResolverLinuxPolkit {
			other = server.ResolverMacOSLocalAuth
		}
		log, buf := logRecorder()
		m, err := Build(pairedConfig(t, []string{ModeNative}, []string{other}), log)
		require.NoError(t, err)
		defer m.Close()
		assert.Empty(t, m.Resolvers())
		assert.Contains(t, buf.String(), NarrowedAwayWarning)
	})

	t.Run("an empty list removes every resolver and warns once", func(t *testing.T) {
		log, buf := logRecorder()
		m, err := Build(pairedConfig(t, []string{ModeNative, ModeDesktop}, []string{}), log)
		require.NoError(t, err)
		defer m.Close()
		assert.Empty(t, m.Resolvers(), "same effect as modes []")
		assert.Nil(t, m.ch)
		assert.Equal(t, 1, bytes.Count(buf.Bytes(), []byte("confirmation.resolvers removes")), "exactly one warning: %s", buf.String())
		assert.Contains(t, buf.String(), "level=WARN")
		assert.Contains(t, buf.String(), "confirmation_unavailable")
	})

	t.Run("a resolver the modes did not enable is not added", func(t *testing.T) {
		log, buf := logRecorder()
		m, err := Build(pairedConfig(t, []string{ModeDesktop}, []string{native, server.ResolverDesktop}), log)
		require.NoError(t, err)
		defer m.Close()
		for _, n := range names(m) {
			assert.Equal(t, server.ResolverDesktop, n, "modes asked for desktop only")
		}
		assert.Empty(t, buf.String())
	})

	t.Run("no mode asked for a resolver: nothing to warn about", func(t *testing.T) {
		log, buf := logRecorder()
		m, err := Build(Config{Modes: nil, Resolvers: []string{}}, log)
		require.NoError(t, err)
		defer m.Close()
		assert.Empty(t, m.Resolvers())
		assert.Empty(t, buf.String())
	})
}

// cli is not a resolver name in confirmation.resolvers: narrowing never removes
// it, and it alone does not satisfy a native-prompt requirement.
func TestResolversDoNotAffectCLI(t *testing.T) {
	log, buf := logRecorder()
	cfg := Config{Modes: []string{ModeNative, ModeCLI}, Resolvers: []string{}, SocketDir: shortDir(t)}
	m, err := Build(cfg, log)
	require.NoError(t, err)
	defer m.Close()
	assert.Equal(t, []string{server.ResolverCLI}, names(m), "cli stays when every other resolver is narrowed away")
	assert.Contains(t, buf.String(), NarrowedAwayCLIWarning, "the native resolver the modes asked for is gone")
	assert.NotContains(t, buf.String(), "confirmation_unavailable", "cli remains: the warning must not claim a refusal")
	assert.NotContains(t, buf.String(), NarrowedAwayWarning)
	assert.False(t, server.IsPresenceResolver(server.ResolverCLI))
}

func TestCLIModeFromFileBuildsTheCLIResolver(t *testing.T) {
	yes := true
	modes, explicit := FileModes(nil, &yes)
	require.True(t, explicit)
	require.Contains(t, modes, ModeCLI)
	log, _ := logRecorder()
	m, err := Build(Config{Modes: []string{ModeCLI}, SocketDir: shortDir(t)}, log)
	require.NoError(t, err)
	defer m.Close()
	assert.Equal(t, []string{server.ResolverCLI}, names(m))
}

// The warning has fixed text, names the key, and fires when narrowing removes
// every resolver the effective modes asked for.
func TestNarrowingToNothingEmitsTheStartupWarning(t *testing.T) {
	log, buf := logRecorder()
	m, err := Build(pairedConfig(t, []string{ModeNative, ModeDesktop}, []string{}), log)
	require.NoError(t, err)
	defer m.Close()
	out := buf.String()
	assert.Contains(t, out, "level=WARN")
	assert.Contains(t, out, "confirmation.resolvers")
	assert.Contains(t, out, NarrowedAwayWarning, "fixed text")
	assert.Equal(t, 1, bytes.Count(buf.Bytes(), []byte("confirmation.resolvers removes")))
}

// An empty confirmation.resolvers runs none of desktop, macos-localauth and
// linux-polkit; cli, when enabled, is unaffected.
func TestEmptyResolversRunNoPresenceOrDesktopResolver(t *testing.T) {
	log, _ := logRecorder()
	m, err := Build(pairedConfig(t, []string{ModeNative, ModeDesktop}, []string{}), log)
	require.NoError(t, err)
	defer m.Close()
	assert.Empty(t, m.Resolvers())

	cfg := pairedConfig(t, []string{ModeNative, ModeDesktop, ModeCLI}, []string{})
	m2, err := Build(cfg, log)
	require.NoError(t, err)
	defer m2.Close()
	got := names(m2)
	assert.Equal(t, []string{server.ResolverCLI}, got)
	for _, n := range []string{server.ResolverDesktop, server.ResolverMacOSLocalAuth, server.ResolverLinuxPolkit} {
		assert.NotContains(t, got, n)
	}
}
