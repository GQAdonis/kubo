package server

import (
	"context"
	"fmt"
	"slices"

	"github.com/ipfs/kubo/mcp/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolNameMetaKey is the _meta key of a tool result naming the tool that
// produced it, so an MCP App view can pick the right component without
// relying on the host to pass tool info.
const toolNameMetaKey = "ai.ipfs/tool"

// appManifest is the embedded view manifest. A malformed one is a build
// defect (mcp/app has a test for it), so a failure only disables the views.
var appManifest, appManifestErr = app.LoadManifest()

func init() { registerBuildHook(registerAppViews) }

// appViewFor returns the view that a tool is linked to.
func appViewFor(tool string) (app.View, bool) {
	if appManifest == nil {
		return app.View{}, false
	}
	for _, v := range appManifest.Views {
		if slices.ContainsFunc(v.Tools, func(t app.Tool) bool { return t.Name == tool }) {
			return v, true
		}
	}
	return app.View{}, false
}

// appToolMeta is the _meta that links a tool to its view: the SEP-1865 nested
// key and the legacy flat key older hosts read. Visibility stays at the
// default so the view may call the read-only tools through the host.
func appToolMeta(tool string) mcp.Meta {
	v, ok := appViewFor(tool)
	if !ok {
		return nil
	}
	return mcp.Meta{
		"ui":             map[string]any{"resourceUri": v.URI},
		"ui/resourceUri": v.URI,
	}
}

// registerAppViews serves one ui:// resource per view that has at least one
// registered tool, unless Options.NoAppViews is set. No csp, permissions or domain is set, so the host applies
// its deny-by-default policy.
func registerAppViews(s *Server) {
	if s.opts.NoAppViews {
		return
	}
	if appManifestErr != nil {
		s.log.Warn("MCP App views unavailable", "error", appManifestErr)
		return
	}
	for _, v := range appManifest.Views {
		if !slices.ContainsFunc(v.Tools, func(t app.Tool) bool { return s.toolRegistered(t.Name) }) {
			continue
		}
		s.mcp.AddResource(&mcp.Resource{
			URI:         v.URI,
			Name:        v.ID,
			Title:       v.Name,
			Description: v.Description,
			MIMEType:    app.MIMEType,
			Meta:        mcp.Meta(appManifest.ResourceMeta),
		}, appResourceHandler(v))
	}
}

func appResourceHandler(v app.View) mcp.ResourceHandler {
	return func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		html, err := app.HTML(v)
		if err != nil {
			return nil, fmt.Errorf("read app view: %w", err)
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      v.URI,
			MIMEType: app.MIMEType,
			Text:     string(html),
			Meta:     mcp.Meta(appManifest.ResourceMeta),
		}}}, nil
	}
}
