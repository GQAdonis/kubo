# Proposal: desktop-agent-settings-bridge

Phase `agent-backend-settings`, change 19 of 28 in plan revision 3, Round 5. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-42).

## Why

Desktop's Web UI bridge is read-only and its settings are read once at window creation; the settings page needs a validated write bridge, Keychain key storage, push updates and tray entries.

## What Changes

- 1.1: Write bridge `agentSettings.*` answered only for the trusted main frame; setters in the main process.
- 1.2: Key write-only into `safeStorage`; push events replace the read-once at window creation; window recreated when the sidecar port changes.
- 1.3: Tray entries (W9).
- 1.4: Touch ID on backend change and egress extension.
- 1.5: Hardening tests (renderer cannot widen CSP; untrusted frames refused).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `../ipfs-desktop/src/agent/**`, `src/webui/**`, `src/tray.js`, `test/**`
- Depends on: agent-settings-contract, desktop-mcp-sidecar, team-ownership-agent-backend
- Size: M-L; goals: 1, 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
