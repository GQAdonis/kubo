# Design: mcp-agent-principal-enforcement

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **Point-in-time limits are stated.** A UAR bound to `0.0.0.0` answers identically on loopback, so check-uar does not claim to detect the bind; the shipped profile is the supported configuration.
- **Fail closed per principal.** A failed check disables that principal only; the server and other principals keep running.
- **Stdio is outside the egress guarantee.** A host that spawns `ipfs mcp` over stdio cannot be identified; this is documented, as for any stdio MCP host.

### Contract gaps resolved (architect, 2026-10-09, ADR-0007)

- **Settings shape.** `check-uar` accepts only `mcp/contract/agentic/uar-effective-settings.schema.json` (the engineer's field names, plus integer `revision`, `provider_host` literals `local` and `disabled`, optional `native_tools.shell_tools`, `mcp_servers[].transport` enum). It reads it only from `settingsPath` in `mcp/contract/agentic/uar-preflight.json`, which is `null`: no guessed `/api/uar/settings`. UAR at `a7cb9729` serves an array of per-key records there, with an `x-uar-admin-key` header and embedders as provider names, which cannot satisfy the check (fixture `mcp/contract/fixtures/uar/observed-records-a7cb9729.json`).
- **Version pin.** In the contract (`uar-preflight.json` `minVersion`, with `pinnedFrom` evidence), read through `contract.LoadUARPreflight()`; set by the architect in `agentic-uar-integration` 1.2 from `mcp/uar/spike/REPORT.md`, together with `settingsPath`. Now `null`. `config/examples/uar-principal.json` keeps no `min_version`; its expected check-uar verdict today is `refuse`.
- **No new keys** for the recheck interval (moved to `recheckInterval: "30s"` in the contract, bounded to at most 60s) or for UAR's `ipfs mcp` entry (bound listen address; a mismatch withdraws writes).
- **Operator identity under `--confirm=cli`** stays without write grants. No exemption key; the supported path is a `human-mcp` principal with `write_groups` and `shell_capable: false`, which the refusal message must name.
- **Check-uar acceptance fixtures** for task 1.3: `mcp/contract/fixtures/uar/` (README lists the expected verdict of each).

## Risks / Trade-offs

- [check-uar couples Kubo to UAR's settings API shape] -> a shape change fails closed; fixtures are refreshed in `uar-integration-spike`.
