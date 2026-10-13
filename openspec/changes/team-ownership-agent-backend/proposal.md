# Proposal: team-ownership-agent-backend

Phase `agent-backend-settings`, change 1 of 28 in plan revision 3, Round 1. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (D10, W5).

## Why

This phase edits paths no role owns: most of the Web UI fork, Desktop tray, sidecar and branding files, kubo `PRODUCT.md`, `docs/config.md`, `docs/changelogs/v0.44.md` and the know-me-cluster manifests. AGENTS.md routes such paths through the architect and an OpenSpec change before anyone edits them, and `team.json` itself is unowned, so the operator applies the patch.

## What Changes

- 1.1: Record the ownership extension in the proposal: `ui-ux-engineer` gains the whole `../ipfs-webui/**` fork (with `src/webmcp/**` writes still requiring security review), `../ipfs-desktop/src/tray.js`, Desktop branding assets (`assets/icons/**`, `assets/build/**`) and kubo `PRODUCT.md`; `mcp-server-engineer` gains `../ipfs-desktop/src/daemon/mcp-sidecar*.js`, the wiring lines in `src/daemon/index.js`, and `../ipfs-desktop/test/**/mcp-sidecar*` (W5); the architect gains `docs/config.md` (pointer entries only), `docs/changelogs/v0.44.md` (TOC and Highlights only; W5) and a know-me-cluster feature branch under `namespaces/ipfs/**`; the `WebUIPath` edit stays with the architect under D17.
- 1.2: Write the exact `team.json` patch and the native re-export command into the change folder, not applied (W5: `team.json` and the exports are unowned).
- 1.3: After OG-11, verify the applied grants.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: team governance only; no system behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect drafts; operator applies (OG-11)
- Scope: `openspec/changes/team-ownership-agent-backend/`
- Depends on: none
- Size: S; goals: all goals
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
