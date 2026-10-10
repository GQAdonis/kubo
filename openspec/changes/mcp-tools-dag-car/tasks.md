# Tasks: mcp-tools-dag-car

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-tools-dag-car`, backend task IDs 1.1, 1.2. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-pinning-with-confirmation`. Scheduled in Round 4.

## 1. DAG and CAR tools

- [x] 1.1 Implement `dag_get`, `dag_stat`, `dag_put`, `car_import` (explicit `pin_roots`) and `car_export`. Verify: unit tests match the contract schemas
- [x] 1.2 Write unit and test/cli tests using the existing `test/cli/fixtures/*.car`. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes against a harness daemon
