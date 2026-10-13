# Proposal: mcp-embeddings-surrealdb

Phase `ipfs-mcp-server`, plan revision 2 (new change). N7: G12 SurrealDB store in embedded and remote modes, and the embedder client, inside `ipfs mcp`. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G12 requires embeddings stored in SurrealDB with vector search in both embedded and remote modes, an operator-configured embedder and nothing on by default. The plan's re-score under the goal-fit gate (plan section D7) puts embedding and storage in `ipfs mcp` for both topologies. Mechanisms, concretely: remote mode connects to an operator-given SurrealDB URL; embedded mode is a SurrealDB process that `ipfs mcp` starts and stops itself (`surreal start --bind 127.0.0.1:<ephemeral> surrealkv://<operator data dir>`) from an operator-supplied binary that Kubo never downloads or ships. `surrealdb.c.go` (cgo plus a Rust library) is excluded: Kubo has no cgo.

## What Changes

- First, verify the SurrealDB 3.x HTTP and RPC APIs, HNSW DDL and KNN syntax and `surreal start` flags from primary docs, and build-probe `github.com/surrealdb/surrealdb.go` v1.7.0 (`CGO_ENABLED=0 make build`, binary delta against a 3 MB limit). Default client: the standard library over SurrealDB's HTTP API, adding no module; `surrealdb.go` only with an operator answer (Round 0 R2) and a passing probe (G12-1).
- Store schema keyed by CID (`cid`, `path`, `mime`, `chunk_index`, `model`, `dims`, `vector`) with an HNSW index of the configured dimension (G12-2).
- Remote mode: URL, namespace and database from config, credentials from env or file, no default.
- Embedded mode: absolute binary path (no PATH lookup, no download), minimum version check, loopback ephemeral port, per-start random credentials in a 0600 file, stop on exit and crash cleanup (G12-3).
- Embedder client: operator-configured OpenAI-compatible `/embeddings` endpoint (model, dimension, key from env or file), host checked against `egress_ack`, no default.
- Per-mode acceptance tests and the embeddings section of the docs.

## Capabilities

### New Capabilities

- `mcp-embeddings-surrealdb`: N7: G12 SurrealDB store in embedded and remote modes, and the embedder client, inside `ipfs mcp`

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`. Off unless an `embeddings` section is configured; `noagent` removes it. Kubo never ships, downloads or links SurrealDB (BSL 1.1 stays outside Kubo artifacts). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.
