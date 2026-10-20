package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

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
	// sub is the `sub` of the person behind a session identity; empty for a
	// static bearer. It is internal: it is never written to a response, and
	// only a digest of it reaches a confirm token.
	sub string
}

// BindingKey is what the MCP session binding and every confirmation binding
// key on. For a static bearer it is the principal ID, as before. For a session
// identity it adds a digest of the token's `sub`, so two people mapped to one
// principal cannot continue each other's MCP session or answer each other's
// pending confirmations (ADR-0013 amendment, mcp-oidc-hardening D4). The digest
// keeps the subject itself out of the signed confirm tokens a client can read.
func (p Principal) BindingKey() string {
	if p.sub == "" {
		return p.ID
	}
	sum := sha256.Sum256([]byte(p.sub))
	return p.ID + "#" + hex.EncodeToString(sum[:16])
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
