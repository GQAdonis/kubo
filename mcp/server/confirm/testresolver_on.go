//go:build mcptestresolver

package confirm

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipfs/kubo/mcp/server"
)

// Test-only build (go build -tags mcptestresolver). When IPFS_MCP_TEST_RESOLVER_DIR
// is set, a resolver named "desktop" is added whose decisions come from files
// that the test writes: <dir>/<id>.approve or <dir>/<id>.deny. Each pending
// confirmation also gets <dir>/<id>.pending so the test learns the ID.
// IPFS_MCP_TEST_CONFIRM_TTL shortens the confirmation lifetime (a Go duration).
// None of this exists in a production build, and the test resolver is not
// reachable through --confirm.
const (
	envTestDir = "IPFS_MCP_TEST_RESOLVER_DIR"
	envTestTTL = "IPFS_MCP_TEST_CONFIRM_TTL"
)

func testResolvers(_ Config, log *slog.Logger) server.ResolverSet {
	var set server.ResolverSet
	if d, err := time.ParseDuration(os.Getenv(envTestTTL)); err == nil && d > 0 {
		set.ConfirmTTL = d
	}
	if dir := os.Getenv(envTestDir); dir != "" {
		set.Resolvers = []server.Resolver{&fileResolver{dir: dir, log: log}}
	}
	return set
}

// fileResolver stands in for the Desktop window in test/cli.
type fileResolver struct {
	dir string
	log *slog.Logger
}

func (r *fileResolver) Name() string { return server.ResolverDesktop }

func (r *fileResolver) Request(ctx context.Context, p server.Pending, resolve server.ResolveFunc) error {
	if err := os.WriteFile(filepath.Join(r.dir, p.ID+".pending"), []byte(p.Summary.Text), 0o600); err != nil {
		return err
	}
	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for suffix, approve := range map[string]bool{".approve": true, ".deny": false} {
					if _, err := os.Stat(filepath.Join(r.dir, p.ID+suffix)); err == nil {
						if err := resolve(approve); err != nil && !strings.Contains(err.Error(), "resolved") {
							r.log.Warn("test resolver", "error", err)
						}
						return
					}
				}
			}
		}
	}()
	return nil
}
