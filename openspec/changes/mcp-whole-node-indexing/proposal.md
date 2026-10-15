# Proposal: mcp-whole-node-indexing

Phase `agent-backend-settings`, change 32 of 36 in plan revision 4 (proposed, pending OG-16), Round 5. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011 sections V4, V5.

## Why

G12 indexes only what is added through `ipfs mcp`. "Search all files on the node" needs a way to index what the operator already pins and keeps in MFS, and the Web UI search box needs an HTTP route. Both live in `ipfs mcp`: bulk indexing must be operator-started, confirmed, capped, resumable and never automatic, read only local blocks, and change nothing on the node; the route must reuse the search tool's schemas and authorization.

## What Changes

- 1.1: Bulk run engine over pins and MFS with offline reads, caps, one run at a time, persisted state, and no auto-resume.
- 1.2: Tools `ipfs_index_bulk_start`, `ipfs_index_bulk_status`, `ipfs_index_bulk_cancel` per the contract, confirmed on every transport.
- 1.3: Principal and egress checks at start and per file; the operator identity cannot index.
- 1.4: No CID injection and no network fetch, proven with a recorded call list and a `test/cli` run.
- 1.5: HTTP routes `POST /index/search` and `GET /index/status` per the contract's route descriptor.
- 1.6: `--print-rpc-allowed-paths` covers the RPC calls bulk indexing uses.
- 1.7: Disabled states.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

- `mcp-whole-node-indexing`: see `specs/mcp-whole-node-indexing/spec.md`.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer
- Scope: `mcp/server/**`, `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*_test.go`
- Depends on: vector-search-contract, mcp-content-extractor; mcp-http-transport-auth and mcp-agent-principal-enforcement (delivered) for the route's authorization
- Size: L; goals: 6
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change; only existing RPC calls are used (`pin/ls`, `files/stat`, `ls`, `cat`; the MFS walk is `files/stat /` once, then offline `ls` and offline `files/stat` on `/ipfs/<root>/...`). Nothing is added, pinned or announced (AGENTS.md "No silent CID injection"). Every new feature keeps an off switch (AGENTS.md).
