# Proposal: mcp-tools-content

Phase `ipfs-mcp-server`, change 8 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Push/pull of content is the most basic agent need (G2). It must preserve the default CID recipe and avoid local-filesystem exfiltration.

## What Changes

- `ipfs_add`: bytes or text only, with a size cap. It is a confirmed write, takes explicit `pin`/`provide` parameters and uses Kubo's default recipe.
- `ipfs_cat`: offset and length, with a size cap; returns untrusted content.
- `ipfs_get` and `ipfs_ls`, returning `resource_link` for large or directory content.
- Resource handlers for `ipfs://` and `ipns://`.

## Capabilities

### New Capabilities

- `mcp-content-tools`: MCP tools and resources for adding, reading, listing and fetching UnixFS content

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`. Adding content from local file paths is deliberately not supported.
