package contract

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
)

// TestIndexRouteDescriptor pins the vector-search-contract 1.5 acceptance: the
// /index/* route descriptor validates against its schema, references the tool
// schemas instead of restating them (no inline copy of a tool shape), scopes
// both routes to ui and human-mcp principals with the index group, lists admin
// as refused, and maps the named error codes to HTTP statuses.
func TestIndexRouteDescriptor(t *testing.T) {
	doc := readJSON(t, "routes/index.json")
	if err := resolveRef(t, "routes/index.schema.json").Validate(doc); err != nil {
		t.Fatalf("routes/index.json does not match routes/index.schema.json: %v", err)
	}

	var d struct {
		Authorization struct {
			PrincipalKinds []string `json:"principalKinds"`
			Group          string   `json:"group"`
			RefusedKinds   []string `json:"refusedKinds"`
		} `json:"authorization"`
		Routes []struct {
			Method  string   `json:"method"`
			Path    string   `json:"path"`
			Tool    string   `json:"tool"`
			Tools   []string `json:"tools"`
			Request *struct {
				Schema string `json:"schema"`
			} `json:"request"`
			Response struct {
				Schema                   string `json:"schema"`
				AdditionalPropertiesFrom map[string]struct {
					Ref string `json:"$ref"`
				} `json:"additionalPropertiesFrom"`
			} `json:"response"`
		} `json:"routes"`
		Errors struct {
			Shape   string `json:"shape"`
			Mapping []struct {
				Code   string `json:"code"`
				Status int    `json:"status"`
			} `json:"mapping"`
		} `json:"errors"`
	}
	readInto(t, "routes/index.json", &d)

	if !slices.Equal(d.Authorization.PrincipalKinds, []string{"ui", "human-mcp"}) {
		t.Errorf("principalKinds = %v, want [ui human-mcp]", d.Authorization.PrincipalKinds)
	}
	if d.Authorization.Group != "index" {
		t.Errorf("group = %q, want index", d.Authorization.Group)
	}
	if !slices.Contains(d.Authorization.RefusedKinds, "admin") {
		t.Errorf("refusedKinds = %v, admin must be listed as refused", d.Authorization.RefusedKinds)
	}

	c := mustLoad(t)
	var search, status bool
	for _, r := range d.Routes {
		tools := slices.Clone(r.Tools)
		if r.Tool != "" {
			tools = append(tools, r.Tool)
		}
		for _, name := range tools {
			tl, ok := c.Tool(name)
			if !ok {
				t.Errorf("%s %s names unknown tool %s", r.Method, r.Path, name)
				continue
			}
			if tl.Group != "index" {
				t.Errorf("%s %s: tool %s is in group %q, want index", r.Method, r.Path, name, tl.Group)
			}
		}
		switch r.Path {
		case "/index/search":
			search = true
			if r.Method != "POST" || r.Tool != "ipfs_index_search" {
				t.Errorf("/index/search: method %q tool %q, want POST ipfs_index_search", r.Method, r.Tool)
			}
			if r.Request == nil {
				t.Fatal("/index/search has no request reference")
			}
			tl, _ := c.Tool("ipfs_index_search")
			if r.Request.Schema != tl.InputSchema {
				t.Errorf("/index/search request schema = %q, catalog says %q", r.Request.Schema, tl.InputSchema)
			}
			if r.Response.Schema != tl.OutputSchema {
				t.Errorf("/index/search response schema = %q, catalog says %q", r.Response.Schema, tl.OutputSchema)
			}
		case "/index/status":
			status = true
			if r.Method != "GET" {
				t.Errorf("/index/status: method %q, want GET", r.Method)
			}
			tl, _ := c.Tool("ipfs_index_status")
			if r.Response.Schema != tl.OutputSchema {
				t.Errorf("/index/status response schema = %q, catalog says %q", r.Response.Schema, tl.OutputSchema)
			}
			ref, ok := r.Response.AdditionalPropertiesFrom["bulk_run"]
			if !ok {
				t.Error("/index/status response lacks the bulk_run addition of ADR-0011 V5")
			} else if !strings.HasPrefix(ref.Ref, "schemas/tools/ipfs_index_bulk_status.output.json#/") {
				t.Errorf("bulk_run $ref = %q, want a pointer into ipfs_index_bulk_status.output.json", ref.Ref)
			}
		default:
			t.Errorf("unexpected route %s %s", r.Method, r.Path)
		}
	}
	if !search || !status {
		t.Errorf("descriptor names /index/search=%v, /index/status=%v; both are required", search, status)
	}

	// Every schema the descriptor references must exist in the embedded
	// contract and be a tool schema, never an inline copy.
	for _, p := range []string{
		"schemas/tools/ipfs_index_search.input.json",
		"schemas/tools/ipfs_index_search.output.json",
		"schemas/tools/ipfs_index_status.output.json",
		"schemas/tools/ipfs_index_bulk_status.output.json",
	} {
		if _, err := fs.ReadFile(files, p); err != nil {
			t.Errorf("referenced schema %s: %v", p, err)
		}
	}
	for _, key := range []string{"request", "response"} {
		for _, obj := range routeSubObjects(doc, key) {
			for _, kw := range []string{"type", "properties", "items", "required", "enum"} {
				if _, bad := obj[kw]; bad {
					t.Errorf("a route %s object carries JSON Schema keyword %q: an inline copy of a tool shape is forbidden", key, kw)
				}
			}
		}
	}

	if d.Errors.Shape != "schemas/common.json#/$defs/ToolError" {
		t.Errorf("errors.shape = %q, want the shared ToolError reference", d.Errors.Shape)
	}
	want := map[string]int{
		"not_configured":          404,
		"unauthenticated":         401,
		"permission_denied":       403,
		"egress_not_acknowledged": 403,
		"invalid_argument":        400,
		"index_unavailable":       503,
	}
	got := make(map[string]int, len(d.Errors.Mapping))
	for _, m := range d.Errors.Mapping {
		if _, dup := got[m.Code]; dup {
			t.Errorf("errors.mapping lists code %q twice", m.Code)
		}
		got[m.Code] = m.Status
	}
	for code, status := range want {
		if got[code] != status {
			t.Errorf("errors.mapping[%s] = %d, want %d", code, got[code], status)
		}
	}
}

// routeSubObjects returns the "request" and "response" sub-objects of every
// route entry in the raw descriptor document.
func routeSubObjects(doc any, key string) []map[string]any {
	m, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	routes, ok := m["routes"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, r := range routes {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if sub, ok := rm[key].(map[string]any); ok {
			out = append(out, sub)
		}
	}
	return out
}
