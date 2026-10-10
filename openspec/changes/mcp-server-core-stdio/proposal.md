# Proposal: mcp-server-core-stdio

Phase `ipfs-mcp-server`, change 4 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The operator goal is MCP server support in the Kubo `ipfs` CLI. This change delivers the first working `ipfs mcp` end to end over stdio against a real daemon. It also sets the per-group registration pattern that later tool changes extend.

## What Changes

- Package `github.com/ipfs/kubo/mcp/server`, which builds a go-sdk v1.8.0 server from the embedded contract and refuses to start when a tool has no matching contract entry.
- `ipfs mcp`, a CLI-only local command (`cmd/ipfs/kubo` `localCommands`, `SetDoesNotUseRepo`). It is not part of `/api/v0`.
- Daemon discovery from `--api`, `$IPFS_PATH/api` or a URL, with `--api-auth`. It never opens the repo.
- Version gate via `/api/v0/version`, and per-tool degradation via `/api/v0/commands` for stock Kubo daemons.
- Tools `ipfs_node_info` and `ipfs_node_status`, and the capability flags `--tools` and `--read-only`.
- A `nomcp` build tag that compiles the command out.

## Capabilities

### New Capabilities

- `ipfs-mcp-command`: the `ipfs mcp` command: stdio MCP server lifecycle, daemon discovery, version gating, per-tool degradation and capability switches

### Modified Capabilities

None.

## Impact

New `mcp/server/**`, `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*_test.go`. Root `go.mod`/`go.sum` gain go-sdk v1.8.0 and its modules (accepted, answer 2). `/api/v0`, the gateway, CID defaults and wire behavior are unchanged.
