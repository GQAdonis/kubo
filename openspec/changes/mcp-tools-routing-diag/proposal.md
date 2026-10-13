# Proposal: mcp-tools-routing-diag

Phase `ipfs-mcp-server`, change 12 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Troubleshooting and provide/find operations are part of G2. `repo gc` is destructive, and provide must honor the operator's provide settings.

## What Changes

- Tools: `routing_findprovs`, `routing_findpeer`, `routing_provide` (write), `swarm_peers`, `stats_bw`, `repo_stat` and `repo_gc` (destructive).

## Capabilities

### New Capabilities

- `mcp-routing-diag-tools`: MCP tools for content routing, providing, and node diagnostics including garbage collection

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`.
