# Tasks: mcp-tools-ipns-keys

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-tools-ipns-keys`, backend task IDs 1.1, 1.2. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-pinning-with-confirmation`. Scheduled in Round 4.

## 1. IPNS and key tools

- [x] 1.1 Implement `ipns_publish`, `ipns_resolve`, `key_list`, `key_gen`, `key_rename` and `key_rm`, with no export or import. Verify: unit tests match the contract schemas
- [x] 1.2 Write test/cli real-daemon tests, including publish/resolve and `key_rm` confirmation. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes against a harness daemon
