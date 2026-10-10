# Design: mcp-llm-proxy-loop

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **One client for UAR and other servers.** Whether UAR accepts client-supplied `tools` and returns `tool_calls` without executing its own tools is [unverified] (G10-1); a failure makes UAR an unsupported backend, OpenAI-compatible servers remain.
- **The loop owns no transport.** AG-UI (N12) and A2A (N13) adapt its events, so both reuse one confirmation mapping.
- **Minimal by design.** No memory, planning or multi-agent features; retrieval is a tool. This is the maintenance cost the fork now carries (addendum revision 3).
- **`noagent` build tag.** Builders can keep MCP and drop every agent surface.

## Risks / Trade-offs

- [A loopback backend that forwards upstream is indistinguishable from a local one] -> `egress_ack` is the operator's control; the limitation is documented (addendum G10 "Remaining partial guarantee").
- [G10-2: no local server handles tool calls well enough] -> G10 local is reported unmet; a remote backend with `egress_ack` still works.
