# Proposal: webmcp-tool-package

**CONDITIONAL:** this change is applied only if `webmcp-host-spike` reports GO for at least one host (all four GO criteria met). If both hosts are NO-GO, the change is deferred and ADR-0002 is recorded as Deferred.

Phase `ipfs-mcp-server`, change 13 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G8 requires WebMCP tools in the Web UI that share the MCP contract. A framework-free package generated from `mcp/contract` keeps one source of truth across the Go server and the browser. The Web UI is React 16 and the MCP App is React 19, so the package must not depend on React.

## What Changes

- `mcp/webmcp`: TypeScript tool definitions generated from the contract, with WebMCP annotations derived per the contract rules.
- Executors over `kubo-rpc-client`, on the same major version as the Web UI fork (^6.1.0).
- A confirmation hook interface that the host page must implement. Write and destructive tools cannot run without it.

## Capabilities

### New Capabilities

- `webmcp-tools`: framework-free WebMCP tool definitions and RPC executors generated from the shared contract

### Modified Capabilities

None.

## Impact

New `mcp/webmcp/**` with its own JS toolchain. No Go changes.
