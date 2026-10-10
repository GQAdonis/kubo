# Design: mcp-embeddings-surrealdb

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **`ipfs mcp` embeds and stores in both topologies.** Every agent write except WebMCP passes through `ipfs mcp`, including UAR in topology B, so UAR-side embedding is not needed (plan D7).
- **Embedded means supervised, not linked.** True in-process SurrealDB from Go needs cgo; the docs say so.
- **Standard library first.** Avoids five new root modules unless the operator accepts `surrealdb.go`.

## Binding to the implemented loop (Round 6, architect)

Checked against `mcp/server` after `mcp-llm-proxy-loop`. The change is ready to dispatch; it needs nothing new from the loop.

- **Embedder host and egress.** `Config.BackendHosts()` (`mcp/server/principals.go`) already includes the embedder `base_url` and a remote store endpoint, so configured principals must acknowledge them through the existing MCP-side rule. At embed time the check is the loop's rule, not the MCP one: `ipfs mcp` itself sends the content, so the triggering identity's `egress_ack` must cover the embedder host (`AckCovers`) whether or not the identity is configured. The operator identity (stdio, `--auth-token-*`) has no `egress_ack`, so its adds are not auto-indexed; that is the safe direction and is documented, not worked around.
- **Do not reuse `llm_client.go`.** The chat client and `agent_settings.go` are built only without `noagent`. The index tools are MCP tools in the contract and stay available in `noagent` builds (they are off until `embeddings` is configured), so the `/embeddings` client lives in its own file or package without that build tag and shares only untagged helpers (`HostFromURL`, `AckCovers`, the env/file key loading if it is moved to an untagged file). `nomcp` still removes all of it.
- **Prerequisites present on this machine** (2026-10-09): `surreal` 3.3.0 at `/Users/gqadonis/.prometheus/bin/surreal` (operator-supplied absolute path, as task 1.3 requires) and `ollama` for a local embedder in task 1.5. Tests must still pick free ports and never use 11434 or SurrealDB's default ports as fixed values.

## Risks / Trade-offs

- [Kubo supervising a third-party process] -> off by default, operator-supplied binary, loopback only, lifecycle-bound, orphan test (G12-3).
- [No HNSW from Go] -> G12 deferred; an in-memory fallback is not accepted (G12-2).
