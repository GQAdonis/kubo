# Tasks: uar-conformance-request

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `uar-conformance-request`, backend task IDs 1.1, 1.2. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `team-ownership-grant`. Scheduled in Round 1.

## 1. Cross-team request

- [x] 1.1 Write `mcp/uar/requests/CONFORMANCE.md` with every requested item, its acceptance check and the published-release-only gate rule. Verify: every item names an observable check that can be run against a published UAR release
- [ ] 1.2 The operator files the request with the UAR team and records the issue or PR link in `mcp/uar/requests/CONFORMANCE.md`. Verify: the link is recorded with the date
