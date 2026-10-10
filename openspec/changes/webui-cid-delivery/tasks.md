# Tasks: webui-cid-delivery

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `webui-cid-delivery`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `webui-webmcp-integration`. Scheduled in Round 6.

## 1. Web UI delivery

- [ ] 1.1 The operator selects D1, D2 or D3 after the spike report. Verify: the choice is recorded in `operator-answers.md` with the date
- [ ] 1.2 If D2: bump `WebUIPath`/`WebUIPaths` with test/cli checks of the `/webui` redirect and `Gateway.NoFetch`. Otherwise, write the D1/D3 docs in `docs/mcp/`. Verify: D2 `make build && go test ./test/cli/... -run TestWebUI -v` passes; for D1/D3, the docs are reviewed by the operator
- [ ] 1.3 If D2: add a changelog Highlights entry (TOC and Highlights only). Verify: no auto-generated changelog section is modified
