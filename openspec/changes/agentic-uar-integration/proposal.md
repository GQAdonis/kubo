# Proposal: agentic-uar-integration

Phase `ipfs-mcp-server`, plan revision 2 (new change). N3 (CONDITIONAL on N2 GO): shipped UAR profile and topology-B end-to-end tests. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

**CONDITIONAL.** This change runs only if its gate passes (see What Changes).

**Gate result (2026-10-09): NO-GO** (ADR-0008, `docs/mcp/0008-topology-b-gate-no-go.md`). Every task is deferred and reported unmet for phase `ipfs-mcp-server`.

## Why

When the topology-B gate passes and the spike is GO, Kubo ships a tested UAR profile with safe values that check-uar accepts and pins the minimum UAR version, and proves topology B end to end. UAR's defaults (remote default model and memory embedder, `0.0.0.0` bind, optional `terminal_exec`) conflict with the Kubo rules, so the profile is the only supported topology-B configuration.

## What Changes

- `mcp/uar/profile/`: loopback bind on a non-reserved port, `jwt_required: true`, an `llm.model` placeholder that must be edited, local FastEmbed embedders, `terminal_exec` off, no non-Kubo MCP servers, and an `mcp.json` that connects to `ipfs mcp --http` (`127.0.0.1:5050` by default) over HTTP only; the profile never spawns `ipfs mcp` over stdio.
- The minimum UAR version pin from the published release that passed the gate; check-uar refuses older versions via the agent card.
- A `kind: uar` principal snippet for the `ipfs mcp` config (read-only by default, `egress_ack` naming the model and embedder hosts).
- End-to-end tests against the published release: UAR-hosted AG-UI and A2A reads, native-prompt-gated writes, loopback-only network check, agent card path and task states.
- UAR as the topology-A LLM backend (moved from `mcp-llm-proxy-loop`, Round 6): `ipfs mcp` verifies UAR's effective upstream model provider per agent session through the pinned `check-uar` settings read before any `contentEgress` result reaches it. This needs the same pinned settings endpoint as topology B, so it is gated with this change.
- Egress in topology B is enforced only through the `--http` principal's `egress_ack` and check-uar; a stdio-spawned UAR is unsupported and outside the guarantee (stated limitation).

## Capabilities

### New Capabilities

- `agentic-uar-integration`: N3 (CONDITIONAL on N2 GO): shipped UAR profile and topology-B end-to-end tests

### Modified Capabilities

None.

## Impact

`mcp/uar/profile/**` (architect), `test/cli/mcp_uar_*_test.go` (mcp-server-engineer). No new Kubo endpoint. Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.
