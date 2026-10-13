# Proposal: ipfs-agent-skills

Phase `ipfs-mcp-server`, change 15 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G5 requires agentskills.io-compliant skills for push/pull, pinning, IPNS, MFS, DAG/CAR and troubleshooting, so agents use the tools correctly, including the confirmation flow.

## What Changes

- Six skills in `mcp/skills`. Each references the contract tool names, explains confirmation, and gives an `ipfs` CLI fallback.
- No skill sets CID-recipe defaults or shells out while holding the repo lock.
- Validation with `skills-ref validate`, plus trigger evaluations.

## Capabilities

### New Capabilities

- `ipfs-agent-skills`: agentskills.io skills that teach agents to use the IPFS MCP tools and CLI fallbacks safely

### Modified Capabilities

None.

## Impact

New `mcp/skills/**`. No code.
