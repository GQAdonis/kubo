# Proposal: mcp-plugin-marketplaces

Phase `ipfs-mcp-server`, change 19 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G5 packaging this phase covers Claude Code, Codex and OpenCode (`goals.md` "## Scope changes (operator-approved)", G5). Each plugin bundles the skills and launches `ipfs mcp`. Kimi plugin packaging is out of scope.

## What Changes

- Verify each target's manifest format against primary documentation first. All three formats are unverified.
- Claude Code, Codex and OpenCode plugins and marketplaces, launching `{"command":"ipfs","args":["mcp"]}`, with an optional remote HTTP entry.
- Install and load each plugin in the real harness.

## Capabilities

### New Capabilities

- `ipfs-agent-plugins`: plugin and marketplace packages that install the IPFS skills and register `ipfs mcp` in Claude Code, Codex and OpenCode

### Modified Capabilities

None.

## Impact

New `.claude-plugin/**`, `.agents/plugins/**`, `mcp/plugin/**`.
