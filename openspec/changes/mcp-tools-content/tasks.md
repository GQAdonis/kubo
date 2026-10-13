# Tasks: mcp-tools-content

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-tools-content`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-pinning-with-confirmation`. Scheduled in Round 4.

## 1. Content tools

- [x] 1.1 Implement `ipfs_add` (bytes/text, cap, explicit pin/provide, confirmed write), `ipfs_cat` (range, cap), `ipfs_get` and `ipfs_ls`. Verify: unit tests with a fake RPC match the contract schemas
- [x] 1.2 Implement the `ipfs://` and `ipns://` resource handlers and `resource_link` results for large or directory content. Verify: unit tests resolve both URI forms
- [x] 1.3 Write test/cli real-daemon tests, including CID equality with `ipfs add` defaults and confirmation on add. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes against a harness daemon
