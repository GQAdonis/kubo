//go:build !nomcp

package kubo

import (
	"fmt"
	"testing"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	s5TokenA = "s5-secret-token-aaaaaaaaaaaaaaaaaaaaaaaa"
	s5TokenB = "s5-secret-token-bbbbbbbbbbbbbbbbbbbbbbbb"
)

func s5Config(listen string, principals ...string) string {
	body := fmt.Sprintf(`{"http":{"enabled":true,"listen":%q}`, listen)
	if len(principals) > 0 {
		body += `,"principals":[`
		for i, p := range principals {
			if i > 0 {
				body += ","
			}
			body += p
		}
		body += `]`
	}
	return body + `}`
}

func s5Principal(name, env string) string {
	return fmt.Sprintf(`{"name":%q,"kind":"human-mcp","credential":{"env":%q}}`, name, env)
}

// The S5 gap: principals in the config whose credentials all fail to resolve
// used to leave an anonymous loopback listener, silently losing the intended
// authentication.
func TestHTTPStartupFailsWhenNoConfiguredPrincipalResolves(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	t.Setenv("S5_ABSENT_A", "")
	t.Setenv("S5_ABSENT_B", "")
	file := writeMCPConfig(t, s5Config("127.0.0.1:5251", s5Principal("reader", "S5_ABSENT_A"), s5Principal("writer", "S5_ABSENT_B")))

	_, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.Error(t, err)
	for _, want := range []string{"reader", "env S5_ABSENT_A", "writer", "env S5_ABSENT_B", "anonymous"} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestHTTPStartupNeverPrintsASecretWhenNoPrincipalResolves(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	t.Setenv("S5_ABSENT_A", "")
	file := writeMCPConfig(t, s5Config("127.0.0.1:5251", s5Principal("reader", "S5_ABSENT_A")))
	t.Setenv("S5_UNRELATED", s5TokenA)

	_, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), s5TokenA)
}

func TestHTTPStartupKeepsResolvedPrincipalsWhenOthersAreMissing(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	t.Setenv("S5_PRESENT", s5TokenA)
	t.Setenv("S5_ABSENT_B", "")
	file := writeMCPConfig(t, s5Config("127.0.0.1:5251", s5Principal("reader", "S5_PRESENT"), s5Principal("writer", "S5_ABSENT_B")))

	s, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	require.NotNil(t, s.http)
	assert.False(t, s.http.anonymous, "a resolved principal keeps authentication on")
	assert.NotNil(t, s.http.opts.Verifier)
	require.Len(t, s.http.warnings, 1, "only the unresolved principal is disabled, with a warning")
	assert.Contains(t, s.http.warnings[0], "writer")
	assert.NotContains(t, s.http.warnings[0], s5TokenA)
}

func TestHTTPStartupWithNoPrincipalsStaysAnonymousOnLoopback(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, s5Config("127.0.0.1:5251"))

	s, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.NoError(t, err)
	require.NotNil(t, s.http)
	assert.True(t, s.http.anonymous, "the stock local default is unchanged")
	assert.Nil(t, s.http.opts.Verifier)
}

func TestHTTPStartupWithNoPrincipalsRefusesANonLoopbackListener(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	file := writeMCPConfig(t, s5Config("0.0.0.0:5251"))

	_, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to listen on non-loopback")
}

// An operator token is a credential: unresolved principals next to it leave
// authentication on, so startup continues with the warning.
func TestHTTPStartupOperatorTokenWithUnresolvedPrincipalsStillAuthenticates(t *testing.T) {
	t.Setenv(mcpConfigEnv, "")
	t.Setenv("S5_ABSENT_A", "")
	t.Setenv("S5_OPERATOR", s5TokenB)
	file := writeMCPConfig(t, s5Config("127.0.0.1:5251", s5Principal("reader", "S5_ABSENT_A")))

	s, err := loadSettingsForTest(mcpRequest(cmds.OptMap{mcpConfigOptionName: file, mcpAuthTokenEnvOptionName: "S5_OPERATOR"}))
	require.NoError(t, err)
	assert.False(t, s.http.anonymous)
	assert.Len(t, s.http.warnings, 1)
}
