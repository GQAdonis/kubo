package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwarmPeersPaginates(t *testing.T) {
	var peers []any
	for i := range 7 {
		peers = append(peers, map[string]any{
			"Addr":      fmt.Sprintf("/ip4/192.0.2.%d/tcp/4001", i+1),
			"Peer":      "12D3KooW" + strings.Repeat(string(rune('a'+i)), 44),
			"Latency":   "12ms",
			"Direction": i % 3,
		})
	}
	d := newFakeDaemon(t)
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"swarm/peers": func(_ []string, opts map[string]any) (any, error) {
			return map[string]any{"Peers": peers}, nil
		},
	}
	cs := routingServer(t, d, Options{})

	seen := map[string]struct{}{}
	cursor, pages := "", 0
	for {
		args := map[string]any{"limit": 3}
		if cursor != "" {
			args["cursor"] = cursor
		}
		raw := callOK(t, cs, "swarm_peers", args)
		validateOutput(t, "swarm_peers", raw)
		var out swarmPeersOutput
		require.NoError(t, json.Unmarshal(raw, &out))
		require.LessOrEqual(t, len(out.Peers), 3)
		for _, p := range out.Peers {
			_, dup := seen[p.ID]
			assert.False(t, dup, "no peer is returned twice")
			seen[p.ID] = struct{}{}
			assert.Contains(t, []string{"inbound", "outbound", "unknown"}, p.Direction)
		}
		pages++
		if !out.Page.HasMore {
			assert.Empty(t, out.Page.NextCursor)
			break
		}
		cursor = out.Page.NextCursor
	}
	assert.Len(t, seen, 7)
	assert.Equal(t, 3, pages)
	opts := d.log[len(d.log)-1].Opts
	assert.Equal(t, true, opts["direction"])
	assert.Equal(t, true, opts["latency"])
}

func TestSwarmPeersEmptyAndBadCursor(t *testing.T) {
	d := newFakeDaemon(t)
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"swarm/peers": func([]string, map[string]any) (any, error) { return map[string]any{"Peers": nil}, nil },
	}
	cs := routingServer(t, d, Options{})
	raw := callOK(t, cs, "swarm_peers", nil)
	validateOutput(t, "swarm_peers", raw)
	var out swarmPeersOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Empty(t, out.Peers)
	assert.False(t, out.Page.HasMore)

	res := toolCall(t, cs, "swarm_peers", map[string]any{"cursor": "not-a-cursor!"})
	require.True(t, res.IsError)
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
}

func TestStatsBw(t *testing.T) {
	d := newFakeDaemon(t)
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"stats/bw": func(_ []string, opts map[string]any) (any, error) {
			return map[string]any{"TotalIn": 1500, "TotalOut": 2500.0, "RateIn": 12.5, "RateOut": 0}, nil
		},
	}
	cs := routingServer(t, d, Options{})

	raw := callOK(t, cs, "stats_bw", nil)
	validateOutput(t, "stats_bw", raw)
	var out statsBwOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, statsBwOutput{TotalIn: 1500, TotalOut: 2500, RateIn: 12.5}, out)
	assert.Equal(t, false, d.log[len(d.log)-1].Opts["poll"])

	callOK(t, cs, "stats_bw", map[string]any{"peer_id": testPeerA, "protocol": "/ipfs/bitswap/1.2.0"})
	opts := d.log[len(d.log)-1].Opts
	assert.Equal(t, testPeerA, opts["peer"])
	assert.Equal(t, "/ipfs/bitswap/1.2.0", opts["proto"])
}

func TestRepoStatOmitsLocalPaths(t *testing.T) {
	d := newFakeDaemon(t)
	d.handle = map[string]func([]string, map[string]any) (any, error){
		"repo/stat": func([]string, map[string]any) (any, error) {
			return map[string]any{
				"RepoSize": 4096, "StorageMax": 10000000000, "NumObjects": 12,
				"RepoPath": "/home/someone/.ipfs", "Version": "fs-repo@18",
			}, nil
		},
	}
	cs := routingServer(t, d, Options{})
	raw := callOK(t, cs, "repo_stat", nil)
	validateOutput(t, "repo_stat", raw)
	var out repoStatOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, repoStatOutput{RepoSize: 4096, StorageMax: 10000000000, NumObjects: 12, Version: "fs-repo@18"}, out)
	assert.NotContains(t, string(raw), "/home/someone")
	assert.NotContains(t, string(raw), "RepoPath")
}

// gcDaemon answers repo/gc with n removed blocks and the given errors.
func gcDaemon(t *testing.T, n int, errs ...string) (*fakeDaemon, *int) {
	t.Helper()
	d := newFakeDaemon(t)
	runs := new(int)
	d.stream = map[string]func([]string, map[string]any) ([]any, error){
		"repo/gc": func([]string, map[string]any) ([]any, error) {
			*runs++
			lines := make([]any, 0, n+len(errs))
			for i := range n {
				lines = append(lines, map[string]any{"Key": map[string]any{"/": testCID(t, 1000+i)}})
			}
			for _, e := range errs {
				lines = append(lines, map[string]any{"Error": e})
			}
			return lines, nil
		},
	}
	return d, runs
}

func TestRepoGcIsOffByDefault(t *testing.T) {
	d, _ := gcDaemon(t, 1)
	cs := routingServer(t, d, Options{})
	assert.NotContains(t, toolNames(t, cs), "repo_gc")
}

func TestRepoGcNeedsDestructiveConfirmation(t *testing.T) {
	d, runs := gcDaemon(t, 3)
	s := mustNew(t, d, Options{Tools: []string{"diag", "destructive"}})

	plain := connect(t, s)
	res := toolCall(t, plain, "repo_gc", nil)
	require.True(t, res.IsError)
	assert.Equal(t, CodeConfirmUnavailable, decodeToolError(t, res).Code)
	assert.Zero(t, *runs, "no garbage collection runs without a confirmation")

	declined := connectElicit(t, s, func(*mcp.ElicitRequest) *mcp.ElicitResult {
		return &mcp.ElicitResult{Action: "decline"}
	})
	res = toolCall(t, declined, "repo_gc", nil)
	assert.Equal(t, CodeConfirmDeclined, decodeToolError(t, res).Code)
	assert.Zero(t, *runs)

	var prompt string
	accepted := connectElicit(t, s, func(req *mcp.ElicitRequest) *mcp.ElicitResult {
		prompt = req.Params.Message
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	})
	raw := callOK(t, accepted, "repo_gc", nil)
	validateOutput(t, "repo_gc", raw)
	var out repoGcOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, 3, out.RemovedCount)
	assert.Len(t, out.RemovedSample, 3)
	assert.Equal(t, 1, *runs)
	assert.Contains(t, prompt, "garbage collection")
}

func TestRepoGcBoundsSampleAndErrors(t *testing.T) {
	errs := make([]string, 0, 150)
	for i := range 150 {
		errs = append(errs, fmt.Sprintf("failed to remove block %d", i))
	}
	d, _ := gcDaemon(t, 250, errs...)
	cs := routingServer(t, d, Options{Tools: []string{"diag", "destructive"}})
	raw := callOK(t, cs, "repo_gc", nil)
	validateOutput(t, "repo_gc", raw)
	var out repoGcOutput
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, 250, out.RemovedCount, "the count is exact")
	assert.Len(t, out.RemovedSample, gcSampleSize, "the sample is capped by the contract")
	assert.Len(t, out.Errors, gcMaxErrors)
	assert.Equal(t, true, d.log[len(d.log)-1].Opts["stream-errors"])
}

func TestReadOnlyModeHidesRoutingWritesAndGc(t *testing.T) {
	d := newFakeDaemon(t)
	cs := routingServer(t, d, Options{ReadOnly: true, Tools: []string{"routing", "diag", "destructive"}})
	assert.ElementsMatch(t, []string{"routing_findprovs", "routing_findpeer", "swarm_peers", "stats_bw", "repo_stat"}, toolNames(t, cs))
}
