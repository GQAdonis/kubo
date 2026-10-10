# Proposal: mcp-agent-principal-enforcement

Phase `ipfs-mcp-server`, plan revision 2 (new change). N8: principal kinds, egress acknowledgement, check-uar and the stdio guard. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The guarantees in G10 and G12 (no default endpoint, no content leaves the machine without operator configuration) and the confirmation guarantee must be enforced by code Kubo ships, not delegated to an agent runtime. The addendum adjudication added three requirements: re-check UAR per session, refuse shell-capable tools for write principals, and count only published UAR releases.

## What Changes

- Principal policy: principals are configured entities with a kind (`human-mcp`, `ui`, `a2a`, `uar`), read-only by default, with write and destructive groups granted by name. A token that maps to no configured principal gets nothing.
- Egress acknowledgement: tools marked `contentEgress` are refused to a principal without an `egress_ack` naming the model and embedder hosts it may forward to; ipfs mcp's own provider and embedder hosts are checked against their sections. Refusals are structured errors, never empty results.
- `ipfs mcp check-uar <url>` and automatic re-checks for `kind: uar` at every session initialize and every run that uses UAR as provider: agent-card version at or above the profile minimum (published releases only), authentication required, effective model, embedders and governance from the settings API (admin credential from env), and write groups refused when `terminal_exec` or any non-Kubo MCP server is configured. Unreadable settings fail closed unless the operator records `check: assert`.
- Shell-capable refusal: when `--confirm=cli` is enabled, every write principal needs an operator assertion `shell_capable: false`; otherwise its write groups are refused.
- Stdio guard: `ipfs mcp` refuses `contentEgress` tools to a stdio client whose MCP `clientInfo` identifies UAR (stdio-spawned UAR is forbidden; this is a misconfiguration guard, not a security boundary, because `clientInfo` is self-reported).

## Capabilities

### New Capabilities

- `mcp-agent-principals`: N8: principal kinds, egress acknowledgement, check-uar and the stdio guard

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `cmd/ipfs/kubo/mcp*.go` (the `check-uar` subcommand), `test/cli/mcp_*_test.go`. No daemon, `config/` or `docs/config.md` change.
