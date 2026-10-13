# Proposal: agent-backend-uar

Phase `agent-backend-settings`, change 22 of 28 in plan revision 3, Round 6. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (ADR-0010; supersedes `agentic-uar-integration` and `uar-integration-spike` 1.2-1.5; D13, D14).

## Why

Goal 2: a UAR agent with its own prompt, the kubo MCP tools and the IPFS skills, reached through `ipfs mcp /agui` so the island contract stays unchanged. Gated on a published UAR release.

## What Changes

- 1.1: Mark the two superseded changes and write ADR-0010.
- 1.2: `backend.kind: uar`: provision the agent against the released API (W9).
- 1.3: Proxy AG-UI runs and translate interrupts (`tool_approval`, `mcp_elicitation`) to the kubo shape.
- 1.4: Pin `uar-preflight.json` to the release tag and digest; set `check-uar` minimum version.
- 1.5: Real-service test against the published release.
- 1.6: Enable the UAR form in the settings page against the real API.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

- `mcp-agent-backend-uar`: see `specs/mcp-agent-backend-uar/spec.md`.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect (1.1); mcp-server-engineer (1.2-1.5); ui-ux-engineer (1.6)
- Scope: `docs/mcp/0010-*.md`, superseded change folders, `mcp/server/**`, `mcp/uar/**`, `test/cli/mcp_uar_*`, Web UI settings page
- Depends on: agent-settings-contract, uar-conformance-release, mcp-agent-hot-reload, mcp-agent-settings-api, webui-agent-settings-page (W4); tasks 1.2-1.6 also OG-4
- Size: L; goals: 2
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
