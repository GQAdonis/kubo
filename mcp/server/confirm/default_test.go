package confirm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredDefaultsToNativeAndDesktop(t *testing.T) {
	current.mu.Lock()
	saved := current.cfg
	current.cfg = nil
	current.mu.Unlock()
	t.Cleanup(func() { current.mu.Lock(); current.cfg = saved; current.mu.Unlock() })

	t.Setenv(EnvModes, "")
	cfg, err := configured()
	require.NoError(t, err)
	assert.Equal(t, []string{ModeNative, ModeDesktop}, cfg.Modes, "the human gate is on by default")
	assert.False(t, cfg.Has(ModeCLI), "cli is never a default")

	t.Setenv(EnvModes, "none")
	cfg, err = configured()
	require.NoError(t, err)
	assert.Empty(t, cfg.Modes, "none is the off switch")

	t.Setenv(EnvModes, "cli")
	cfg, err = configured()
	require.NoError(t, err)
	assert.Equal(t, []string{ModeCLI}, cfg.Modes)

	t.Setenv(EnvModes, "bogus")
	_, err = configured()
	assert.Error(t, err)

	// Mutating DefaultModes through the result must not leak.
	t.Setenv(EnvModes, "")
	cfg, _ = configured()
	cfg.Modes[0] = "x"
	assert.Equal(t, ModeNative, DefaultModes[0])
}
