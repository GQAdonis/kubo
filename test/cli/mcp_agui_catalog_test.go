//go:build !noagent

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-agui-a2ui-registry 1.1 against the built binary:
// GET /agui/catalog serves the generated catalog byte for byte behind the same
// protections as /agui, the run stream advertises it once after RUN_STARTED,
// and agent.a2ui: false disables both.

// aguiCatalogConfig is aguiConfig with an explicit agent.a2ui (nil: the key is
// absent, so the default applies).
func aguiCatalogConfig(t *testing.T, llmBase string, a2ui *bool) string {
	t.Helper()
	agent := map[string]any{"agui": true, "origin_allowlist": []string{aguiOriginWeb}}
	if a2ui != nil {
		agent["a2ui"] = *a2ui
	}
	cfg := map[string]any{
		"agent": agent,
		"principals": []any{
			map[string]any{"name": "chat", "kind": "ui", "credential": map[string]any{"env": "CLI_AGUI_CHAT_TOKEN"},
				"write_groups": []string{"pin"}, "egress_ack": []string{"local"}},
			map[string]any{"name": "reader", "kind": "human-mcp", "credential": map[string]any{"env": "CLI_MCP_READER_TOKEN"}},
			map[string]any{"name": "peer", "kind": "a2a", "credential": map[string]any{"env": "CLI_MCP_PEER_TOKEN"}, "groups": []string{"node", "pin"}},
		},
	}
	if llmBase != "" {
		cfg["llm"] = map[string]any{"base_url": llmBase, "model": "m"}
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "mcp-agui-catalog.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

// get sends one GET to the server and returns the whole response.
func aguiCatalogGet(t *testing.T, p *mcpHTTPProc, token string, hdr map[string]string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, p.base+"/agui/catalog", nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, body
}

func TestMCPAGUICatalog(t *testing.T) {
	t.Run("the route serves the generated catalog byte for byte, behind the /agui protections", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("must never be asked"))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiCatalogConfig(t, model.baseURL(), nil))

		want, err := os.ReadFile(filepath.Join("..", "..", "mcp", "ui", "catalog", "ipfs-mcp.catalog.json"))
		require.NoError(t, err)
		status, hdr, body := aguiCatalogGet(t, p, aguiChatToken, nil)
		require.Equal(t, http.StatusOK, status, "%s", body)
		assert.Equal(t, "application/json", hdr.Get("Content-Type"))
		assert.Equal(t, want, body, "the route serves mcp/ui/catalog/ipfs-mcp.catalog.json unchanged")

		status, _, _ = aguiCatalogGet(t, p, "", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "no token")
		for name, tok := range map[string]string{"human-mcp": mcpHTTPReaderToken, "a2a": mcpHTTPPeerToken} {
			status, _, _ = aguiCatalogGet(t, p, tok, nil)
			assert.Equal(t, http.StatusForbidden, status, "%s principal", name)
		}
		assert.Equal(t, http.StatusMethodNotAllowed,
			p.status(mcpHTTPRequest{method: http.MethodPost, path: "/agui/catalog", token: aguiChatToken, body: "{}"}), "only GET")
		status, _, _ = aguiCatalogGet(t, p, aguiChatToken, map[string]string{"Origin": "https://evil.example"})
		assert.Equal(t, http.StatusForbidden, status, "an origin outside agent.origin_allowlist")

		status, hdr, _ = aguiCatalogGet(t, p, aguiChatToken, map[string]string{"Origin": aguiOriginWeb})
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, aguiOriginWeb, hdr.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "GET, OPTIONS", hdr.Get("Access-Control-Allow-Methods"))
		assert.Empty(t, hdr.Get("Access-Control-Allow-Credentials"))

		assert.Zero(t, model.hits(), "catalog discovery makes no model call")
		assertOnlyDaemonTraffic(t, p, node)
	})

	t.Run("the run stream advertises the catalog once, immediately after RUN_STARTED", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("Done."))
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiCatalogConfig(t, model.baseURL(), nil))

		in := aguiInput("thread-cat", "run-1", "hi")
		r := aguiPost(t, p, aguiChatToken, in, nil)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		require.GreaterOrEqual(t, len(r.Events), 2, "%v", r.types())
		require.Equal(t, "RUN_STARTED", r.Events[0]["type"])
		ad := r.Events[1]
		assert.Equal(t, "CUSTOM", ad["type"], "the advertisement follows RUN_STARTED: %v", r.types())
		assert.Equal(t, "a2ui.catalog", ad["name"])
		value, ok := ad["value"].(map[string]any)
		require.True(t, ok, "advertisement value is %v", ad["value"])
		assert.Equal(t, "/agui/catalog", value["catalogUrl"])
		assert.Equal(t, "https://github.com/GQAdonis/kubo/blob/master/mcp/ui/catalog/ipfs-mcp.catalog.json", value["catalogId"])
		var ads int
		for _, ev := range r.Events {
			if ev["type"] == "CUSTOM" && ev["name"] == "a2ui.catalog" {
				ads++
			}
		}
		assert.Equal(t, 1, ads, "exactly one catalog advertisement per run")
		validateAGUIFixtureSchema(t, in, r.Events)
	})

	t.Run("agent.a2ui false disables the route and the advertisement", func(t *testing.T) {
		t.Parallel()
		node := agentLLMNode(t)
		model := newCLIModel(t, cliSayStep("Done."))
		off := false
		p := startMCPHTTPArgs(t, node, aguiEnvVars(), "--mcp-config", aguiCatalogConfig(t, model.baseURL(), &off))

		status, _, _ := aguiCatalogGet(t, p, aguiChatToken, nil)
		assert.Equal(t, http.StatusNotFound, status, "agent.a2ui: false unmounts the catalog route")

		r := aguiPost(t, p, aguiChatToken, aguiInput("thread-cat-off", "run-1", "hi"), nil)
		require.Equal(t, http.StatusOK, r.Status, r.Body)
		for _, ev := range r.Events {
			assert.NotEqual(t, "CUSTOM", fmt.Sprint(ev["type"]), "no CUSTOM event when agent.a2ui is off: %v", ev)
		}
		assert.Contains(t, p.stderr.String(), "agent.a2ui is false")
	})
}
