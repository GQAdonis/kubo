package server

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinServer starts a server with the pin tools over a fake daemon whose
// client can elicit and approves everything, so the tests here exercise the
// tool bodies and schemas, not the gate.
func pinServer(t *testing.T, initial map[string]fakePin, opts Options) (*mcp.ClientSession, *fakeDaemon, *pinStore) {
	t.Helper()
	d := newFakeDaemon(t)
	store := installPins(d, initial)
	s := mustNew(t, d, opts)
	return connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	}), d, store
}

func TestPinAddMatchesOutputSchema(t *testing.T) {
	c := testCID(t, 1)
	cs, d, store := pinServer(t, nil, Options{})

	raw := callOK(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + c, "name": "docs"})
	var out pinAddOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, []string{c}, out.Pins)
	assert.Equal(t, "docs", out.Name)
	validateOutput(t, "pin_add", out)
	assert.True(t, store.has(c))
	assert.Equal(t, 1, d.count("pin/add"))
	last := d.log[len(d.log)-1]
	assert.Equal(t, []string{"/ipfs/" + c}, last.Args)
	assert.Equal(t, true, last.Opts["recursive"], "recursive defaults to true")
	assert.Equal(t, "docs", last.Opts["name"])
}

func TestPinAddNonRecursive(t *testing.T) {
	c := testCID(t, 2)
	cs, d, _ := pinServer(t, nil, Options{})
	callOK(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + c, "recursive": false})
	assert.Equal(t, false, d.log[len(d.log)-1].Opts["recursive"])
}

func TestPinRmAndUpdateMatchOutputSchemas(t *testing.T) {
	a, b := testCID(t, 3), testCID(t, 4)
	cs, _, store := pinServer(t, map[string]fakePin{a: {Type: "recursive"}}, Options{Tools: []string{"pin", "destructive"}})

	var upd pinUpdateOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_update", map[string]any{"from": "/ipfs/" + a, "to": "/ipfs/" + b}), &upd))
	assert.Equal(t, pinUpdateOutput{From: a, To: b, UnpinnedOld: true}, upd)
	validateOutput(t, "pin_update", upd)
	assert.False(t, store.has(a), "unpin defaults to true")
	assert.True(t, store.has(b))

	var rm pinRmOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + b}), &rm))
	assert.Equal(t, []string{b}, rm.Unpinned)
	validateOutput(t, "pin_rm", rm)
	assert.False(t, store.has(b))
}

func TestPinUpdateKeepsOldPin(t *testing.T) {
	a, b := testCID(t, 5), testCID(t, 6)
	cs, _, store := pinServer(t, map[string]fakePin{a: {Type: "recursive"}}, Options{Tools: []string{"pin", "destructive"}})
	var upd pinUpdateOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_update", map[string]any{"from": "/ipfs/" + a, "to": "/ipfs/" + b, "unpin": false}), &upd))
	assert.False(t, upd.UnpinnedOld)
	assert.True(t, store.has(a))
}

func TestPinRmUnknownPinIsNotFoundToolError(t *testing.T) {
	cs, _, _ := pinServer(t, nil, Options{Tools: []string{"pin", "destructive"}})
	res := toolCall(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 7)})
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeRPCError, te.Code)
	assert.Equal(t, "pin_rm", te.Tool)
	validateAgainstCommon(t, "ToolError", res.StructuredContent)
}

func TestPinLsPaginatesAndMatchesSchema(t *testing.T) {
	initial := map[string]fakePin{}
	for i := range 7 {
		initial[testCID(t, 100+i)] = fakePin{Type: "recursive", Name: "n"}
	}
	cs, _, _ := pinServer(t, initial, Options{})

	seen := map[string]struct{}{}
	cursor := ""
	pages := 0
	for {
		args := map[string]any{"limit": 3}
		if cursor != "" {
			args["cursor"] = cursor
		}
		var out pinLsOutput
		require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_ls", args), &out))
		validateOutput(t, "pin_ls", out)
		require.LessOrEqual(t, len(out.Pins), 3)
		for _, p := range out.Pins {
			_, dup := seen[p.CID]
			assert.False(t, dup, "no pin is returned twice")
			seen[p.CID] = struct{}{}
		}
		pages++
		if !out.Page.HasMore {
			assert.Empty(t, out.Page.NextCursor)
			break
		}
		require.NotEmpty(t, out.Page.NextCursor)
		cursor = out.Page.NextCursor
	}
	assert.Len(t, seen, 7, "the cursor yields the rest")
	assert.Equal(t, 3, pages)
}

func TestPinLsFilters(t *testing.T) {
	a, b, c := testCID(t, 11), testCID(t, 12), testCID(t, 13)
	cs, _, _ := pinServer(t, map[string]fakePin{
		a: {Type: "recursive", Name: "site docs"},
		b: {Type: "direct", Name: "photos"},
		c: {Type: "recursive"},
	}, Options{})

	list := func(args map[string]any) pinLsOutput {
		var out pinLsOutput
		require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_ls", args), &out))
		validateOutput(t, "pin_ls", out)
		return out
	}
	assert.Len(t, list(nil).Pins, 3)
	assert.Equal(t, []pinEntry{{CID: b, Type: "direct", Name: "photos"}}, list(map[string]any{"type": "direct"}).Pins)
	assert.Equal(t, []pinEntry{{CID: a, Type: "recursive", Name: "site docs"}}, list(map[string]any{"name": "docs"}).Pins)
	assert.Equal(t, []pinEntry{{CID: b, Type: "direct", Name: "photos"}}, list(map[string]any{"path": "/ipfs/" + b}).Pins)
	assert.Empty(t, list(map[string]any{"path": "/ipfs/" + testCID(t, 99)}).Pins, "an unpinned path lists nothing")
}

func TestPinLsRejectsBadCursor(t *testing.T) {
	cs, _, _ := pinServer(t, nil, Options{})
	res := toolCall(t, cs, "pin_ls", map[string]any{"cursor": "not-a-cursor!"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
}

func TestPinVerifyPaginatesAndStopsEarly(t *testing.T) {
	cs, d, store := pinServer(t, nil, Options{})
	for i := range 5 {
		store.verifyLines = append(store.verifyLines, pinVerifyLine{Cid: testCID(t, 200+i), Ok: i != 3})
	}
	store.verifyLines[3].BadNodes = []struct {
		Cid string
		Err string
	}{{Cid: testCID(t, 300), Err: "block not found"}}

	var first pinVerifyOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_verify", map[string]any{"limit": 2}), &first))
	validateOutput(t, "pin_verify", first)
	require.Len(t, first.Results, 2)
	assert.True(t, first.Page.HasMore)

	var second pinVerifyOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_verify", map[string]any{"limit": 2, "cursor": first.Page.NextCursor}), &second))
	validateOutput(t, "pin_verify", second)
	require.Len(t, second.Results, 2)
	assert.Equal(t, store.verifyLines[3].Cid, second.Results[1].CID)
	assert.False(t, second.Results[1].OK)
	require.Len(t, second.Results[1].BadNodes, 1)
	assert.Equal(t, "block not found", second.Results[1].BadNodes[0].Error)

	var third pinVerifyOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_verify", map[string]any{"limit": 2, "cursor": second.Page.NextCursor}), &third))
	assert.Len(t, third.Results, 1)
	assert.False(t, third.Page.HasMore)
	assert.Equal(t, 3, d.count("pin/verify"))
}

func TestPinVerifyOnlyFailures(t *testing.T) {
	cs, d, store := pinServer(t, nil, Options{})
	store.verifyLines = []pinVerifyLine{{Cid: testCID(t, 400), Ok: true}, {Cid: testCID(t, 401), Ok: false}}
	var out pinVerifyOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "pin_verify", map[string]any{"only_failures": true}), &out))
	require.Len(t, out.Results, 1)
	assert.Equal(t, testCID(t, 401), out.Results[0].CID)
	assert.Equal(t, false, d.log[len(d.log)-1].Opts["verbose"])
	validateOutput(t, "pin_verify", out)
}

func TestReadOnlyPinToolsNeverPrompt(t *testing.T) {
	d := newFakeDaemon(t)
	installPins(d, nil)
	s := mustNew(t, d, Options{})
	prompts := 0
	cs := connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		prompts++
		return &mcp.ElicitResult{Action: "decline"}
	})
	callOK(t, cs, "pin_ls", nil)
	callOK(t, cs, "pin_verify", nil)
	assert.Zero(t, prompts)
}

func TestNormalizePinType(t *testing.T) {
	assert.Equal(t, "indirect", normalizePinType("indirect through bafy"))
	assert.Equal(t, "direct", normalizePinType("direct"))
	assert.Equal(t, "recursive", normalizePinType("recursive"))
}
