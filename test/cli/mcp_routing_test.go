package cli

import (
	"testing"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpRoutingNode starts an isolated daemon with the test profile.
func mcpRoutingNode(t *testing.T, mutate func(*config.Config)) *harness.Node {
	t.Helper()
	node := harness.NewT(t).NewNode().Init("--profile=test")
	if mutate != nil {
		node.UpdateConfig(mutate)
	}
	node.StartDaemon()
	t.Cleanup(func() { node.StopDaemon() })
	return node
}

func hasLocalBlock(node *harness.Node, cid string) bool {
	return node.RunIPFS("block", "stat", "--offline", cid).ExitCode() == 0
}

func TestMCPRouting(t *testing.T) {
	t.Parallel()

	t.Run("routing_provide makes no announcement when Provide.Enabled=false", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, func(cfg *config.Config) { cfg.Provide.Enabled = config.False })
		cid := node.IPFSAddStr("do not announce me", "--cid-version=1")
		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options())

		res := callMCP(t, cs, "routing_provide", map[string]any{"cid": cid})
		requireMatchesOutputSchema(t, "routing_provide", res)
		sc := res.StructuredContent.(map[string]any)
		assert.Equal(t, true, sc["providing_disabled"])
		assert.Equal(t, false, sc["announced"])
		assert.Equal(t, cid, sc["cid"])
		require.Len(t, e.prompts, 1, "the call was still confirmed")
		assert.Contains(t, e.prompts[0], cid)
	})

	t.Run("routing_provide on an isolated node reports a result or a tool error", func(t *testing.T) {
		t.Parallel()
		// With providing enabled but no peers the daemon itself may refuse
		// ("no connected peers"); both outcomes are valid, and neither may
		// be a protocol failure.
		node := mcpRoutingNode(t, nil)
		cid := node.IPFSAddStr("announce me if you can", "--cid-version=1")
		cs := mcpClient(t, node, (&elicitor{action: "accept"}).options())

		res := callMCP(t, cs, "routing_provide", map[string]any{"cid": cid})
		if res.IsError {
			assert.NotEmpty(t, mcpToolErrorCode(t, res))
			return
		}
		requireMatchesOutputSchema(t, "routing_provide", res)
		assert.Equal(t, false, res.StructuredContent.(map[string]any)["providing_disabled"])
	})

	t.Run("routing_provide announces nothing without a confirmation", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		cid := node.IPFSAddStr("unconfirmed provide", "--cid-version=1")

		plain := mcpClient(t, node, nil)
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, callMCP(t, plain, "routing_provide", map[string]any{"cid": cid})))
		declining := mcpClient(t, node, (&elicitor{action: "decline"}).options())
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, callMCP(t, declining, "routing_provide", map[string]any{"cid": cid})))
	})

	t.Run("routing_findprovs and routing_findpeer tolerate an isolated node", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		cid := node.IPFSAddStr("who has me", "--cid-version=1")
		cs := mcpClient(t, node, nil)

		res := callMCP(t, cs, "routing_findprovs", map[string]any{"cid": cid, "num_providers": 1})
		if res.IsError {
			assert.NotEmpty(t, mcpToolErrorCode(t, res))
		} else {
			requireMatchesOutputSchema(t, "routing_findprovs", res)
		}

		peer := callMCP(t, cs, "routing_findpeer", map[string]any{"peer_id": "12D3KooWRBy97UB99e3J6hiPesre1MZeuNQvfan4gBziswrRJsNK"})
		if peer.IsError {
			assert.NotEmpty(t, mcpToolErrorCode(t, peer))
		} else {
			requireMatchesOutputSchema(t, "routing_findpeer", peer)
		}
	})

	t.Run("routing tools are read-only for findprovs and findpeer in read-only mode", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		cs := mcpClient(t, node, nil, "--read-only", "--tools", "routing,diag,destructive")
		assert.ElementsMatch(t, []string{"routing_findprovs", "routing_findpeer", "swarm_peers", "stats_bw", "repo_stat"}, mcpNames(t, cs))
	})
}

func TestMCPRoutingDiag(t *testing.T) {
	t.Parallel()

	t.Run("swarm_peers sees a connected peer", func(t *testing.T) {
		t.Parallel()
		nodes := harness.NewT(t).NewNodes(2).Init("--profile=test")
		nodes.StartDaemons().Connect()
		t.Cleanup(func() { nodes.StopDaemons() })
		cs := mcpClient(t, nodes[0], nil)

		res := callMCP(t, cs, "swarm_peers", map[string]any{"limit": 10})
		requireMatchesOutputSchema(t, "swarm_peers", res)
		var ids []string
		for _, p := range res.StructuredContent.(map[string]any)["peers"].([]any) {
			ids = append(ids, p.(map[string]any)["id"].(string))
		}
		assert.Contains(t, ids, nodes[1].PeerID().String())
	})

	t.Run("swarm_peers on an isolated node is empty", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		cs := mcpClient(t, node, nil)
		res := callMCP(t, cs, "swarm_peers", map[string]any{})
		requireMatchesOutputSchema(t, "swarm_peers", res)
		assert.Empty(t, res.StructuredContent.(map[string]any)["peers"])
	})

	t.Run("stats_bw and repo_stat match the schemas and leak no paths", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		node.IPFSAddStr("fill the repo", "--cid-version=1")
		cs := mcpClient(t, node, nil)

		bw := callMCP(t, cs, "stats_bw", map[string]any{})
		requireMatchesOutputSchema(t, "stats_bw", bw)

		repo := callMCP(t, cs, "repo_stat", map[string]any{})
		requireMatchesOutputSchema(t, "repo_stat", repo)
		sc := repo.StructuredContent.(map[string]any)
		assert.NotZero(t, sc["num_objects"])
		assert.NotContains(t, sc, "repo_path")
		text, ok := repo.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.NotContains(t, text.Text, node.Dir)
	})

	t.Run("repo_gc is off by default", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		cs := mcpClient(t, node, nil)
		assert.NotContains(t, mcpNames(t, cs), "repo_gc")
		assert.Contains(t, mcpNames(t, cs), "repo_stat")
	})

	t.Run("an unconfirmed repo_gc collects nothing", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		cid := node.IPFSAddStr("garbage, not pinned", "--cid-version=1", "--pin=false")
		require.True(t, hasLocalBlock(node, cid))

		plain := mcpClient(t, node, nil, "--tools", "diag,destructive")
		assert.Equal(t, "confirmation_unavailable", mcpToolErrorCode(t, callMCP(t, plain, "repo_gc", map[string]any{})))
		assert.True(t, hasLocalBlock(node, cid))

		declines := &elicitor{action: "decline"}
		declining := mcpClient(t, node, declines.options(), "--tools", "diag,destructive")
		assert.Equal(t, "confirmation_declined", mcpToolErrorCode(t, callMCP(t, declining, "repo_gc", map[string]any{})))
		require.Len(t, declines.prompts, 1)
		assert.True(t, hasLocalBlock(node, cid), "the block survived")
	})

	t.Run("a confirmed repo_gc removes unpinned blocks and keeps pins", func(t *testing.T) {
		t.Parallel()
		node := mcpRoutingNode(t, nil)
		garbage := node.IPFSAddStr("collect me", "--cid-version=1", "--pin=false")
		kept := node.IPFSAddStr("keep me", "--cid-version=1")
		require.True(t, hasLocalBlock(node, garbage))

		e := &elicitor{action: "accept"}
		cs := mcpClient(t, node, e.options(), "--tools", "diag,destructive")
		res := callMCP(t, cs, "repo_gc", map[string]any{})
		requireMatchesOutputSchema(t, "repo_gc", res)
		require.Len(t, e.prompts, 1)
		sc := res.StructuredContent.(map[string]any)
		assert.GreaterOrEqual(t, sc["removed_count"], float64(1))
		assert.Contains(t, sc["removed_sample"], garbage)
		assert.False(t, hasLocalBlock(node, garbage), "unpinned block collected")
		assert.True(t, hasLocalBlock(node, kept), "pinned block kept")
	})
}
