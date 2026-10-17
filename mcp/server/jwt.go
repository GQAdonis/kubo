package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	// go-jose is pinned to the version already in the module graph (v4.1.4,
	// which p2p-forge v0.10.1 requires) until p2p-forge ships v4.1.6; do not
	// bump it ahead of that release (AGENTS.md, transitive dependency bumps).
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// maxJWTBytes bounds the token the verifier parses. A real ID token is about a
// kilobyte; the limit keeps unauthenticated input from costing more than that.
const maxJWTBytes = 16 << 10

// Reasons a token was refused. They are logged at debug level for the operator
// and never returned to the caller, who only learns the request was 401.
const (
	jwtRejectMalformed = "malformed"
	jwtRejectHeader    = "kid_or_alg"
	jwtRejectKey       = "unknown_key"
	jwtRejectKeysDown  = "keys_unavailable"
	jwtRejectSignature = "signature"
	jwtRejectClaims    = "claims"
	jwtRejectTime      = "time"
	jwtRejectRoute     = "route"
)

// errTokenInvalid is what every refusal returns. It says nothing about which
// check failed.
var errTokenInvalid = fmt.Errorf("%w: token not accepted", auth.ErrInvalidToken)

// VerifiedToken is what a token that passed validation says. It never holds the
// token itself.
type VerifiedToken struct {
	// Subject is the `sub` claim: the immutable identity ID.
	Subject string
	// Email is the `email` claim, for display only. It selects nothing.
	Email string
	// AMR is the `amr` claim: the authentication methods of the session.
	AMR []string
	// AuthTime is the `auth_time` claim, or the zero time when absent.
	AuthTime time.Time
	// Expiry is the `exp` claim.
	Expiry time.Time
}

// idTokenClaims is the claim set the verifier reads.
type idTokenClaims struct {
	jwt.Claims
	Email    string           `json:"email"`
	AMR      []string         `json:"amr"`
	AuthTime *jwt.NumericDate `json:"auth_time"`
}

// JWTOptions configures NewJWTVerifier.
type JWTOptions struct {
	// OIDC is the resolved auth.oidc section (Config.OIDCSettings).
	OIDC OIDCSettings
	// Client fetches the key set; nil is a client with a 10 s timeout that
	// follows no redirect.
	Client *http.Client
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Logger receives diagnostics; nil discards them. It never receives a
	// token.
	Logger *slog.Logger
	// Principals are the configured principals a subject can resolve to.
	Principals []SessionPrincipal
	// OriginAllowlist is agent.origin_allowlist: the page origins a session
	// identity may send a non-safe request from.
	OriginAllowlist []string
}

// JWTVerifier is an AuthVerifier for OpenID Connect ID tokens (ADR-0013
// section 4). It checks the signature against the issuer's key set, then the
// claims; it holds no other trust anchor and reads no header besides the token.
type JWTVerifier struct {
	cfg      OIDCSettings
	keys     *jwksCache
	now      func() time.Time
	log      *slog.Logger
	subjects map[string][]SessionPrincipal
	origins  []string
}

// NewJWTVerifier builds the verifier. It fetches nothing: the key set is read
// on the first token that needs it, or by Warm.
func NewJWTVerifier(o JWTOptions) (*JWTVerifier, error) {
	if o.OIDC.Issuer == "" || len(o.OIDC.Audiences) == 0 || o.OIDC.JWKSURL == "" || len(o.OIDC.Algorithms) == 0 {
		return nil, errors.New("jwt verifier: issuer, audiences, jwks_url and algorithms are required")
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	subjects, err := indexSessionPrincipals(o.Principals)
	if err != nil {
		return nil, err
	}
	origins, err := normalizeOrigins(o.OriginAllowlist)
	if err != nil {
		return nil, fmt.Errorf("agent.origin_allowlist: %w", err)
	}
	return &JWTVerifier{
		origins:  origins,
		cfg:      o.OIDC,
		keys:     newJWKSCache(o.OIDC.JWKSURL, o.OIDC.JWKSCacheTTL, o.Client, now),
		now:      now,
		log:      log,
		subjects: subjects,
	}, nil
}

// Warm fetches the key set once. An error names the URL (userinfo removed) and
// is the caller's to log; it does not affect later requests.
func (v *JWTVerifier) Warm(ctx context.Context) error { return v.keys.warm(ctx) }

// JWKSURL is the configured key set address, userinfo removed, for messages.
func (v *JWTVerifier) JWKSURL() string { return redactURL(v.cfg.JWKSURL) }

// looksLikeJWS reports whether s is shaped like a compact JWS: three non-empty
// base64url segments. It lets the chain keep opaque static tokens away from the
// JWT path.
func looksLikeJWS(s string) bool {
	if len(s) > maxJWTBytes || strings.Count(s, ".") != 2 {
		return false
	}
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return false
		}
		for i := range len(seg) {
			c := seg[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}

// validate checks the signature and the claims of token.
func (v *JWTVerifier) validate(ctx context.Context, token string) (VerifiedToken, error) {
	if !looksLikeJWS(token) {
		return v.reject(jwtRejectMalformed)
	}
	// ParseSigned accepts only the configured algorithms, so none and the
	// HMAC family fail here, before any key is looked at.
	parsed, err := jwt.ParseSigned(token, v.cfg.Algorithms)
	if err != nil || len(parsed.Headers) != 1 {
		return v.reject(jwtRejectMalformed)
	}
	hdr := parsed.Headers[0]
	if hdr.KeyID == "" || !slices.Contains(v.cfg.Algorithms, jose.SignatureAlgorithm(hdr.Algorithm)) {
		return v.reject(jwtRejectHeader)
	}
	key, err := v.keys.key(ctx, hdr.KeyID)
	switch {
	case errors.Is(err, errNoSuchKey):
		return v.reject(jwtRejectKey)
	case err != nil:
		v.log.Warn("jwt keys unavailable", "error", err.Error())
		return v.reject(jwtRejectKeysDown)
	}
	// A key that names an algorithm signs only with it.
	if key.Algorithm != "" && key.Algorithm != hdr.Algorithm {
		return v.reject(jwtRejectHeader)
	}
	var claims idTokenClaims
	if err := parsed.Claims(key.Key, &claims); err != nil {
		return v.reject(jwtRejectSignature)
	}
	if claims.Subject == "" || claims.Expiry == nil {
		return v.reject(jwtRejectClaims)
	}
	err = claims.ValidateWithLeeway(jwt.Expected{
		Issuer:      v.cfg.Issuer,
		AnyAudience: v.cfg.Audiences,
		Time:        v.now(),
	}, v.cfg.ClockSkew)
	switch {
	case err == nil:
	case errors.Is(err, jwt.ErrExpired), errors.Is(err, jwt.ErrNotValidYet), errors.Is(err, jwt.ErrIssuedInTheFuture):
		return v.reject(jwtRejectTime)
	default:
		return v.reject(jwtRejectClaims)
	}
	out := VerifiedToken{
		Subject: claims.Subject,
		Email:   claims.Email,
		AMR:     slices.Clone(claims.AMR),
		Expiry:  claims.Expiry.Time(),
	}
	if claims.AuthTime != nil {
		out.AuthTime = claims.AuthTime.Time()
	}
	return out, nil
}

func (v *JWTVerifier) reject(reason string) (VerifiedToken, error) {
	v.log.Debug("jwt rejected", "reason", reason)
	return VerifiedToken{}, errTokenInvalid
}

// Verify implements AuthVerifier. The route comes from the request path: a
// token is refused (401) on a route auth.oidc.routes does not list, before any
// key is fetched. A valid token whose subject maps to no principal accepted on
// the route is a Denial (403).
func (v *JWTVerifier) Verify(ctx context.Context, token string, req *http.Request) (Identity, time.Time, error) {
	route := ""
	if req != nil && req.URL != nil {
		route = oidcRouteOf(req.URL.Path)
	}
	if route == "" || !slices.Contains(v.cfg.Routes, route) {
		_, err := v.reject(jwtRejectRoute)
		return Identity{}, time.Time{}, err
	}
	vt, err := v.validate(ctx, token)
	if err != nil {
		return Identity{}, time.Time{}, err
	}
	id, err := v.resolve(vt, route)
	if err != nil {
		return Identity{}, time.Time{}, err
	}
	return id, vt.Expiry, nil
}

// ProtectedResourceMetadata implements AuthVerifier. The verifier advertises no
// authorization server: sign-in happens at the console gateway.
func (v *JWTVerifier) ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata {
	return nil
}
