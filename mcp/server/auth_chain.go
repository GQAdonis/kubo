package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// ChainVerifier tries the static bearer tokens first and, only when none
// matches and the token is shaped like a JWS, the JWT verifier. The static match
// is the same constant-time comparison as without a JWT verifier, so enabling
// auth.oidc never changes how a static token authenticates.
type ChainVerifier struct {
	static *StaticVerifier
	jwt    *JWTVerifier
}

// NewChainVerifier chains a static verifier (which may hold no credential) and
// a JWT verifier. A nil jwt makes the chain a plain static verifier.
func NewChainVerifier(static *StaticVerifier, jwt *JWTVerifier) *ChainVerifier {
	return &ChainVerifier{static: static, jwt: jwt}
}

// Verify implements AuthVerifier.
func (c *ChainVerifier) Verify(ctx context.Context, token string, req *http.Request) (Identity, time.Time, error) {
	id, exp, err := c.static.Verify(ctx, token, req)
	if err == nil || !errors.Is(err, auth.ErrInvalidToken) {
		return id, exp, err
	}
	if c.jwt == nil || !looksLikeJWS(token) {
		return Identity{}, time.Time{}, err
	}
	return c.jwt.Verify(ctx, token, req)
}

// ProtectedResourceMetadata implements AuthVerifier. Neither verifier has an
// authorization server to advertise.
func (c *ChainVerifier) ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata {
	return nil
}

// Identities lists the static identities, for the startup report.
func (c *ChainVerifier) Identities() []Identity { return c.static.Identities() }

// Warm prefetches the key set of the JWT verifier.
func (c *ChainVerifier) Warm(ctx context.Context) error {
	if c.jwt == nil {
		return nil
	}
	return c.jwt.Warm(ctx)
}

// JWKSURL is the key set address, userinfo removed.
func (c *ChainVerifier) JWKSURL() string {
	if c.jwt == nil {
		return ""
	}
	return c.jwt.JWKSURL()
}

// ClockSkew is the tolerance the bearer middleware applies to a verdict's
// expiry, so a token that is expired within auth.oidc.clock_skew is not
// refused by the middleware after the verifier accepted it.
func (c *ChainVerifier) ClockSkew() time.Duration {
	if c.jwt == nil {
		return 0
	}
	return c.jwt.cfg.ClockSkew
}

// adoptLogger gives the JWT verifier the listener's logger when it has none.
func (c *ChainVerifier) adoptLogger(l *slog.Logger) {
	if c.jwt != nil {
		c.jwt.adoptLogger(l)
		c.jwt.warnOpenRoutes()
	}
}
