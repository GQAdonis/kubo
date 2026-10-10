//go:build !noagent

package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingTransport fails every request and counts how many it saw, so a test
// can prove that nothing tried to reach a model.
type countingTransport struct{ n atomic.Int64 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.n.Add(1)
	return nil, assertErr("dialed")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func mustParse(t *testing.T, doc string) *Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(doc))
	require.NoError(t, err)
	return cfg
}

func TestAgentOffWhenUnconfigured(t *testing.T) {
	cfg := mustParse(t, `{}`)
	set, err := cfg.ResolveAgent(LLMOverrides{})
	require.NoError(t, err)
	assert.Nil(t, set, "no base URL and no model means the agent is off")

	// With no settings there is no client to hold a dialer: enabling the
	// agent is a no-op and nothing is mounted.
	s := mustNew(t, newFakeDaemon(t), Options{})
	require.NoError(t, s.EnableAgent(set))
	assert.Nil(t, s.Agent(), "nothing is mounted")
}

func TestAgentBuildsWithoutDialingWhenConfigured(t *testing.T) {
	ct := &countingTransport{}
	cfg := mustParse(t, `{"llm":{"base_url":"http://127.0.0.1:59999/v1","model":"m"}}`)
	set, err := cfg.ResolveAgent(LLMOverrides{})
	require.NoError(t, err)
	require.NotNil(t, set)
	set.HTTPClient = &http.Client{Transport: ct}

	s := mustNew(t, newFakeDaemon(t), Options{})
	require.NoError(t, s.EnableAgent(set))
	require.NotNil(t, s.Agent())
	assert.Equal(t, "127.0.0.1:59999", s.Agent().BackendHost())
	assert.Zero(t, ct.n.Load(), "configuring a backend makes no model call by itself")
}

func TestAgentHasNoDefaultEndpointOrModel(t *testing.T) {
	cfg := mustParse(t, `{}`)
	_, err := cfg.ResolveAgent(LLMOverrides{Model: "m"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm.base_url")
	_, err = cfg.ResolveAgent(LLMOverrides{BaseURL: "http://127.0.0.1:1/v1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm.model")
	_, err = cfg.ResolveAgent(LLMOverrides{KeyEnv: "SOME_KEY"})
	require.Error(t, err, "a key flag without a backend is a mistake, not a silent no-op")
}

func TestLiteralKeyInConfigIsRejectedAndNeverEchoed(t *testing.T) {
	const literal = "sk-live-0123456789abcdefSECRET"
	raw, err := os.ReadFile(filepath.Join("..", "contract", "config", "invalid", "literal-llm-key.json"))
	require.NoError(t, err)
	_, err = ParseConfig(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key", "the error names the field")

	_, err = ParseConfig([]byte(`{"llm":{"base_url":"http://127.0.0.1:1/v1","model":"m","api_key":"` + literal + `"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key")
	assert.NotContains(t, err.Error(), literal, "the validator must not quote the key")
}

func TestLiteralKeyFlagsAreRejectedAndNeverEchoed(t *testing.T) {
	const literal = "sk-live-0123456789abcdefSECRET"
	cfg := mustParse(t, `{}`)
	base := LLMOverrides{BaseURL: "http://127.0.0.1:1/v1", Model: "m"}

	over := base
	over.KeyEnv = literal
	_, err := cfg.ResolveAgent(over)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--llm-api-key-env")
	assert.NotContains(t, err.Error(), literal)

	over = base
	over.KeyFile = literal
	_, err = cfg.ResolveAgent(over)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--llm-api-key-file")
	assert.NotContains(t, err.Error(), literal)

	over = base
	over.KeyEnv, over.KeyFile = "A_KEY", "/tmp/k"
	_, err = cfg.ResolveAgent(over)
	require.Error(t, err, "only one source")
}

func TestKeyOnlyFromEnvOrFile(t *testing.T) {
	var got string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.Header.Get("Authorization")
		return nil, assertErr("stop")
	})
	run := func(t *testing.T, over LLMOverrides) {
		t.Helper()
		cfg := mustParse(t, `{}`)
		over.BaseURL, over.Model = "http://127.0.0.1:1/v1", "m"
		set, err := cfg.ResolveAgent(over)
		require.NoError(t, err)
		set.HTTPClient = &http.Client{Transport: rt}
		c, err := NewLLMClient(*set)
		require.NoError(t, err)
		got = ""
		_, _ = c.Chat(t.Context(), nil, nil, nil)
	}

	t.Run("env", func(t *testing.T) {
		t.Setenv("IPFS_MCP_TEST_KEY", "env-secret-1")
		run(t, LLMOverrides{KeyEnv: "IPFS_MCP_TEST_KEY"})
		assert.Equal(t, "Bearer env-secret-1", got)
	})
	t.Run("file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "key")
		require.NoError(t, os.WriteFile(f, []byte("file-secret-2\n"), 0o600))
		run(t, LLMOverrides{KeyFile: f})
		assert.Equal(t, "Bearer file-secret-2", got)
	})
	t.Run("unset env fails at build, not silently without a key", func(t *testing.T) {
		cfg := mustParse(t, `{}`)
		set, err := cfg.ResolveAgent(LLMOverrides{BaseURL: "http://127.0.0.1:1/v1", Model: "m", KeyEnv: "IPFS_MCP_TEST_UNSET_KEY"})
		require.NoError(t, err)
		_, err = NewLLMClient(*set)
		require.Error(t, err)
	})
	t.Run("none sends no header", func(t *testing.T) {
		run(t, LLMOverrides{})
		assert.Empty(t, got)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBackendURLValidation(t *testing.T) {
	cfg := mustParse(t, `{}`)
	for name, tc := range map[string]struct {
		url, want string
	}{
		"credentials":     {"http://user:pw@127.0.0.1:1/v1", "must not contain credentials"},
		"query":           {"http://127.0.0.1:1/v1?key=abc", "query or fragment"},
		"scheme":          {"ftp://127.0.0.1/v1", "http or https"},
		"hostless":        {"http:///v1", "valid http(s) URL"},
		"not a url":       {"://", "valid http(s) URL"},
		"fragment":        {"http://127.0.0.1:1/v1#k", "query or fragment"},
		"userinfo secret": {"https://sk-abc@example.com/v1", "must not contain credentials"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := cfg.ResolveAgent(LLMOverrides{BaseURL: tc.url, Model: "m"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), "pw", "no value from the URL is echoed")
			assert.NotContains(t, err.Error(), "sk-abc")
		})
	}
}

func TestKeyNotSentOverPlainHTTPToRemoteHost(t *testing.T) {
	cfg := mustParse(t, `{}`)
	_, err := cfg.ResolveAgent(LLMOverrides{BaseURL: "http://models.example.net/v1", Model: "m", KeyEnv: "A_KEY"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plain http")

	_, err = cfg.ResolveAgent(LLMOverrides{BaseURL: "https://models.example.net/v1", Model: "m", KeyEnv: "A_KEY"})
	require.NoError(t, err)
	_, err = cfg.ResolveAgent(LLMOverrides{BaseURL: "http://127.0.0.1:1/v1", Model: "m", KeyEnv: "A_KEY"})
	require.NoError(t, err, "loopback needs no TLS")
	_, err = cfg.ResolveAgent(LLMOverrides{BaseURL: "http://models.example.net/v1", Model: "m"})
	require.NoError(t, err, "without a key there is nothing to protect")
}

func TestFlagsOverrideConfigFile(t *testing.T) {
	cfg := mustParse(t, `{"llm":{"base_url":"http://127.0.0.1:1/v1","model":"from-file","timeout":"5s"},"agent":{"loop":{"max_steps":3,"wall_clock":"7s"}}}`)
	set, err := cfg.ResolveAgent(LLMOverrides{Model: "from-flag"})
	require.NoError(t, err)
	assert.Equal(t, "from-flag", set.Model)
	assert.Equal(t, "http://127.0.0.1:1/v1", set.BaseURL)
	assert.Equal(t, 5*1000*1000*1000, int(set.Timeout))
	assert.Equal(t, 3, set.Limits.MaxSteps)
	assert.Equal(t, DefaultMaxTokens, set.Limits.MaxTokens)
	assert.Equal(t, 7*1000*1000*1000, int(set.Limits.WallClock))
}

func TestSecretNeverPrints(t *testing.T) {
	s := secret("hunter2-value")
	assert.NotContains(t, s.String(), "hunter2")
	assert.NotContains(t, (&AgentSettings{APIKey: &SecretRef{Env: "A_KEY"}}).Describe(), "hunter2")
	type holder struct{ K secret }
	assert.NotContains(t, (&struct{ H holder }{holder{s}}).H.K.GoString(), "hunter2")
}

func TestLimitDefaultsMatchContract(t *testing.T) {
	// The contract schema owns the defaults; the loop must not drift from them.
	raw, err := contract.FS().Open("config/ipfs-mcp-config.schema.json")
	require.NoError(t, err)
	defer raw.Close()
	var buf [1 << 17]byte
	n, _ := raw.Read(buf[:])
	doc := string(buf[:n])
	assert.Contains(t, doc, `"default": 8,`)
	assert.Contains(t, doc, `"default": 32768,`)
	assert.Contains(t, doc, `"default": "120s"`)
	assert.Equal(t, 8, DefaultMaxSteps)
	assert.Equal(t, 32768, DefaultMaxTokens)
	assert.Equal(t, 120, int(DefaultWallClock.Seconds()))
}
