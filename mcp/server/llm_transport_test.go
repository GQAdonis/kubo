//go:build !noagent

package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The model client dials the configured backend itself, like the embedder and
// the other clients that carry operator-named content or keys: the proxy
// environment never sees the prompts or the API key. (A loopback backend is not
// proxied by Go in any case, so the transport is what the test pins.)
func TestLLMClientIgnoresTheProxyEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")

	c, err := NewLLMClient(AgentSettings{BaseURL: "https://llm.example.test/v1", Model: "m"})
	require.NoError(t, err)
	tr, ok := c.hc.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, tr.Proxy)
}
