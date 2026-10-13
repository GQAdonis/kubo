# Proposal: mcp-user-docs

Phase `ipfs-mcp-server`, change 20 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Every feature needs documentation of how to use it and how to turn it off (AGENTS.md User Agency). Operators need the confirmation limits and the remote-exposure guidance.

## What Changes

- A `docs/mcp` guide covering:
- Installation from local release binaries, and stdio and HTTP client configuration.
- Static bearer authentication and least-privilege `API.Authorizations`.
- Confirmation behavior and its limits (T2).
- Use against stock Kubo and IPFS Desktop daemons.
- Every off switch: tool groups, `--read-only`, HTTP off by default, `nomcp`.
- A `docs/changelogs/v0.44.md` TOC and Highlights entry.
- A check that `docs/config.md` needs no entry, because no config keys were added.

## Capabilities

### New Capabilities

None (`skip_specs: true`, documentation only).

### Modified Capabilities

None.

## Impact

`docs/mcp/**`, `mcp/README.md` (links), `docs/changelogs/v0.44.md` (TOC and Highlights only).

## Plan revision 2

Plan revision 2 adds tasks 1.4-1.6: the agentic guide (G9-G11), the embeddings guide (G12) and the chat setup. Task 1.3 now writes `docs/mcp/config-reference.md`: the `ipfs mcp` flags and the config file named by `--mcp-config` (or `IPFS_MCP_CONFIG`) are new configuration, so every option is documented with its default, off switch and disabled-state test. No Kubo repo config key (`config/`, `docs/config.md`) is added by any change in this phase; if one were, that change would add its `docs/config.md` entry.
