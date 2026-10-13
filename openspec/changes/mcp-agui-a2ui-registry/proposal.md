# Proposal: mcp-agui-a2ui-registry

Phase `agent-backend-settings`, change 6 of 28 in plan revision 3, Round 2. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (D15).

## Why

Goal 3 asks the proxy to serve the A2UI control registry over AG-UI. The server emits no A2UI today; the island builds surfaces locally. This change serves the catalog and emits server-built surfaces, and adds the liter-llm real-service check.

## What Changes

- 1.1: Serve the embedded catalog (`mcp/ui/catalog/ipfs-mcp.catalog.json`) at the contract route and advertise it in the run stream.
- 1.2: Emit A2UI v0.9 surface messages as `CUSTOM` events built server-side per `mcp/contract/derivations/a2ui-components.json`.
- 1.3: Island renders server-emitted surfaces and keeps the local ToolCallCard fallback.
- 1.4: liter-llm real-service test (R7): a `test/cli` test that runs only when `IPFS_MCP_TEST_LLM_BASE_URL` and a key env name are set (operator-configured, never defaulted).
- 1.5: `agent.a2ui` off switch and disabled-state test (W6).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

- `mcp-agui-a2ui`: see `specs/mcp-agui-a2ui/spec.md`.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer (1.1, 1.2, 1.4, 1.5); ui-ux-engineer (1.3)
- Scope: `mcp/server/**`, `mcp/chat/**`, `test/cli/mcp_*`
- Depends on: agent-settings-contract
- Size: M; goals: 3
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
