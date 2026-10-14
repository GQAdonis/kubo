# Proposal: desktop-search-sidecar

Phase `agent-backend-settings`, change 34 of 36 in plan revision 4 (proposed, pending OG-16), Round 6. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011 section V8 (Desktop).

## Why

The operator wants vector search on IPFS Desktop (macOS arm64) as well as on LKE. Desktop already runs `ipfs mcp` as a sidecar (`desktop-mcp-sidecar`). Search needs that sidecar to run SurrealDB and xberg in embedded mode from operator-installed binaries, with a local embedder by default, an explicit confirmation for any non-loopback host, and an off switch. Search is off by default on Desktop: it spends CPU and disk, and it is not one of the agent features `AGENTS.md` lists as default-on.

## What Changes

- 1.1: Verify the macOS arm64 xberg CLI `v1.3.10` and record how it behaves under Desktop (checksum, quarantine, serve flags, cache path, model downloads).
- 1.2: Sidecar config generation: the `embeddings` block in embedded mode for the store and the extractor when Search is on and the binary paths are set.
- 1.3: Egress: non-loopback hosts added to the Desktop `ui` principal's `egress_ack` only after a host-naming confirmation with Touch ID.
- 1.4: Changing search settings restarts the sidecar; a running bulk run pauses.
- 1.5: Desktop settings UI: a Search section, off by default.
- 1.6: Disabled-state test.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owners: mcp-server-engineer (1.1-1.4: `../ipfs-desktop/src/daemon/mcp-sidecar*.js`, the wiring lines in `src/daemon/index.js`, `../ipfs-desktop/test/**/mcp-sidecar*`); ui-ux-engineer (1.5-1.6: `../ipfs-desktop/src/agent/**`, `../ipfs-desktop/test/**`)
- Depends on: desktop-mcp-sidecar, desktop-agent-settings-bridge, vector-search-contract, mcp-content-extractor, mcp-media-describer, mcp-whole-node-indexing; OG-19 (whether Desktop bundles the binaries; the plan assumes it does not)
- Size: M-L; goals: 6, 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Desktop downloads no binary and no model on its own; every new feature keeps an off switch (AGENTS.md).
