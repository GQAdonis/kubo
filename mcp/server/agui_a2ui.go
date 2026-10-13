//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/ipfs/kubo/mcp/contract"
)

// A2UI over AG-UI (change mcp-agui-a2ui-registry, decision D15): when a run
// produces a tool result, the server builds the A2UI v0.9 surface for it and
// streams the surface as AG-UI CUSTOM events, one A2UI message per event. The
// derivations file (mcp/contract/derivations/a2ui-components.json, embedded in
// the contract package) is the only source: the catalog ID, the component
// names the server may emit and the chat rules that say which chat component
// wraps a call. A client that ignores CUSTOM events loses nothing: the island
// keeps its local ToolCallCard fallback for the same calls.
//
// Only ToolCallCard is built here. ApprovalCard renders an interrupt, which
// the RUN_FINISHED outcome already carries; MessageStream and the view
// components are the client's own (ToolCallCard embeds the bound view and the
// EgressBadge itself, given the acknowledged hosts only the server knows).

const (
	// a2uiProtocolVersion is the A2UI version of every emitted message.
	a2uiProtocolVersion = "v0.9"
	// aguiA2UIEventName is the name of the AG-UI CUSTOM events that carry A2UI
	// messages: {"type":"CUSTOM","name":"a2ui","value":<one A2UI message>}.
	aguiA2UIEventName = "a2ui"
	// a2uiCardRule is the chatRules key of the tool-call wrapper component.
	a2uiCardRule = "ToolCallCard"
	// a2uiEgressRule is the chatRules key that marks results carrying node
	// content; the card then names the principal's acknowledged hosts.
	a2uiEgressRule = "EgressBadge"
)

// a2uiComponentFile is derivations/a2ui-components.json.
type a2uiComponentFile struct {
	CatalogID string `json:"catalogId"`
	Serve     struct {
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
	ChatRules map[string]string `json:"chatRules"`
}

// a2uiEmitter builds the surface messages of one tool result. Every component
// name it can emit comes from the derivations' component list, which is also
// the published catalog's: a rule naming something the list does not contain
// fails at construction, not at stream time.
type a2uiEmitter struct {
	cat        *contract.Catalog
	catalogID  string
	components map[string]struct{}
	rules      map[string]string
	// catalogRoute is serve.route and catalogEvent aguiEvents.catalog: where
	// the catalog is served and the name of the per-run advertisement.
	catalogRoute string
	catalogEvent string
}

// newA2UIEmitter loads the derivations and checks them against the catalog:
// the card rule must exist and name a catalog component, and every chat rule
// must evaluate for every tool, so a malformed derivations file fails the
// /agui mount instead of a run mid-stream.
func newA2UIEmitter(cat *contract.Catalog) (*a2uiEmitter, error) {
	data, err := fs.ReadFile(contract.FS(), "derivations/a2ui-components.json")
	if err != nil {
		return nil, fmt.Errorf("read a2ui derivations: %w", err)
	}
	var f a2uiComponentFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse a2ui derivations: %w", err)
	}
	if f.CatalogID == "" {
		return nil, fmt.Errorf("a2ui derivations: catalogId is empty")
	}
	// Catalog discovery (mcp-agui-a2ui-registry 1.1): the derivations name the
	// route the catalog is served at and the CUSTOM event names. The server and
	// the contract must agree, so a rename on either side fails the /agui mount
	// instead of advertising a route nothing serves.
	if f.Serve.Route != RouteAGUICatalog {
		return nil, fmt.Errorf("a2ui derivations: serve.route %q, the server mounts %s", f.Serve.Route, RouteAGUICatalog)
	}
	if f.AguiEvents.Surface != aguiA2UIEventName {
		return nil, fmt.Errorf("a2ui derivations: aguiEvents.surface %q, the server emits %s", f.AguiEvents.Surface, aguiA2UIEventName)
	}
	if f.AguiEvents.Catalog == "" {
		return nil, fmt.Errorf("a2ui derivations: aguiEvents.catalog is empty")
	}
	e := &a2uiEmitter{cat: cat, catalogID: f.CatalogID, rules: f.ChatRules, components: make(map[string]struct{}, len(f.Components)),
		catalogRoute: f.Serve.Route, catalogEvent: f.AguiEvents.Catalog}
	for _, c := range f.Components {
		if c.Name == "" {
			return nil, fmt.Errorf("a2ui derivations: a component has no name")
		}
		e.components[c.Name] = struct{}{}
	}
	if _, ok := e.components[a2uiCardRule]; !ok {
		return nil, fmt.Errorf("a2ui derivations: %s is not a component", a2uiCardRule)
	}
	if _, ok := e.rules[a2uiCardRule]; !ok {
		return nil, fmt.Errorf("a2ui derivations: no chat rule for %s", a2uiCardRule)
	}
	for _, t := range cat.Tools {
		for name, rule := range e.rules {
			if _, err := cat.EvalRule(rule, t); err != nil {
				return nil, fmt.Errorf("a2ui chat rule %s for %s: %w", name, t.Name, err)
			}
		}
	}
	return e, nil
}

// ruleOn reports whether the named chat rule evaluates true for the tool. The
// rules were validated for every tool at construction, so an evaluation error
// cannot happen here and reads as off.
func (e *a2uiEmitter) ruleOn(name string, t contract.Tool) bool {
	rule, ok := e.rules[name]
	if !ok {
		return false
	}
	v, err := e.cat.EvalRule(rule, t)
	if err != nil {
		return false
	}
	on, _ := v.(bool)
	return on
}

// surfaceEvents returns the CUSTOM events for one tool result: createSurface,
// then updateComponents with the card, then updateDataModel with the call in
// the island's wire shape (mcp/ui ChatToolCallWire). ack is the principal's
// egress_ack; it is sent only when the egress chat rule applies to the tool.
// A tool outside the catalog, or one the card rule excludes, emits nothing.
func (e *a2uiEmitter) surfaceEvents(ev Event, ack []string) []aguiEvent {
	t, ok := e.cat.Tool(ev.Tool)
	if !ok || !e.ruleOn(a2uiCardRule, t) {
		return nil
	}
	status := "done"
	if ev.IsError {
		status = "error"
	}
	surface := "call-" + ev.ToolCallID
	card := map[string]any{"id": "root", "component": a2uiCardRule, "call": map[string]any{"path": "/call"}}
	if e.ruleOn(a2uiEgressRule, t) && len(ack) > 0 {
		card["acknowledgedHosts"] = ack
	}
	call := map[string]any{
		"id":        ev.ToolCallID,
		"name":      ev.Tool,
		"arguments": ev.Arguments,
		"result":    ev.Content,
		"status":    status,
	}
	msgs := []any{
		map[string]any{"version": a2uiProtocolVersion, "createSurface": map[string]any{"surfaceId": surface, "catalogId": e.catalogID}},
		map[string]any{"version": a2uiProtocolVersion, "updateComponents": map[string]any{"surfaceId": surface, "components": []any{card}}},
		map[string]any{"version": a2uiProtocolVersion, "updateDataModel": map[string]any{"surfaceId": surface, "path": "/call", "value": call}},
	}
	out := make([]aguiEvent, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, aguiEvent{"type": "CUSTOM", "name": aguiA2UIEventName, "value": m})
	}
	return out
}

// catalogAdvertisement is the one CUSTOM event per run that names the catalog
// and where to fetch it (derivations aguiEvents.catalog), sent immediately
// after RUN_STARTED.
func (e *a2uiEmitter) catalogAdvertisement() aguiEvent {
	return aguiEvent{"type": "CUSTOM", "name": e.catalogEvent, "value": map[string]any{
		"catalogId":  e.catalogID,
		"catalogUrl": e.catalogRoute,
	}}
}
