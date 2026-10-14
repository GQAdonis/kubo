package server

import "context"

// Principal kinds from ADR-0004.
const (
	// KindHumanMCP is an MCP host a human drives, such as Claude Code.
	KindHumanMCP = "human-mcp"
	// KindUI is the chat island of the MCP App or IPFS Desktop.
	KindUI = "ui"
	// KindA2A is a remote agent calling over the A2A endpoint.
	KindA2A = "a2a"
	// KindUAR is the Universal Agent Runtime.
	KindUAR = "uar"
	// KindAdmin is the runtime-settings administrator (ADR-0009 section 3). Its
	// scope is the settings route only: it has no tool access, so no route
	// serves it by default; the settings route mounts with Kinds [KindAdmin].
	KindAdmin = "admin"
)

// Principal is who is calling. Stdio sessions run as the local human-mcp
// principal; the HTTP transports attach the principal their authentication
// verifier produced with WithPrincipal.
type Principal struct {
	// ID is a stable name; it is bound into confirm tokens.
	ID string
	// Kind is one of the Kind constants.
	Kind string
}

// LocalPrincipal is the principal of a stdio session.
var LocalPrincipal = Principal{ID: "local-stdio", Kind: KindHumanMCP}

// AcceptsToken reports whether the principal may be issued, and may present,
// the two-step confirm token. Principals that cannot elicit and have no human
// at the keyboard (a2a, uar) never can: a token from them would only prove
// that the agent asked twice (ADR-0003 section 4).
func (p Principal) AcceptsToken() bool {
	return p.Kind != KindA2A && p.Kind != KindUAR
}

type principalKey struct{}

// WithPrincipal returns a context that carries the calling principal.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFromContext returns the calling principal, or LocalPrincipal when
// the transport attached none.
func PrincipalFromContext(ctx context.Context) Principal {
	if p, ok := ctx.Value(principalKey{}).(Principal); ok && p.ID != "" {
		return p
	}
	return LocalPrincipal
}
