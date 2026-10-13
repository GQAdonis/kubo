# Design: mcp-http-transport-auth

## Context

See proposal.md and ADR-0003. go-sdk v1.8.0 provides `StreamableHTTPHandler` (Host check, `CrossOriginProtection`, `MaxRequestBodyBytes`), `auth.RequireBearerToken` with `TokenVerifier` (`auth/auth.go:41,97`) and `ProtectedResourceMetadataHandler` (`auth/auth.go:188`).

## Goals / Non-Goals

**Goals:**
- Tool parity with stdio.
- A verifier swap without handler changes.

**Non-Goals:**
- Ory integration.
- TLS termination (left to a reverse proxy, documented).

## Decisions

- **`AuthVerifier` returns a principal and scopes.** Scopes map to tool groups (`ipfs:read`, `ipfs:write`, `ipfs:admin`). The static verifier grants the scopes configured for its token.
- **Fail closed.** If a non-loopback address is given without a verifier, the process exits with an error.
- **The RFC 9728 metadata endpoint is served only when the verifier supplies metadata.** The static verifier supplies none.

- **Default listen address `127.0.0.1:5050`.** Operator decision (2026-10-09): 4000 is the liter-llm proxy and other nearby ports are in use on the operator machine. Overridable by flag and env. Tests use dynamically chosen free ports.
- **One listener, mounted routes.** AG-UI, A2A and the agent card mount on this listener through one interface, so they cannot bypass auth or origin checks.

## Risks / Trade-offs

- [Full tool set reachable remotely (T3)] → Loopback default, mandatory token off loopback, scopes, and the least-privilege RPC helper.
