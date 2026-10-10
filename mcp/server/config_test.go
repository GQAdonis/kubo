package server

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readContractFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(contract.FS(), name)
	require.NoError(t, err)
	return data
}

func TestParseConfigAcceptsTheContractExamples(t *testing.T) {
	entries, err := fs.ReadDir(contract.FS(), "config/examples")
	require.NoError(t, err)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		t.Run(e.Name(), func(t *testing.T) {
			_, err := ParseConfig(readContractFile(t, path.Join("config/examples", e.Name())))
			require.NoError(t, err)
		})
	}
}

func TestParseConfigRejectsTheContractInvalidExamples(t *testing.T) {
	var exp struct {
		Cases map[string]string `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(readContractFile(t, "config/invalid/expectations.json"), &exp))
	for file := range exp.Cases {
		t.Run(file, func(t *testing.T) {
			_, err := ParseConfig(readContractFile(t, path.Join("config/invalid", file)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "config schema")
		})
	}
}

func TestParseConfigRejectsLiteralSecretsAndReservedPorts(t *testing.T) {
	_, err := ParseConfig(readContractFile(t, "config/invalid/literal-principal-token.json"))
	require.Error(t, err, "a literal credential is not a SecretRef")
	assert.Contains(t, err.Error(), "credential")
	assert.NotContains(t, err.Error(), "s3cr3t-token", "the error never repeats a literal secret")
	assert.Contains(t, err.Error(), "<redacted>")

	for _, listen := range []string{"127.0.0.1:4000", "/ip4/127.0.0.1/tcp/5001", "127.0.0.1:8080"} {
		_, err := ParseConfig([]byte(`{"http":{"enabled":true,"listen":"` + listen + `"}}`))
		require.Error(t, err, listen)
	}
	_, err = ParseConfig([]byte(`{"principals":[{"kind":"ui","credential":{"env":"X_TOKEN"}}]}`))
	require.Error(t, err, "a principal needs a name")
	_, err = ParseConfig([]byte(`{"principals":[{"name":"a","kind":"human-mcp","credential":{"env":"lower"}}]}`))
	require.Error(t, err, "an env name is upper case")
	_, err = ParseConfig([]byte(`{"principals":[{"name":"a","kind":"human-mcp","credential":{"file":"relative/token"}}]}`))
	require.Error(t, err, "a credential file is absolute")
	_, err = ParseConfig([]byte(`{"http":{"max_body_bytes":10}}`))
	require.Error(t, err)
	_, err = ParseConfig([]byte(`{"unknown":1}`))
	require.Error(t, err)
	_, err = ParseConfig([]byte(`not json`))
	require.Error(t, err)
}

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{}`))
	require.NoError(t, err)
	assert.False(t, cfg.HTTP.Enabled, "HTTP is off by default")
	assert.Equal(t, DefaultHTTPListen, cfg.HTTP.Listen)
	assert.Equal(t, DefaultMaxBodyBytes, cfg.HTTP.MaxBodyBytes)
	d, err := cfg.SessionTimeoutDuration()
	require.NoError(t, err)
	assert.Equal(t, DefaultSessionTimeout, d)
	assert.Empty(t, cfg.HTTP.AllowedOrigins, "every cross-origin request is rejected")
	assert.Empty(t, cfg.Principals)
	assert.False(t, cfg.GrantsDestructive())

	cfg, err = ParseConfig([]byte(`{"http":{"session_timeout":"90s"}}`))
	require.NoError(t, err)
	d, err = cfg.SessionTimeoutDuration()
	require.NoError(t, err)
	assert.Equal(t, 90e9, float64(d))
}

func TestConfigPrincipalsAreReadOnlyUnlessGranted(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_READER", "reader-secret-0123456789")
	t.Setenv("IPFS_MCP_TEST_WRITER", "writer-secret-0123456789")
	tokenFile := filepath.Join(t.TempDir(), "admin.token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("admin-secret-0123456789\n"), 0o600))

	cfg, err := ParseConfig([]byte(`{"principals":[
		{"name":"reader","kind":"human-mcp","credential":{"env":"IPFS_MCP_TEST_READER"}},
		{"name":"writer","kind":"ui","credential":{"env":"IPFS_MCP_TEST_WRITER"},"write_groups":["pin"],"groups":["pin","node"]},
		{"name":"admin","kind":"human-mcp","credential":{"file":` + strconv.Quote(tokenFile) + `},"write_groups":["pin"],"destructive_groups":["pin"]}
	]}`))
	require.NoError(t, err)
	assert.True(t, cfg.GrantsDestructive())
	creds, err := cfg.Credentials()
	require.NoError(t, err)
	require.Len(t, creds, 3)

	reader := creds[0].Identity
	assert.Equal(t, []string{AllGroups}, reader.Groups, "groups defaults to every group")
	assert.Empty(t, reader.WriteGroups)
	assert.Empty(t, reader.DestructiveGroups)
	assert.Equal(t, []string{ScopeRead}, reader.Scopes)
	assert.Equal(t, []string{ScopeRead, ScopeWrite}, creds[1].Identity.Scopes)
	assert.ElementsMatch(t, []string{ScopeRead, ScopeWrite, ScopeAdmin}, creds[2].Identity.Scopes)

	v, err := NewStaticVerifier(creds)
	require.NoError(t, err)
	id, _, err := v.Verify(t.Context(), "admin-secret-0123456789", nil)
	require.NoError(t, err)
	assert.Equal(t, "admin", id.ID)
}

func TestConfigCredentialsFailWhenASecretIsMissing(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"principals":[
		{"name":"ghost","kind":"human-mcp","credential":{"env":"IPFS_MCP_TEST_NOT_SET_ANYWHERE"}}]}`))
	require.NoError(t, err)
	_, err = cfg.Credentials()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
	assert.NotContains(t, err.Error(), "secret")
}

func TestLoadConfigReportsTheFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	file := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"http":{"listen":"127.0.0.1:4000"}}`), 0o600))
	_, err = LoadConfig(file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), file)
}

func TestOperatorIdentityFollowsTheDestructiveSelector(t *testing.T) {
	plain, err := OperatorIdentity(false)
	require.NoError(t, err)
	assert.Equal(t, OperatorPrincipalID, plain.ID)
	assert.Empty(t, plain.DestructiveGroups)
	assert.NotContains(t, plain.Scopes, ScopeAdmin)
	assert.Contains(t, plain.Scopes, ScopeWrite)

	full, err := OperatorIdentity(true)
	require.NoError(t, err)
	assert.Contains(t, full.Scopes, ScopeAdmin)
	assert.NotEmpty(t, full.DestructiveGroups)
}

// TestReservedPortsMatchTheContractSchema keeps ReservedPorts in step with the
// ListenAddress definition in the contract's config schema.
func TestReservedPortsMatchTheContractSchema(t *testing.T) {
	var schema struct {
		Defs struct {
			Listen struct {
				Not struct {
					Pattern string `json:"pattern"`
				} `json:"not"`
			} `json:"ListenAddress"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(readContractFile(t, configSchemaPath), &schema))
	m := regexp.MustCompile(`\(([0-9|]+)\)\$$`).FindStringSubmatch(schema.Defs.Listen.Not.Pattern)
	require.Len(t, m, 2, schema.Defs.Listen.Not.Pattern)
	var fromSchema []int
	for _, p := range strings.Split(m[1], "|") {
		n, err := strconv.Atoi(p)
		require.NoError(t, err)
		fromSchema = append(fromSchema, n)
	}
	assert.ElementsMatch(t, fromSchema, ReservedPorts)
	assert.NotContains(t, strings.Split(DefaultHTTPListen, ":"), "4000")
	l, err := ParseListen(DefaultHTTPListen)
	require.NoError(t, err)
	assert.False(t, slices.Contains(ReservedPorts, l.Port), "the default is not a reserved port")
}

func TestParseListenForms(t *testing.T) {
	for _, tc := range []struct {
		in       string
		host     string
		loopback bool
	}{
		{"", "127.0.0.1", true},
		{"127.0.0.1:0", "127.0.0.1", true},
		{"[::1]:0", "::1", true},
		{"localhost:0", "localhost", true},
		{"/ip4/127.0.0.1/tcp/0", "127.0.0.1", true},
		{"/ip6/::1/tcp/0", "::1", true},
		{"/dns4/localhost/tcp/0", "localhost", true},
		{"0.0.0.0:0", "0.0.0.0", false},
		{"[::]:0", "::", false},
		{"10.0.0.5:0", "10.0.0.5", false},
		{"mcp.example.test:0", "mcp.example.test", false},
	} {
		l, err := ParseListen(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.host, l.Host, tc.in)
		assert.Equal(t, tc.loopback, l.IsLoopback(), tc.in)
	}
	l, err := ParseListen("localhost:0")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:0", l.BindAddr(), "localhost binds IPv4 loopback")
	for _, bad := range []string{"nonsense", "127.0.0.1:abc", "/ip4/127.0.0.1/udp/0", "/ip4/127.0.0.1", "/unix/tmp/sock"} {
		_, err := ParseListen(bad)
		require.Error(t, err, bad)
	}
}

func TestConfigConfirmationModes(t *testing.T) {
	absent, err := ParseConfig([]byte(`{}`))
	require.NoError(t, err)
	assert.Nil(t, absent.Confirmation.Modes, "unset leaves the default to the caller")

	off, err := ParseConfig([]byte(`{"confirmation":{"modes":[]}}`))
	require.NoError(t, err)
	require.NotNil(t, off.Confirmation.Modes)
	assert.Empty(t, *off.Confirmation.Modes, "an empty list turns the resolvers off")

	some, err := ParseConfig([]byte(`{"confirmation":{"modes":["native","cli"]}}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"native", "cli"}, *some.Confirmation.Modes)

	_, err = ParseConfig([]byte(`{"confirmation":{"modes":["agui-interrupt"]}}`))
	require.Error(t, err)
}
