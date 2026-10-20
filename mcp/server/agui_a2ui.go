//go:build !noagent

package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/mcp/ui/catalog"
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
	// aguiA2UIEventName is the name of the AG-UI CUSTOM events that carry A2UI
	// messages: {"type":"CUSTOM","name":"a2ui","value":<one A2UI message>}.
	aguiA2UIEventName = "a2ui"
	// a2uiCardRule is the chatRules key of the tool-call wrapper component.
	a2uiCardRule = "ToolCallCard"
	// a2uiEgressRule is the chatRules key that marks results carrying node
	// content; the card then names the principal's acknowledged hosts.
	a2uiEgressRule = "EgressBadge"
)

// a2uiEmitter builds the surface messages of one tool result. Every component
// name it can emit comes from the derivations' component list, which is also
// the published catalog's: a rule naming something the list does not contain
// fails at construction, not at stream time.
type a2uiEmitter struct {
	cat        *contract.Catalog
	tuple      catalogTuple
	bindings   *uiBindingSet
	outputs    map[string]*jsonschema.Resolved
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
	bindings, err := deriveUIBindings(cat, f)
	if err != nil {
		return nil, err
	}
	if err := validatePublishedCatalog(catalog.Bytes(), f); err != nil {
		return nil, err
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
	e := &a2uiEmitter{cat: cat, tuple: f.catalogTuple, bindings: bindings, outputs: make(map[string]*jsonschema.Resolved, len(cat.Tools)), rules: f.ChatRules, components: make(map[string]struct{}, len(f.Components)),
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
		raw, err := contract.BundleSchema(t.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("a2ui output schema for %s: %w", t.Name, err)
		}
		output, err := resolveSchema(raw)
		if err != nil {
			return nil, fmt.Errorf("a2ui output schema for %s: %w", t.Name, err)
		}
		e.outputs[t.Name] = output
		for name, rule := range e.rules {
			if _, err := cat.EvalRule(rule, t); err != nil {
				return nil, fmt.Errorf("a2ui chat rule %s for %s: %w", name, t.Name, err)
			}
		}
	}
	return e, nil
}

func validatePublishedCatalog(raw []byte, descriptor a2uiComponentFile) error {
	var published struct {
		ID        string `json:"$id"`
		CatalogID string `json:"catalogId"`
		Metadata  struct {
			CatalogVersion  string `json:"catalogVersion"`
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"x-ipfs-mcp"`
		Components map[string]json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(raw, &published); err != nil {
		return fmt.Errorf("parse published A2UI catalog: %w", err)
	}
	if published.ID != descriptor.CatalogID || published.CatalogID != descriptor.CatalogID || published.Metadata.CatalogVersion != descriptor.CatalogVersion || published.Metadata.ProtocolVersion != descriptor.ProtocolVersion {
		return fmt.Errorf("published A2UI catalog does not match the canonical catalog tuple")
	}
	for _, component := range descriptor.Components {
		if _, ok := published.Components[component.Name]; !ok {
			return fmt.Errorf("published A2UI catalog omits component %q", component.Name)
		}
	}
	return nil
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
	structured, valid := e.validatedSuccess(ev)
	if !valid {
		return nil
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
		// This is the validated producing tool's structured output. Its ordinary
		// TOOL_CALL_RESULT/model text keeps framing and resource-link fallbacks;
		// those delimiters are not a second JSON encoding for the view to guess.
		"result": string(structured),
		"status": "done",
	}
	msgs := []any{
		map[string]any{"version": e.tuple.ProtocolVersion, "createSurface": map[string]any{"surfaceId": surface, "catalogId": e.tuple.CatalogID}},
		map[string]any{"version": e.tuple.ProtocolVersion, "updateComponents": map[string]any{"surfaceId": surface, "components": []any{card}}},
		map[string]any{"version": e.tuple.ProtocolVersion, "updateDataModel": map[string]any{"surfaceId": surface, "path": "/call", "value": call}},
	}
	out := make([]aguiEvent, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, aguiEvent{"type": "CUSTOM", "name": aguiA2UIEventName, "value": m})
	}
	return out
}

// A tool label, isError=false or a JSON-looking text fallback cannot establish
// producer provenance. Only the actual handler's structured result is checked.
func (e *a2uiEmitter) validatedSuccess(ev Event) (json.RawMessage, bool) {
	r := ev.toolResult
	if ev.Type != EventToolResult || ev.ToolCallID == "" || ev.IsError || r == nil || r.IsError || r.InputRequests != nil || r.RequestState != "" || r.Meta[toolNameMetaKey] != ev.Tool {
		return nil, false
	}
	binding, ok := e.bindings.tools[ev.Tool]
	if !ok || binding.Component == "" {
		return nil, false
	}
	if binding.View != "" {
		view, ok := e.bindings.views[binding.View]
		if !ok || !slices.Contains(view.AllowedComponents, binding.Component) {
			return nil, false
		}
	}
	raw, err := json.Marshal(r.StructuredContent)
	if err != nil || len(raw) > maxToolResultBytes {
		return nil, false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	output := e.outputs[ev.Tool]
	if output == nil || output.Validate(value) != nil {
		return nil, false
	}
	return raw, true
}

// catalogAdvertisement is the one CUSTOM event per run that names the catalog
// and where to fetch it (derivations aguiEvents.catalog), sent immediately
// after RUN_STARTED.
func (e *a2uiEmitter) catalogAdvertisement() aguiEvent {
	return aguiEvent{"type": "CUSTOM", "name": e.catalogEvent, "value": map[string]any{
		"catalogId":       e.tuple.CatalogID,
		"catalogVersion":  e.tuple.CatalogVersion,
		"protocolVersion": e.tuple.ProtocolVersion,
		"catalogUrl":      e.catalogRoute,
	}}
}
