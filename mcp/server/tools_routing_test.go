package server

import (
	"encoding/json"
	"fmt"
	"testing"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPeerA = "12D3KooWRBy97UB99e3J6hiPesre1MZeuNQvfan4gBziswrRJsNK"
	testPeerB = "12D3KooWSoCW4yFzoSp1fFFWZ1Sj4XTG1fBvLsfZjDcPZp2yjVzF"
)

func providerEvent(id string, addrs ...string) map[string]any {
	return map[string]any{
		"Type":      queryEventProvider,
		"Responses": []any{map[string]any{"ID": id, "Addrs": addrs}},
	}
}

// routingServer starts a server with the routing and diag groups over d; the
// client approves every confirmation.
func routingServer(t *testing.T, d *fakeDaemon, opts Options) *mcp.ClientSession {
	t.Helper()
	s := mustNew(t, d, opts)
	return connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
}

func TestRoutingFindProvs(t *testing.T) {
	c := testCID(t, 40)
	d := newFakeDaemon(t)
	d.stream = map[string]func([]string, map[string]any) ([]any, error){
		"routing/findprovs": func(args []string, opts map[string]any) ([]any, error) {
			return []any{
				map[string]any{"Type": 0, "ID": testPeerA},
				providerEvent(testPeerA, "/ip4/192.0.2.1/tcp/4001"),
				providerEvent(testPeerA, "/ip4/192.0.2.1/tcp/4001"), // duplicate
				map[string]any{"Type": queryEventQueryError, "Extra": "dial failed"},
				providerEvent(testPeerB),
			}, nil
		},
	}
	cs := routingServer(t, d, Options{})

	raw := callOK(t, cs, "routing_findprovs", map[string]any{"cid": c})
	validateOutput(t, "routing_findprovs", raw)
	var out routingFindProvsOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, c, out.CID)
	assert.Equal(t, []peerAddrs{
		{ID: testPeerA, Addrs: []string{"/ip4/192.0.2.1/tcp/4001"}},
		{ID: testPeerB, Addrs: []string{}},
	}, out.Providers, "events of other kinds and duplicates are dropped")
	assert.Equal(t, 20, d.log[len(d.log)-1].Opts["num-providers"], "the contract default")

	raw = callOK(t, cs, "routing_findprovs", map[string]any{"cid": c, "num_providers": 1})
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Len(t, out.Providers, 1, "reading stops at num_providers")
}

func TestRoutingFindProvsNoneFoundIsEmptyNotAnError(t *testing.T) {
	d := newFakeDaemon(t)
	d.stream = map[string]func([]string, map[string]any) ([]any, error){
		"routing/findprovs": func([]string, map[string]any) ([]any, error) { return nil, nil },
	}
	cs := routingServer(t, d, Options{})
	raw := callOK(t, cs, "routing_findprovs", map[string]any{"cid": testCID(t, 41)})
	validateOutput(t, "routing_findprovs", raw)
	var out routingFindProvsOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Empty(t, out.Providers)
}

func TestRoutingFindProvsOfflineIsAToolError(t *testing.T) {
	d := newFakeDaemon(t)
	d.stream = map[string]func([]string, map[string]any) ([]any, error){
		"routing/findprovs": func([]string, map[string]any) ([]any, error) {
			return nil, &RPCError{Command: "routing/findprovs", Message: "this command must be run in online mode"}
		},
	}
	cs := routingServer(t, d, Options{})
	res := toolCall(t, cs, "routing_findprovs", map[string]any{"cid": testCID(t, 42)})
	require.True(t, res.IsError)
	assert.Equal(t, CodeRPCError, decodeToolError(t, res).Code)
}

func TestRoutingFindPeer(t *testing.T) {
	d := newFakeDaemon(t)
	d.stream = map[string]func([]string, map[string]any) ([]any, error){
		"routing/findpeer": func(args []string, _ map[string]any) ([]any, error) {
			if args[0] != testPeerA {
				return []any{map[string]any{"Type": queryEventQueryError, "Extra": "routing: not found"}}, nil
			}
			return []any{
				map[string]any{"Type": 1, "ID": testPeerB},
				map[string]any{"Type": queryEventFinalPeer, "Responses": []any{map[string]any{"ID": testPeerA, "Addrs": []string{"/ip4/192.0.2.9/tcp/4001"}}}},
			}, nil
		},
	}
	cs := routingServer(t, d, Options{})

	raw := callOK(t, cs, "routing_findpeer", map[string]any{"peer_id": testPeerA})
	validateOutput(t, "routing_findpeer", raw)
	var out peerAddrs
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, peerAddrs{ID: testPeerA, Addrs: []string{"/ip4/192.0.2.9/tcp/4001"}}, out)

	res := toolCall(t, cs, "routing_findpeer", map[string]any{"peer_id": testPeerB})
	require.True(t, res.IsError)
	te := decodeToolError(t, res)
	assert.Equal(t, CodeNotFound, te.Code)
	assert.Contains(t, te.Message, "routing: not found")
}

// provideDaemon answers config and routing/provide.
func provideDaemon(t *testing.T, enabled any, configErr error) (*fakeDaemon, *int) {
	t.Helper()
	d := newFakeDaemon(t)
	provided := new(int)
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"config": func(args []string, _ map[string]any) (any, error) {
			if args[0] != "Provide.Enabled" {
				return nil, fmt.Errorf("fake: unexpected config key %s", args[0])
			}
			if configErr != nil {
				return nil, configErr
			}
			return map[string]any{"Key": "Provide.Enabled", "Value": enabled}, nil
		},
	}
	d.stream = map[string]func([]string, map[string]any) ([]any, error){
		"routing/provide": func([]string, map[string]any) ([]any, error) {
			*provided++
			return []any{map[string]any{"Type": 0}}, nil
		},
	}
	return d, provided
}

func TestRoutingProvideAnnounces(t *testing.T) {
	c := testCID(t, 43)
	for _, enabled := range []any{true, nil} {
		d, provided := provideDaemon(t, enabled, nil)
		cs := routingServer(t, d, Options{})
		raw := callOK(t, cs, "routing_provide", map[string]any{"cid": c, "recursive": true})
		validateOutput(t, "routing_provide", raw)
		var out routingProvideOutput
		require.NoError(t, json.Unmarshal(raw, &out))
		assert.Equal(t, routingProvideOutput{CID: c, Announced: true}, out, "Provide.Enabled=%v", enabled)
		assert.Equal(t, 1, *provided)
		last := d.log[len(d.log)-1]
		assert.Equal(t, []string{c}, last.Args)
		assert.Equal(t, true, last.Opts["recursive"])
	}
}

func TestRoutingProvideDisabledMakesNoAnnouncement(t *testing.T) {
	c := testCID(t, 44)
	d, provided := provideDaemon(t, false, nil)
	cs := routingServer(t, d, Options{})
	raw := callOK(t, cs, "routing_provide", map[string]any{"cid": c})
	validateOutput(t, "routing_provide", raw)
	var out routingProvideOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, routingProvideOutput{CID: c, Announced: false, ProvidingDisabled: true}, out)
	assert.Zero(t, *provided, "the daemon is never asked to announce")
	assert.Zero(t, d.count("routing/provide"))
}

func TestRoutingProvideFailsClosedWhenConfigIsUnreadable(t *testing.T) {
	d, provided := provideDaemon(t, true, &RPCError{Command: "config", Message: "access denied"})
	cs := routingServer(t, d, Options{})
	res := toolCall(t, cs, "routing_provide", map[string]any{"cid": testCID(t, 45)})
	require.True(t, res.IsError)
	assert.Zero(t, *provided, "an unknown setting never leads to an announcement")
}

func TestRoutingProvideWithoutConfigAuthorizationLeavesTheDecisionToTheDaemon(t *testing.T) {
	forbidden := &RPCError{Command: "config", Message: "not allowed", Code: cmds.ErrForbidden}

	d, provided := provideDaemon(t, true, forbidden)
	cs := routingServer(t, d, Options{})
	raw := callOK(t, cs, "routing_provide", map[string]any{"cid": testCID(t, 50)})
	var out routingProvideOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.True(t, out.Announced)
	assert.Equal(t, 1, *provided)

	// The daemon enforces Provide.Enabled itself and refuses before announcing.
	d, _ = provideDaemon(t, true, forbidden)
	d.stream["routing/provide"] = func([]string, map[string]any) ([]any, error) {
		return nil, &RPCError{Command: "routing/provide", Message: "invalid configuration: Provide.Enabled is set to 'false'"}
	}
	cs = routingServer(t, d, Options{})
	raw = callOK(t, cs, "routing_provide", map[string]any{"cid": testCID(t, 51)})
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.True(t, out.ProvidingDisabled)
	assert.False(t, out.Announced)
}

func TestRoutingProvideUnsetKeyIsTheDefault(t *testing.T) {
	d, provided := provideDaemon(t, true, &RPCError{Command: "config", Message: "key has no attribute 'Enabled'"})
	cs := routingServer(t, d, Options{})
	raw := callOK(t, cs, "routing_provide", map[string]any{"cid": testCID(t, 46)})
	var out routingProvideOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.True(t, out.Announced)
	assert.Equal(t, 1, *provided)
}

func TestRoutingProvideHonorsDaemonRefusal(t *testing.T) {
	d, _ := provideDaemon(t, true, nil)
	d.stream["routing/provide"] = func([]string, map[string]any) ([]any, error) {
		return nil, &RPCError{Command: "routing/provide", Message: "invalid configuration: Provide.Enabled is set to 'false'"}
	}
	cs := routingServer(t, d, Options{})
	raw := callOK(t, cs, "routing_provide", map[string]any{"cid": testCID(t, 47)})
	var out routingProvideOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.True(t, out.ProvidingDisabled)
	assert.False(t, out.Announced)
}

func TestRoutingProvideReportsQueryErrors(t *testing.T) {
	d, _ := provideDaemon(t, true, nil)
	d.stream["routing/provide"] = func([]string, map[string]any) ([]any, error) {
		return []any{map[string]any{"Type": queryEventQueryError, "Extra": "no peers"}}, nil
	}
	cs := routingServer(t, d, Options{})
	res := toolCall(t, cs, "routing_provide", map[string]any{"cid": testCID(t, 48)})
	require.True(t, res.IsError)
	assert.Contains(t, textOf(t, res), "no peers")
}

func TestRoutingProvideIsConfirmedAsAWrite(t *testing.T) {
	d, provided := provideDaemon(t, true, nil)
	s := mustNew(t, d, Options{})
	plain := connect(t, s)
	res := toolCall(t, plain, "routing_provide", map[string]any{"cid": testCID(t, 49)})
	require.True(t, res.IsError)
	assert.Equal(t, CodeConfirmUnavailable, decodeToolError(t, res).Code)
	assert.Zero(t, *provided, "nothing is announced without a confirmation")
	assert.Zero(t, d.count("config"))
}

func TestRoutingToolsAreOpenWorld(t *testing.T) {
	d := newFakeDaemon(t)
	cs := routingServer(t, d, Options{})
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	open := map[string]bool{}
	for _, tool := range res.Tools {
		open[tool.Name] = tool.Annotations.OpenWorldHint != nil && *tool.Annotations.OpenWorldHint
	}
	for _, n := range []string{"routing_findprovs", "routing_findpeer", "routing_provide"} {
		assert.True(t, open[n], n)
	}
	for _, n := range []string{"swarm_peers", "stats_bw", "repo_stat"} {
		assert.False(t, open[n], n)
	}
}
