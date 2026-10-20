# Proposal: praxis-decision-model-v2

Phase `agent-backend-settings › praxis-agentic-ux`, change 1 of 8, child plan r1, Round 1. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

The shared approval model in `mcp/ui/src/approval/model.ts` has five phases (`pending`, `waiting_system_prompt`, `approved`, `denied`, `expired`). KnowMe Praxis agentic workspace container spec ed. 1.2 asks for more than that:

- WS-05: an approval shows the object, destination, scope and named authority, and is bound to an effect digest. The assessment marks WS-05 **P**: the card names tool, CIDs/paths and class, and the confirm token binds the repeat call (ADR-0003), but no effect digest is shown and nothing invalidates the card when the effect changes.
- API-06: an interrupt ends a run, an expired decision is rejected, and the digest must match at continuation. Assessment: **P**, no digest re-check surfaced in the UI.
- CT-05: a timeout is not a failure; the outcome is reconciled. Today the card has no state for "we do not know yet".
- HOME-10 #15: "approval resolved elsewhere" is a required screen state. Assessment: it exists only in the model, never on screen.

The model also says "Approved" for a call that has not run. Praxis keeps accepted distinct from committed; the current copy "Approved. The call runs once." blurs the two.

This is analysis decision D5 (decision-log PX-07).

## What Changes

- Extend `APPROVAL_PHASES` from five to ten states: `pending`, `waiting_system_prompt`, `approved`, `committed`, `denied`, `expired`, `invalidated`, `resolved_elsewhere`, `uncertain`, `needs_resubmit`. Badge words and controls come verbatim from the analysis table (see `design.md` and the spec delta).
- Change the `approved` copy to "Approved, not yet run". Add `committed` ("Done", with "View receipt") as the only state that says the effect ran.
- Add card fields: prepared object (tool and human description), destination (node, remote pinning service or IPNS name), scope (CIDs or paths), class in words (Write or Destructive), named authority (principal kind and name), egress badge, and a short effect digest (`sha256:7f3a…c1`).
- Invalidate a card when the effect digest at run time differs from the digest the person saw.
- `resolved_elsewhere` names the surface that answered, from the existing `RESOLVER_PLACE` words.
- `uncertain` when the outcome does not arrive before the timeout; it is never shown as a failure.
- `needs_resubmit` for a request restored after a server or host restart whose confirm token can no longer resume.
- Additive, optional contract fields so the server can supply the digest, destination and resolving surface: `effect_digest`, `destination`, `resolved_by` on the pending-confirmation payload in `mcp/contract/schemas`. The contract's `executed` state maps to `committed`.
- Update `ApprovalCard`, `ConfirmationDialog` and the `ApprovalCard` / `ConfirmationDialog` entries in `mcp/ui/catalog/ipfs-mcp.catalog.json` (regenerated, not hand-edited), and the "Confirmation states" table in `DESIGN.md`.

## Capabilities

### New Capabilities

- `ipfs-decision-model`: the ten-state decision model, card fields, digest binding and recovery states shared by every approval surface (MCP App views, A2UI catalog, chat island, Web UI, Desktop native window).

### Modified Capabilities

None archived. The unarchived requirement "Approval card truthfulness" from `mcp-ui-component-library` (`ipfs-ui-components`) still holds and is restated inside `ipfs-decision-model` for the `pending` and `waiting_system_prompt` states.

## Impact

- Owner: ui-ux-engineer (model, components, catalog, `DESIGN.md`); mcp-server-engineer for the additive contract fields and the server emitting `effect_digest`, `destination` and `resolved_by`.
- Scope:
  - `mcp/ui/src/approval/model.ts`, `mcp/ui/src/approval/ApprovalPanel.tsx`
  - `mcp/ui/src/chat/ApprovalCard.tsx`, `mcp/ui/src/chat/EgressBadge.tsx`, `mcp/ui/src/components/confirmation-dialog/**`
  - `mcp/ui/catalog/ipfs-mcp.catalog.json` (regenerated with `pnpm gen:catalog`)
  - `mcp/app/src/**` where views render a confirmation
  - `mcp/contract/schemas/**` confirmation schema (additive optional fields), regenerated `mcp/ui/src/contract/generated/confirmation.ts`
  - `mcp/server/**` confirmation path: compute and emit the effect digest, compare it at confirm time
  - `DESIGN.md` "Confirmation states (trust surface)"
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. The contract change is additive and optional; older clients ignore the new fields. ADR-0003 (confirm token binds the repeat call) is unchanged; the digest is an extra check on top.
- Size: M.

## Dependencies

None. Round 1. Changes 4 (`praxis-decision-inbox`), 6 (`praxis-mcp-app-a2ui-seam`) and 8 (`praxis-desktop-attention`) depend on this one.
