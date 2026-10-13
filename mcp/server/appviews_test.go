package server

import (
	"slices"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/app"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppManifestToolsExistInContract(t *testing.T) {
	require.NoError(t, appManifestErr)
	cat, err := contract.Load()
	require.NoError(t, err)
	for _, v := range appManifest.Views {
		for _, tool := range v.Tools {
			_, ok := cat.Tool(tool.Name)
			assert.True(t, ok, "view %s links unknown tool %s", v.ID, tool.Name)
		}
	}
}

func TestAppViewResources(t *testing.T) {
	// Every group, so every view has a registered tool.
	s := mustNew(t, newFakeDaemon(t), Options{Version: "test", Tools: allGroupNames(t)})
	cs := connect(t, s)

	list, err := cs.ListResources(t.Context(), nil)
	require.NoError(t, err)
	byURI := map[string]*mcp.Resource{}
	for _, r := range list.Resources {
		byURI[r.URI] = r
	}
	for _, v := range appManifest.Views {
		r, ok := byURI[v.URI]
		require.True(t, ok, "resources/list lacks %s", v.URI)
		assert.Equal(t, app.MIMEType, r.MIMEType)
		assert.Equal(t, map[string]any{"ui": map[string]any{"prefersBorder": false}}, map[string]any(r.Meta))

		got, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: v.URI})
		require.NoError(t, err)
		require.Len(t, got.Contents, 1)
		want, err := app.HTML(v)
		require.NoError(t, err)
		assert.Equal(t, app.MIMEType, got.Contents[0].MIMEType)
		assert.Equal(t, string(want), got.Contents[0].Text)
		ui, _ := got.Contents[0].Meta["ui"].(map[string]any)
		for _, forbidden := range []string{"csp", "permissions", "domain"} {
			assert.NotContains(t, ui, forbidden)
		}
	}
}

func TestAppToolsLinkToViews(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{Version: "test", Tools: allGroupNames(t)})
	cs := connect(t, s)
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)

	linked := 0
	for _, tool := range res.Tools {
		v, ok := appViewFor(tool.Name)
		if !ok {
			assert.NotContains(t, tool.Meta, "ui", "%s has no view", tool.Name)
			continue
		}
		linked++
		assert.Equal(t, map[string]any{"resourceUri": v.URI}, tool.Meta["ui"], tool.Name)
		assert.Equal(t, v.URI, tool.Meta["ui/resourceUri"], tool.Name)
	}
	assert.Positive(t, linked)
}

func TestAppViewsFollowRegisteredTools(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{Version: "test", ReadOnly: true, Disabled: []string{"pin_ls", "pin_verify"}})
	cs := connect(t, s)
	list, err := cs.ListResources(t.Context(), nil)
	require.NoError(t, err)
	uris := make([]string, 0, len(list.Resources))
	for _, r := range list.Resources {
		uris = append(uris, r.URI)
	}
	assert.NotContains(t, uris, "ui://ipfs-mcp/pins", "a view with no registered tool is not served")
	assert.Contains(t, uris, "ui://ipfs-mcp/node-status")
}

func TestResultsNameTheirTool(t *testing.T) {
	cs := connect(t, mustNew(t, newFakeDaemon(t), Options{Version: "test"}))
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{}})
	require.NoError(t, err)
	assert.Equal(t, "ipfs_node_info", res.Meta[toolNameMetaKey])
	bad, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{"bogus": 1}})
	require.NoError(t, err)
	assert.True(t, bad.IsError)
	assert.Equal(t, "ipfs_node_info", bad.Meta[toolNameMetaKey])
}

func allGroupNames(t *testing.T) []string {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	names := make([]string, 0, len(cat.Groups))
	for _, g := range cat.Groups {
		names = append(names, g.Name)
	}
	return slices.Clone(names)
}

func TestAppViewsOffSwitch(t *testing.T) {
	on := mustNew(t, newFakeDaemon(t), Options{Version: "test", Tools: allGroupNames(t)})
	off := mustNew(t, newFakeDaemon(t), Options{Version: "test", Tools: allGroupNames(t), NoAppViews: true})
	csOn, csOff := connect(t, on), connect(t, off)

	list, err := csOff.ListResources(t.Context(), nil)
	require.NoError(t, err)
	for _, r := range list.Resources {
		assert.False(t, strings.HasPrefix(r.URI, "ui://"), "no ui:// resource with views off: %s", r.URI)
	}
	_, err = csOff.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: appManifest.Views[0].URI})
	assert.Error(t, err)

	toolsOn, err := csOn.ListTools(t.Context(), nil)
	require.NoError(t, err)
	toolsOff, err := csOff.ListTools(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, len(toolsOn.Tools), len(toolsOff.Tools), "every tool is still offered")
	linkedOn := 0
	for _, tool := range toolsOn.Tools {
		if _, ok := tool.Meta["ui"]; ok {
			linkedOn++
		}
	}
	require.Positive(t, linkedOn, "the control run has linked tools")
	for _, tool := range toolsOff.Tools {
		assert.NotContains(t, tool.Meta, "ui", tool.Name)
		assert.NotContains(t, tool.Meta, "ui/resourceUri", tool.Name)
	}

	// The result of a tool is the same either way.
	resOn, err := csOn.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{}})
	require.NoError(t, err)
	resOff, err := csOff.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{}})
	require.NoError(t, err)
	assert.Equal(t, resOn.IsError, resOff.IsError)
	assert.Equal(t, resOn.StructuredContent, resOff.StructuredContent)
}

func TestAppViewsConfigKey(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{}`))
	require.NoError(t, err)
	assert.True(t, cfg.Tools.AppViewsEnabled(), "the default is on")

	cfg, err = ParseConfig([]byte(`{"tools": {"app_views": false}}`))
	require.NoError(t, err)
	assert.False(t, cfg.Tools.AppViewsEnabled())

	cfg, err = ParseConfig([]byte(`{"tools": {"app_views": true, "groups": ["pin"]}}`))
	require.NoError(t, err)
	assert.True(t, cfg.Tools.AppViewsEnabled())

	_, err = ParseConfig([]byte(`{"tools": {"app_views": "no"}}`))
	assert.Error(t, err, "a non-boolean value is rejected by the schema")
}
