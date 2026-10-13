# Proposal: agent-chat-island-integration

Phase `agent-backend-settings`, change 18 of 28 in plan revision 3, Round 5. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-41).

## Why

The chat island must follow the shell theme, show a 'pick a model' state when no backend is configured, and match the new launcher style.

## What Changes

- 1.1: `theme` attribute from the shell theme.
- 1.2: Empty state on 503 `backend_not_configured` with a link to `#/settings/agent`.
- 1.3: Launcher restyle.
- 1.4: Rebuild `mcp/chat` and re-vendor into `public/agent-chat/` with the hash manifest.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `mcp/chat/**`, `../ipfs-webui/src/agent-chat/**`, `public/agent-chat/**`
- Depends on: webui-promethity-foundation, mcp-agent-hot-reload, mcp-agui-a2ui-registry
- Size: M; goals: 4, 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
