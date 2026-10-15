//go:build !noagent

package cli

import (
	"net/http"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression tests of agent-features-default-on 1.3. They pin what a stock
// `ipfs mcp --http` serves with no agent flags and no agent section in the
// config: the MCP route and nothing else. The agent endpoints are opt-in at
// the server (IPFS Desktop and the Web UI turn their own features on, not
// Kubo's listener), and the settings route needs both an admin principal and
// agent.settings.enabled, which defaults to true. Each test runs the built
// binary against a daemon with an isolated IPFS_PATH; every listener binds an
// OS-chosen loopback port.

// defaultsTokens are the credentials the tests present; the empty one is no
// credential at all.
var defaultsTokens = []string{"", mcpHTTPReaderToken, mcpHTTPAdminToken, mcpHTTPPeerToken, mcpHTTPSettingsToken}

// requireNoAgentEndpoints asserts that /agui, /a2a and the agent card are not
// served, whatever credential is presented, and that the listener says nothing
// about enabling them.
func requireNoAgentEndpoints(t *testing.T, p *mcpHTTPProc) {
	t.Helper()
	for _, tok := range defaultsTokens {
		for _, r := range []mcpHTTPRequest{
			{method: http.MethodPost, path: "/agui", token: tok, body: "{}"},
			{method: http.MethodPost, path: "/a2a", token: tok, body: "{}"},
			{method: http.MethodGet, path: "/.well-known/agent-card.json", token: tok},
		} {
			assert.Equal(t, http.StatusNotFound, p.status(r), "%s %s with token %q", r.method, r.path, tok)
		}
	}
	assert.Equal(t, http.StatusNotFound, p.status(mcpHTTPRequest{method: http.MethodOptions, path: "/agui", header: map[string]string{
		"Origin": "http://app.example", "Access-Control-Request-Method": "POST"}}))
	for _, said := range []string{"AG-UI endpoint enabled", "A2A endpoint enabled"} {
		assert.NotContains(t, p.stderr.String(), said)
	}
}

func TestMCPStockDefaultsServeNoAgentEndpoints(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	// The shared test config: four principals, none of them an admin, no
	// "agent" section, no --agui and no --llm-* flags.
	p := startMCPHTTP(t, node, noAmbientConfig)

	requireNoAgentEndpoints(t, p)
	for _, r := range []mcpHTTPRequest{
		{method: http.MethodGet, path: "/agent/settings"},
		{method: http.MethodPut, path: "/agent/settings", body: "{}"},
		{method: http.MethodPost, path: "/agent/settings/test-connection", body: "{}"},
		{method: http.MethodGet, path: "/agent/settings/models"},
		{method: http.MethodGet, path: "/agent/settings/events"},
	} {
		for _, tok := range defaultsTokens {
			r.token = tok
			assert.Equal(t, http.StatusNotFound, p.status(r), "no admin principal is configured: %s %s with token %q", r.method, r.path, tok)
		}
	}
	assert.NotContains(t, p.stderr.String(), "agent settings API enabled")
	assert.Equal(t, http.StatusOK, p.status(mcpHTTPRequest{token: mcpHTTPReaderToken, body: mcpHTTPInitialize}),
		"the MCP route itself is served; stderr: %s", p.stderr.String())
}

func TestMCPStockDefaultsAdminPrincipalOpensOnlyTheSettingsRoute(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	cfg := principalConfig(t, `{
	  "principals": [
	    {"name": "settings-admin", "kind": "admin", "credential": {"env": "CLI_MCP_SETTINGS_TOKEN"}},
	    {"name": "reader", "kind": "human-mcp", "credential": {"env": "CLI_MCP_READER_TOKEN"}}
	  ]
	}`)
	p := startMCPHTTPArgs(t, node, append([]string{"CLI_MCP_SETTINGS_TOKEN=" + mcpHTTPSettingsToken}, noAmbientConfig...),
		"--mcp-config", cfg)

	// An admin principal is the one thing the settings route needs; the other
	// agent endpoints stay off without their flag or config key.
	requireNoAgentEndpoints(t, p)
	assert.Equal(t, http.StatusOK, p.status(mcpHTTPRequest{method: http.MethodGet, path: "/agent/settings", token: mcpHTTPSettingsToken}),
		"stderr: %s", p.stderr.String())
	for _, tok := range []string{"", mcpHTTPReaderToken} {
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, p.status(mcpHTTPRequest{method: http.MethodGet, path: "/agent/settings", token: tok}),
			"only the admin principal may read the settings (token %q)", tok)
	}
	require.Equal(t, http.StatusOK, p.status(mcpHTTPRequest{token: mcpHTTPReaderToken, body: mcpHTTPInitialize}))
}
