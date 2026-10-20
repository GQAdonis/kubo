# Reflection — agent-backend-settings › praxis-agentic-ux

Stage: reflect · Date: 2026-10-20

## Goal movement

| Goal | Result | Evidence |
|---|---|---|
| 1. One common agentic experience, Praxis-conformant where it applies | **Met at design level** | Prototype covers WS-01/03/05/06, HOME-01/02/03/07/08, API-06, CT-05, UI-01/02/08/10, SEC-04/06, WEB-06; conformance table in `DESIGN-PROPOSAL.md` §2 |
| 2. Preserve every IPFS function | **Met at design level** | Files, Pins, Explore, Peers, Diagnostics, Settings (5 sections), tray groups; change 8 keeps every tray function in "node controls" |
| 3. Decision model v2 | **Met** | 10 states drawn and interactive; `praxis-decision-model-v2` spec delta with one scenario per state |
| 4. Decision for every clause not adopted | **Met** | `analysis.md` deviations + `decision-log.md` PX-00..PX-16; `DESIGN-PROPOSAL.md` §11 |
| 5. Design deliverables only, no source changes | **Met** | No Go, webui or desktop file edited; 8 changes `validate --strict` clean |

Nothing was rendered, opened, run or tested. The prototype passed a JavaScript syntax check only. Goal results are design-level, not behavioural.

## What went well

- The research was reused: the assessment came from the earlier spec digest and repo surveys rather than a second sweep.
- Following the repo convention (fork changes in Kubo `openspec/changes/` with `skip_specs: true`) avoided creating OpenSpec roots in the forks.
- The decision model became the spine. One card and one state machine made four surfaces converge without per-surface design.

## What to watch

- **Brand assumption PX-02 is unconfirmed.** The operator did not answer the visual-system question. Only `praxis-visual-roles` depends on it.
- **Change 1 needs server work.** It adds optional contract fields (`effect_digest`, `destination`, `resolved_by`) and a digest re-check on confirm. That is Go work for mcp-server-engineer and puts the Kubo blocking constraints back in play.
- **Change 4 invents an endpoint.** The read-only per-principal decisions route, its in-memory history (200 entries or 7 days) and the `ipfs.decision.changed` CUSTOM event need an ADR before implementation.
- **Change 7 adds copy beyond the brief.** The subagent wrote a publish/share line, "A CID is an address, not a password." Needs operator review.
- **Praxis is "Proposed integration"**, edition 1.2. Clause IDs may move.
- **HG-1 is still unproven on hardware.** The Desktop design keeps Touch ID and says nothing stronger.

## Side effects

The OpenSpec refresh before phase creation force-updated generated tool integrations to OpenSpec 1.14.3 (authored specs untouched; backups in `~/.prometheus/openspec/backups/`).

## Artifact quality summary

No artifact-refiner run and no independent adversarial review were dispatched in this child. Self-review only.

## Recommended next

1. The operator confirms or flips PX-02, and reviews the change 7 copy.
2. Write an ADR for the decisions endpoint (change 4) and the contract additions (change 1).
3. Resume the parent `agent-backend-settings` at `lke-agent-settings` 1.1. Schedule the praxis changes as a later plan revision of the parent, or as their own phase, in rounds R1 → R4.
