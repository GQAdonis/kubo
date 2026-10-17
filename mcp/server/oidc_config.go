package server

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// AuthConfig is the auth section. It holds only the OpenID Connect switch for
// now; static bearers are configured through principals.
type AuthConfig struct {
	OIDC OIDCConfig `json:"oidc"`
}

// OIDCConfig is auth.oidc (ADR-0013 section 4): the verifier of OpenID Connect
// ID tokens forwarded from the console session. It is off unless Enabled is
// true, and then nothing is fetched, no JWT is accepted and /auth/session is not
// mounted. Pointer and slice fields tell an absent key (the contract default)
// from an explicit empty value.
type OIDCConfig struct {
	Enabled   bool     `json:"enabled"`
	Issuer    string   `json:"issuer"`
	Audiences []string `json:"audiences"`
	JWKSURL   string   `json:"jwks_url"`
	// Algorithms nil is the default ["RS256"].
	Algorithms []string `json:"algorithms"`
	ClockSkew  string   `json:"clock_skew"`
	// JWKSCacheTTL is auth.oidc.jwks_cache_ttl.
	JWKSCacheTTL string `json:"jwks_cache_ttl"`
	// Routes nil is the default list; [] accepts a JWT on no route.
	Routes *[]string `json:"routes"`
	// RequireOrigin nil is the default, true.
	RequireOrigin *bool `json:"require_origin"`
}

// Defaults and bounds of auth.oidc, matching the contract's config schema.
const (
	DefaultOIDCClockSkew    = 30 * time.Second
	MaxOIDCClockSkew        = 2 * time.Minute
	DefaultOIDCJWKSCacheTTL = 15 * time.Minute

	// Routes a JWT identity may be accepted on (auth.oidc.routes).
	OIDCRouteMCP      = "mcp"
	OIDCRouteA2A      = "a2a"
	OIDCRouteAGUI     = "agui"
	OIDCRouteSettings = "settings"
	OIDCRouteIndex    = "index"
	OIDCRouteSession  = "session"
)

// DefaultOIDCRoutes are the routes that accept a JWT when auth.oidc.routes is
// absent. mcp and a2a are deliberately not among them.
func DefaultOIDCRoutes() []string {
	return []string{OIDCRouteAGUI, OIDCRouteSettings, OIDCRouteIndex, OIDCRouteSession}
}

// OIDCSettings is auth.oidc with the defaults filled in and every value parsed.
type OIDCSettings struct {
	Issuer        string
	Audiences     []string
	JWKSURL       string
	Algorithms    []jose.SignatureAlgorithm
	ClockSkew     time.Duration
	JWKSCacheTTL  time.Duration
	Routes        []string
	RequireOrigin bool
}

// supportedAlgorithms are the asymmetric JWS algorithms the verifier accepts.
// None and the HMAC family are absent on purpose: with a JWKS of public keys, a
// symmetric algorithm lets anyone sign a token with a key they can download.
var supportedAlgorithms = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.PS256, jose.PS384, jose.PS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.EdDSA,
}

// validate checks the rules the JSON schema leaves to the loader. It runs
// whether or not the section is enabled, so a bad value never waits for the
// day the switch is turned on.
func (c OIDCConfig) validate() error {
	_, err := c.resolve()
	return err
}

// resolve parses the section. When it is not enabled the returned settings are
// the zero value, apart from the values that were checked.
func (c OIDCConfig) resolve() (OIDCSettings, error) {
	var s OIDCSettings
	var err error
	if s.ClockSkew, err = parseOIDCDuration("auth.oidc.clock_skew", c.ClockSkew, DefaultOIDCClockSkew); err != nil {
		return s, err
	}
	if s.ClockSkew > MaxOIDCClockSkew {
		return s, fmt.Errorf("auth.oidc.clock_skew %q is out of range: it must be at most %s", c.ClockSkew, MaxOIDCClockSkew)
	}
	if s.JWKSCacheTTL, err = parseOIDCDuration("auth.oidc.jwks_cache_ttl", c.JWKSCacheTTL, DefaultOIDCJWKSCacheTTL); err != nil {
		return s, err
	}
	if s.JWKSCacheTTL <= 0 {
		return s, fmt.Errorf("auth.oidc.jwks_cache_ttl %q must be positive", c.JWKSCacheTTL)
	}
	if s.Algorithms, err = parseAlgorithms(c.Algorithms); err != nil {
		return s, err
	}
	s.Routes = DefaultOIDCRoutes()
	if c.Routes != nil {
		s.Routes = slices.Clone(*c.Routes)
	}
	s.RequireOrigin = c.RequireOrigin == nil || *c.RequireOrigin
	if !c.Enabled {
		return s, nil
	}
	if c.Issuer == "" {
		return s, errors.New("auth.oidc.issuer is required when auth.oidc.enabled is true")
	}
	if len(c.Audiences) == 0 {
		return s, errors.New("auth.oidc.audiences must list at least one audience when auth.oidc.enabled is true")
	}
	if c.JWKSURL == "" {
		return s, errors.New("auth.oidc.jwks_url is required when auth.oidc.enabled is true")
	}
	if err := checkJWKSURL(c.JWKSURL); err != nil {
		return s, err
	}
	s.Issuer, s.JWKSURL = c.Issuer, c.JWKSURL
	s.Audiences = slices.Clone(c.Audiences)
	return s, nil
}

func parseOIDCDuration(key, v string, def time.Duration) (time.Duration, error) {
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s %q is not a valid non-negative duration", key, v)
	}
	return d, nil
}

// parseAlgorithms turns auth.oidc.algorithms into JWS algorithms, refusing any
// that is not asymmetric.
func parseAlgorithms(names []string) ([]jose.SignatureAlgorithm, error) {
	if names == nil {
		return []jose.SignatureAlgorithm{jose.RS256}, nil
	}
	if len(names) == 0 {
		return nil, errors.New("auth.oidc.algorithms must list at least one algorithm")
	}
	out := make([]jose.SignatureAlgorithm, 0, len(names))
	for _, n := range names {
		alg := jose.SignatureAlgorithm(n)
		if !slices.Contains(supportedAlgorithms, alg) {
			return nil, fmt.Errorf("auth.oidc.algorithms: %q is not an accepted algorithm (asymmetric JWS algorithms only: none and HS* are refused)", n)
		}
		out = append(out, alg)
	}
	return out, nil
}

// checkJWKSURL requires https, or http for a loopback host.
func checkJWKSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("auth.oidc.jwks_url is not a valid URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
	}
	return errors.New("auth.oidc.jwks_url must be an https URL, or an http URL on a loopback host")
}
