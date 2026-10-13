# Proposal: webui-agent-settings-page

Phase `agent-backend-settings`, change 17 of 28 in plan revision 3, Round 5. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-40).

## Why

The Web UI has only a chat toggle, an endpoint field and a token. Goal 1 needs a settings page with a backend choice, test connection, model list, apply feedback and off switches, shared by cloud and Desktop.

## What Changes

- 1.1: Lazy `#/settings/agent` route before `/settings*`; provider with injected `SettingsClient` (HTTP to the console origin; Desktop bridge per the contract).
- 1.2: OpenAI-compatible form: base URL, write-only key, test connection, model combobox.
- 1.3: UAR form with honest unavailable states (enabled later by `agent-backend-uar` 1.6).
- 1.4: Feature switches and the fixed "Writes and deletes ask you first" PolicyDecision row; no default flips here (W9).
- 1.5: ApplyStatus states; typed re-confirmation for backend and egress changes; admin token in memory or `sessionStorage` only; 401 means "Sign in again".
- 1.6: First run: two-step list, no preselected backend; old `AgentChatSettings`/`WebMcpSettings` boxes removed with a summary row on `/settings`; `data-id` hooks kept.
- 1.7: Jest and e2e tests for every state and both clients.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `../ipfs-webui/src/settings/agent/**`, `src/bundles/routes.js`, `SettingsPage.js`, `src/agent-chat/**` settings parts
- Depends on: webui-settings-primitives, agent-settings-contract, mcp-agent-settings-api (not `agent-backend-uar`: the UAR form ships with honest unavailable states; W4)
- Size: L; goals: 1, 2, 3
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
