// Package app embeds the built MCP App views (mcp/app/dist) so the Go server can serve them as
// ui://ipfs-mcp/<view> resources without a JavaScript toolchain in `make build`.
//
// dist/ is committed; `pnpm check:dist` (scripts/check-dist.mjs) proves it matches the TypeScript
// sources. No CI workflow runs it yet, so run it by hand (or in `pnpm check`) before committing dist/.
// This package registers nothing: resource registration and the tool _meta.ui links live in
// mcp/server and are described in INTEGRATION.md.
package app

import (
	"embed"
	"encoding/json"
	"fmt"
)

// MIMEType is the MIME type of an MCP App resource (SEP-1865).
const MIMEType = "text/html;profile=mcp-app"

//go:embed dist/*.html dist/manifest.json
var files embed.FS

// Tool is a tool linked to a view.
type Tool struct {
	Name         string `json:"name"`
	Confirmation string `json:"confirmation"`
	Component    string `json:"component"`
}

// View is one MCP App view as listed in dist/manifest.json.
type View struct {
	ID          string `json:"id"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	File        string `json:"file"`
	Bytes       int    `json:"bytes"`
	SHA256      string `json:"sha256"`
	Tools       []Tool `json:"tools"`
}

// Manifest is dist/manifest.json.
type Manifest struct {
	Schema   int    `json:"schema"`
	MimeType string `json:"mimeType"`
	CSP      string `json:"csp"`
	// ResourceMeta is the _meta to set on each resource (resources/list and the resources/read
	// content item), for example {"ui": {"prefersBorder": false}}. It carries no csp and no
	// permissions: the host applies its deny-by-default policy.
	ResourceMeta map[string]any `json:"resourceMeta"`
	Views        []View         `json:"views"`
}

// LoadManifest parses the embedded manifest.
func LoadManifest() (*Manifest, error) {
	raw, err := files.ReadFile("dist/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse embedded manifest: %w", err)
	}
	return &m, nil
}

// HTML returns the single-file document for a view.
func HTML(v View) ([]byte, error) {
	b, err := files.ReadFile("dist/" + v.File)
	if err != nil {
		return nil, fmt.Errorf("read embedded view %s: %w", v.ID, err)
	}
	return b, nil
}
