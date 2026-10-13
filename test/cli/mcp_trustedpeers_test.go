package cli

import (
	"encoding/json"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPTrustedPeers(t *testing.T) {
	t.Parallel()

	nodes := harness.NewT(t).NewNodes(3).Init("--profile=test")
	nodes.StartDaemons().Connect()
	t.Cleanup(func() { nodes.StopDaemons() })
	listed, other := nodes[1].PeerID().String(), nodes[2].PeerID().String()

	untrusted := func(t *testing.T, args ...string) (map[string]bool, bool) {
		t.Helper()
		cs := mcpClient(t, nodes[0], nil, args...)
		res := callMCP(t, cs, "swarm_peers", map[string]any{"limit": 10})
		requireMatchesOutputSchema(t, "swarm_peers", res)
		got := map[string]bool{}
		for _, p := range res.StructuredContent.(map[string]any)["peers"].([]any) {
			m := p.(map[string]any)
			u, ok := m["untrusted"].(bool)
			require.True(t, ok, "entry %v carries untrusted", m)
			// A peer can hold several connections; one untrusted one wins.
			got[m["id"].(string)] = got[m["id"].(string)] || u
		}
		return got, res.Meta["ai.ipfs/untrusted-content"] == true
	}

	t.Run("default config marks every peer untrusted and flags the result", func(t *testing.T) {
		got, flagged := untrusted(t)
		assert.True(t, got[listed])
		assert.True(t, got[other])
		assert.True(t, flagged)
	})

	t.Run("a listed peer is trusted, the others stay untrusted", func(t *testing.T) {
		cfg := principalConfig(t, `{"trusted_peers":["`+listed+`"]}`)
		got, flagged := untrusted(t, "--mcp-config", cfg)
		assert.False(t, got[listed])
		assert.True(t, got[other])
		assert.True(t, flagged, "one untrusted entry flags the result")
	})

	t.Run("ipfs_node_info is unchanged by the list", func(t *testing.T) {
		cfg := principalConfig(t, `{"trusted_peers":["`+listed+`"]}`)
		cs := mcpClient(t, nodes[0], nil, "--mcp-config", cfg)
		res := callMCP(t, cs, "ipfs_node_info", map[string]any{})
		require.False(t, res.IsError)
		raw, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), `"untrusted"`)
		assert.Nil(t, res.Meta["ai.ipfs/untrusted-content"])
	})

	t.Run("a non-peer-ID entry is refused at startup", func(t *testing.T) {
		cfg := principalConfig(t, `{"trusted_peers":["go-ipfs/0.4.23"]}`)
		res := nodes[0].RunIPFS("mcp", "--mcp-config", cfg)
		assert.NotEqual(t, 0, res.ExitCode())
		assert.Contains(t, res.Stderr.String(), "trusted_peers")
	})
}
