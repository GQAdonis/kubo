# Proposal: desktop-mcp-sidecar

Phase `agent-backend-settings`, change 16 of 28 in plan revision 3, Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: none.

## Why

Goal 4 requires Desktop to start the local agent endpoint. Desktop never starts `ipfs mcp` today; it needs a supervised loopback child with port discovery, Keychain-held tokens and an off switch.

## What Changes

- 1.1: Supervise `ipfs mcp --http=127.0.0.1:<port> --agui --confirm=native,desktop --settings-dir=<userData>/mcp`, tied to the daemon lifecycle, restart with backoff, stderr to the log; stop by tracked PID only.
- 1.2: Port discovery (R4): prefer 5050, else a free loopback port, never a silent fallback.
- 1.3: Generate `ui` and `admin` tokens, store them in `safeStorage`, pass them and the LLM key by environment names.
- 1.4: Off switch `mcpSidecar.enabled` (store key, tray toggle).
- 1.5: Idle RSS capture of the child into a new dated evidence folder.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer
- Scope: `../ipfs-desktop/src/daemon/mcp-sidecar*.js` (+ wiring), `../ipfs-desktop/test/**/mcp-sidecar*`
- Depends on: team-ownership-agent-backend, mcp-agent-settings-api
- Size: M-L; goals: 4, 1
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
