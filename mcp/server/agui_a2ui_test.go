//go:build !noagent

package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a2uiDerivationsForTest reads the derivations file. Its component list is the
// catalog's: mcp/ui/catalog/ipfs-mcp.catalog.json is generated from it, and
// the go:embed directive cannot reach mcp/ui from this package, so the derivations are the
// server-side source of the catalog's component names.
func a2uiDerivationsForTest(t *testing.T) a2uiComponentFile {
	t.Helper()
	data, err := fs.ReadFile(contract.FS(), "derivations/a2ui-components.json")
	require.NoError(t, err)
	var f a2uiComponentFile
	require.NoError(t, json.Unmarshal(data, &f))
	return f
}

// a2uiComponentNames is the set of component names in the catalog.
func a2uiComponentNames(t *testing.T) map[string]struct{} {
	t.Helper()
	f := a2uiDerivationsForTest(t)
	names := make(map[string]struct{}, len(f.Components))
	for _, c := range f.Components {
		names[c.Name] = struct{}{}
	}
	return names
}

// customSurfaceMessages unwraps the A2UI message of every CUSTOM event named
// a2ui in the stream.
func customSurfaceMessages(t *testing.T, events []aguiEvent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range events {
		if e["type"] == "CUSTOM" && e["name"] == aguiA2UIEventName {
			m, ok := e["value"].(map[string]any)
			require.True(t, ok, "CUSTOM a2ui value is %T, want an object", e["value"])
			out = append(out, m)
		}
	}
	return out
}

// assertSurfaceMessages asserts the A2UI v0.9 envelope of every message and
// that every emitted component name is in the catalog — the acceptance check
// of mcp-agui-a2ui-registry 1.2.
func assertSurfaceMessages(t *testing.T, msgs []map[string]any) {
	t.Helper()
	names := a2uiComponentNames(t)
	catalogID := a2uiDerivationsForTest(t).CatalogID
	for _, m := range msgs {
		assert.Equal(t, a2uiProtocolVersion, m["version"], "every A2UI message carries the protocol version")
		if cs, ok := m["createSurface"].(map[string]any); ok {
			assert.Equal(t, catalogID, cs["catalogId"], "createSurface names the catalog ID")
		}
		uc, ok := m["updateComponents"].(map[string]any)
		if !ok {
			continue
		}
		for _, c := range uc["components"].([]any) {
			name, ok := c.(map[string]any)["component"].(string)
			require.True(t, ok, "a component entry without a name: %v", c)
			assert.Contains(t, names, name, "emitted component %s is not in the catalog", name)
		}
	}
}

// TestA2UISurfaceComponentsAreInTheCatalog builds the surface of a success and
// an error result for every tool in the catalog and asserts every emitted
// component name is in the catalog.
func TestA2UISurfaceComponentsAreInTheCatalog(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	em, err := newA2UIEmitter(cat)
	require.NoError(t, err)
	var all []map[string]any
	for _, tool := range cat.Tools {
		for _, isErr := range []bool{false, true} {
			ev := Event{Type: EventToolResult, ToolCallID: "tc-x", Tool: tool.Name, Arguments: `{}`, Content: `{}`, IsError: isErr}
			events := em.surfaceEvents(ev, []string{"local"})
			require.Len(t, events, 3, "%s (error=%v): createSurface, updateComponents, updateDataModel", tool.Name, isErr)
			all = append(all, customSurfaceMessages(t, events)...)
		}
	}
	assertSurfaceMessages(t, all)
}

// TestA2UISurfaceSkipsUnknownTools: a tool the catalog does not name emits
// nothing (cannot happen in a run; the offered tools are catalog tools).
func TestA2UISurfaceSkipsUnknownTools(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	em, err := newA2UIEmitter(cat)
	require.NoError(t, err)
	assert.Nil(t, em.surfaceEvents(Event{Type: EventToolResult, ToolCallID: "tc-x", Tool: "not_a_tool", Content: "{}"}, nil))
}

// TestAGUIToolResultEmitsA2UISurface runs a tool call through the real
// endpoint and checks the emitted surface: the catalog ID, the card bound to
// the call in the island's wire shape, and the acknowledged hosts of an
// egress-rule tool, which only the server knows.
func TestAGUIToolResultEmitsA2UISurface(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{},
		callStep(40, scriptedCall{ID: "tc-1", Name: "pin_ls", Args: map[string]any{"type": "recursive"}}),
		sayStep("You have one recursive pin, named wiki."))
	r := e.post(aguiTokChat, aguiBody("thread-a2ui", "r1", []any{userMsg("m1", "Which pins do I have?")}), nil)
	require.Equal(t, http.StatusOK, r.Status)

	msgs := customSurfaceMessages(t, r.Events)
	require.Len(t, msgs, 3, "one surface per tool result: createSurface, updateComponents, updateDataModel")
	assertSurfaceMessages(t, msgs)

	create := msgs[0]["createSurface"].(map[string]any)
	assert.Equal(t, "call-tc-1", create["surfaceId"], "the island's local fallback uses the same surface ID")

	card := msgs[1]["updateComponents"].(map[string]any)["components"].([]any)[0].(map[string]any)
	assert.Equal(t, "ToolCallCard", card["component"])
	assert.Equal(t, map[string]any{"path": "/call"}, card["call"])
	cat, err := contract.Load()
	require.NoError(t, err)
	tool, ok := cat.Tool("pin_ls")
	require.True(t, ok)
	if tool.ContentEgress {
		assert.Equal(t, []any{"local"}, card["acknowledgedHosts"], "the chat principal's egress_ack")
	} else {
		assert.NotContains(t, card, "acknowledgedHosts")
	}

	call := msgs[2]["updateDataModel"].(map[string]any)["value"].(map[string]any)
	assert.Equal(t, "call-tc-1", msgs[2]["updateDataModel"].(map[string]any)["surfaceId"])
	assert.Equal(t, "/call", msgs[2]["updateDataModel"].(map[string]any)["path"])
	assert.Equal(t, "tc-1", call["id"])
	assert.Equal(t, "pin_ls", call["name"])
	assert.JSONEq(t, `{"type":"recursive"}`, call["arguments"].(string))
	assert.Equal(t, "done", call["status"])
	assert.Contains(t, call["result"], "wiki")
}

// TestAGUIErrorResultEmitsAnErrorSurface: a refused call still gets a surface,
// with status error and the ToolError text the island renders as a notice.
func TestAGUIErrorResultEmitsAnErrorSurface(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{},
		callStep(40, scriptedCall{ID: "tc-1", Name: "ipfs_cat", Args: map[string]any{"path": testCID(t, 1)}}),
		sayStep("I cannot read that file."))
	r := e.post(aguiTokNoAck, aguiBody("thread-egress", "r1", []any{userMsg("m1", "read it")}), nil)
	require.Equal(t, http.StatusOK, r.Status)
	msgs := customSurfaceMessages(t, r.Events)
	require.Len(t, msgs, 3)
	assertSurfaceMessages(t, msgs)
	call := msgs[2]["updateDataModel"].(map[string]any)["value"].(map[string]any)
	assert.Equal(t, "error", call["status"])
	assert.Contains(t, call["result"], CodeEgressNotAcknowledged)
	card := msgs[1]["updateComponents"].(map[string]any)["components"].([]any)[0].(map[string]any)
	assert.NotContains(t, card, "acknowledgedHosts", "the principal acknowledged no host")
}

// TestNoA2UISurfaceEventsWhenDisabled is the acceptance check of
// mcp-agui-a2ui-registry 1.5 (W6), named in the config reference's agent.a2ui
// row: with agent.a2ui off the run stream carries no A2UI CUSTOM event. The
// stream is otherwise unchanged — TOOL_CALL_RESULT still goes out, so the
// island's local ToolCallCard fallback renders.
func TestNoA2UISurfaceEventsWhenDisabled(t *testing.T) {
	off := false
	e := newAGUIEnv(t, aguiEnvOpts{a2ui: &off},
		callStep(40, scriptedCall{ID: "tc-1", Name: "pin_ls", Args: map[string]any{"type": "recursive"}}),
		sayStep("You have one recursive pin, named wiki."))
	r := e.post(aguiTokChat, aguiBody("thread-a2ui-off", "r1", []any{userMsg("m1", "Which pins do I have?")}), nil)
	require.Equal(t, http.StatusOK, r.Status)

	for _, ev := range r.Events {
		assert.NotEqual(t, "CUSTOM", ev["type"], "no CUSTOM event when agent.a2ui is off: %v", ev)
	}
	var results int
	for _, ev := range r.Events {
		if ev["type"] == "TOOL_CALL_RESULT" {
			results++
			assert.Equal(t, "tc-1", ev["toolCallId"], "the local fallback still gets the result")
		}
	}
	assert.Equal(t, 1, results, "the run stream is otherwise unchanged")
}

// TestAgentConfigA2UIDefaultsOn: the off switch defaults on wherever /agui is
// on (the contract schema's agent.a2ui default); only an explicit false stops
// the surfaces.
func TestAgentConfigA2UIDefaultsOn(t *testing.T) {
	assert.True(t, AgentConfig{}.A2UIEnabled(), "absent means the default, on")
	off := false
	assert.False(t, AgentConfig{A2UI: &off}.A2UIEnabled())
	on := true
	assert.True(t, AgentConfig{A2UI: &on}.A2UIEnabled())
}
