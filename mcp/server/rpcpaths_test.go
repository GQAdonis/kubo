package server

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pathsOf returns the RPC paths of the catalog tools with the given
// confirmation classes in the given groups, as AllowedPaths entries.
func pathsOf(t *testing.T, groups []string, classes ...string) []string {
	t.Helper()
	cat, err := contract.Load()
	require.NoError(t, err)
	var out []string
	for _, tool := range cat.Tools {
		if slices.Contains(groups, tool.Group) && slices.Contains(classes, tool.Confirmation) {
			for _, c := range tool.RPC {
				out = append(out, rpcPathPrefix+c)
			}
		}
	}
	return out
}

func TestRPCAllowedPathsReadOnlyListsOnlyReadCommands(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	paths, err := RPCAllowedPaths(Options{ReadOnly: true})
	require.NoError(t, err)

	// Every entry is a full /api/v0 path, sorted and unique.
	assert.True(t, slices.IsSorted(paths))
	assert.Equal(t, len(paths), len(slices.Compact(slices.Clone(paths))))
	for _, p := range paths {
		assert.True(t, strings.HasPrefix(p, rpcPathPrefix), p)
	}

	want := append(pathsOf(t, cat.ToolSelection.Default.Groups, contract.ClassNone), rpcPathPrefix+"version", rpcPathPrefix+"commands")
	for _, p := range want {
		assert.Contains(t, paths, p)
	}
	// No path that only a write or destructive tool needs.
	readOnly := make(map[string]struct{})
	for _, p := range want {
		readOnly[p] = struct{}{}
	}
	for _, p := range pathsOf(t, cat.ToolSelection.Default.Groups, contract.ClassWrite, contract.ClassDestructive) {
		if _, shared := readOnly[p]; !shared {
			assert.NotContains(t, paths, p, "read-only must not allow %s", p)
		}
	}
	assert.NotContains(t, paths, rpcPathPrefix+"pin/add")
	assert.NotContains(t, paths, rpcPathPrefix+"pin/rm")
	assert.Contains(t, paths, rpcPathPrefix+"pin/ls")
}

func TestRPCAllowedPathsFollowTheToolSelection(t *testing.T) {
	pinOnly, err := RPCAllowedPaths(Options{Tools: []string{"pin"}})
	require.NoError(t, err)
	assert.Contains(t, pinOnly, rpcPathPrefix+"pin/add", "write tools are in the default selection")
	assert.NotContains(t, pinOnly, rpcPathPrefix+"pin/rm", "destructive tools are off by default")
	assert.NotContains(t, pinOnly, rpcPathPrefix+"dag/get", "other groups are not")

	withDestructive, err := RPCAllowedPaths(Options{Tools: []string{"pin", "destructive"}})
	require.NoError(t, err)
	assert.Contains(t, withDestructive, rpcPathPrefix+"pin/rm")

	disabled, err := RPCAllowedPaths(Options{Tools: []string{"pin"}, Disabled: []string{"pin_add"}})
	require.NoError(t, err)
	assert.NotContains(t, disabled, rpcPathPrefix+"pin/add")

	_, err = RPCAllowedPaths(Options{Tools: []string{"nope"}})
	require.Error(t, err)
}

func TestRPCAuthorizationEntryIsJSONWithoutARealSecret(t *testing.T) {
	paths, err := RPCAllowedPaths(Options{ReadOnly: true})
	require.NoError(t, err)
	raw, err := RPCAuthorizationEntry(paths)
	require.NoError(t, err)
	var entry struct {
		AuthSecret   string
		AllowedPaths []string
	}
	require.NoError(t, json.Unmarshal(raw, &entry))
	assert.Equal(t, paths, entry.AllowedPaths)
	assert.Contains(t, entry.AuthSecret, "REPLACE")
}

func TestRPCAllowedPathsIncludeOptionalAndRequired(t *testing.T) {
	paths, err := RPCAllowedPaths(Options{})
	require.NoError(t, err)
	assert.Contains(t, paths, "/api/v0/provide/once", "routing_provide optional rpc")
	assert.Contains(t, paths, "/api/v0/dag/resolve", "dag_get required rpc")
	assert.NotContains(t, paths, "/api/v0/config", "no prefix that grants config writes")
}

func TestRPCAllowedPathsCoverIndexSearchAndDestructivePinChecks(t *testing.T) {
	index, err := RPCAllowedPaths(Options{Tools: []string{"index"}})
	require.NoError(t, err)
	assert.Contains(t, index, rpcPathPrefix+"cat", "ipfs_index_search reads hits back with cat")

	for group, want := range map[string][]string{
		"pin":  {"pin/rm", "pin/update", "pin/ls"},
		"diag": {"repo/gc", "pin/ls"},
	} {
		paths, err := RPCAllowedPaths(Options{Tools: []string{group, "destructive"}})
		require.NoError(t, err, group)
		for _, p := range want {
			assert.Contains(t, paths, rpcPathPrefix+p, group)
		}
	}
}
