# Proposal: phase-integration-gate

Phase `agent-backend-settings`, change 27 of 28 in plan revision 3, Round 10. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-60 plus the backend gate).

## Why

One integration gate at the phase boundary through the real production paths, with independent reviewer and security-reviewer passes and one correction cycle.

## What Changes

- 1.1: New dated evidence folder with README; gate plan lists every acceptance check.
- 1.2: Kubo: `make mod_tidy` if any `go.mod` changed, `go fmt`, `make build`, `make -O test_go_lint`, `go test ./mcp/...`, `test/cli` MCP tests on non-default ports, `TestCommandDocsWidth`, stability pins (`TestDefaultMatchesExpectedProfile`, `test/cli/gateway_test.go`, `TestTelemetry`).
- 1.3: Packaged Desktop launch from the candidate `.app`: sidecar up, port discovery with 5050 occupied, settings apply without restart, run in flight finishes on the old backend, Touch ID approve, sidecar off switch; closes carry-over A2.
- 1.4: Web UI: Jest, Playwright e2e, axe light and dark for every route and settings state, keyboard traversal, reduced motion, screenshots 320/768/1024/1440 in both themes, bundle size, no-remote-asset scan.
- 1.5: Real services: liter-llm (agent-run); LKE after OG-13, operator-run because it needs the basic-auth password and bearers (WARNING-4): re-run both `lke-agent-settings` 1.8 parts; carry-over A4 on the cloud console per S2: a chat read, a **non-destructive** write (`ipfs_pin_add` of the already-pinned Web UI CID `bafybeiblix77cfttmrmz5h54qacay5f4z5bgajc736ufiyoy5cus54stki`) confirmed by the island interrupt gesture, and a destructive chat request refused with `confirmation_unavailable` (expected); A3 WebMCP in Codex and Kimi (operator-run; inconclusive runs recorded as deferred); UAR if OG-4 (agent-run).
- 1.6: Independent `reviewer` (Claude Code, separate context).
- 1.7: Independent `security-reviewer` (Claude Code, separate context).
- 1.8: One batched correction by the owning roles.
- 1.9: Confirmation by both reviewers.
- 1.10: Close carry-over in the old phase and reconcile runtime tasks by begin/end (gotcha 2026-10-10); record goal 2 status for OG-10.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: independent review and verification; no behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect coordinates; builders run checks on Kimi Code; reviewer and security-reviewer on Claude Code
- Scope: all repos
- Depends on: every change in Rounds 1-9 (`agent-backend-uar` only if OG-4 passed); OG-13 for 1.5
- Size: L; goals: 1-5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
