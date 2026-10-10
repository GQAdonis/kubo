# Tasks: webmcp-tool-package

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `webmcp-tool-package`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-tool-contract`, `mcp-pinning-with-confirmation`, `webmcp-host-spike`. Scheduled in Round 4.

## 1. WebMCP tool package

- [ ] 1.1 Write the generator from `mcp/contract` to TypeScript definitions with derived WebMCP annotations, excluding destructive tools. Verify: the generated output is checked in, and a drift test passes
- [ ] 1.2 Implement executors over `kubo-rpc-client` and the confirmation hook interface. Verify: unit tests show no RPC call happens when the hook declines
- [ ] 1.3 Write unit tests, plus integration tests against a real `ipfs` daemon started from `PATH` with a temporary `IPFS_PATH` and non-default ports. Verify: the package test command passes locally
