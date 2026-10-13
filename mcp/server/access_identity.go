package server

import "context"

type identityKey struct{}

// WithIdentity returns a context that carries the authenticated identity the
// HTTP layer resolved for the request, after policy was applied to it.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFromContext returns the identity of the calling HTTP session, or
// false for stdio and in-process callers, which have none.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}
