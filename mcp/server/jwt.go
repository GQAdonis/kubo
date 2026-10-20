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

	// go-jose v4.1.6 comes in through the temporary GQAdonis/p2p-forge fork
	// replace in go.mod (operator decision 2026-10-17); return to an upstream
	// p2p-forge release once ipshipyard/p2p-forge#96 ships.
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
	jwtRejectAZP       = "azp"
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
	// AZP is the authorized party (OIDC Core 3.1.3.7).
	AZP string `json:"azp"`
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
	// ownLog is true when the caller supplied a logger.
	ownLog bool
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
	ownLog := log != nil
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
	// A hand-built OIDCSettings with no cap gets the default: the cap has no
	// off value.
	maxStale := o.OIDC.JWKSMaxStale
	if maxStale <= 0 {
		maxStale = DefaultOIDCJWKSMaxStale
	}
	v := &JWTVerifier{
		origins:  origins,
		ownLog:   ownLog,
		cfg:      o.OIDC,
		keys:     newJWKSCache(o.OIDC.JWKSURL, o.OIDC.JWKSCacheTTL, maxStale, o.Client, now),
		now:      now,
		log:      log,
		subjects: subjects,
	}
	v.keys.onStale = func(age time.Duration) {
		v.log.Warn("jwks key set is older than auth.oidc.jwks_max_stale: session tokens are refused until it is fetched again",
			"age", age.Round(time.Second).String(), "max_stale", maxStale.String(), "jwks", redactURL(o.OIDC.JWKSURL))
	}
	return v, nil
}

// adoptLogger makes l the verifier's logger unless the caller supplied one. The
// listener calls it once, before it serves.
func (v *JWTVerifier) adoptLogger(l *slog.Logger) {
	if !v.ownLog && l != nil {
		v.log = l
	}
}

// warnOpenRoutes logs one Warn line when a route other than the session report
// accepts a JWT and agent.origin_allowlist is empty: require_origin then refuses
// every state-changing session request, which is safe but looks like a fault.
// Startup continues.
func (v *JWTVerifier) warnOpenRoutes() {
	if len(v.origins) > 0 || !v.cfg.RequireOrigin {
		return
	}
	if !slices.ContainsFunc(v.cfg.Routes, func(r string) bool { return r != OIDCRouteSession }) {
		return
	}
	v.log.Warn("auth.oidc.routes lists routes beyond session but agent.origin_allowlist is empty: state-changing session requests will be refused until it lists the console origin",
		"routes", strings.Join(v.cfg.Routes, ","))
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
	if !v.azpOK(claims) {
		return v.reject(jwtRejectAZP)
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

// azpOK applies OIDC Core 3.1.3.7 steps 4 and 5: an ID token with several
// audiences must carry an azp, and an azp must be one of the configured
// audiences. go-jose has already checked that some aud is configured.
func (v *JWTVerifier) azpOK(c idTokenClaims) bool {
	if c.AZP == "" {
		return len(c.Audience) <= 1
	}
	return slices.Contains(v.cfg.Audiences, c.AZP)
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
	if err := v.checkOrigin(req); err != nil {
		return Identity{}, time.Time{}, err
	}
	id, err := v.resolve(vt, route)
	if err != nil {
		return Identity{}, time.Time{}, err
	}
	return id, vt.Expiry, nil
}

// checkOrigin applies auth.oidc.require_origin. A session identity rides on a
// cookie the gateway turned into a bearer, so a page on a sibling site can make
// the browser send it; a state-changing request must therefore name a page
// origin the operator listed. A request with no Origin fails. Static bearers
// never reach this check.
func (v *JWTVerifier) checkOrigin(req *http.Request) error {
	if !v.cfg.RequireOrigin {
		return nil
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}
	if origin := strings.ToLower(req.Header.Get("Origin")); origin != "" && slices.Contains(v.origins, origin) {
		return nil
	}
	v.log.Warn("session request refused: origin missing or not listed in agent.origin_allowlist", "method", req.Method, "path", req.URL.Path)
	return &Denial{Status: http.StatusForbidden, Code: DenialOriginRequired, Message: "a session request that changes state must send an Origin listed in agent.origin_allowlist"}
}

// ProtectedResourceMetadata implements AuthVerifier. The verifier advertises no
// authorization server: sign-in happens at the console gateway.
func (v *JWTVerifier) ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata {
	return nil
}
