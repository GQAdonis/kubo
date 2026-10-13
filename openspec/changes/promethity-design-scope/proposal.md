# Proposal: promethity-design-scope

Phase `agent-backend-settings`, change 4 of 28 in plan revision 3, Round 2. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-00).

## Why

`DESIGN.md` and `PRODUCT.md` scope the Promethity identity to `mcp/ui` and `mcp/app` only. Goal 5 extends it to the whole Web UI fork and the Desktop windows, with recorded exceptions (form-control edge for WCAG 1.4.11, theme default, font fallback, Explore seam).

## What Changes

- 1.1: Extend `DESIGN.md` scope to the Web UI fork and Desktop windows; record D7, D16, the Latin-only font fallback (R8) and the Explore seam (D9).
- 1.2: Extend `PRODUCT.md` scope; record D6, D8, D11; answer the "how far the identity applies" open decision.
- 1.3: Start `docs/PROMETHITY-FORK.md` (file, why, upstream-merge note).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: architecture decisions, contract schemas and documentation; the behavior they describe is specified in the changes that implement it.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: kubo `DESIGN.md`, `PRODUCT.md`; `../ipfs-webui/docs/PROMETHITY-FORK.md`
- Depends on: team-ownership-agent-backend
- Size: S; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
