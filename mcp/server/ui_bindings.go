package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/ipfs/kubo/mcp/contract"
)

// These are projections of the embedded descriptor, never independently
// maintained view, URI, component or version tables.
type catalogTuple struct {
	CatalogID       string `json:"catalogId"`
	CatalogVersion  string `json:"catalogVersion"`
	ProtocolVersion string `json:"protocolVersion"`
}

type uiViewBinding struct {
	View              string   `json:"view"`
	Component         string   `json:"component"`
	AllowedComponents []string `json:"allowedComponents"`
	URI               string   `json:"uri"`
	File              string   `json:"file"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
}

type a2uiComponentFile struct {
	catalogTuple
	Serve struct {
		Route string `json:"route"`
	} `json:"serve"`
	AguiEvents struct {
		Surface string `json:"surface"`
		Catalog string `json:"catalog"`
	} `json:"aguiEvents"`
	Components []struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	} `json:"components"`
	ChatRules   map[string]string `json:"chatRules"`
	Negotiation struct {
		MaxEncodedBytes int `json:"maxEncodedBytes"`
	} `json:"negotiation"`
	ViewBindings []uiViewBinding `json:"viewBindings"`
	ExternalApps struct {
		MIMEType    string            `json:"mimeType"`
		MetadataKey string            `json:"metadataKey"`
		Declaration map[string]string `json:"declaration"`
	} `json:"externalApps"`
}

type uiToolBinding struct {
	View         string
	Component    string
	OutputSchema string
}

type uiBindingSet struct {
	descriptor a2uiComponentFile
	views      map[string]uiViewBinding
	tools      map[string]uiToolBinding
}

var canonicalUIBindings, canonicalUIBindingsErr = loadUIBindings()

func loadUIBindings() (*uiBindingSet, error) {
	raw, err := fs.ReadFile(contract.FS(), "derivations/a2ui-components.json")
	if err != nil {
		return nil, fmt.Errorf("read UI descriptor: %w", err)
	}
	var descriptor a2uiComponentFile
	if err := json.Unmarshal(raw, &descriptor); err != nil {
		return nil, fmt.Errorf("parse UI descriptor: %w", err)
	}
	cat, err := contract.Load()
	if err != nil {
		return nil, err
	}
	return deriveUIBindings(cat, descriptor)
}

func deriveUIBindings(cat *contract.Catalog, d a2uiComponentFile) (*uiBindingSet, error) {
	if d.CatalogID == "" || d.CatalogVersion == "" || d.ProtocolVersion == "" || d.Negotiation.MaxEncodedBytes != 4096 {
		return nil, fmt.Errorf("UI descriptor: missing catalog tuple or invalid capability byte bound")
	}
	b := &uiBindingSet{descriptor: d, views: make(map[string]uiViewBinding, len(d.ViewBindings)), tools: make(map[string]uiToolBinding, len(cat.Tools))}
	components := make(map[string]string, len(d.Components))
	for _, c := range d.Components {
		if c.Name == "" || components[c.Name] != "" || !slices.Contains([]string{"view", "chat"}, c.Kind) {
			return nil, fmt.Errorf("UI descriptor: missing or duplicate component")
		}
		components[c.Name] = c.Kind
	}
	uriSet, fileSet := map[string]struct{}{}, map[string]struct{}{}
	for _, v := range d.ViewBindings {
		_, duplicate := b.views[v.View]
		_, uriDuplicate := uriSet[v.URI]
		_, fileDuplicate := fileSet[v.File]
		if v.View == "" || duplicate || uriDuplicate || fileDuplicate || !strings.HasPrefix(v.URI, "ui://") || v.File == "" || strings.ContainsAny(v.File, "/\\") || !strings.HasSuffix(v.File, ".html") || !slices.Contains(v.AllowedComponents, v.Component) {
			return nil, fmt.Errorf("UI descriptor: invalid or duplicate view %q", v.View)
		}
		allowed := make(map[string]struct{}, len(v.AllowedComponents))
		for _, name := range v.AllowedComponents {
			if _, duplicate := allowed[name]; duplicate || components[name] != "view" {
				return nil, fmt.Errorf("UI descriptor: invalid allowed component for %s", v.View)
			}
			allowed[name] = struct{}{}
		}
		b.views[v.View] = v
		uriSet[v.URI] = struct{}{}
		fileSet[v.File] = struct{}{}
	}
	for _, t := range cat.Tools {
		binding := uiToolBinding{OutputSchema: t.OutputSchema}
		if t.UI.A2UIComponent != nil {
			binding.Component = *t.UI.A2UIComponent
			if components[binding.Component] != "view" {
				return nil, fmt.Errorf("UI descriptor: unknown result component for %s", t.Name)
			}
		}
		if t.UI.AppView != nil {
			binding.View = *t.UI.AppView
			v, ok := b.views[binding.View]
			if !ok || !slices.Contains(v.AllowedComponents, binding.Component) {
				return nil, fmt.Errorf("UI descriptor: incompatible tool/view/component for %s", t.Name)
			}
		}
		// The output schema must resolve solely inside the embedded contract.
		if _, err := contract.BundleSchema(t.OutputSchema); err != nil {
			return nil, fmt.Errorf("UI output binding for %s: %w", t.Name, err)
		}
		b.tools[t.Name] = binding
	}
	return b, nil
}
