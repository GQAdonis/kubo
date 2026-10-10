// Package contract embeds the IPFS MCP tool contract (catalog, JSON Schemas,
// derivation rules, fixtures and the ipfs mcp config schema) and exposes the
// catalog to Go consumers such as mcp/server.
//
// The JSON files in this directory are the single source of truth. This
// package only loads them, enforces the cross-field rules that JSON Schema
// cannot express, and interprets the derivation rules. Full JSON Schema
// validation of the contract lives in this package's tests.
package contract

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
)

//go:embed catalog.json catalog.schema.json schemas confirmation resources derivations agentic fixtures config
var files embed.FS

// FS returns the embedded contract files, rooted at mcp/contract.
func FS() fs.FS { return files }

// Confirmation classes.
const (
	ClassNone        = "none"
	ClassWrite       = "write"
	ClassDestructive = "destructive"
)

// UntrustedContentMetaKey is the CallToolResult _meta key, set to true, on
// every successful result of a tool whose catalog entry has
// returnsUntrustedContent. Its value is untrusted data, never instructions.
const UntrustedContentMetaKey = "ai.ipfs/untrusted-content"

// Catalog is catalog.json.
type Catalog struct {
	ContractVersion string        `json:"contractVersion"`
	MinKuboVersion  string        `json:"minKuboVersion"`
	ToolSelection   ToolSelection `json:"toolSelection"`
	Groups          []Group       `json:"groups"`
	Tools           []Tool        `json:"tools"`
}

// ToolSelection defines the default tool set and the destructive selector
// (ADR-0003 section 2).
type ToolSelection struct {
	DestructiveSelector string `json:"destructiveSelector"`
	Default             struct {
		Groups  []string `json:"groups"`
		Classes []string `json:"classes"`
	} `json:"default"`
	ReadOnly struct {
		Classes []string `json:"classes"`
	} `json:"readOnly"`
}

// Group is one tool group.
type Group struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Diagnostic  bool   `json:"diagnostic"`
}

// Annotations are the authored MCP ToolAnnotations.
type Annotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
	IdempotentHint  bool `json:"idempotentHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
}

// Tool is one catalog entry.
type Tool struct {
	Name                    string      `json:"name"`
	Group                   string      `json:"group"`
	Title                   string      `json:"title"`
	Description             string      `json:"description"`
	Annotations             Annotations `json:"annotations"`
	Confirmation            string      `json:"confirmation"`
	ReturnsUntrustedContent bool        `json:"returnsUntrustedContent"`
	ContentEgress           bool        `json:"contentEgress"`
	RPC                     []string    `json:"rpc"`
	RPCOptional             []string    `json:"rpcOptional,omitempty"`
	MinKuboVersion          string      `json:"minKuboVersion"`
	Paginated               bool        `json:"paginated"`
	InputSchema             string      `json:"inputSchema"`
	OutputSchema            string      `json:"outputSchema"`
	ConfirmationSummary     string      `json:"confirmationSummary,omitempty"`
	UI                      struct {
		A2UIComponent *string `json:"a2uiComponent"`
		AppView       *string `json:"appView"`
	} `json:"ui"`
	A2A struct {
		Skill   string `json:"skill"`
		Example string `json:"example"`
	} `json:"a2a"`
}

// Load parses the embedded catalog and runs Check.
func Load() (*Catalog, error) {
	data, err := files.ReadFile("catalog.json")
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	return Parse(data)
}

// Parse parses catalog JSON and runs Check.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if err := c.Check(); err != nil {
		return nil, err
	}
	return &c, nil
}

// AllowedRPC returns the RPC command paths an API authorization must allow for
// the tool to work fully: the required rpc list followed by rpcOptional.
// Consumers that print API.Authorizations AllowedPaths use this, not RPC.
func (t Tool) AllowedRPC() []string {
	return slices.Concat(t.RPC, t.RPCOptional)
}

// Tool returns the entry for name.
func (c *Catalog) Tool(name string) (Tool, bool) {
	for _, t := range c.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Group returns the group named name.
func (c *Catalog) Group(name string) (Group, bool) {
	for _, g := range c.Groups {
		if g.Name == name {
			return g, true
		}
	}
	return Group{}, false
}

// CheckToolNames returns an error naming every tool a consumer uses that the
// catalog does not define. mcp/server calls it at startup with its
// registered handlers; generators call it with the names they emit.
func (c *Catalog) CheckToolNames(names []string) error {
	var errs []error
	for _, n := range names {
		if _, ok := c.Tool(n); !ok {
			errs = append(errs, fmt.Errorf("tool %q has no contract entry", n))
		}
	}
	return errors.Join(errs...)
}

// DefaultToolSet returns the names of the tools enabled when the operator
// selects nothing: every default group, confirmation classes none and write.
// Destructive-class tools are never in it.
func (c *Catalog) DefaultToolSet() []string {
	return c.SelectTools(c.ToolSelection.Default.Groups, false)
}

// SelectTools applies a --tools / tools.groups selection. selectors holds
// group names and optionally the destructive selector; readOnly keeps only
// class none.
func (c *Catalog) SelectTools(selectors []string, readOnly bool) []string {
	groups := make(map[string]struct{}, len(selectors))
	destructive := false
	for _, s := range selectors {
		if s == c.ToolSelection.DestructiveSelector {
			destructive = true
			continue
		}
		groups[s] = struct{}{}
	}
	out := make([]string, 0, len(c.Tools))
	for _, t := range c.Tools {
		if _, ok := groups[t.Group]; !ok {
			continue
		}
		switch {
		case t.Confirmation == ClassNone:
		case readOnly:
			continue
		case t.Confirmation == ClassDestructive && !destructive:
			continue
		}
		out = append(out, t.Name)
	}
	return out
}
