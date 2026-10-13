# Proposal: webui-promethity-assets

Phase `agent-backend-settings`, change 9 of 28 in plan revision 3, Round 3. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-11).

## Why

Favicons, manifest, page metadata and logos still carry the IPFS product identity.

## What Changes

- 1.1: Favicon set, `manifest.json` (192, 512, maskable; colours), `index.html` (two `theme-color` metas, title, description, generator).
- 1.2: Wordmark and p-icon components; remove IPFS logos from NavBar and Welcome; never wordmark and icon side by side.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `../ipfs-webui/public/**`, `src/navigation/*.svg`, logo components
- Depends on: promethity-design-scope
- Size: S; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
