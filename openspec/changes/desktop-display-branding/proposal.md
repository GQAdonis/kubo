# Proposal: desktop-display-branding

Phase `agent-backend-settings`, change 15 of 28 in plan revision 3, Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (D11).

## Why

Desktop's icons, window titles and display name are IPFS-branded; changing `productName` would orphan userData and Keychain items, so only display surfaces change.

## What Changes

- 1.1: Tray, dock and build icons from the p-icon set.
- 1.2: Window titles and visible display name; `productName` and `io.ipfs.desktop` unchanged.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `../ipfs-desktop/assets/icons/**`, `assets/build/**`, window titles, display name
- Depends on: webui-promethity-assets, team-ownership-agent-backend
- Size: M; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
