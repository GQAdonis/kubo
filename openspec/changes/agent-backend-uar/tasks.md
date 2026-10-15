# Tasks: agent-backend-uar

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `agent-backend-uar`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: agent-settings-contract, uar-conformance-release, mcp-agent-hot-reload, mcp-agent-settings-api, webui-agent-settings-page (W4); tasks 1.2-1.6 also OG-4. Scheduled in Round 6.

## 1. Tasks

- [x] 1.1 Mark the two superseded changes and write ADR-0010. Acceptance: both old `tasks.md` carry a superseded note; the ADR lists the release gate.
- [ ] 1.2 `backend.kind: uar`: provision the agent against the released API (W9). Acceptance: a fixture test against recorded release responses, and the real-service run of 1.5 shows `GET /api/agents/{id}` returning a system prompt equal to kubo's configured prompt (prefix included), an MCP binding naming the kubo server, and skill bindings equal to the directory names under `mcp/skills/`.
- [ ] 1.3 Proxy AG-UI runs and translate interrupts (`tool_approval`, `mcp_elicitation`) to the kubo shape. Acceptance: fixture tests for both interrupt kinds.
- [ ] 1.4 Pin `uar-preflight.json` to the release tag and digest; set `check-uar` minimum version. Acceptance: no `null` in the preflight file.
- [ ] 1.5 Real-service test against the published release. Acceptance: required gate item once OG-4 has passed.
- [ ] 1.6 Enable the UAR form in the settings page against the real API. Acceptance: no mocked UAR state in production code.
