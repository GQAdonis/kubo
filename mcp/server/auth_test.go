package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticVerifierValidInvalidAndMissingTokens(t *testing.T) {
	v := testVerifier(t)
	for _, tc := range []struct {
		name, token string
		wantID      string
	}{
		{"valid reader", tokReader, "reader"},
		{"valid peer", tokPeer, "peer"},
		{"unknown", "no-such-token-0123456789", ""},
		{"empty", "", ""},
		{"prefix of a valid token", tokReader[:len(tokReader)-1], ""},
		{"valid token plus suffix", tokReader + "x", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, exp, err := v.Verify(t.Context(), tc.token, nil)
			if tc.wantID == "" {
				require.ErrorIs(t, err, auth.ErrInvalidToken)
				assert.Empty(t, id.ID)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, id.ID)
			assert.WithinDuration(t, time.Now().Add(verdictWindow), exp, 5*time.Second, "the verdict expires soon")
		})
	}
	assert.Nil(t, v.ProtectedResourceMetadata())
	assert.Equal(t, 6, v.Len())
}

func TestStaticVerifierRejectsBadCredentials(t *testing.T) {
	ok := NewIdentity(Principal{ID: "a", Kind: KindHumanMCP}, []string{AllGroups}, nil, nil)
	for _, tc := range []struct {
		name  string
		creds []StaticCredential
		want  string
	}{
		{"token with no principal", []StaticCredential{{Token: "0123456789abcdef0123"}}, "no principal"},
		{"principal without a kind", []StaticCredential{{Identity: Identity{Principal: Principal{ID: "x"}}, Token: "0123456789abcdef0123"}}, "no principal"},
		{"short token", []StaticCredential{{Identity: ok, Token: "short"}}, "shorter than"},
		{"empty token", []StaticCredential{{Identity: ok}}, "shorter than"},
		{"duplicate principal", []StaticCredential{
			{Identity: ok, Token: "0123456789abcdef0123"},
			{Identity: ok, Token: "abcdef01234567890123"},
		}, "configured twice"},
		{"shared token", []StaticCredential{
			{Identity: ok, Token: "0123456789abcdef0123"},
			{Identity: NewIdentity(Principal{ID: "b", Kind: KindUI}, nil, nil, nil), Token: "0123456789abcdef0123"},
		}, "share one token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewStaticVerifier(tc.creds)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestTokenVerifierAdapterMapsOntoTheSDK(t *testing.T) {
	adapter := TokenVerifier(testVerifier(t))
	info, err := adapter(t.Context(), tokWriter, nil)
	require.NoError(t, err)
	assert.Equal(t, "writer", info.UserID, "UserID binds the session to the principal")
	assert.ElementsMatch(t, []string{ScopeRead, ScopeWrite}, info.Scopes)
	assert.False(t, info.Expiration.IsZero(), "the SDK rejects a verdict without expiry")
	id, ok := info.Extra[identityExtraKey].(Identity)
	require.True(t, ok)
	assert.Equal(t, KindHumanMCP, id.Kind)

	_, err = adapter(t.Context(), "wrong-token-0123456789", nil)
	require.ErrorIs(t, err, auth.ErrInvalidToken, "the SDK answers 401 only for ErrInvalidToken")
}

func TestSecretRefReadsEnvAndFiles(t *testing.T) {
	t.Setenv("IPFS_MCP_TEST_TOKEN", "  from-env-0123456789  ")
	got, err := SecretRef{Env: "IPFS_MCP_TEST_TOKEN"}.Read()
	require.NoError(t, err)
	assert.Equal(t, "from-env-0123456789", got)

	file := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(file, []byte("from-file-0123456789\n"), 0o600))
	got, err = SecretRef{File: file}.Read()
	require.NoError(t, err)
	assert.Equal(t, "from-file-0123456789", got)

	empty := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.WriteFile(empty, []byte("\n"), 0o600))
	for name, ref := range map[string]SecretRef{
		"unset variable": {Env: "IPFS_MCP_TEST_UNSET_VARIABLE"},
		"relative file":  {File: "token"},
		"missing file":   {File: filepath.Join(t.TempDir(), "absent")},
		"empty file":     {File: empty},
		"both":           {Env: "IPFS_MCP_TEST_TOKEN", File: file},
		"neither":        {},
	} {
		_, err := ref.Read()
		require.Error(t, err, name)
	}
	assert.NotContains(t, SecretRef{Env: "IPFS_MCP_TEST_TOKEN"}.String(), "from-env", "messages never carry the value")
}

func TestIdentityAllowsFollowsScopeAndGroup(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	tool := func(name string) contract.Tool {
		t.Helper()
		tl, ok := cat.Tool(name)
		require.True(t, ok, name)
		return tl
	}
	read := NewIdentity(Principal{ID: "r", Kind: KindHumanMCP}, []string{AllGroups}, nil, nil)
	pinWriter := NewIdentity(Principal{ID: "w", Kind: KindHumanMCP}, []string{"node"}, []string{"pin"}, nil)
	pinAdmin := NewIdentity(Principal{ID: "a", Kind: KindHumanMCP}, []string{"node"}, []string{"pin"}, []string{"pin"})
	none := NewIdentity(Principal{ID: "n", Kind: KindHumanMCP}, nil, nil, nil)

	for _, tc := range []struct {
		name string
		id   Identity
		tool string
		want bool
	}{
		{"read-all reads", read, "pin_ls", true},
		{"read-all cannot write", read, "pin_add", false},
		{"read-all cannot remove", read, "pin_rm", false},
		{"writer reads its listed group only", pinWriter, "pin_ls", false},
		{"writer reads node", pinWriter, "ipfs_node_info", true},
		{"writer writes its group", pinWriter, "pin_add", true},
		{"writer cannot remove", pinWriter, "pin_rm", false},
		{"admin removes", pinAdmin, "pin_rm", true},
		{"no groups grants nothing", none, "ipfs_node_info", false},
	} {
		assert.Equal(t, tc.want, tc.id.Allows(tool(tc.tool)), tc.name)
	}
	assert.ElementsMatch(t, []string{ScopeRead, ScopeWrite, ScopeAdmin}, pinAdmin.Scopes)
	assert.Equal(t, []string{ScopeRead}, read.Scopes)
}
