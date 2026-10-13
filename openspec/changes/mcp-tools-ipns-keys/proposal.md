# Proposal: mcp-tools-ipns-keys

Phase `ipfs-mcp-server`, change 10 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Agents need to publish and resolve IPNS names and manage the keys that sign them (G2). Private key export stays out of scope.

## What Changes

- Tools: `ipns_publish` (write), `ipns_resolve` (untrusted content), `key_list`, `key_gen` (write), `key_rename` (write) and `key_rm` (destructive).
- No key export or import.

## Capabilities

### New Capabilities

- `mcp-ipns-key-tools`: MCP tools for IPNS publish/resolve and key management without key export

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`.
