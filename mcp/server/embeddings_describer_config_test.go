package server

import (
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exampleConfig(t *testing.T, name string) *Config {
	t.Helper()
	raw, err := contract.FS().Open("config/examples/" + name)
	require.NoError(t, err)
	defer raw.Close()
	data := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := raw.Read(buf)
		data = append(data, buf[:n]...)
		if err != nil {
			break
		}
	}
	cfg, err := ParseConfig(data)
	require.NoError(t, err, name)
	return cfg
}

func TestDescriberAndBulkSectionsDecodeFromTheContractExamples(t *testing.T) {
	t.Setenv("IPFS_MCP_DESCRIBER_KEY", "describer-key")
	t.Setenv("SURREAL_CREDENTIALS", "root:pw")
	cfg := exampleConfig(t, "vector-search-embedded.json")
	e := cfg.Embeddings
	require.NotNil(t, e.Describer)
	assert.Equal(t, "vision-local", e.Describer.Model)
	assert.Equal(t, []string{"image"}, e.Describer.Kinds)
	assert.True(t, e.Bulk.IsEnabled())
	assert.Equal(t, []string{"pins", "mfs"}, e.Bulk.EffectiveSources())
	assert.Equal(t, "/documents", e.Bulk.EffectiveMFSRoot())
	assert.True(t, e.SearchRouteEnabled())

	set, err := e.Settings()
	require.NoError(t, err)
	require.NoError(t, set.Validate())
	require.NotNil(t, set.Describer)
	assert.Equal(t, "describer-key", set.Describer.APIKey.Reveal())
	assert.Equal(t, []string{"image"}, set.Describer.Kinds)
	assert.Nil(t, set.Describer.MaxRequestsPerHour, "an absent key is the default, not zero")

	// The describer is on this machine, so no principal needs to acknowledge a
	// remote host for it; the loopback host is still a backend host.
	assert.Contains(t, cfg.BackendHosts(), "127.0.0.1:1234")
}

func TestBulkAndSearchRouteOffSwitchesDecode(t *testing.T) {
	cfg := exampleConfig(t, "vector-search-remote.json")
	assert.False(t, cfg.Embeddings.Bulk.IsEnabled())
	assert.False(t, cfg.Embeddings.SearchRouteEnabled())
	assert.Nil(t, cfg.Embeddings.Describer, "no describer block: no description request exists")

	on, err := ParseConfig([]byte(`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"/b/surreal","data_dir":"/d"},
	  "embedder":{"base_url":"http://127.0.0.1:9/v1","model":"m","dimensions":8}}}`))
	require.NoError(t, err)
	assert.True(t, on.Embeddings.Bulk.IsEnabled(), "bulk is on by default")
	assert.True(t, on.Embeddings.SearchRouteEnabled(), "the search routes are on by default")
	caps := on.Embeddings.Bulk.Caps()
	assert.Equal(t, 1000, caps.MaxFiles)
	assert.EqualValues(t, 1<<30, caps.MaxBytes)
	assert.Equal(t, 30, caps.FilesPerMinute)
	assert.Equal(t, "/", on.Embeddings.Bulk.EffectiveMFSRoot())
	assert.False(t, (*EmbeddingsConfig)(nil).SearchRouteEnabled(), "no embeddings section: no routes")
}

func TestDescriberExplicitZeroAndEmptyValuesAreKept(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"/b/surreal","data_dir":"/d"},
	  "embedder":{"base_url":"http://127.0.0.1:9/v1","model":"m","dimensions":8},
	  "describer":{"base_url":"http://127.0.0.1:1/v1","model":"v","kinds":[],"max_requests_per_hour":0},
	  "bulk":{"sources":[]}}}`))
	require.NoError(t, err)
	d := cfg.Embeddings.Describer
	assert.False(t, d.Enabled())
	set, err := cfg.Embeddings.Settings()
	require.NoError(t, err)
	require.NoError(t, set.Validate())
	require.NotNil(t, set.Describer.MaxRequestsPerHour)
	assert.Zero(t, *set.Describer.MaxRequestsPerHour)
	assert.NotNil(t, set.Describer.Kinds)
	assert.Empty(t, set.Describer.Kinds)
	assert.NotNil(t, cfg.Embeddings.Bulk.Sources)
	assert.Empty(t, cfg.Embeddings.Bulk.EffectiveSources())
	assert.Equal(t, time.Duration(0), set.Describer.Timeout)
}

func TestDescriberKeyReferenceThatDoesNotResolveIsAnErrorThatHidesTheValue(t *testing.T) {
	t.Setenv("IPFS_MCP_DESCRIBER_KEY", "")
	t.Setenv("SURREAL_CREDENTIALS", "root:pw")
	cfg := exampleConfig(t, "vector-search-embedded.json")
	_, err := cfg.Embeddings.Settings()
	require.ErrorContains(t, err, "embeddings.describer.api_key")
}

func TestPlainHTTPDescriberOnAnotherHostIsRefusedAtValidation(t *testing.T) {
	t.Setenv("IPFS_MCP_DESCRIBER_KEY", "k")
	cfg, err := ParseConfig([]byte(`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"/b/surreal","data_dir":"/d"},
	  "embedder":{"base_url":"http://127.0.0.1:9/v1","model":"m","dimensions":8},
	  "describer":{"base_url":"http://describer.example.test/v1","model":"v"}}}`))
	require.NoError(t, err)
	set, err := cfg.Embeddings.Settings()
	require.NoError(t, err)
	require.ErrorContains(t, set.Validate(), "plain http")
}
