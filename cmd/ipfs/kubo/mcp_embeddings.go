//go:build !nomcp

package kubo

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	mcpserver "github.com/ipfs/kubo/mcp/server"
)

// validateEmbeddings checks the embeddings section and reads its credential
// references without starting or contacting anything, so a bad section fails
// before the daemon is contacted.
func validateEmbeddings(cfg *mcpserver.Config) error {
	if cfg == nil || cfg.Embeddings == nil {
		return nil
	}
	set, err := cfg.Embeddings.Settings()
	if err != nil {
		return err
	}
	if err := set.Validate(); err != nil {
		return err
	}
	return nil
}

// enableMCPEmbeddings opens the embeddings feature when the --mcp-config file
// has an embeddings section. With none it does nothing: no process is started,
// no store is opened and no embedder is contacted. It is independent of the
// agent code, so it exists in builds with the noagent tag too. The returned
// function stops the embedded store process and must run when ipfs mcp exits.
func enableMCPEmbeddings(ctx context.Context, srv *mcpserver.Server, s *mcpSettings, logger *slog.Logger) (func(), error) {
	if s.cfg == nil || s.cfg.Embeddings == nil {
		return func() {}, nil
	}
	if err := srv.EnableEmbeddings(ctx, s.cfg.Embeddings, logger); err != nil {
		return nil, fmt.Errorf("embeddings: %w", err)
	}
	fmt.Fprintln(os.Stderr, "ipfs mcp: embeddings enabled: "+srv.Embeddings().Describe())
	// The search routes exist only on the --http listener and only unless
	// embeddings.search_route.enabled is false.
	if s.cfg.Embeddings.SearchRouteEnabled() {
		if err := srv.EnableIndexRoutes(s.cfg.Agent.OriginAllowlist); err != nil {
			return nil, fmt.Errorf("embeddings: %w", err)
		}
	}
	return func() {
		if err := srv.CloseEmbeddings(); err != nil {
			fmt.Fprintf(os.Stderr, "ipfs mcp: stopping the embeddings store: %v\n", err)
		}
	}, nil
}
