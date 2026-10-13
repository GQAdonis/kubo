# Proposal: agent-features-default-on

Phase `agent-backend-settings`, change 21 of 28 in plan revision 3, Round 6. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (D5, D18, W6).

## Why

AGENTS.md `5f019f0df` requires the agent features to ship enabled with off switches; today every one defaults off. Flipping them is a feature launch with docs, a changelog disable example and disabled-state tests, and stored opt-outs must survive.

## What Changes

- 1.1: Web UI: chat `enabled` and WebMCP `enabled` default true; WebMCP `writes` stays per-load and off; absent-keys-only migration (D18) through app boot; disabled-state tests; fork docs.
- 1.2: Desktop: `agentChat.enabled`, `desktopConfirm.enabled` (never win32), `mcpSidecar.enabled` default true; absent-keys-only migration at app start; disabled-state tests; fork docs.
- 1.3: Kubo `test/cli` regression test pinning stock defaults (no `/agui` without `--agui`; no settings route without an admin principal).
- 1.4: Defaults table in `docs/mcp/chat-setup.md` and the config reference (W6).
- 1.5: `v0.44.md` Highlights (W6).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: kubo behavior is unchanged (stock defaults are pinned by a regression test); the default flips live in the forks and the documentation.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer (1.1, 1.2); mcp-server-engineer (1.3); ipfs-agent-architect (1.4, 1.5)
- Scope: Web UI stores, Desktop store, `test/cli/mcp_*`, `docs/mcp/**`, `docs/changelogs/v0.44.md` (TOC and Highlights only)
- Depends on: mcp-agent-settings-api, desktop-mcp-sidecar, webui-agent-settings-page, agent-chat-island-integration, desktop-agent-settings-bridge
- Size: M; goals: 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
