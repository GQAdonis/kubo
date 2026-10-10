package cli

import (
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/app"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPAppViews checks the MCP App wiring through the real binary: the
// ui:// views are listed and readable with the app MIME type, and tools carry
// the _meta link to their view.
func TestMCPAppViews(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()

	p := startMCP(t, node.IPFSBin, node.Dir)
	p.initialize()

	manifest, err := app.LoadManifest()
	require.NoError(t, err)

	listRes := p.rpc("resources/list", map[string]any{})["result"].(map[string]any)
	listed := map[string]map[string]any{}
	for _, r := range listRes["resources"].([]any) {
		res := r.(map[string]any)
		listed[res["uri"].(string)] = res
	}
	// The default tool set registers the content, pins, mfs, dag and
	// node-status tools, so every view is served.
	for _, v := range manifest.Views {
		res, ok := listed[v.URI]
		require.True(t, ok, "resources/list lacks %s", v.URI)
		assert.Equal(t, "text/html;profile=mcp-app", res["mimeType"])
		assert.Equal(t, map[string]any{"ui": map[string]any{"prefersBorder": false}}, res["_meta"])
	}

	read := p.rpc("resources/read", map[string]any{"uri": "ui://ipfs-mcp/pins"})["result"].(map[string]any)
	contents := read["contents"].([]any)
	require.Len(t, contents, 1)
	c := contents[0].(map[string]any)
	assert.Equal(t, "text/html;profile=mcp-app", c["mimeType"])
	assert.Contains(t, c["text"], "<html")

	toolsRes := p.rpc("tools/list", map[string]any{})["result"].(map[string]any)
	var checked bool
	for _, tl := range toolsRes["tools"].([]any) {
		tool := tl.(map[string]any)
		if tool["name"] != "pin_ls" {
			continue
		}
		meta := tool["_meta"].(map[string]any)
		assert.Equal(t, map[string]any{"resourceUri": "ui://ipfs-mcp/pins"}, meta["ui"])
		assert.Equal(t, "ui://ipfs-mcp/pins", meta["ui/resourceUri"])
		checked = true
	}
	require.True(t, checked, "pin_ls is in the default tool set")

	assert.Equal(t, 0, p.closeAndWait(), "stderr: %s", p.stderr.String())
	p.assertCleanStdout()
}

// TestMCPAppViewsOff checks the off switch through the real binary: with
// --no-app-views, or tools.app_views false in the config file, no ui://
// resource is listed and no tool carries _meta.ui, while every tool is still
// offered. It also confirms that the switch is off by default.
func TestMCPAppViewsOff(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()

	toolNames := func(p *mcpProc, viewsOff bool) (names []string, linked []string) {
		toolsRes := p.rpc("tools/list", map[string]any{})["result"].(map[string]any)
		for _, tl := range toolsRes["tools"].([]any) {
			tool := tl.(map[string]any)
			names = append(names, tool["name"].(string))
			if meta, ok := tool["_meta"].(map[string]any); ok {
				if _, ok := meta["ui"]; ok {
					linked = append(linked, tool["name"].(string))
				}
				if viewsOff {
					assert.NotContains(t, meta, "ui/resourceUri", "no legacy link either: %s", tool["name"])
				}
			}
		}
		return names, linked
	}
	uiResources := func(p *mcpProc) []string {
		listRes := p.rpc("resources/list", map[string]any{})["result"].(map[string]any)
		var uris []string
		for _, r := range listRes["resources"].([]any) {
			if uri := r.(map[string]any)["uri"].(string); strings.HasPrefix(uri, "ui://") {
				uris = append(uris, uri)
			}
		}
		return uris
	}

	on := startMCP(t, node.IPFSBin, node.Dir)
	on.initialize()
	allTools, linkedOn := toolNames(on, false)
	require.NotEmpty(t, linkedOn, "views are on by default")
	require.NotEmpty(t, uiResources(on))
	assert.Equal(t, 0, on.closeAndWait(), "stderr: %s", on.stderr.String())

	cfg := principalConfig(t, `{"tools":{"app_views":false}}`)
	for name, args := range map[string][]string{
		"flag":   {"--no-app-views"},
		"config": {"--mcp-config", cfg},
	} {
		t.Run(name, func(t *testing.T) {
			p := startMCP(t, node.IPFSBin, node.Dir, args...)
			p.initialize()
			assert.Empty(t, uiResources(p), "no ui:// resource")
			names, linked := toolNames(p, true)
			assert.Empty(t, linked, "no tool carries _meta.ui")
			assert.ElementsMatch(t, allTools, names, "every tool is still offered")
			assert.Equal(t, 0, p.closeAndWait(), "stderr: %s", p.stderr.String())
			p.assertCleanStdout()
		})
	}
}
