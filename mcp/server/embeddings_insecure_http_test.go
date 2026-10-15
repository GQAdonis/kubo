package server

import (
	"errors"
	"net/http"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stageConfig is an embeddings section whose extractor, describer and (with a
// key) embedder are plain-http in-cluster hosts, with the given allowlist.
func stageConfig(t *testing.T, hostsJSON string) embeddings.Settings {
	t.Helper()
	t.Setenv("IPFS_MCP_TEST_KEY", "k")
	cfg, err := ParseConfig([]byte(`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"/b/surreal","data_dir":"/d"},
	  "embedder":{"base_url":"http://embedder.ipfs.svc:11434/v1","model":"m","dimensions":8,"api_key":{"env":"IPFS_MCP_TEST_KEY"}},
	  "extractor":{"mode":"remote","remote":{"base_url":"http://xberg.ipfs.svc:8000"}},
	  "describer":{"base_url":"http://describer.ipfs.svc:9000/v1","model":"v"},
	  "insecure_http_hosts":` + hostsJSON + `}}`))
	require.NoError(t, err)
	set, err := cfg.Embeddings.Settings()
	require.NoError(t, err)
	return set
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Error("a request left the process")
	return nil, errors.New("unexpected request")
}

func TestInsecureHTTPHostsAllowsOnlyListedHosts(t *testing.T) {
	all := `["xberg.ipfs.svc:8000","describer.ipfs.svc","embedder.ipfs.svc:11434"]`

	t.Run("listed hosts are accepted for each of the three stages", func(t *testing.T) {
		set := stageConfig(t, all)
		require.NoError(t, set.Validate())
		_, err := embeddings.NewEmbedder(set.Embedder, nil)
		require.NoError(t, err)
		_, err = embeddings.NewExtractor(*set.Extractor, nil)
		require.NoError(t, err)
		_, err = embeddings.NewDescriber(*set.Describer, nil)
		require.NoError(t, err)
	})

	t.Run("an empty list refuses all three with a hint", func(t *testing.T) {
		err := stageConfig(t, `[]`).Validate()
		require.Error(t, err)
		for _, key := range []string{"embeddings.extractor.remote.base_url", "embeddings.describer.base_url", "embeddings.embedder.base_url"} {
			assert.ErrorContains(t, err, key)
		}
		assert.ErrorContains(t, err, "embeddings.insecure_http_hosts")
	})

	t.Run("malformed entries are rejected at startup", func(t *testing.T) {
		for _, bad := range []string{"fd00::1:8000", "[fd00::1]:8000", "http://xberg.ipfs.svc", "*.ipfs.svc", "xberg.ipfs.svc/x", "xberg.ipfs.svc:0", "xberg.ipfs.svc:99999", "user@xberg.ipfs.svc", ""} {
			// The config schema rejects most of these at parse time; the
			// settings validation is the backstop for what reaches it
			// otherwise, and for the port range the pattern cannot express.
			set := stageConfig(t, `[]`)
			set.Embedder.InsecureHTTPHosts = []string{bad}
			err := set.Validate()
			require.Error(t, err, bad)
			assert.ErrorContains(t, err, "embeddings.insecure_http_hosts entry", bad)
		}
		_, err := ParseConfig([]byte(`{"embeddings":{"insecure_http_hosts":["fd00::1:8000"]}}`))
		require.ErrorContains(t, err, "insecure_http_hosts")
	})

	t.Run("an unlisted host is refused while a listed one passes", func(t *testing.T) {
		err := stageConfig(t, `["xberg.ipfs.svc:8000"]`).Validate()
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "embeddings.extractor.remote.base_url")
		assert.ErrorContains(t, err, "embeddings.describer.base_url")
		assert.ErrorContains(t, err, "embeddings.embedder.base_url")
	})

	t.Run("host:port matches that port only, a bare host every port", func(t *testing.T) {
		err := stageConfig(t, `["xberg.ipfs.svc:9999","describer.ipfs.svc:9000","embedder.ipfs.svc"]`).Validate()
		require.Error(t, err)
		assert.ErrorContains(t, err, "embeddings.extractor.remote.base_url", "wrong port")
		assert.NotContains(t, err.Error(), "embeddings.describer.base_url", "exact port")
		assert.NotContains(t, err.Error(), "embeddings.embedder.base_url", "bare host")
	})

	t.Run("matching is exact: no parent domain or longer name", func(t *testing.T) {
		err := stageConfig(t, `["ipfs.svc","xberg.ipfs.svc.cluster.local"]`).Validate()
		require.Error(t, err)
		assert.ErrorContains(t, err, "embeddings.extractor.remote.base_url")
	})

	t.Run("matching is case-insensitive", func(t *testing.T) {
		require.NoError(t, stageConfig(t, `["XBERG.ipfs.SVC:8000","Describer.IPFS.svc","EMBEDDER.ipfs.svc:11434"]`).Validate())
	})

	t.Run("a non-loopback https or loopback http host needs no entry", func(t *testing.T) {
		t.Setenv("IPFS_MCP_TEST_KEY", "k")
		cfg, err := ParseConfig([]byte(`{"embeddings":{"mode":"embedded","embedded":{"surreal_binary":"/b/surreal","data_dir":"/d"},
		  "embedder":{"base_url":"http://127.0.0.1:11434/v1","model":"m","dimensions":8,"api_key":{"env":"IPFS_MCP_TEST_KEY"}},
		  "extractor":{"mode":"remote","remote":{"base_url":"https://xberg.example.test"}}}}`))
		require.NoError(t, err)
		set, err := cfg.Embeddings.Settings()
		require.NoError(t, err)
		require.NoError(t, set.Validate())
	})

	t.Run("egress_ack still applies to a listed host", func(t *testing.T) {
		set := stageConfig(t, all)
		hc := &http.Client{Transport: failTransport{t}}
		ctx := t.Context()
		none := func(string) bool { return false }

		emb, err := embeddings.NewEmbedder(set.Embedder, hc)
		require.NoError(t, err)
		_, err = emb.Embed(ctx, none, []string{"x"})
		require.ErrorIs(t, err, embeddings.ErrEgressNotAcknowledged)

		ext, err := embeddings.NewExtractor(*set.Extractor, hc)
		require.NoError(t, err)
		_, err = ext.Extract(ctx, none, embeddings.ExtractRequest{})
		require.ErrorIs(t, err, embeddings.ErrEgressNotAcknowledged)

		desc, err := embeddings.NewDescriber(*set.Describer, hc)
		require.NoError(t, err)
		_, err = desc.Describe(ctx, none, embeddings.DescribeRequest{Kind: embeddings.KindImage, MIME: "image/png", Data: []byte("x")})
		require.ErrorIs(t, err, embeddings.ErrEgressNotAcknowledged)

		// Listing the host in egress_ack is a separate decision.
		assert.False(t, AckCovers(nil, emb.Host()))
		assert.True(t, AckCovers([]string{"embedder.ipfs.svc"}, emb.Host()))
	})

	t.Run("the contract in-cluster example still parses and validates", func(t *testing.T) {
		t.Setenv("SURREAL_CREDENTIALS", "root:pw")
		cfg := exampleConfig(t, "vector-search-in-cluster.json")
		assert.Equal(t, []string{"ipfs-surrealdb.ipfs.svc:8000", "ipfs-xberg.ipfs.svc:8000", "ipfs-embedder.ipfs.svc:11434"}, cfg.Embeddings.InsecureHTTPHosts)
	})
}

func TestInsecureHTTPHostsCoversAStoreWithACredential(t *testing.T) {
	parse := func(t *testing.T, endpoint, credential, hosts string) embeddings.Settings {
		t.Helper()
		t.Setenv("IPFS_MCP_TEST_DB", "root:secret")
		cred := ""
		if credential != "" {
			cred = `,"credential":{"env":"` + credential + `"}`
		}
		cfg, err := ParseConfig([]byte(`{"embeddings":{"mode":"remote","remote":{"endpoint":"` + endpoint + `","namespace":"n","database":"d"` + cred + `},
		  "embedder":{"base_url":"http://127.0.0.1:11434/v1","model":"m","dimensions":8},
		  "insecure_http_hosts":` + hosts + `}}`))
		require.NoError(t, err)
		set, err := cfg.Embeddings.Settings()
		require.NoError(t, err)
		return set
	}

	t.Run("a credential over plain http to an unlisted host is refused naming the key", func(t *testing.T) {
		err := parse(t, "http://surreal.ipfs.svc:8000", "IPFS_MCP_TEST_DB", `[]`).Validate()
		require.Error(t, err)
		assert.ErrorContains(t, err, "embeddings.remote.endpoint")
		assert.ErrorContains(t, err, "embeddings.insecure_http_hosts")
	})
	t.Run("the host in the list is accepted", func(t *testing.T) {
		require.NoError(t, parse(t, "http://surreal.ipfs.svc:8000", "IPFS_MCP_TEST_DB", `["surreal.ipfs.svc:8000"]`).Validate())
	})
	t.Run("another port of the host is still refused", func(t *testing.T) {
		require.Error(t, parse(t, "http://surreal.ipfs.svc:8000", "IPFS_MCP_TEST_DB", `["surreal.ipfs.svc:9000"]`).Validate())
	})
	t.Run("no credential, https and loopback need no entry", func(t *testing.T) {
		require.NoError(t, parse(t, "http://surreal.ipfs.svc:8000", "", `[]`).Validate())
		require.NoError(t, parse(t, "https://surreal.example.test", "IPFS_MCP_TEST_DB", `[]`).Validate())
		require.NoError(t, parse(t, "http://127.0.0.1:8000", "IPFS_MCP_TEST_DB", `[]`).Validate())
	})
}
