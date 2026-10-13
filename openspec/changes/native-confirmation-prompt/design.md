# Design: native-confirmation-prompt

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **Presence over convenience.** The Linux prompt needs an operator-installed polkit action (one-time root step, never installed by Kubo); without it, Linux fails closed. Windows fails closed unless a Desktop window is paired.
- **The Desktop channel is paired.** A pairing key held by Desktop in `safeStorage`; an unpaired client can neither list nor resolve.
- **Clients with elicitation keep elicitation.**

## Risks / Trade-offs

- [The macOS prompt mechanism without cgo is unverified] -> task 1.1 is a gate inside this change; if it fails, macOS uses the Desktop window only, or fails closed.
- [Approval fatigue] -> the prompt names the exact operation and CIDs; read-only tools never prompt.
