# Proposal: mcp-tools-dag-car

Phase `ipfs-mcp-server`, change 11 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Agents working with IPLD data need DAG access and CAR import/export (G2), with root pinning made explicit rather than hidden.

## What Changes

- Tools: `dag_get`, `dag_stat`, `dag_put` (write), `car_import` (write, explicit `pin_roots`) and `car_export` (`resource_link` or capped base64).

## Capabilities

### New Capabilities

- `mcp-dag-car-tools`: MCP tools for IPLD DAG access and CAR import/export

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`. Tests reuse the existing `test/cli/fixtures/*.car` files.
