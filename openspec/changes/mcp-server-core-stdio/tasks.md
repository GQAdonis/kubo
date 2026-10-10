# Tasks: mcp-server-core-stdio

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-server-core-stdio`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-tool-contract`. Scheduled in Round 2.

## 1. Core server and stdio

- [x] 1.1 Implement server construction from the embedded contract, per-group registration files and the startup contract check. Verify: a unit test fails startup when a registered tool lacks a contract entry
- [x] 1.2 Implement daemon discovery (`--api`, `$IPFS_PATH/api`, URL, `--api-auth`), the version gate and per-tool degradation via `/api/v0/commands`, without opening the repo. Verify: unit tests with a fake RPC cover a missing command and an old version
- [x] 1.3 Add the `ipfs mcp` local command with raw stdio, stderr-only logging and a `nomcp` stub. Verify: `make build` succeeds and `ipfs mcp --help` prints usage
- [x] 1.4 Implement `ipfs_node_info` and `ipfs_node_status` per the contract, including the list of unavailable tools. Verify: unit tests check structured output against the contract output schemas
- [x] 1.5 Implement the `--tools` and `--read-only` flags. Verify: unit tests show that disabled groups and write tools are absent from `tools/list`
- [x] 1.6 Add test/cli tests against a harness daemon: stdio initialize/list/call, byte-exact stdout, daemon-absent error, no `repo.lock` held, version gate, disabled group, and `make build GOTAGS=nomcp`. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes
- [x] 1.7 Write the `ipfs mcp` help text. Verify: `go test ./test/cli/... -run TestCommandDocsWidth` passes
