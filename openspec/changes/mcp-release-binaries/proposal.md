# Proposal: mcp-release-binaries

Phase `ipfs-mcp-server`, change 17 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The operator wants release binaries (answer 6). Users on stock Kubo use the fork's `ipfs` binary as an MCP client to their existing node. Publishing and tagging are not part of this phase.

## What Changes

- A local script in `mcp/release` that cross-compiles the fork's `ipfs` through `make build` (ldflags preserved) for linux/darwin/windows amd64/arm64.
- Archives and SHA-256 checksums.
- A release-notes template with the stock-Kubo guidance.
- A smoke test of the host-architecture binary against a harness daemon.
- No `.github` workflow, tags, `version.go` or `bin/mkreleaselog` changes. No publishing.

## Capabilities

### New Capabilities

- `ipfs-release-artifacts`: locally built fork `ipfs` release archives with checksums and release notes for MCP use

### Modified Capabilities

None.

## Impact

New `mcp/release/**`. Kubo release tooling is untouched (AGENTS.md Scope and Safety).
