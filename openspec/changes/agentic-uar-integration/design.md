# Design: agentic-uar-integration

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **Published releases only.** Tests run against the published artifact, never a source build.
- **HTTP only.** The profile's `mcp.json` uses `ipfs mcp --http`; stdio is forbidden for topology B.

## Risks / Trade-offs

- [UAR config keys drift between releases] -> the profile records the release it was tested with; check-uar fails closed on settings-shape changes.
