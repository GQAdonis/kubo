# Tasks: mcp-agui-a2ui-registry

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `mcp-agui-a2ui-registry`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: agent-settings-contract. Scheduled in Round 2.

## 1. Tasks

- [ ] 1.1 Serve the embedded catalog (`mcp/ui/catalog/ipfs-mcp.catalog.json`) at the contract route and advertise it in the run stream. Acceptance: route and event in the contract; no catalog copy outside the generated file.
- [ ] 1.2 Emit A2UI v0.9 surface messages as `CUSTOM` events built server-side per `mcp/contract/derivations/a2ui-components.json`. Acceptance: a test asserts every emitted component name is in the catalog.
- [ ] 1.3 Island renders server-emitted surfaces and keeps the local ToolCallCard fallback. Acceptance: fixture replay test with and without server surfaces.
- [ ] 1.4 liter-llm real-service test (R7): a `test/cli` test that runs only when `IPFS_MCP_TEST_LLM_BASE_URL` and a key env name are set (operator-configured, never defaulted). Acceptance: skips with a clear message when unset; required gate item.
- [ ] 1.5 `agent.a2ui` off switch and disabled-state test (W6). Acceptance: a test asserts no A2UI `CUSTOM` events when off.
