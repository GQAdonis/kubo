//go:build !noagent

package cli

import (
	"net/http"
	"os"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-agent-settings-api 1.7 (W6): the settings route's
// off switches. With agent.settings.enabled=false or no admin principal
// configured, the whole /agent/settings subtree answers 404, no overlay or
// key file is written and no model backend is dialed. The contract pins both
// test names in docs/mcp/config-reference.tests.json. Each test runs the
// built binary against a daemon with an isolated IPFS_PATH; every listener
// binds an OS-chosen loopback port.

// settingsDisabledChecks asserts the disabled state of one running `ipfs mcp
// --http`: the settings subtree 404s even for the admin token, the MCP route
// still works, the settings directory stays empty and the configured model
// backend was never contacted.
func settingsDisabledChecks(t *testing.T, p *mcpHTTPProc, model *cliModel, settingsDir string) {
	t.Helper()
	for _, r := range []mcpHTTPRequest{
		{method: http.MethodGet, path: "/agent/settings", token: mcpHTTPSettingsToken},
		{method: http.MethodPut, path: "/agent/settings", token: mcpHTTPSettingsToken, body: "{}"},
		{method: http.MethodPost, path: "/agent/settings/test-connection", token: mcpHTTPSettingsToken, body: "{}"},
		{method: http.MethodGet, path: "/agent/settings/models", token: mcpHTTPSettingsToken},
		{method: http.MethodGet, path: "/agent/settings/events", token: mcpHTTPSettingsToken},
	} {
		assert.Equal(t, http.StatusNotFound, p.status(r), "%s %s", r.method, r.path)
	}
	assert.NotContains(t, p.stderr.String(), "agent settings API enabled")
	assert.Equal(t, http.StatusOK, p.status(mcpHTTPRequest{token: mcpHTTPReaderToken, body: mcpHTTPInitialize}),
		"the MCP route itself is served; stderr: %s", p.stderr.String())
	entries, err := os.ReadDir(settingsDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is written to the settings directory")
	assert.Zero(t, model.hits(), "the model backend is never dialed")
}

func TestSettingsRouteDisabledAnswers404(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	model := newCLIModel(t)
	settingsDir := t.TempDir()
	// An admin principal is configured, but agent.settings.enabled is false:
	// the route stays unmounted (ADR-0009 section 3).
	cfg := principalConfig(t, `{
	  "llm": {"base_url": "`+model.baseURL()+`", "model": "m-old"},
	  "agent": {"settings": {"enabled": false}},
	  "principals": [
	    {"name": "settings-admin", "kind": "admin", "credential": {"env": "CLI_MCP_SETTINGS_TOKEN"}},
	    {"name": "reader", "kind": "human-mcp", "credential": {"env": "CLI_MCP_READER_TOKEN"}}
	  ]
	}`)
	p := startMCPHTTPArgs(t, node, append([]string{"CLI_MCP_SETTINGS_TOKEN=" + mcpHTTPSettingsToken}, noAmbientConfig...),
		"--mcp-config", cfg, "--settings-dir", settingsDir)
	settingsDisabledChecks(t, p, model, settingsDir)
}

func TestSettingsRouteRequiresAdminPrincipal(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	model := newCLIModel(t)
	settingsDir := t.TempDir()
	// agent.settings.enabled defaults to true, but no admin principal is
	// configured, so there is no kind that may open the route and it is never
	// mounted.
	cfg := principalConfig(t, `{
	  "llm": {"base_url": "`+model.baseURL()+`", "model": "m-old"},
	  "principals": [
	    {"name": "reader", "kind": "human-mcp", "credential": {"env": "CLI_MCP_READER_TOKEN"}},
	    {"name": "peer", "kind": "a2a", "credential": {"env": "CLI_MCP_PEER_TOKEN"}}
	  ]
	}`)
	p := startMCPHTTPArgs(t, node, noAmbientConfig, "--mcp-config", cfg, "--settings-dir", settingsDir)
	// A token that maps to no configured principal gets the same 404: the
	// route is absent, not merely unauthorized.
	assert.Equal(t, http.StatusNotFound,
		p.status(mcpHTTPRequest{method: http.MethodGet, path: "/agent/settings", token: mcpHTTPSettingsToken}))
	settingsDisabledChecks(t, p, model, settingsDir)
}
