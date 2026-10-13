# Tasks: phase-integration-gate

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `phase-integration-gate`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 1.10. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: every change in Rounds 1-9 (`agent-backend-uar` only if OG-4 passed); OG-13 for 1.5. Scheduled in Round 10.

## 1. Tasks

- [ ] 1.1 New dated evidence folder with README; gate plan lists every acceptance check. Acceptance: README maps each check to an output file.
- [ ] 1.2 Kubo: `make mod_tidy` if any `go.mod` changed, `go fmt`, `make build`, `make -O test_go_lint`, `go test ./mcp/...`, `test/cli` MCP tests on non-default ports, `TestCommandDocsWidth`, stability pins (`TestDefaultMatchesExpectedProfile`, `test/cli/gateway_test.go`, `TestTelemetry`). Acceptance: outputs saved; no `/api/v0`, gateway, CID or wire change in the diff.
- [ ] 1.3 Packaged Desktop launch from the candidate `.app`: sidecar up, port discovery with 5050 occupied, settings apply without restart, run in flight finishes on the old backend, Touch ID approve, sidecar off switch; closes carry-over A2. Acceptance: transcript and screenshots.
- [ ] 1.4 Web UI: Jest, Playwright e2e, axe light and dark for every route and settings state, keyboard traversal, reduced motion, screenshots 320/768/1024/1440 in both themes, bundle size, no-remote-asset scan. Acceptance: reports saved.
- [ ] 1.5 Real services: liter-llm (agent-run); LKE after OG-13, operator-run because it needs the basic-auth password and bearers (WARNING-4): re-run both `lke-agent-settings` 1.8 parts; carry-over A4 on the cloud console per S2: a chat read, a **non-destructive** write (`ipfs_pin_add` of the already-pinned Web UI CID `bafybeiblix77cfttmrmz5h54qacay5f4z5bgajc736ufiyoy5cus54stki`) confirmed by the island interrupt gesture, and a destructive chat request refused with `confirmation_unavailable` (expected); A3 WebMCP in Codex and Kimi (operator-run; inconclusive runs recorded as deferred); UAR if OG-4 (agent-run). Acceptance: each item pass, fail or deferred with evidence; the agent records operator output verbatim.
- [ ] 1.6 Independent `reviewer` (Claude Code, separate context). Acceptance: findings file written by the relaying architect, verbatim.
- [ ] 1.7 Independent `security-reviewer` (Claude Code, separate context). Acceptance: findings file written verbatim.
- [ ] 1.8 One batched correction by the owning roles. Acceptance: each finding linked to a commit or rebuttal.
- [ ] 1.9 Confirmation by both reviewers. Acceptance: no open CRITICAL.
- [ ] 1.10 Close carry-over in the old phase and reconcile runtime tasks by begin/end (gotcha 2026-10-10); record goal 2 status for OG-10. Acceptance: `prometheus kbd status --json` shows no pending task the evidence covers.
