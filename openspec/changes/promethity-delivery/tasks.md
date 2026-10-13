# Tasks: promethity-delivery

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `promethity-delivery`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: phase-integration-gate; OG-8, OG-9. Scheduled in Round 11.

## 1. Tasks

- [ ] 1.1 Prepare the CAR of the new Web UI and the re-pin steps for OG-9. Acceptance: CAR root equals the gated CID.
- [ ] 1.2 Build the final linux/amd64 image from the merged kubo commit (new `WebUIPath`) and smoke it like `kubo-candidate-image` 1.3. Acceptance: `ipfs version --commit` equals the merged commit; digest recorded.
- [ ] 1.3 Image-only know-me-cluster PR for OG-9 (WARNING-1: Argo `selfHeal` reverts `kubectl set image`). Acceptance: `git diff` touches only the `image:` line(s); runbook with planned-restart steps and the rollback image (the Round 8 candidate) in the PR body.
- [ ] 1.4 Security-reviewer read of the image PR before OG-9. Acceptance: findings file; no open CRITICAL.
- [ ] 1.5 Install the gated `.app` into `/Applications` (OG-1 pre-approval; quit by bundle id, `ditto`). Acceptance: the installed app's bundled Web UI CID equals the gated CID.
- [ ] 1.6 Update `docs/mcp/progress-report.md` and `docs/PROMETHITY-FORK.md` (W9). Acceptance: entries carry the actual delivery date (ISO, not a placeholder) and the merged commit SHAs of kubo, ipfs-webui, ipfs-desktop and know-me-cluster.
- [ ] 1.7 Record the operator's OG-9 check (console `/webui` serves the new CID; needs credentials, so operator-run). Acceptance: the response root CID recorded verbatim in a new dated evidence folder.
