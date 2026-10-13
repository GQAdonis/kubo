# Proposal: team-ownership-grant

Phase `ipfs-mcp-server`, change 0 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Every later change in phase `ipfs-mcp-server` writes paths that need an explicit team grant: the root `go.mod`/`go.sum`, `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*`, `mcp/webmcp/**`, `mcp/release/**` and the ipfs-webui fork. Today the role instructions still describe a separate Go module and opt-in destructive tools. Both are superseded by decision F4 and operator answers 4-5. This change runs alone in Round 0 so nothing starts on stale ownership.

## What Changes

- Ask the operator one batched approval question: Q1 approve F4 with its flip conditions; Q2 the confirmation fallback for clients without elicitation. Q3 (the Web UI delivery path) is raised here and decided after the spike.
- Record the complete resulting grant table (existing plus new) in `.agent-team/kubo-ipfs-agentic/team.json`, as stated in the plan's CHANGE LIST item 0.
- Rewrite the role instructions superseded by F4, for example `.codex/agents/mcp_server_engineer.toml:3`.
- Re-export the native role definitions (`.claude/agents`, `.codex/agents`, `.kimi-code/agents`, `.opencode/agents`) and check that they are consistent.

## Capabilities

### New Capabilities

None. This change is team governance only, so `skip_specs: true` is set. It changes no system behavior.

### Modified Capabilities

None.

## Impact

Team state only (`.agent-team/**` and the native role exports). No Kubo code changes. Every Round 1+ change depends on this one (prerequisite PR-1).

## Plan revision 2

Tasks 1.1-1.4 are registered and keep their titles. Q2 (confirmation fallback) is already answered in `operator-answers.md` (native OS prompt for agents that cannot elicit), so 1.1 asks only what is still open: Q1 (approve F4), Q3 (raised, decided after the spike) and the revision-2 items R1-R6 listed in the plan's Round 0 section. Tasks 1.5-1.6 add the grants for the new paths: `mcp/uar/**`, `mcp/chat/**`, Web UI fork `src/agent-chat/**`, and the Desktop fork paths owned by ui-ux-engineer (operator answer Q7). `cmd/ipfs/kubo/mcp*.go` already covers the new `ipfs mcp confirm`, `deny` and `check-uar` subcommands.
