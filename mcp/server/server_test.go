package server

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFailsWhenToolLacksContractEntry(t *testing.T) {
	d := newFakeDaemon(t)
	_, err := New(t.Context(), d, Options{extra: []toolImpl{stubTool("ipfs_not_in_contract")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tool "ipfs_not_in_contract" has no contract entry`)
	assert.Empty(t, d.calls, "the contract check runs before the daemon is contacted")
}

func TestNewFailsOnDuplicateRegistration(t *testing.T) {
	_, err := New(t.Context(), newFakeDaemon(t), Options{extra: []toolImpl{stubTool("ipfs_node_info")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered twice")
}

func TestDefaultToolsList(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{Version: "test"})
	cs := connect(t, s)

	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	cat, err := contract.Load()
	require.NoError(t, err)
	got := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	assert.ElementsMatch(t, slices.DeleteFunc(cat.DefaultToolSet(), func(n string) bool { return !slices.Contains(implementedTools(t), n) }), got, "the default set is derived from the contract")
	for _, name := range got {
		def, ok := cat.Tool(name)
		require.True(t, ok, name)
		assert.NotEqual(t, contract.ClassDestructive, def.Confirmation, "%s is destructive and off by default", name)
	}
	for _, tool := range res.Tools {
		want, ok := cat.Tool(tool.Name)
		require.True(t, ok, tool.Name)
		assert.Equal(t, want.Description, tool.Description)
		require.NotNil(t, tool.Annotations)
		assert.Equal(t, want.Annotations.ReadOnlyHint, tool.Annotations.ReadOnlyHint)
		require.NotNil(t, tool.Annotations.DestructiveHint)
		assert.Equal(t, want.Annotations.DestructiveHint, *tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.OutputSchema)
	}
}

func TestOldDaemonRefused(t *testing.T) {
	d := newFakeDaemon(t)
	d.version = "0.28.9"
	_, err := New(t.Context(), d, Options{})
	require.Error(t, err)
	var verErr *VersionError
	require.True(t, errors.As(err, &verErr))
	assert.Equal(t, "0.28.9", verErr.Found)
	assert.Equal(t, "0.29.0", verErr.Minimum)
	assert.Contains(t, err.Error(), testAPI)
	assert.Contains(t, err.Error(), "older than the minimum")
}

func TestVersionGateBoundaries(t *testing.T) {
	for _, tc := range []struct {
		version string
		wantOK  bool
	}{
		{"0.29.0", true},
		{"0.29.0-dev", true},
		{"v0.30.1", true},
		{"0.44.0-rc1", true},
		{"0.28.99", false},
		{"0.9.0", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			d := newFakeDaemon(t)
			d.version = tc.version
			_, err := New(t.Context(), d, Options{})
			if tc.wantOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestUnreachableDaemonNamesEndpoint(t *testing.T) {
	d := newFakeDaemon(t)
	d.callErr = map[string]error{"version": ErrDaemonUnreachable}
	_, err := New(t.Context(), d, Options{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonUnreachable)
	assert.Contains(t, err.Error(), testAPI)
}

func TestMissingCommandDegradesOneTool(t *testing.T) {
	d := newFakeDaemon(t).without("id")
	s := mustNew(t, d, Options{})
	cs := connect(t, s)

	assert.NotContains(t, toolNames(t, cs), "ipfs_node_info")
	assert.Contains(t, toolNames(t, cs), "ipfs_node_status")
	assert.Equal(t, []Unavailable{{Name: "ipfs_node_info", Reason: ReasonMissingRPC, MissingRPC: []string{"id"}}}, missingRPCOnly(s.UnavailableTools()))

	status := callOK(t, cs, "ipfs_node_status", nil)
	var out nodeStatusOutput
	require.NoError(t, json.Unmarshal(status, &out))
	assert.True(t, out.DaemonReachable)
	assert.True(t, out.VersionSupported)
	assert.Equal(t, []Unavailable{{Name: "ipfs_node_info", Reason: ReasonMissingRPC, MissingRPC: []string{"id"}}}, missingRPCOnly(out.Unavailable))
}

// missingRPCOnly keeps the entries that report a missing RPC command.
func missingRPCOnly(in []Unavailable) []Unavailable {
	var out []Unavailable
	for _, u := range in {
		if u.Reason == ReasonMissingRPC {
			out = append(out, u)
		}
	}
	return out
}

func TestCommandListingDeniedKeepsTools(t *testing.T) {
	d := newFakeDaemon(t)
	d.callErr = map[string]error{"commands": &RPCError{Command: "commands", Message: "forbidden"}}
	s := mustNew(t, d, Options{Tools: []string{"node"}})
	assert.Equal(t, []string{"ipfs_node_info", "ipfs_node_status"}, s.ToolNames())
}

func TestToolTooNewForDaemon(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	info := &DaemonInfo{Version: daemonVersion{Version: "0.29.0"}}
	tool, _ := cat.Tool("ipfs_node_info")
	tool.MinKuboVersion = "0.40.0"
	u := info.checkTool(tool)
	require.NotNil(t, u)
	assert.Equal(t, ReasonDaemonTooOld, u.Reason)
}

func TestSelectionDisabledGroupAndReadOnly(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	defaults := cat.ToolSelection.Default.Groups
	require.GreaterOrEqual(t, len(defaults), 2, "the semantics need at least two default groups")
	destructive := cat.ToolSelection.DestructiveSelector
	disabledGroup := defaults[len(defaults)-1]
	withoutDisabled := slices.DeleteFunc(slices.Clone(defaults), func(g string) bool { return g == disabledGroup })

	implemented := implementedTools(t)

	// unavailableFor derives the reason each catalog tool outside want is
	// withheld: a disabled group beats read-only, which beats destructive.
	unavailableFor := func(selectors []string, readOnly bool, want []string) map[string]string {
		groups := map[string]struct{}{}
		for _, sel := range selectors {
			groups[sel] = struct{}{}
		}
		out := map[string]string{}
		for _, tool := range cat.Tools {
			if slices.Contains(want, tool.Name) || !slices.Contains(implemented, tool.Name) {
				continue
			}
			switch _, on := groups[tool.Group]; {
			case !on:
				out[tool.Name] = ReasonGroupDisabled
			case readOnly:
				out[tool.Name] = ReasonReadOnly
			default:
				out[tool.Name] = ReasonDestructiveDisabled
			}
		}
		return out
	}
	cases := []struct {
		name      string
		opts      Options
		selectors []string // effective group selectors, destructive included
		readOnly  bool
	}{
		{name: "default offers writes but not destructive", selectors: defaults},
		{name: "read-only hides write and destructive", opts: Options{ReadOnly: true}, selectors: defaults, readOnly: true},
		{name: "disabled group is absent", opts: Options{Tools: withoutDisabled}, selectors: withoutDisabled},
		{name: "destructive selector enables destructive tools", opts: Options{Tools: append(slices.Clone(defaults), destructive)}, selectors: append(slices.Clone(defaults), destructive)},
		{name: "destructive alone adds to the defaults", opts: Options{Tools: []string{destructive}}, selectors: append(slices.Clone(defaults), destructive)},
		{name: "group_disabled wins over destructive_disabled", opts: Options{Tools: defaults[:1]}, selectors: defaults[:1]},
		{name: "read-only wins over destructive", opts: Options{Tools: append(slices.Clone(defaults), destructive), ReadOnly: true}, selectors: append(slices.Clone(defaults), destructive), readOnly: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := slices.DeleteFunc(cat.SelectTools(tc.selectors, tc.readOnly), func(n string) bool { return !slices.Contains(implemented, n) })
			s := mustNew(t, newFakeDaemon(t), tc.opts)
			cs := connect(t, s)
			assert.ElementsMatch(t, want, toolNames(t, cs))

			got := map[string]string{}
			for _, u := range s.UnavailableTools() {
				got[u.Name] = u.Reason
			}
			wantReasons := unavailableFor(tc.selectors, tc.readOnly, want)
			for name, reason := range wantReasons {
				assert.Equal(t, reason, got[name], name)
			}
			for _, name := range want {
				assert.NotContains(t, got, name, "an offered tool is not listed unavailable")
			}
			if tc.readOnly {
				for _, name := range toolNames(t, cs) {
					def, _ := cat.Tool(name)
					assert.Equal(t, contract.ClassNone, def.Confirmation, "%s changes state but read-only is on", name)
				}
			}
		})
	}
}

func TestSelectionRejectsUnknownGroup(t *testing.T) {
	_, err := New(t.Context(), newFakeDaemon(t), Options{Tools: []string{"node", "bogus"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown tool group "bogus"`)
}

func TestStatusReportsSelection(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{Tools: []string{"node", "destructive"}, ReadOnly: true})
	cs := connect(t, s)
	var out nodeStatusOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "ipfs_node_status", nil), &out))
	assert.True(t, out.ReadOnly)
	assert.Equal(t, []string{"node", "destructive"}, out.EnabledGroups)
	assert.Equal(t, "stdio", out.Transport)
	assert.Equal(t, "0.29.0", out.MinKuboVersion)
}

func TestStatusReportsUnreachableDaemon(t *testing.T) {
	d := newFakeDaemon(t)
	s := mustNew(t, d, Options{})
	cs := connect(t, s)
	d.callErr = map[string]error{"version": ErrDaemonUnreachable}

	var out nodeStatusOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "ipfs_node_status", nil), &out))
	assert.False(t, out.DaemonReachable)
	assert.False(t, out.VersionSupported)
	validateOutput(t, "ipfs_node_status", out)
}

func TestNodeInfoOutputMatchesSchema(t *testing.T) {
	cs := connect(t, mustNew(t, newFakeDaemon(t), Options{}))
	raw := callOK(t, cs, "ipfs_node_info", nil)
	var out nodeInfoOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, testPeerID, out.PeerID)
	assert.Equal(t, "0.44.0-dev", out.Version)
	validateOutput(t, "ipfs_node_info", out)
}

func TestNodeStatusOutputMatchesSchema(t *testing.T) {
	d := newFakeDaemon(t).without("id")
	cs := connect(t, mustNew(t, d, Options{Tools: []string{"node", "destructive"}}))
	var generic any
	require.NoError(t, json.Unmarshal(callOK(t, cs, "ipfs_node_status", nil), &generic))
	validateOutput(t, "ipfs_node_status", generic)
}

func TestCallResultHasStructuredAndTextContent(t *testing.T) {
	cs := connect(t, mustNew(t, newFakeDaemon(t), Options{}))
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.NotNil(t, res.StructuredContent)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	structured, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, text.Text, string(structured))
}

func TestInvalidArgumentsAreToolErrors(t *testing.T) {
	cs := connect(t, mustNew(t, newFakeDaemon(t), Options{}))
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info", Arguments: map[string]any{"unexpected": 1}})
	require.NoError(t, err, "tool failures are results, not protocol errors")
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeInvalidArgument, te.Code)
	assert.Equal(t, "ipfs_node_info", te.Tool)
	validateAgainstCommon(t, "ToolError", res.StructuredContent)
}

func TestDaemonFailureIsToolError(t *testing.T) {
	d := newFakeDaemon(t)
	cs := connect(t, mustNew(t, d, Options{}))
	d.callErr = map[string]error{"id": ErrDaemonUnreachable}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info"})
	require.NoError(t, err)
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeDaemonUnreachable, te.Code)
	assert.True(t, te.Retryable)
	validateAgainstCommon(t, "ToolError", res.StructuredContent)
}

func TestRPCErrorMapping(t *testing.T) {
	d := newFakeDaemon(t)
	cs := connect(t, mustNew(t, d, Options{}))
	d.callErr = map[string]error{"id": &RPCError{Command: "id", Message: "something broke"}}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "ipfs_node_info"})
	require.NoError(t, err)
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeRPCError, te.Code)
	assert.Equal(t, "id", te.Details["rpc_command"])
}

// implementedTools lists the tools a fully enabled server registers. The
// catalog may define tools this build does not implement yet.
func implementedTools(t *testing.T) []string {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	selectors := make([]string, 0, len(cat.Groups)+1)
	for _, g := range cat.Groups {
		selectors = append(selectors, g.Name)
	}
	selectors = append(selectors, cat.ToolSelection.DestructiveSelector)
	return toolNames(t, connect(t, mustNew(t, newFakeDaemon(t), Options{Tools: selectors})))
}
