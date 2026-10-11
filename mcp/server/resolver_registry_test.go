package server

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolverProvidersAreConsulted(t *testing.T) {
	providers.mu.Lock()
	saved := providers.list
	providers.list = nil
	providers.mu.Unlock()
	t.Cleanup(func() {
		providers.mu.Lock()
		providers.list = saved
		providers.mu.Unlock()
	})

	base := newFakeResolver(ResolverCLI)
	got, ttl, err := withProviders(t.Context(), slog.Default(), []Resolver{base})
	require.NoError(t, err)
	assert.Equal(t, []Resolver{base}, got)
	assert.Zero(t, ttl, "no provider, no change")

	extra := newFakeResolver(ResolverDesktop)
	RegisterResolverProvider(func(context.Context, *slog.Logger) (ResolverSet, error) {
		return ResolverSet{Resolvers: []Resolver{extra}, ConfirmTTL: 5 * time.Second}, nil
	})
	got, ttl, err = withProviders(t.Context(), slog.Default(), []Resolver{base})
	require.NoError(t, err)
	assert.Equal(t, []Resolver{base, extra}, got)
	assert.Equal(t, 5*time.Second, ttl)
}
