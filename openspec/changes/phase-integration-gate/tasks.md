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

## Plan revision 4 amendment (OG-16, 2026-10-14)

- Dependencies add the eight goal-6 changes (`vector-search-contract`, `mcp-content-extractor`, `mcp-media-describer`, `mcp-whole-node-indexing`, `webui-vector-search`, `desktop-search-sidecar`, `vector-search-security-review`, `lke-vector-search`; the last only if OG-21 passed, otherwise carried open like `agent-backend-uar` under D13).
- Task 1.2 adds `go test ./mcp/...` for the new packages and the `test/cli` tests `TestMCPExtractorDisabled`, `TestMCPDescriberDisabled`, `TestMCPBulkIndexNoInjection`, `TestMCPBulkIndexDisabled`, `TestMCPIndexSearchRouteDisabled`.
- Task 1.4 adds the Web UI `search.test.js` e2e and axe run; task 1.3 adds the Desktop search on and off checks.
- Task 1.5 adds the xberg real-service test (PR-13), and the describer test only if OG-17 allowed one (PR-15).
- Tasks 1.6 and 1.7: reviewer and security-reviewer scopes add ADR-0011 and the eight changes; the disabled-state evidence lists every new off switch.
