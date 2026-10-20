package server

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/ipfs/kubo/mcp/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolNameMetaKey is the _meta key of a tool result naming the tool that
// produced it, so an MCP App view can pick the right component without
// relying on the host to pass tool info.
const toolNameMetaKey = "ai.ipfs/tool"

// appManifest is the built projection. When resources are enabled, startup
// rejects a malformed manifest or disagreement with the canonical descriptor.
var appManifest, appManifestErr = app.LoadManifest()

func init() { registerBuildHook(registerAppViews) }

// appViewFor returns the view that a tool is linked to.
func appViewFor(tool string) (app.View, bool) {
	if appManifest == nil || canonicalUIBindingsErr != nil || appManifestErr != nil {
		return app.View{}, false
	}
	binding, ok := canonicalUIBindings.tools[tool]
	if !ok || binding.View == "" {
		return app.View{}, false
	}
	for _, v := range appManifest.Views {
		if v.ID == binding.View {
			return v, true
		}
	}
	return app.View{}, false
}

// The manifest is a built projection, never the source for tool/view mapping.
// Reject disagreement explicitly before resources or tool links are registered.
func validateAppBindings() error {
	if canonicalUIBindingsErr != nil {
		return canonicalUIBindingsErr
	}
	if appManifestErr != nil {
		return appManifestErr
	}
	return validateAppProjection(appManifest, canonicalUIBindings)
}

func validateAppProjection(manifest *app.Manifest, bindings *uiBindingSet) error {
	d := bindings.descriptor
	if manifest == nil || manifest.MimeType != d.ExternalApps.MIMEType || app.MIMEType != d.ExternalApps.MIMEType || len(manifest.Views) != len(d.ViewBindings) {
		return fmt.Errorf("MCP App manifest disagrees with canonical view descriptor")
	}
	seen := make(map[string]struct{}, len(manifest.Views))
	for _, v := range manifest.Views {
		binding, ok := bindings.views[v.ID]
		if _, duplicate := seen[v.ID]; duplicate || !ok || v.URI != binding.URI || v.File != binding.File || v.Name != binding.Title || v.Description != binding.Description {
			return fmt.Errorf("MCP App manifest has incompatible view %q", v.ID)
		}
		seen[v.ID] = struct{}{}
		tools := make(map[string]struct{}, len(v.Tools))
		for _, tool := range v.Tools {
			b, ok := bindings.tools[tool.Name]
			if _, duplicate := tools[tool.Name]; duplicate || !ok || b.View != v.ID || b.Component != tool.Component {
				return fmt.Errorf("MCP App manifest has incompatible tool %q", tool.Name)
			}
			tools[tool.Name] = struct{}{}
		}
		for name, b := range bindings.tools {
			if b.View == v.ID {
				if _, ok := tools[name]; !ok {
					return fmt.Errorf("MCP App manifest omits tool %q", name)
				}
			}
		}
	}
	return nil
}

func appResourceMeta() mcp.Meta {
	meta := maps.Clone(appManifest.ResourceMeta)
	if meta == nil {
		meta = map[string]any{}
	}
	d := canonicalUIBindings.descriptor.ExternalApps
	meta[d.MetadataKey] = maps.Clone(d.Declaration)
	return mcp.Meta(meta)
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
	if err := validateAppBindings(); err != nil {
		s.log.Warn("MCP App views unavailable", "error", err)
		return
	}
	for _, binding := range canonicalUIBindings.descriptor.ViewBindings {
		if !slices.ContainsFunc(s.registered, func(name string) bool { return canonicalUIBindings.tools[name].View == binding.View }) {
			continue
		}
		var v app.View
		for _, candidate := range appManifest.Views {
			if candidate.ID == binding.View {
				v = candidate
				break
			}
		}
		s.mcp.AddResource(&mcp.Resource{
			URI:         v.URI,
			Name:        v.ID,
			Title:       v.Name,
			Description: v.Description,
			MIMEType:    app.MIMEType,
			Meta:        appResourceMeta(),
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
			Meta:     appResourceMeta(),
		}}}, nil
	}
}
