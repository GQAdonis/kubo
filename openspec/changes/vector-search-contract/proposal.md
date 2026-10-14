# Proposal: vector-search-contract

Phase `agent-backend-settings`, change 29 of 36 in plan revision 4 (proposed, pending OG-16), Round 3. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011 (`docs/mcp/0011-whole-node-vector-search.md`). Notes: tasks 1.7 and 1.8 wait at task level for `mcp-media-describer` and `mcp-whole-node-indexing` (Round 5).

## Why

Whole-node vector search (goal 6) adds two indexing stages, bulk indexing, three tools and two HTTP routes to `ipfs mcp`. `mcp/contract/` is the single source of truth for every tool, key and route, so the contract and the config keys, each with a default and an off switch, must exist before any builder starts.

## What Changes

- 1.1: Move ADR-0011 to `Accepted` with the operator's OG-16, OG-17, OG-18 and OG-19 answers quoted, amending the sections they override.
- 1.2: Config schema keys under `embeddings` (`extractor`, `describer`, `bulk`, `search_route`), each with `default`, `offSwitch` and `ownerChange`; valid and invalid config examples.
- 1.3: The content-kinds derivation (`mcp/contract/derivations/content-kinds.json` and its schema): kinds `text`, `document`, `code`, `data`, `image`, `video`, their MIME types, the stage that handles each, and the `source` labels.
- 1.4: Tool contract: catalog entries and 2020-12 input and output schemas for `ipfs_index_bulk_start`, `ipfs_index_bulk_status`, `ipfs_index_bulk_cancel`; additive optional fields on `ipfs_index_search` and `ipfs_index_cid`; MCP annotations and agentic metadata.
- 1.5: Route descriptor for `POST /index/search` and `GET /index/status`, referencing the tool schemas by path, with principal kinds, group, egress rule and error-to-status mapping.
- 1.6: Regenerate `docs/mcp/config-reference.md` with the existing generator.
- 1.7: User docs after implementation: `docs/mcp/embeddings.md` and `docs/mcp/user-guide.md`.
- 1.8: Changelog TOC and Highlights in `docs/changelogs/v0.44.md` with a concrete disable example.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: contract files, ADR and documentation only; the behavior they describe is specified in `mcp-content-extractor`, `mcp-media-describer` and `mcp-whole-node-indexing`.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect
- Scope: `docs/mcp/0011-*.md`, `mcp/contract/**`, `docs/mcp/config-reference.md`, `docs/mcp/embeddings.md`, `docs/mcp/user-guide.md`, `docs/changelogs/v0.44.md` (TOC and Highlights only)
- Depends on: agent-settings-contract (Round 1, done); the delivered G12 changes `mcp-embeddings-surrealdb` and `mcp-embedding-index-lifecycle`; OG-16 before any task
- Size: M; goals: 6
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change; no Kubo repo config key (D12). Every new key has a default and an off switch (AGENTS.md).
