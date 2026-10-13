//go:build !noagent

package cli

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/ipfs/kubo/mcp/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// R7 of agent-backend-settings: liter-llm compatibility of the agent loop,
// proven against the real operator-configured liter-llm service. There is no
// default endpoint, key or model: the test runs only when the operator names
// all three through the environment, and skips with a clear message otherwise
// (the phase-integration-gate records the skip as an unmet gate item). The key
// is passed exactly as in production: IPFS_MCP_TEST_LLM_API_KEY_ENV carries the
// NAME of the environment variable that holds it, and the value of that
// variable is never read, logged or asserted on here.
//
// Required environment (all operator-configured, never defaulted):
//
//	IPFS_MCP_TEST_LLM_BASE_URL     OpenAI-compatible base URL of liter-llm, e.g. https://liter.example/v1
//	IPFS_MCP_TEST_LLM_API_KEY_ENV  name of the environment variable holding the API key
//	IPFS_MCP_TEST_LLM_MODEL        model to run the round trip with
const (
	literBaseURLEnv  = "IPFS_MCP_TEST_LLM_BASE_URL"
	literKeyEnvName  = "IPFS_MCP_TEST_LLM_API_KEY_ENV"
	literModelEnvVar = "IPFS_MCP_TEST_LLM_MODEL"
)

// literSettings returns the agent settings for the configured liter-llm
// service, or skips the test with a clear message when the operator has not
// configured it. A configured-but-unusable setting fails the test: the
// operator opted in, and a silent skip would hide a broken gate run. The key
// value is only resolved by the loop's own SecretRef, never by this test.
func literSettings(t *testing.T) *mcpserver.AgentSettings {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(literBaseURLEnv))
	keyEnv := strings.TrimSpace(os.Getenv(literKeyEnvName))
	model := strings.TrimSpace(os.Getenv(literModelEnvVar))
	var missing []string
	if base == "" {
		missing = append(missing, literBaseURLEnv)
	}
	if keyEnv == "" {
		missing = append(missing, literKeyEnvName)
	}
	if model == "" {
		missing = append(missing, literModelEnvVar)
	}
	if len(missing) > 0 {
		t.Skipf("liter-llm real-service test (R7) not configured: set %s (%s names the environment variable that holds the API key; its value is never set or echoed here); R7 liter-llm compatibility is unverified until this test runs",
			strings.Join(missing, ", "), literKeyEnvName)
	}
	u, err := url.Parse(base)
	require.NoError(t, err, "%s must be a URL", literBaseURLEnv)
	require.Empty(t, u.User, "%s must not carry credentials in the URL", literBaseURLEnv)
	// The loop sends an Authorization header to this host, so plain http is
	// only acceptable on loopback.
	if u.Scheme != "https" {
		h := u.Hostname()
		require.True(t, u.Scheme == "http" && (h == "127.0.0.1" || h == "::1" || h == "localhost"),
			"%s must be https (the loop sends an Authorization header to this host); http is accepted for loopback only", literBaseURLEnv)
	}
	keyRef := mcpserver.SecretRef{Env: keyEnv}
	_, err = keyRef.Read()
	require.NoError(t, err, "the operator configured the test but %s is unusable", literKeyEnvName)
	return &mcpserver.AgentSettings{
		BaseURL: base,
		Model:   model,
		APIKey:  &keyRef,
		Timeout: 5 * time.Minute,
		Limits:  mcpserver.LoopLimits{MaxSteps: 4, MaxTokens: 16384, WallClock: 6 * time.Minute},
	}
}

// TestMCPAgentLLMLiter runs one tool-call round trip against the real
// liter-llm service and an isolated harness daemon: the model must ask for
// ipfs_node_info, the loop must run it against the node, and the model must
// answer after the result. This is the R7 evidence that the OpenAI-compatible
// client works with the operator's liter-llm backend, not only with a local
// Ollama (G10-1/G10-2) or the scripted server.
func TestMCPAgentLLMLiter(t *testing.T) {
	set := literSettings(t)
	node := agentLLMNode(t)
	srv := agentLLMServer(t, node, set, []string{"node"})

	// liter-llm is a remote host; content egress to it is never acknowledged
	// here, so the round trip uses a non-egress tool only (G10-4 is covered
	// against the scripted server in TestMCPAgentLLM).
	u, err := url.Parse(set.BaseURL)
	require.NoError(t, err)
	host := u.Host

	// A remote model may fumble one call; each attempt is judged on its own
	// and logged, as in TestMCPAgentLLMOllama.
	const attempts = 2
	var reasons []string
	for i := 1; i <= attempts; i++ {
		ok, why := ollamaAttempt(t, srv, node.PeerID().String(), set.Model)
		t.Logf("liter-llm attempt %d/%d with %s via %s: %s", i, attempts, set.Model, host, why)
		if ok {
			reasons = nil
			break
		}
		reasons = append(reasons, why)
	}
	assert.Empty(t, reasons, "no tool-call round trip completed in %d attempts against liter-llm at %s with %s (R7: the agent loop is not compatible with the operator's liter-llm backend)", attempts, host, set.Model)
}
