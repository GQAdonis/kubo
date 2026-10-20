# Plan — agent-backend-settings › praxis-agentic-ux

Stage: plan · Plan revision: child r1 · Inputs: `assessment.md`, `analysis.md`, `decision-log.md`

## Deliverables of this child (execute stage)

1. **Design prototype** — Open Design project `b1a6131b-e247-45b0-937f-4a09ed47a99c`: `index.html` + `app.js` + `assets/` (Promethity fonts, logos). Clickable, both themes, covers every surface below.
2. **Design proposal** — `DESIGN-PROPOSAL.md` in the same project: rationale, clause-by-clause conformance map, per-repo implementation map, assumptions.
3. **OpenSpec changes** — eight changes in `openspec/changes/praxis-*` (this repo). Kubo-owned changes carry spec deltas; webui and desktop changes use `skip_specs: true` per PX-16.

No Go, webui or desktop source edits. Kubo blocking constraints (`make build`, fmt, lint) are unaffected because no Go file changes.

## Ordered changes

| # | Change | Repo / owner | Implements | Depends on |
|---|---|---|---|---|
| 1 | `praxis-decision-model-v2` | kubo `mcp/ui`, `mcp/app` · ui-ux-engineer | D5; WS-05, API-06, CT-05, HOME-10 #15 | — |
| 2 | `praxis-visual-roles` | ipfs-webui `src/promethity/`, kubo `mcp/ui` tokens · ui-ux-engineer | D6; UI-07 roles | — |
| 3 | `praxis-shell-modes` | ipfs-webui `src/App.js`, `src/navigation/`, new `src/shell/` · ui-ux-engineer | D1, D2, D3; WS-01/03/07, HOME-01/03 | 2 |
| 4 | `praxis-decision-inbox` | ipfs-webui `src/activity/` + kubo `mcp/ui` `DecisionInbox` · ui-ux-engineer, mcp-server-engineer (decisions list endpoint) | D4; HOME-07/08, API-06 | 1, 3 |
| 5 | `praxis-context-inspector` | kubo `mcp/chat` island + ipfs-webui context bridge · ui-ux-engineer | D7, D8; WS-06, CX-01, turn anatomy | 3 |
| 6 | `praxis-mcp-app-a2ui-seam` | kubo `mcp/app`, `mcp/ui` + webui mount · mcp-server-engineer | D9; FR-12, UI-02/08/10 | 1 |
| 7 | `praxis-origin-and-cid-honesty` | ipfs-webui `src/agent-chat/`, `src/files/`, `src/pins/` copy · ui-ux-engineer | D10, D11; WEB-06, SEC-06 | 3 |
| 8 | `praxis-desktop-attention` | ipfs-desktop `src/tray.js`, `src/agent/confirm/ui/` · ui-ux-engineer | D12; HOME-07, WS-05 | 1, 4 |

Rounds: R1 = {1, 2}; R2 = {3, 6}; R3 = {4, 5, 7}; R4 = {8}.

## Task model assignments

| Task | Model | Why |
|---|---|---|
| agent-backend-settings::praxis-agentic-ux / prototype | opus | multi-surface interaction design |
| agent-backend-settings::praxis-agentic-ux / design-proposal | opus | conformance reasoning |
| agent-backend-settings::praxis-agentic-ux / openspec-changes | opus (subagent) | structured authoring in repo conventions |

## Acceptance for this child

- Every Praxis clause marked P or X in `assessment.md` is either addressed by a change above or recorded as a deviation in `analysis.md`.
- Every IPFS function in the "MUST survive" list appears in the prototype and in a change's scope.
- Decision model v2 states each appear at least once in the prototype.
- The eight change folders exist with `proposal.md` and `tasks.md`; Kubo-owned changes also have `design.md` and `specs/**/spec.md`.

## Risks

- Brand assumption PX-02 may be overturned → only change 2 is affected.
- Praxis is "Proposed integration"; clause IDs may move in a later edition → changes cite edition 1.2.
- HG-1 still unproven → change 8 keeps the existing Touch ID gate and says so.

## Recalled lessons applied

- From `commerce-removal` reflection: a pass that does not exercise the delivered behaviour is not proof → the prototype is described as unrendered and untested unless the operator asks.
