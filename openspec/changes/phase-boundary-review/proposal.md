# Proposal: phase-boundary-review

Phase `ipfs-mcp-server`, change 21 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G7 requires independent review at the phase boundary. The `goals.md` scope note makes it two-part and both parts blocking: workspace review by the security-reviewer and reviewer subagents, and a cross-family judge via `dispatch-judge.sh`.

## What Changes

- Verify the judge route first: `/Users/gqadonis/.prometheus/plugins/prometheus-skill-pack/generations/6d017e41ae80e34101146cd060e8a14e34361b066e7263d3074466e7c414820d/skills/adversarial-review/scripts/dispatch-judge.sh` exists and is executable, and the liter-llm gateway answers. Otherwise record `pending_review`.
- Run every blocking constraint from `.kbd-orchestrator/constraints.md` and all MCP test/cli suites.
- Independent security review and quality review in separate contexts.
- The cross-family judge (gpt-6.1-sol) over the phase-boundary diff. If it cannot run, the phase stays `pending_review`. There is no same-family fallback.
- One batched correction cycle.
- Operator approval and a commit including AGENTS.md, under the `agents-md-separate-pr` waiver recorded in `.kbd-orchestrator/constraints.md`.

## Capabilities

### New Capabilities

None (`skip_specs: true`, review and gates only).

### Modified Capabilities

None.

## Impact

Findings under `.agent-team/kubo-ipfs-agentic/findings/**` and `.kbd-orchestrator/phases/ipfs-mcp-server/review/`. No new behavior.

## Plan revision 2

Not yet registered, so its tasks are renumbered: new task 1.3 runs the G9-G12 acceptance set, the security review covers the agent endpoints, egress, the confirmation resolvers, the supervised SurrealDB process and both forks, and the four warnings carried from the addendum's cross-model re-review are checked in 1.5.
