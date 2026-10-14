# Proposal: mcp-content-extractor

Phase `agent-backend-settings`, change 30 of 36 in plan revision 4 (proposed, pending OG-16), Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011 sections V1, V2, V7.

## Why

G12 indexes only `text/plain` and `text/markdown`. The operator wants PDF, Office, source code, JSON and CSV, image text and video audio searchable. Upstream xberg (`v1.3.10`) extracts these to Markdown over a REST API; `ipfs mcp` must call it as a service, never link it, and feed its output into the existing chunk, embed and store path.

## What Changes

- 1.1: Verify xberg `v1.3.10` behavior the design depends on (serve flags, loopback bind, offline model switch, endpoints that read server-side paths or URLs), then a standard-library extractor client for `POST /extract`.
- 1.2: Embedded mode: supervise an operator-supplied `xberg` binary on loopback, like `surreal`.
- 1.3: Kind routing from the contract's content-kinds derivation; derived chunk text stored with `source`.
- 1.4: Egress: the remote extractor host joins the `egress_ack` check.
- 1.5: Real-service test against xberg `v1.3.10`.
- 1.6: Disabled states: without an `extractor` block, indexing is exactly G12.
- 1.7: No CID injection: extraction adds, pins and announces nothing.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

- `mcp-content-extractor`: see `specs/mcp-content-extractor/spec.md`.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer
- Scope: `mcp/server/**` (including `mcp/server/embeddings/**`), `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*_test.go`
- Depends on: vector-search-contract (1.2-1.4); the delivered G12 changes `mcp-embeddings-surrealdb` and `mcp-embedding-index-lifecycle`
- Size: L; goals: 6
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change; no change to any of the three `go.mod` files (no xberg module, no cgo). Every new feature keeps an off switch (AGENTS.md).
