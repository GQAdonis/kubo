# Tasks: mcp-tools-routing-diag

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-tools-routing-diag`, backend task IDs 1.1, 1.2. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-pinning-with-confirmation`. Scheduled in Round 4.

## 1. Routing and diagnostics tools

- [x] 1.1 Implement `routing_findprovs`, `routing_findpeer`, `routing_provide`, `swarm_peers`, `stats_bw`, `repo_stat` and `repo_gc`. Verify: unit tests match the contract schemas
- [x] 1.2 Write unit and test/cli tests, including that `Provide.Enabled=false` silences `routing_provide`, and `repo_gc` confirmation. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes against a harness daemon
