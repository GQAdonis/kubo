# Design: webui-agent-chat

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **Off means not loaded.** With the toggle off, the island bundle is never fetched.
- **The chat token is stored in browser storage (operator decision, 2026-10-10).** The `ui` principal token is kept in `localStorage` under its own key in `src/agent-chat/settings-store.js`, replacing the memory-only behavior of 1.1-1.2, so the token survives a reload (task 1.4). Trade-off: any script that runs in the Web UI origin can read the token, including script injected through an XSS defect in the Web UI or its dependencies; the token then lets that script act as the `ui` principal on `/agui` and on `/mcp`, which also accepts `ui` principals. Mitigations: switching the chat off clears the stored token; a "Forget token" control in Settings removes it; Settings copy states the token is stored in this browser profile; and the server limits what a leaked token can do, because it authenticates only the `ui` principal, whose tool groups and `egress_ack` the operator sets, and whose writes still need confirmation (native prompt for destructive calls). The token never grants `/api/v0` access. Desktop is unaffected: it gets the token from the `safeStorage` bridge (`tokenProvider`), not from `localStorage`.

## Risks / Trade-offs

- [Settings conflicts with the WebMCP toggle] -> ordered after `webui-webmcp-integration`; if that change is deferred (spike NO-GO), this one proceeds alone.
