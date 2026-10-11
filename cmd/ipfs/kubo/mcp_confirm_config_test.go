//go:build !nomcp

package kubo

import (
	"testing"
	"time"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/kubo/mcp/server/confirm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// confirmation.cli and confirmation.resolvers through the same resolution the
// command runs (change native-confirmation-prompt, task 1.6).

func TestConfirmationCLIKeyEnablesCLIMode(t *testing.T) {
	t.Setenv(confirm.EnvModes, "")
	t.Setenv(mcpConfigEnv, "")

	// cli true alone extends the default modes.
	file := writeMCPConfig(t, `{"confirmation": {"cli": true}}`)
	modes, explicit, err := confirmModesForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.True(t, explicit)
	assert.Equal(t, []string{confirm.ModeNative, confirm.ModeDesktop, confirm.ModeCLI}, modes)

	// It extends the listed modes, too.
	file = writeMCPConfig(t, `{"confirmation": {"cli": true, "modes": ["native"]}}`)
	modes, _, err = confirmModesForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.Equal(t, []string{confirm.ModeNative, confirm.ModeCLI}, modes)

	// cli false adds nothing.
	file = writeMCPConfig(t, `{"confirmation": {"cli": false}}`)
	_, explicit, err = confirmModesForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.False(t, explicit, "cli: false changes nothing: the default applies")
}

func TestConfirmationCLIKeyLosesToFlagAndEnvironment(t *testing.T) {
	t.Setenv(confirm.EnvModes, "")
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, `{"confirmation": {"cli": true}}`)

	modes, explicit, err := confirmModesForTest(mcpRequest(cmds.OptMap{mcpConfirmOptionName: "native", mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.True(t, explicit)
	assert.Equal(t, []string{confirm.ModeNative}, modes, "--confirm wins: cli stays off")

	t.Setenv(confirm.EnvModes, "desktop")
	modes, _, err = confirmModesForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.Equal(t, []string{confirm.ModeDesktop}, modes, "IPFS_MCP_CONFIRM wins over the file")
}

func TestConfirmationCLIContradictionIsAConfigError(t *testing.T) {
	t.Setenv(confirm.EnvModes, "")
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, `{"confirmation": {"cli": false, "modes": ["cli"]}}`)
	_, _, err := confirmModesForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation.cli")
	assert.Contains(t, err.Error(), "confirmation.modes")
}

// confirmation.resolvers narrows whatever decided the modes and can only make
// the gate stricter.
func TestConfirmationResolversNarrowEverySource(t *testing.T) {
	t.Setenv(confirm.EnvModes, "")
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, `{"confirmation": {"resolvers": ["desktop"], "modes": ["native", "desktop"]}}`)

	// From the file.
	c, err := confirmConfigForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.Equal(t, []string{confirm.ModeNative, confirm.ModeDesktop}, c.Modes)
	assert.Equal(t, []string{"desktop"}, c.Resolvers)

	// From --confirm.
	c, err = confirmConfigForTest(mcpRequest(cmds.OptMap{mcpConfirmOptionName: "native", mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.Equal(t, []string{confirm.ModeNative}, c.Modes)
	assert.Equal(t, []string{"desktop"}, c.Resolvers, "applies after the flag decided the modes")

	// From IPFS_MCP_CONFIRM.
	t.Setenv(confirm.EnvModes, "native")
	c, err = confirmConfigForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.Equal(t, []string{confirm.ModeNative}, c.Modes)
	assert.Equal(t, []string{"desktop"}, c.Resolvers)
	t.Setenv(confirm.EnvModes, "")

	// From the default.
	file = writeMCPConfig(t, `{"confirmation": {"resolvers": ["linux-polkit"]}}`)
	c, err = confirmConfigForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.Equal(t, confirm.DefaultModes, c.Modes)
	assert.Equal(t, []string{"linux-polkit"}, c.Resolvers)

	// An empty list is kept (no resolver), unlike an absent key (all).
	file = writeMCPConfig(t, `{"confirmation": {"resolvers": []}}`)
	c, err = confirmConfigForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	require.NotNil(t, c.Resolvers)
	assert.Empty(t, c.Resolvers)

	// Absent: nil, so nothing is narrowed; no file: the default modes.
	c, err = confirmConfigForTest(mcpRequest(cmds.OptMap{}))
	require.NoError(t, err)
	assert.Nil(t, c.Resolvers)
	assert.Equal(t, confirm.DefaultModes, c.Modes)
}

func TestConfirmationTTLIsReadFromTheConfigFile(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, `{"confirmation": {"ttl": "45s"}}`)
	s, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	ttl, err := s.cfg.Confirmation.TTLDuration()
	require.NoError(t, err)
	assert.Equal(t, 45*time.Second, ttl)

	_, err = loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: writeMCPConfig(t, `{"confirmation": {"ttl": "11m"}}`)}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation.ttl")
}

// The *ForTest helpers read the config file once, as the command does, and
// pass the parse on.
func confirmModesForTest(req *cmds.Request) ([]string, bool, error) {
	cfg, err := loadMCPConfig(req)
	if err != nil {
		return nil, false, err
	}
	return confirmModes(req, cfg)
}

func confirmConfigForTest(req *cmds.Request) (confirm.Config, error) {
	cfg, err := loadMCPConfig(req)
	if err != nil {
		return confirm.Config{}, err
	}
	return confirmConfig(req, cfg)
}

func loadSettingsForTest(req *cmds.Request) (*mcpSettings, error) {
	cfg, err := loadMCPConfig(req)
	if err != nil {
		return nil, err
	}
	return loadMCPSettings(req, cfg)
}
