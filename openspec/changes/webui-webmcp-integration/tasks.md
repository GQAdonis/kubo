# Tasks: webui-webmcp-integration

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `webui-webmcp-integration`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `webmcp-tool-package`, `webmcp-host-spike`. Scheduled in Round 5.

## 1. Web UI WebMCP integration

- [ ] 1.1 Add the `src/webmcp/` bundle in the fork, registering `mcp/webmcp` tools only when the toggle is on and `document.modelContext` exists. Verify: Jest tests with a mocked `modelContext` show no registration when either condition is false
- [ ] 1.2 Add the Settings toggles (off by default) and the in-page confirmation modal. Verify: Jest tests show that cancel sends no RPC request
- [ ] 1.3 Run Jest with the mocked `modelContext`, plus `npm run eslint` and typecheck per the fork's `AGENTS.md`. Verify: all three pass
- [ ] 1.4 The operator runs the fork build in each spike-GO host. Verify: the transcript shows a confirmed write and a read tool call with results
