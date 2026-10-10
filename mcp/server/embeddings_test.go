package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingsOffByDefault(t *testing.T) {
	t.Parallel()
	cfg, err := ParseConfig([]byte(`{}`))
	require.NoError(t, err)
	require.Nil(t, cfg.Embeddings)
	require.Empty(t, cfg.BackendHosts())

	s := &Server{}
	require.NoError(t, s.EnableEmbeddings(t.Context(), cfg.Embeddings, nil))
	require.Nil(t, s.Embeddings(), "no embeddings section means no service, no process, no connection")
	require.NoError(t, s.CloseEmbeddings())
}

func TestEmbeddingsSettingsFromConfig(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "embedder.key")
	require.NoError(t, os.WriteFile(keyFile, []byte("sk-file\n"), 0o600))
	t.Setenv("TEST_SURREAL_CREDENTIALS", "root:pw")
	cfg, err := ParseConfig([]byte(`{"embeddings":{"mode":"remote",
	  "remote":{"endpoint":"https://surreal.internal.example:8000","namespace":"ipfs","database":"index","credential":{"env":"TEST_SURREAL_CREDENTIALS"}},
	  "embedder":{"base_url":"https://embed.internal.example/v1","model":"bge-m3","dimensions":1024,"api_key":{"file":"` + keyFile + `"}}}}`))
	require.NoError(t, err)
	set, err := cfg.Embeddings.Settings()
	require.NoError(t, err)
	require.NoError(t, set.Validate())
	require.Equal(t, "root:pw", set.Remote.Credential.Reveal())
	require.Equal(t, "sk-file", set.Embedder.APIKey.Reveal())
	require.ElementsMatch(t, []string{"surreal.internal.example:8000", "embed.internal.example"}, cfg.BackendHosts())

	t.Setenv("TEST_SURREAL_CREDENTIALS", "")
	_, err = cfg.Embeddings.Settings()
	require.ErrorContains(t, err, "embeddings.remote.credential")
	require.NotContains(t, err.Error(), "root:pw")
}

func TestEmbeddingsEmbeddedSettingsRefuseRelativeBinary(t *testing.T) {
	t.Parallel()
	_, err := ParseConfig([]byte(`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"surreal","data_dir":"/var/idx"},
	  "embedder":{"base_url":"http://127.0.0.1:9/v1","model":"m","dimensions":8}}}`))
	require.Error(t, err, "the config schema refuses a relative binary path")
}

func TestEmbeddingsModeWithoutItsBlockIsRefused(t *testing.T) {
	t.Parallel()
	// The contract schema requires the block of the chosen mode, and names it.
	_, err := ParseConfig([]byte(`{"embeddings":{"mode":"remote","embedder":{"base_url":"http://127.0.0.1:9/v1","model":"m","dimensions":8}}}`))
	require.ErrorContains(t, err, `"remote"`)
}

func TestEgressCoversUsesTheIdentityAck(t *testing.T) {
	t.Parallel()
	operator, err := OperatorIdentity(false)
	require.NoError(t, err)
	require.False(t, EgressCovers(operator)("127.0.0.1:11434"), "the operator identity has no egress_ack")

	id := operator
	id.EgressAck = []string{EgressLocal}
	require.True(t, EgressCovers(id)("127.0.0.1:11434"))
	require.False(t, EgressCovers(id)("embed.example"))
}

func TestEmbeddingsPolicyFromConfig(t *testing.T) {
	t.Parallel()
	const base = `"mode":"embedded","embedded":{"surreal_binary":"/opt/surreal","data_dir":"/var/idx"},
	  "embedder":{"base_url":"http://127.0.0.1:9/v1","model":"m","dimensions":8}`

	cfg, err := ParseConfig([]byte(`{"embeddings":{` + base + `}}`))
	require.NoError(t, err)
	set, err := cfg.Embeddings.Settings()
	require.NoError(t, err)
	require.NoError(t, set.Validate())
	pol := set.Policy.Effective()
	require.False(t, pol.Auto, "no automatic indexing unless configured")
	require.Equal(t, []string{"text/plain", "text/markdown"}, pol.MIMEAllowlist)
	require.Equal(t, 1048576, pol.MaxBytes)
	require.Equal(t, 2000, pol.ChunkChars)

	cfg, err = ParseConfig([]byte(`{"embeddings":{` + base + `,"policy":{"auto":true,"mime_allowlist":[],"max_bytes":4096,"chunk_chars":300}}}`))
	require.NoError(t, err)
	set, err = cfg.Embeddings.Settings()
	require.NoError(t, err)
	require.NoError(t, set.Validate())
	pol = set.Policy.Effective()
	require.True(t, pol.Auto)
	require.Empty(t, pol.MIMEAllowlist, "an empty allowlist indexes nothing")
	require.Equal(t, 4096, pol.MaxBytes)
	require.Equal(t, 300, pol.ChunkChars)

	_, err = ParseConfig([]byte(`{"embeddings":{` + base + `,"policy":{"chunk_chars":10}}}`))
	require.Error(t, err, "the contract schema bounds chunk_chars")
}
