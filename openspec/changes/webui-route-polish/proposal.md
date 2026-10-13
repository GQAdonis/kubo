# Proposal: webui-route-polish

Phase `agent-backend-settings`, change 20 of 28 in plan revision 3, Round 5. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-50; D9).

## Why

After the shell and data-viz changes, the remaining routes (Files, Pins, Peers, Diagnostics, Welcome, Explore) need token-level polish and a sweep for stragglers.

## What Changes

- 1.1: Files, Pins, Peers, Diagnostics on tokens (W9).
- 1.2: Welcome illustrations replaced (W9).
- 1.3: `/explore` token-level theming only (W9).
- 1.4: Straggler sweep across fork-edited `src`.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: Files, Pins, Peers, Diagnostics, Welcome, Explore wrapper
- Depends on: webui-shell-redesign, webui-dataviz-reskin
- Size: M; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
