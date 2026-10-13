# Proposal: mcp-tools-mfs

Phase `ipfs-mcp-server`, change 9 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Agents organize content through MFS (G2). These commands have no typed CoreAPI, so output decoding must be pinned against a real daemon.

## What Changes

- Tools: `mfs_ls`, `mfs_stat`, `mfs_read`, `mfs_write`, `mfs_mkdir`, `mfs_cp`, `mfs_mv` and `mfs_rm` (destructive).
- All go through the raw RPC `files/*` commands.

## Capabilities

### New Capabilities

- `mcp-mfs-tools`: MCP tools for the Mutable File System of the connected node

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`.
