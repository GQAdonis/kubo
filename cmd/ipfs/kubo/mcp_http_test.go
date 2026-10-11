//go:build !nomcp

package kubo

import (
	"os"
	"path/filepath"
	"testing"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/kubo/core/commands"
	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/ipfs/kubo/mcp/server/confirm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// httpFlagValue parses --http=<v> the way the command line parser does.
func httpFlagValue(t *testing.T, v string) any {
	t.Helper()
	for _, o := range mcpCmd.Options {
		if o.Name() == mcpHTTPOptionName {
			got, err := o.Parse(v)
			require.NoError(t, err)
			return got
		}
	}
	t.Fatal("--http is not an option of ipfs mcp")
	return nil
}

func writeMCPConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mcp.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func mcpRequest(opts cmds.OptMap) *cmds.Request { return &cmds.Request{Options: opts} }

func TestHTTPFlagFalseOverridesConfigEnabled(t *testing.T) {
	t.Setenv(mcpserver.HTTPListenEnv, "")
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, `{"http": {"enabled": true}}`)

	// The config file alone turns HTTP on.
	s, err := loadMCPSettings(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	require.NotNil(t, s.http, "http.enabled in the config file serves HTTP")

	// An explicit --http=false wins over the file.
	s, err = loadMCPSettings(mcpRequest(cmds.OptMap{mcpConfigOptionName: file, mcpHTTPOptionName: httpFlagValue(t, "false")}))
	require.NoError(t, err)
	assert.Nil(t, s.http, "--http=false must keep the server on stdio")
	assert.Equal(t, "stdio", s.transport())

	// --http=true and --http=<addr> keep it on.
	s, err = loadMCPSettings(mcpRequest(cmds.OptMap{mcpConfigOptionName: file, mcpHTTPOptionName: httpFlagValue(t, "true")}))
	require.NoError(t, err)
	assert.NotNil(t, s.http)
}

func TestHTTPFlagOnWithoutConfigAndOffByDefault(t *testing.T) {
	t.Setenv(mcpserver.HTTPListenEnv, "")
	t.Setenv(mcpConfigEnv, "")

	s, err := loadMCPSettings(mcpRequest(cmds.OptMap{}))
	require.NoError(t, err)
	assert.Nil(t, s.http, "HTTP is off unless enabled")

	s, err = loadMCPSettings(mcpRequest(cmds.OptMap{mcpHTTPOptionName: httpFlagValue(t, "true")}))
	require.NoError(t, err)
	require.NotNil(t, s.http)
	assert.Equal(t, mcpserver.DefaultHTTPListen, s.http.opts.Listen)
}

// The address that --http=<addr> names belongs to the request that carried it:
// a later invocation in the same process does not inherit it.
func TestHTTPAddressIsPerInvocation(t *testing.T) {
	t.Setenv(mcpserver.HTTPListenEnv, "")
	t.Setenv(mcpConfigEnv, "")

	first, err := loadMCPSettings(mcpRequest(cmds.OptMap{mcpHTTPOptionName: httpFlagValue(t, "127.0.0.1:5051")}))
	require.NoError(t, err)
	require.NotNil(t, first.http)
	assert.Equal(t, "127.0.0.1:5051", first.http.opts.Listen)

	second, err := loadMCPSettings(mcpRequest(cmds.OptMap{mcpHTTPOptionName: httpFlagValue(t, "true")}))
	require.NoError(t, err)
	require.NotNil(t, second.http)
	assert.Equal(t, mcpserver.DefaultHTTPListen, second.http.opts.Listen, "a bare --http does not reuse the previous address")

	third, err := loadMCPSettings(mcpRequest(cmds.OptMap{mcpHTTPOptionName: httpFlagValue(t, "127.0.0.1:5052")}))
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:5052", third.http.opts.Listen)
}

func TestHTTPFlagRejectsABadAddress(t *testing.T) {
	for _, o := range mcpCmd.Options {
		if o.Name() == mcpHTTPOptionName {
			_, err := o.Parse("not an address")
			assert.Error(t, err)
			// A port of another local service is refused.
			_, err = o.Parse("127.0.0.1:5001")
			assert.Error(t, err)
		}
	}
}

// --- RPC secret (R11-M2) ----------------------------------------------------

func TestResolveAPIAuth(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_RPC_SECRET", "bearer:from-env")
	file := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(file, []byte("bearer:from-file\n"), 0o600))

	got, onArgv, err := resolveAPIAuth(mcpRequest(cmds.OptMap{mcpAPIAuthEnvOptionName: "IPFS_MCP_TEST_RPC_SECRET"}))
	require.NoError(t, err)
	assert.Equal(t, "bearer:from-env", got)
	assert.False(t, onArgv)

	got, onArgv, err = resolveAPIAuth(mcpRequest(cmds.OptMap{mcpAPIAuthFileOptionName: file}))
	require.NoError(t, err)
	assert.Equal(t, "bearer:from-file", got, "the trailing newline is trimmed")
	assert.False(t, onArgv)

	got, onArgv, err = resolveAPIAuth(mcpRequest(cmds.OptMap{commands.ApiAuthOption: "bearer:on-argv"}))
	require.NoError(t, err)
	assert.Equal(t, "bearer:on-argv", got)
	assert.True(t, onArgv, "a secret on the command line is flagged so the command warns")

	got, onArgv, err = resolveAPIAuth(mcpRequest(cmds.OptMap{}))
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.False(t, onArgv)
}

func TestResolveAPIAuthRefusals(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_RPC_SECRET", "bearer:x")
	_, _, err := resolveAPIAuth(mcpRequest(cmds.OptMap{
		mcpAPIAuthEnvOptionName: "IPFS_MCP_TEST_RPC_SECRET", commands.ApiAuthOption: "bearer:y",
	}))
	require.Error(t, err, "two sources are ambiguous")

	_, _, err = resolveAPIAuth(mcpRequest(cmds.OptMap{mcpAPIAuthEnvOptionName: "IPFS_MCP_TEST_UNSET_VARIABLE"}))
	require.Error(t, err, "an unset variable is an error, not an empty secret")
	assert.NotContains(t, err.Error(), "bearer:", "the message never carries a secret")

	_, _, err = resolveAPIAuth(mcpRequest(cmds.OptMap{mcpAPIAuthFileOptionName: "relative/secret"}))
	require.Error(t, err, "a relative path is refused")
}

// --- --confirm (round 3) ----------------------------------------------------

func TestConfirmFlagGivenEmptyMeansNoResolvers(t *testing.T) {
	t.Setenv(confirm.EnvModes, "native")
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, `{"confirmation": {"modes": ["cli"]}}`)

	// Given but empty: off, over the environment and the config file.
	modes, explicit, err := confirmModes(mcpRequest(cmds.OptMap{mcpConfirmOptionName: "", mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.True(t, explicit)
	assert.Empty(t, modes, "an explicit empty --confirm turns the resolvers off")

	// Absent: the environment decides, then the file, then the default.
	modes, explicit, err = confirmModes(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.True(t, explicit)
	assert.Equal(t, []string{confirm.ModeNative}, modes)

	t.Setenv(confirm.EnvModes, "")
	modes, explicit, err = confirmModes(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	assert.True(t, explicit)
	assert.Equal(t, []string{confirm.ModeCLI}, modes)

	_, explicit, err = confirmModes(mcpRequest(cmds.OptMap{}))
	require.NoError(t, err)
	assert.False(t, explicit, "nothing set: the default applies")

	// A named value still wins over the environment.
	t.Setenv(confirm.EnvModes, "native")
	modes, _, err = confirmModes(mcpRequest(cmds.OptMap{mcpConfirmOptionName: "none"}))
	require.NoError(t, err)
	assert.Empty(t, modes)
}
