package confirm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ipfs/kubo/mcp/server"
)

// Manager owns the resolvers built from a Config and the channel they share.
type Manager struct {
	resolvers []server.Resolver
	ch        *channel
}

// Resolvers returns the resolvers to put in server.Options.
func (m *Manager) Resolvers() []server.Resolver { return m.resolvers }

// Close stops the channel and removes its socket.
func (m *Manager) Close() {
	if m.ch != nil {
		m.ch.close()
	}
}

// Build creates the resolvers the config names. A mechanism that cannot be
// offered here is left out and logged, which leaves a non-eliciting client
// with confirmation_unavailable; it is never replaced by a weaker one. Only an
// unusable config (a socket that cannot be made private) is an error.
func Build(cfg Config, log *slog.Logger) (*Manager, error) {
	m := &Manager{}
	if cfg.Has(ModeNative) {
		m.resolvers = append(m.resolvers, newNativeResolver(platformFactor(), log))
	}
	wantDesktop, wantCLI := cfg.Has(ModeDesktop), cfg.Has(ModeCLI)
	if !wantDesktop && !wantCLI {
		return m, nil
	}
	var verifier []byte
	if wantDesktop {
		v, err := loadVerifier(cfg.pairingFile())
		switch {
		case err == nil:
			verifier = v
		case errors.Is(err, ErrNotPaired):
			log.Warn("desktop confirmation requested but not paired; it will refuse", "pairing_file", cfg.pairingFile())
			wantDesktop = false
		default:
			return nil, fmt.Errorf("desktop confirmation: %w", err)
		}
	}
	if !wantDesktop && !wantCLI {
		return m, nil
	}
	ch, err := newChannel(log, cfg.socketDir(), verifier, wantCLI)
	if err != nil {
		return nil, err
	}
	m.ch = ch
	if wantDesktop {
		m.resolvers = append(m.resolvers, &channelResolver{resolverName: server.ResolverDesktop, ch: ch})
	}
	if wantCLI {
		m.resolvers = append(m.resolvers, &channelResolver{resolverName: server.ResolverCLI, ch: ch})
	}
	log.Info("confirmation channel listening", "socket", ch.sockPath)
	return m, nil
}

var (
	activeMu sync.Mutex
	active   []*Manager
)

func init() {
	server.RegisterResolverProvider(provide)
}

// provide is the server.ResolverProvider of this package.
func provide(_ context.Context, log *slog.Logger) (server.ResolverSet, error) {
	cfg, err := configured()
	if err != nil {
		return server.ResolverSet{}, err
	}
	extra := testResolvers(cfg, log)
	if len(cfg.Modes) == 0 && len(extra.Resolvers) == 0 {
		return extra, nil
	}
	m, err := Build(cfg, log)
	if err != nil {
		return server.ResolverSet{}, err
	}
	activeMu.Lock()
	active = append(active, m)
	activeMu.Unlock()
	extra.Resolvers = append(m.Resolvers(), extra.Resolvers...)
	return extra, nil
}

// Shutdown closes every channel opened by the provider. The command that
// served calls it on exit.
func Shutdown() {
	activeMu.Lock()
	ms := active
	active = nil
	activeMu.Unlock()
	for _, m := range ms {
		m.Close()
	}
}
