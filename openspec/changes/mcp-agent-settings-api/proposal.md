# Proposal: mcp-agent-settings-api

Phase `agent-backend-settings`, change 10 of 28 in plan revision 3, Round 3. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: none.

## Why

A settings UI needs a server resource to read and change the backend without a restart, protected by a settings-scoped principal, never returning secrets, and with an off switch.

## What Changes

- 1.1: Principal kind `admin`: settings scope only, refused on `/mcp`, `/agui`, `/a2a`; a configured principal whose token environment variable or file is absent is disabled with a warning, not fatal.
- 1.2: `GET`/`PUT` settings with `ETag`/`If-Match`, shared validation, atomic apply through `ApplyAgentSettings`; never returns key material; values never logged.
- 1.3: Overlay persistence in `--settings-dir` (0600; flags > overlay > config); write-only key stored as a 0600 file becoming `SecretRef{File}`; `accept_secrets: false` refuses key fields with `secrets_not_accepted`; an absent `llm.api_key` file yields `api_key.set=false` and no startup failure.
- 1.4: Test-connection (candidate URL only, https or loopback http, no credentials in URL, bounded timeout, nothing persisted) and model list.
- 1.5: Change events (SSE) carrying only the new revision.
- 1.6: Egress extension (D4): only the newly chosen host, only with the typed host confirmation; on Desktop the bridge adds Touch ID.
- 1.7: Off switches and disabled states (W6): `agent.settings.enabled=false` and no admin principal give 404, nothing written, nothing dialed; `GOTAGS=noagent` removes the route.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

- `mcp-agent-settings-api`: see `specs/mcp-agent-settings-api/spec.md`.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer
- Scope: `mcp/server/**`, `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*`
- Depends on: agent-settings-contract, mcp-agent-hot-reload
- Size: L; goals: 1, 3
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
