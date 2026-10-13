# Proposal: webui-shell-redesign

Phase `agent-backend-settings`, change 11 of 28 in plan revision 3, Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-20).

## Why

The shell (top bar, navigation rail, panels, buttons) uses navy and washed-blue Tachyons styling with borders and shadows that the Promethity brand forbids.

## What Changes

- 1.1: `App.js` shell (surface rail, ground top bar, no inline hex) and `NavBar` (active `accent-text` + 600, no bars).
- 1.2: `Box`, `Title`, `button`, `Notify`, `Connected`, tour skin on tokens; fills, no borders, no shadows.
- 1.3: Lucide icons for nav and settings.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `../ipfs-webui/src/App.js`, `src/navigation/**`, `Box`, `Title`, `button`, `Notify`, `Connected`, tour skin
- Depends on: webui-promethity-foundation, webui-promethity-assets
- Size: L; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
