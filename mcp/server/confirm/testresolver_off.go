//go:build !mcptestresolver

package confirm

import (
	"log/slog"

	"github.com/ipfs/kubo/mcp/server"
)

// testResolvers adds nothing in a production build. The test-only resolver
// exists only under the mcptestresolver build tag.
func testResolvers(Config, *slog.Logger) server.ResolverSet { return server.ResolverSet{} }
