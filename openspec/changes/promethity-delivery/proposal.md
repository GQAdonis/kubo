# Proposal: promethity-delivery

Phase `agent-backend-settings`, change 28 of 28 in plan revision 3, Round 11. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (outward part of UI-70).

## Why

After the gate and the operator's merges, the new Web UI reaches LKE through an image-only PR and a CAR re-pin, Desktop is installed, and the progress report is updated.

## What Changes

- 1.1: Prepare the CAR of the new Web UI and the re-pin steps for OG-9.
- 1.2: Build the final linux/amd64 image from the merged kubo commit (new `WebUIPath`) and smoke it like `kubo-candidate-image` 1.3.
- 1.3: Image-only know-me-cluster PR for OG-9 (WARNING-1: Argo `selfHeal` reverts `kubectl set image`).
- 1.4: Security-reviewer read of the image PR before OG-9.
- 1.5: Install the gated `.app` into `/Applications` (OG-1 pre-approval; quit by bundle id, `ditto`).
- 1.6: Update `docs/mcp/progress-report.md` and `docs/PROMETHITY-FORK.md` (W9).
- 1.7: Record the operator's OG-9 check (console `/webui` serves the new CID; needs credentials, so operator-run).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: builds, pins and installs already-specified artifacts; the only kubo edit is the operator-approved WebUIPath value.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer (1.1, 1.5, 1.6 fork part); mcp-server-engineer (1.2); ipfs-agent-architect (1.3, 1.6 kubo part, 1.7); security-reviewer (1.4)
- Scope: Desktop install, LKE inputs, progress report
- Depends on: phase-integration-gate; OG-8, OG-9
- Size: M; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
