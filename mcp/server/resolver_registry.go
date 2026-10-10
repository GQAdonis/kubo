package server

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ResolverSet is what a ResolverProvider contributes to a new Server.
type ResolverSet struct {
	// Resolvers are added after Options.Resolvers.
	Resolvers []Resolver
	// ConfirmTTL, when positive, replaces the default confirmation lifetime.
	// Only test builds set it.
	ConfirmTTL time.Duration
}

// ResolverProvider supplies out-of-band human-presence resolvers when a
// Server is built. A provider that cannot offer a mechanism returns the
// resolvers it can: a missing mechanism must leave the confirmation
// unavailable, never approved.
type ResolverProvider func(ctx context.Context, log *slog.Logger) (ResolverSet, error)

var providers struct {
	mu   sync.Mutex
	list []ResolverProvider
}

// RegisterResolverProvider adds a provider that every later New consults. The
// native confirmation package registers itself this way, so the server stays
// free of platform code and a build without that package has no resolver.
func RegisterResolverProvider(p ResolverProvider) {
	providers.mu.Lock()
	defer providers.mu.Unlock()
	providers.list = append(providers.list, p)
}

// withProviders returns base plus every registered provider's resolvers and
// the confirmation lifetime override, if any.
func withProviders(ctx context.Context, log *slog.Logger, base []Resolver) ([]Resolver, time.Duration, error) {
	providers.mu.Lock()
	list := append([]ResolverProvider(nil), providers.list...)
	providers.mu.Unlock()
	out := append([]Resolver(nil), base...)
	var ttl time.Duration
	for _, p := range list {
		set, err := p(ctx, log)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, set.Resolvers...)
		if set.ConfirmTTL > 0 {
			ttl = set.ConfirmTTL
		}
	}
	return out, ttl, nil
}
