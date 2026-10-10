# Proposal: webmcp-host-spike

Phase `ipfs-mcp-server`, change 3 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The operator wants WebMCP support for Codex desktop and Kimi desktop now. Whether either host exposes `document.modelContext` to pages in its embedded browser is unverified. G8 implementation must not start on an assumption.

## What Changes

- A static, no-side-effect probe page (`mcp/webmcp/spike/`). It feature-detects `document.modelContext` and legacy `navigator.modelContext`, registers an `echo` tool that returns a nonce, and reports secure-context status, user agent and permissions policy.
- The operator runs the probe in Codex desktop and in Kimi desktop.
- **GO criteria, per host, all required:** `document.modelContext` is present; registration of `echo` succeeds; the host agent invokes it; the agent transcript contains the returned nonce.
- MCP or MCP App support in the host is recorded as fallback evidence only and is NOT WebMCP GO.

## Capabilities

### New Capabilities

None. This is a spike with no product behavior (`skip_specs: true`).

### Modified Capabilities

None.

## Impact

`mcp/webmcp/spike/` (probe page and report). It gates `webmcp-tool-package`, `webui-webmcp-integration` and `webui-cid-delivery`. A NO-GO for both hosts defers those three changes, and ADR-0002 is recorded as Deferred.
