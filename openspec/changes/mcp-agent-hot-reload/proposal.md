# Proposal: mcp-agent-hot-reload

Phase `agent-backend-settings`, change 5 of 28 in plan revision 3, Round 2. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: none.

## Why

Today the agent backend is fixed at startup: config read once, agent built once, handlers capture the agent, and `/agui` is not mounted without a backend. Goal 1 requires that saving settings applies without a manual restart, so the backend must be swappable while runs are in flight.

## What Changes

- 1.1: Hold `LLMClient`, `backendHost`, limits and system prompt in one snapshot behind `atomic.Pointer`, read once per run; AG-UI and A2A read the same pointer; `sessions` and `buckets` survive swaps.
- 1.2: A resumed interrupt reuses its run's original snapshot or fails with a stable code; the previous key is dropped once no run uses it; no pending confirmation is lost across a swap (R3).
- 1.3: Mount `/agui` whenever enabled; with no backend it answers 503 `backend_not_configured`; update help text (`mcp_agent.go:39-56`) and the schema text.
- 1.4: Configurable system prompt with a fixed, non-removable safety prefix.
- 1.5: Internal apply entry point (`ApplyAgentSettings(snapshot)`) using the same validation as `ResolveAgent`, plus a change-notification hook.
- 1.6: Unit and `test/cli` tests for 1.1-1.5, plus disabled states (`--agui` off: 404; `GOTAGS=noagent` builds without the route).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

- `mcp-agent-runtime-settings`: see `specs/mcp-agent-runtime-settings/spec.md`.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer
- Scope: `mcp/server/**`, `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*`
- Depends on: agent-settings-contract
- Size: M-L; goals: 1, 3
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
