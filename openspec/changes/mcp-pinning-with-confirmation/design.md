# Design: mcp-pinning-with-confirmation

## Context

See proposal.md and ADR-0003. go-sdk v1.8.0 exposes `ServerSession.Elicit` (`mcp/server.go:1729`), and client capabilities are available after initialize.

## Goals / Non-Goals

**Goals:**
- No side effect happens without confirmation.
- Replay, tampering and cross-session reuse are rejected.

**Non-Goals:**
- The out-of-band resolvers (native OS prompt, Desktop window, headless CLI). They are `native-confirmation-prompt` (N14); this change only adds the external-resolution state (task 1.5).

## Decisions

- **Elicitation first.** A decline or cancel aborts the call with no side effect.
- **Fallback token.** HMAC-SHA256 with a per-process random key, over session ID, principal, tool name, canonical JSON of the arguments without the token, and expiry. Tokens are single-use, kept in an in-memory spent set, with a 120-second TTL. The first call returns a structured `confirmation_required` result whose summary names the operation and its CIDs or paths.
- **Read-only tools never prompt.**

## Risks / Trade-offs

- [The token proves the agent asked twice, not that a human agreed (T2)] → Annotations drive host approval prompts, elicitation is preferred, and the limitation is documented.
- [A process restart invalidates tokens] → Acceptable. The caller retries the first call.
