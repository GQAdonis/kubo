# Proposal: webui-dataviz-reskin

Phase `agent-backend-settings`, change 12 of 28 in plan revision 3, Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-21).

## Why

Charts and the peers map use raw hex colours and gradients; data visualization must come from the design tokens in both themes.

## What Changes

- 1.1: `tokens.js` resolving CSS variables at render and on theme change; Chart.js series from it.
- 1.2: d3 map flat fills from tokens.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `CountryChart.js`, `NetworkTraffic.js`, `NodeBandwidthChart.js`, `WorldMap.js`, `src/promethity/tokens.js`
- Depends on: webui-promethity-foundation
- Size: S-M; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
