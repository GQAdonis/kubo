# Tasks: mcp-tools-mfs

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-tools-mfs`, backend task IDs 1.1, 1.2. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-pinning-with-confirmation`. Scheduled in Round 4.

## 1. MFS tools

- [x] 1.1 Implement the `mfs_*` tools via `HttpApi.Request("files/...")` with decoding to the contract schemas. Verify: unit tests decode recorded `files/*` responses
- [x] 1.2 Write test/cli real-daemon tests for decoding and for destructive `mfs_rm` confirmation. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes against a harness daemon
