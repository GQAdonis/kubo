package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func providersDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	d := newFakeDaemon(t)
	d.stream = map[string]func([]string, map[string]any) ([]any, error){
		"routing/findprovs": func([]string, map[string]any) ([]any, error) {
			return []any{
				providerEvent(testPeerA, "/ip4/192.0.2.1/tcp/4001"),
				providerEvent(testPeerB, "/ip4/192.0.2.2/tcp/4001"),
			}, nil
		},
		"routing/findpeer": func([]string, map[string]any) ([]any, error) {
			return []any{map[string]any{"Type": queryEventFinalPeer, "Responses": []any{
				map[string]any{"ID": testPeerA, "Addrs": []string{"/ip4/192.0.2.9/tcp/4001"}}}}}, nil
		},
	}
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"swarm/peers": func([]string, map[string]any) (any, error) {
			return map[string]any{"Peers": []any{
				map[string]any{"Addr": "/ip4/192.0.2.1/tcp/4001", "Peer": testPeerA, "Direction": 1},
				map[string]any{"Addr": "/ip4/192.0.2.2/tcp/4001", "Peer": testPeerB, "Direction": 2},
			}}, nil
		},
	}
	return d
}

func untrustedOf(t *testing.T, raw json.RawMessage, key string) map[string]bool {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	var entries []any
	if key == "" {
		entries = []any{doc}
	} else {
		entries, _ = doc[key].([]any)
	}
	got := map[string]bool{}
	for _, e := range entries {
		m := e.(map[string]any)
		u, ok := m["untrusted"].(bool)
		require.True(t, ok, "entry %v has a boolean untrusted", m)
		got[m["id"].(string)] = u
	}
	return got
}

func TestTrustedPeersMarkEntries(t *testing.T) {
	cases := []struct {
		name    string
		trusted []string
		want    map[string]bool
		flagged bool
	}{
		{"empty list", nil, map[string]bool{testPeerA: true, testPeerB: true}, true},
		{"mixed", []string{testPeerA}, map[string]bool{testPeerA: false, testPeerB: true}, true},
		{"all trusted", []string{testPeerA, testPeerB}, map[string]bool{testPeerA: false, testPeerB: false}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := routingServer(t, providersDaemon(t), Options{TrustedPeers: tc.trusted})

			res := toolCall(t, cs, "routing_findprovs", map[string]any{"cid": testCID(t, 40)})
			require.False(t, res.IsError)
			raw, err := json.Marshal(res.StructuredContent)
			require.NoError(t, err)
			validateOutput(t, "routing_findprovs", json.RawMessage(raw))
			assert.Equal(t, tc.want, untrustedOf(t, raw, "providers"))
			assert.Equal(t, tc.flagged, res.Meta[untrustedMetaKey] == true)

			res = toolCall(t, cs, "swarm_peers", map[string]any{})
			require.False(t, res.IsError)
			raw, err = json.Marshal(res.StructuredContent)
			require.NoError(t, err)
			validateOutput(t, "swarm_peers", json.RawMessage(raw))
			assert.Equal(t, tc.want, untrustedOf(t, raw, "peers"))
			assert.Equal(t, tc.flagged, res.Meta[untrustedMetaKey] == true)

			res = toolCall(t, cs, "routing_findpeer", map[string]any{"peer_id": testPeerA})
			require.False(t, res.IsError)
			raw, err = json.Marshal(res.StructuredContent)
			require.NoError(t, err)
			validateOutput(t, "routing_findpeer", json.RawMessage(raw))
			wantA := tc.want[testPeerA]
			assert.Equal(t, map[string]bool{testPeerA: wantA}, untrustedOf(t, raw, ""))
			assert.Equal(t, wantA, res.Meta[untrustedMetaKey] == true)
		})
	}
}

func TestNodeInfoIsNotPeerSupplied(t *testing.T) {
	d := newFakeDaemon(t)
	cs := routingServer(t, d, Options{TrustedPeers: []string{testPeerA}})
	res := toolCall(t, cs, "ipfs_node_info", map[string]any{})
	if res.IsError {
		t.Skip("ipfs_node_info needs daemon handlers this fake does not provide")
	}
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"untrusted"`)
	assert.Nil(t, res.Meta[untrustedMetaKey])
}

func TestTrustedPeersValidation(t *testing.T) {
	_, err := ParseConfig([]byte(`{"trusted_peers":["` + testPeerA + `"]}`))
	require.NoError(t, err)

	_, err = ParseConfig([]byte(`{"trusted_peers":["go-ipfs/0.4.23"]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trusted_peers")

	// Passes the schema's length bound but is not a peer ID.
	_, err = ParseConfig([]byte(`{"trusted_peers":["` + "QmNotAPeerIDQmNotAPeerIDQmNotAPeerID" + `"]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trusted_peers")

	_, err = New(t.Context(), newFakeDaemon(t), Options{TrustedPeers: []string{"nope"}})
	require.Error(t, err)
}
