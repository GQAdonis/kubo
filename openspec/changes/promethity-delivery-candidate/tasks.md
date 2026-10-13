# Tasks: promethity-delivery-candidate

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `promethity-delivery-candidate`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: every UI change, agent-features-default-on, mcp-agui-a2ui-registry, settings-security-review, kubo-candidate-image; OG-7 for 1.3. Scheduled in Round 9.

## 1. Tasks

- [ ] 1.1 Production Web UI build and its CID. Acceptance: CID recorded with the build commit.
- [ ] 1.2 Local retrieval check of the CID on a non-default-port node. Acceptance: `ipfs dag stat` on the CID succeeds there.
- [ ] 1.3 After OG-7, set `WebUIPath` to the new CID on the branch and `make build`. Acceptance: a one-line diff in `webui.go`, made after the `kubo-candidate-image` 1.1 build so the OG-12 image does not point at an unpinned CID.
- [ ] 1.4 Package Desktop with the candidate Web UI and fork binary (not installed). Acceptance: `.app` built under the Desktop `dist/`.
