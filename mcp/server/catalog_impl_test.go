package server

import (
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryCatalogToolHasAnImplementation fails when the contract defines a
// tool no handler implements, or a handler has no contract entry. The index
// tools wait for the embeddings feature; they count as implemented because
// they are offered the moment embeddings are configured.
func TestEveryCatalogToolHasAnImplementation(t *testing.T) {
	cat, err := contract.Load()
	require.NoError(t, err)
	selectors := make([]string, 0, len(cat.Groups)+1)
	for _, g := range cat.Groups {
		selectors = append(selectors, g.Name)
	}
	selectors = append(selectors, cat.ToolSelection.DestructiveSelector)
	s := mustNew(t, newFakeDaemon(t), Options{Tools: selectors})

	implemented := map[string]struct{}{}
	for _, name := range s.registered {
		implemented[name] = struct{}{}
	}
	for _, impl := range s.indexImpls {
		implemented[impl.Name] = struct{}{}
	}

	for _, tool := range cat.Tools {
		assert.Contains(t, implemented, tool.Name, "catalog tool %s has no implementation", tool.Name)
	}
	names := make([]string, 0, len(implemented))
	for name := range implemented {
		names = append(names, name)
	}
	assert.NoError(t, cat.CheckToolNames(names), "every implemented tool has a contract entry")
	assert.Len(t, implemented, len(cat.Tools), "the implemented set and the catalog are the same size")
}
