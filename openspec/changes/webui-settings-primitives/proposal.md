# Proposal: webui-settings-primitives

Phase `agent-backend-settings`, change 13 of 28 in plan revision 3, Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-30).

## Why

The settings page needs accessible primitives the brand does not define (switch, segmented control, combobox, status line, apply bar) in a React 16 shell without React 18 APIs.

## What Changes

- 1.1: Field, Switch (`role=switch`), SegmentedControl (radio group).
- 1.2: Combobox per the APG pattern (W9).
- 1.3: StatusLine, ApplyBar, PolicyDecision row (W9).
- 1.4: Unit and axe tests for both themes.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `../ipfs-webui/src/promethity/components/**`
- Depends on: webui-promethity-foundation
- Size: M-L; goals: 1, 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
