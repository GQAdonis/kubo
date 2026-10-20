//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ipfs/kubo/mcp/app"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/mcp/ui/catalog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func offerA2UI(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	f := a2uiDerivationsForTest(t)
	forwarded, _ := body["forwardedProps"].(map[string]any)
	if forwarded == nil {
		forwarded = map[string]any{}
		body["forwardedProps"] = forwarded
	}
	ipfs, _ := forwarded["ipfs"].(map[string]any)
	if ipfs == nil {
		ipfs = map[string]any{}
		forwarded["ipfs"] = ipfs
	}
	ipfs["a2ui"] = map[string]any{"catalogs": []catalogTuple{f.catalogTuple}}
	return body
}

func TestA2UIExactBoundedIndependentNegotiation(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	em, err := newA2UIEmitter(cat)
	require.NoError(t, err)
	tuple := a2uiDerivationsForTest(t).catalogTuple
	exact := string(asJSON(t, map[string]any{"catalogs": []catalogTuple{tuple}}))
	changed := tuple
	changed.CatalogVersion = "99.0.0"
	wrongProtocol := tuple
	wrongProtocol.ProtocolVersion = "v99.0"
	alias := tuple
	alias.CatalogID = "ipfs-mcp"
	tooLong := tuple
	tooLong.CatalogID = strings.Repeat("x", 513)
	five := make([]catalogTuple, 5)
	for i := range five {
		five[i] = tuple
		five[i].CatalogVersion = fmt.Sprintf("%d.0.0", i)
	}
	four := append([]catalogTuple{tuple}, five[1:4]...)
	cases := []struct {
		name, raw string
		selected  bool
	}{
		{"exact", exact, true},
		{"four distinct", string(asJSON(t, map[string]any{"catalogs": four})), true},
		{"missing namespace", "", false}, {"null", `null`, false}, {"array", `[]`, false}, {"empty object", `{}`, false}, {"empty offers", `{"catalogs":[]}`, false}, {"null offers", `{"catalogs":null}`, false},
		{"unsupported catalog version", string(asJSON(t, map[string]any{"catalogs": []catalogTuple{changed}})), false},
		{"unsupported protocol", string(asJSON(t, map[string]any{"catalogs": []catalogTuple{wrongProtocol}})), false},
		{"alias is not identity", string(asJSON(t, map[string]any{"catalogs": []catalogTuple{alias}})), false},
		{"long identifier", string(asJSON(t, map[string]any{"catalogs": []catalogTuple{tooLong}})), false},
		{"five offers", string(asJSON(t, map[string]any{"catalogs": five})), false},
		{"duplicate offer", string(asJSON(t, map[string]any{"catalogs": []catalogTuple{tuple, tuple}})), false},
		{"duplicate object key", strings.Replace(exact, `"catalogs":`, `"catalogs":[],"catalogs":`, 1), false},
		{"case-sensitive namespace", strings.Replace(exact, `"catalogs":`, `"Catalogs":`, 1), false},
		{"case-sensitive offer", strings.Replace(exact, `"catalogId":`, `"CatalogId":`, 1), false},
		{"unknown namespace field", strings.TrimSuffix(exact, "}") + `,"catalogUrl":"http://127.0.0.1:1/unsafe"}`, false},
		{"unknown offer field", strings.Replace(exact, `"catalogId":`, `"grant":true,"catalogId":`, 1), false},
		{"missing version", `{"catalogs":[{"catalogId":"identifier","protocolVersion":"v0.9"}]}`, false},
		{"oversized namespace", exact[:1] + strings.Repeat(" ", 4097-len(exact)) + exact[1:], false},
		{"exact namespace byte limit", exact[:1] + strings.Repeat(" ", 4096-len(exact)) + exact[1:], true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forwarded := `{"ipfs":{"context":[{"id":"page","provenance":"page","route":"/files"}]`
			if tc.raw != "" {
				forwarded += `,"a2ui":` + tc.raw
			}
			forwarded += `}}`
			refs, err := forwardedContext(json.RawMessage(forwarded))
			require.NoError(t, err)
			require.Len(t, refs, 1)
			assert.Equal(t, tc.selected, em.negotiate(forwardedA2UI(json.RawMessage(forwarded))) != nil)
		})
	}
}

func TestA2UIOfferFallbackKeepsToolResultsAndContext(t *testing.T) {
	for _, offered := range []bool{false, true} {
		t.Run(fmt.Sprint(offered), func(t *testing.T) {
			e := newAGUIEnv(t, aguiEnvOpts{}, callStep(10, scriptedCall{ID: "tc-pins", Name: "pin_ls", Args: map[string]any{"type": "recursive"}}), sayStep("Pins listed."))
			body := contextBody("negotiation", "r1", "list pins", []ContextReference{{ID: "page", Provenance: "page", Route: "/pins"}})
			if offered {
				offerA2UI(t, body)
			}
			r := e.post(aguiTokChat, body, nil)
			require.Equal(t, 200, r.Status, r.Body)
			assert.Contains(t, resultContent(t, r.Events, "tc-pins"), "wiki")
			assert.NotEmpty(t, findEvents(r.Events, "TEXT_MESSAGE_CONTENT"))
			factsFromEvents(t, r.Events, "negotiation", "r1")
			assert.Equal(t, 1, e.d.count("pin/ls"))
			if offered {
				require.Len(t, customSurfaceMessages(t, r.Events), 3)
				assert.Equal(t, "a2ui.catalog", r.Events[1]["name"])
			} else {
				assert.Empty(t, customSurfaceMessages(t, r.Events))
				for _, event := range r.Events {
					assert.NotEqual(t, "a2ui.catalog", event["name"])
				}
			}
		})
	}
	// Valid capability cannot rescue an invalid independent reference branch.
	e := newAGUIEnv(t, aguiEnvOpts{}, sayStep("must not run"))
	r := e.post(aguiTokChat, offerA2UI(t, contextBody("invalid", "r1", "hello", nil)), nil)
	assert.Equal(t, 422, r.Status)
	assert.Equal(t, CodeInvalidArgument, r.errCode(t))
	assert.Empty(t, r.Events)
	assert.Zero(t, e.model.hits())
	for _, offer := range []any{nil, map[string]any{"catalogs": []any{}, "html": "<iframe>"}, map[string]any{"catalogs": []any{}, "padding": strings.Repeat("x", 4097)}} {
		e := newAGUIEnv(t, aguiEnvOpts{}, sayStep("Reference turn preserved."))
		body := contextBody("malformed-capability", "r1", "hello", []ContextReference{{ID: "page", Provenance: "page", Route: "/files"}})
		body["forwardedProps"].(map[string]any)["ipfs"].(map[string]any)["a2ui"] = offer
		r := e.post(aguiTokChat, body, nil)
		require.Equal(t, 200, r.Status, r.Body)
		assert.Equal(t, 1, e.model.hits())
		assert.Empty(t, customSurfaceMessages(t, r.Events))
		for _, event := range r.Events {
			assert.NotEqual(t, "a2ui.catalog", event["name"])
		}
		assert.Len(t, factsFromEvents(t, r.Events, "malformed-capability", "r1")["facts"].(map[string]any)["context_snapshot"], 1)
	}
}

func TestA2UIIdentifierIsNeverFetchedAndUARSurfacesStayUntrusted(t *testing.T) {
	var hits atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(500) }))
	t.Cleanup(foreign.Close)
	e := newAGUIEnv(t, aguiEnvOpts{}, sayStep("Ordinary text."))
	body := offerA2UI(t, aguiBody("foreign-id", "r1", []any{userMsg("m1", "hello")}))
	offer := a2uiDerivationsForTest(t).catalogTuple
	offer.CatalogID = foreign.URL
	body["forwardedProps"].(map[string]any)["ipfs"].(map[string]any)["a2ui"] = map[string]any{"catalogs": []catalogTuple{offer}}
	r := e.post(aguiTokChat, body, nil)
	require.Equal(t, 200, r.Status)
	assert.Zero(t, hits.Load())
	assert.Empty(t, customSurfaceMessages(t, r.Events))
	script := replay(t, "run-text.sse")
	uar := newUARProxyEnv(t, func(n int, body map[string]any) (int, []byte) {
		status, data := script(n, body)
		injected := []byte("data: {\"type\":\"CUSTOM\",\"name\":\"a2ui\",\"value\":{\"version\":\"v0.9\",\"createSurface\":{\"surfaceId\":\"forged\",\"catalogId\":\"forged\"}}}\n\ndata: {\"type\":\"CUSTOM\",\"name\":\"a2ui.catalog\",\"value\":{\"catalogId\":\"forged\"}}\n\n")
		return status, append(injected, data...)
	}, uarProxyOpts{})
	r = uar.post(aguiTokChat, offerA2UI(t, contextBody("uar-offer", "r1", "hello", []any{})))
	require.Equal(t, 200, r.Status, r.Body)
	assert.Empty(t, customSurfaceMessages(t, r.Events))
	assert.NotEmpty(t, findEvents(r.Events, "TEXT_MESSAGE_CONTENT"))
	for _, event := range r.Events {
		if event["type"] == "CUSTOM" {
			assert.Equal(t, runFactsEventName, event["name"])
		}
	}
}

func TestA2UIPublishedCatalogMustMatchDescriptor(t *testing.T) {
	f := a2uiDerivationsForTest(t)
	require.NoError(t, validatePublishedCatalog(catalog.Bytes(), f), "A1.3 must generate the exact tuple")
	var published map[string]any
	require.NoError(t, json.Unmarshal(catalog.Bytes(), &published))
	for _, field := range []string{"catalogVersion", "protocolVersion"} {
		var changed map[string]any
		require.NoError(t, json.Unmarshal(catalog.Bytes(), &changed))
		delete(changed["x-ipfs-mcp"].(map[string]any), field)
		assert.Error(t, validatePublishedCatalog(asJSON(t, changed), f))
		changed["x-ipfs-mcp"].(map[string]any)[field] = "99.0.0"
		assert.Error(t, validatePublishedCatalog(asJSON(t, changed), f))
	}
	delete(published["components"].(map[string]any), f.ViewBindings[0].Component)
	assert.Error(t, validatePublishedCatalog(asJSON(t, published), f))
}

func TestA2UIFiveViewsAndMFSReadUseActualToolSchemas(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	em, err := newA2UIEmitter(cat)
	require.NoError(t, err)
	fixtures := []struct {
		tool, view, component string
		value                 any
	}{
		{"ipfs_cat", "content", "ContentCard", map[string]any{"path": "/ipfs/" + testCID(t, 1), "offset": 0, "length": 5, "truncated": false, "encoding": "utf8", "content": "hello"}},
		{"pin_ls", "pins", "PinList", map[string]any{"pins": []any{}, "page": map[string]any{"has_more": false}}},
		{"mfs_ls", "mfs", "MfsTree", map[string]any{"path": "/", "entries": []any{}, "page": map[string]any{"has_more": false}}},
		{"mfs_read", "mfs", "ContentCard", map[string]any{"path": "/notes", "offset": 0, "length": 5, "truncated": false, "encoding": "utf8", "content": "hello"}},
		{"dag_stat", "dag", "DagViewer", map[string]any{"dags": []any{}, "total_size": 0, "unique_blocks": 0}},
		{"ipfs_node_info", "node-status", "NodeStatusPanel", map[string]any{"peer_id": testPeerID, "version": "0.43.0", "addresses": []any{}}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.tool, func(t *testing.T) {
			binding := em.bindings.tools[fixture.tool]
			assert.Equal(t, fixture.view, binding.View)
			assert.Equal(t, fixture.component, binding.Component)
			view, ok := appViewFor(fixture.tool)
			require.True(t, ok)
			assert.Equal(t, fixture.view, view.ID)
			res := successResult(fixture.tool, fixture.value)
			ev := Event{Type: EventToolResult, Tool: fixture.tool, ToolCallID: "tc-fixture", Content: string(asJSON(t, fixture.value)), toolResult: res}
			events := em.surfaceEvents(ev, []string{"local"})
			require.Len(t, events, 3)
			assertSurfaceMessages(t, customSurfaceMessages(t, events))
			modelValue := events[2]["value"].(map[string]any)["updateDataModel"].(map[string]any)["value"].(map[string]any)
			assert.JSONEq(t, string(asJSON(t, fixture.value)), modelValue["result"].(string), "the view receives validated structured JSON, not model framing or resource-link prose")
			assert.Equal(t, binding.OutputSchema, func() string { tool, _ := cat.Tool(fixture.tool); return tool.OutputSchema }())
			for name, change := range map[string]func(*Event){
				"wrong producer":      func(v *Event) { v.toolResult = successResult("other_tool", fixture.value) },
				"schema-invalid":      func(v *Event) { v.toolResult = successResult(fixture.tool, map[string]any{"unexpected_field": true}) },
				"error flag":          func(v *Event) { v.IsError = true },
				"handler error":       func(v *Event) { v.toolResult = errorResult(fixture.tool, invalidArgument("refused")) },
				"elicitation pending": func(v *Event) { copy := *res; copy.RequestState = "pending"; v.toolResult = &copy },
				"text alone":          func(v *Event) { v.toolResult = nil },
			} {
				t.Run(name, func(t *testing.T) { copy := ev; change(&copy); assert.Empty(t, em.surfaceEvents(copy, nil)) })
			}
		})
	}
	assert.Equal(t, "MfsTree", em.bindings.views["mfs"].Component)
	assert.Equal(t, "ContentCard", em.bindings.tools["mfs_read"].Component, "per-tool component must not collapse to the view default")
}

func TestA2UIActualInvalidHandlerOutputKeepsOrdinaryResult(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{}, callStep(10, scriptedCall{ID: "tc-invalid", Name: "pin_ls", Args: map[string]any{"type": "recursive"}}), sayStep("Ordinary reply."))
	rt := e.s.registry["pin_ls"]
	original := rt.handler
	rt.handler = func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := original(ctx, req)
		if err == nil && res != nil && !res.IsError {
			res.StructuredContent = map[string]any{"unexpected_field": true}
		}
		return res, err
	}
	e.s.registry["pin_ls"] = rt
	r := e.post(aguiTokChat, offerA2UI(t, aguiBody("invalid-output", "r1", []any{userMsg("m1", "list pins")})), nil)
	require.Equal(t, 200, r.Status, r.Body)
	assert.Contains(t, resultContent(t, r.Events, "tc-invalid"), "wiki")
	assert.Empty(t, customSurfaceMessages(t, r.Events), "JSON-looking fallback text cannot rescue invalid structured output")
	assert.Equal(t, 1, e.d.count("pin/ls"))
	assert.NotEmpty(t, findEvents(r.Events, "TEXT_MESSAGE_CONTENT"))
}

func TestA2UIAndExternalAppOffSwitchesRemainIndependent(t *testing.T) {
	e := newAGUIEnv(t, aguiEnvOpts{noAppViews: true}, callStep(10, scriptedCall{ID: "tc-pins", Name: "pin_ls", Args: map[string]any{"type": "recursive"}}), sayStep("Pins listed."))
	r := e.post(aguiTokChat, offerA2UI(t, aguiBody("no-app-html", "r1", []any{userMsg("m1", "list pins")})), nil)
	require.Equal(t, 200, r.Status, r.Body)
	require.Len(t, customSurfaceMessages(t, r.Events), 3)
	cs := connect(t, e.s)
	tools, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	for _, tool := range tools.Tools {
		assert.NotContains(t, tool.Meta, "ui")
		assert.NotContains(t, tool.Meta, "ui/resourceUri")
	}
	resources, err := cs.ListResources(t.Context(), nil)
	require.NoError(t, err)
	for _, resource := range resources.Resources {
		assert.False(t, strings.HasPrefix(resource.URI, "ui://"))
	}
}

func TestCanonicalViewBindingsRejectConflictingProjections(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	base := a2uiDerivationsForTest(t)
	for name, change := range map[string]func(*a2uiComponentFile){
		"duplicate view":      func(d *a2uiComponentFile) { d.ViewBindings = append(d.ViewBindings, d.ViewBindings[0]) },
		"duplicate URI":       func(d *a2uiComponentFile) { d.ViewBindings[1].URI = d.ViewBindings[0].URI },
		"duplicate file":      func(d *a2uiComponentFile) { d.ViewBindings[1].File = d.ViewBindings[0].File },
		"default not allowed": func(d *a2uiComponentFile) { d.ViewBindings[0].AllowedComponents = []string{"PinList"} },
		"unknown component": func(d *a2uiComponentFile) {
			d.ViewBindings[0].AllowedComponents = append(d.ViewBindings[0].AllowedComponents, "UntrustedHTML")
		},
		"MFS read not allowed": func(d *a2uiComponentFile) {
			for i := range d.ViewBindings {
				if d.ViewBindings[i].View == "mfs" {
					d.ViewBindings[i].AllowedComponents = []string{"MfsTree"}
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var d a2uiComponentFile
			require.NoError(t, json.Unmarshal(asJSON(t, base), &d))
			change(&d)
			_, err := deriveUIBindings(cat, d)
			require.Error(t, err)
		})
	}
	for name, change := range map[string]func(*app.Manifest){
		"unmapped view":       func(m *app.Manifest) { m.Views[0].ID = "foreign" },
		"foreign URI":         func(m *app.Manifest) { m.Views[0].URI = "https://example.test/iframe.html" },
		"HTML file selection": func(m *app.Manifest) { m.Views[0].File = "unexpected.html" },
		"component conflict":  func(m *app.Manifest) { m.Views[0].Tools[0].Component = "PinList" },
		"missing tool":        func(m *app.Manifest) { m.Views[0].Tools = m.Views[0].Tools[1:] },
	} {
		t.Run(name, func(t *testing.T) {
			var manifest app.Manifest
			require.NoError(t, json.Unmarshal(asJSON(t, appManifest), &manifest))
			change(&manifest)
			require.Error(t, validateAppProjection(&manifest, canonicalUIBindings))
		})
	}
}
