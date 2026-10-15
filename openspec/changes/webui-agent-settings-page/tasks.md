# Tasks: webui-agent-settings-page

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-agent-settings-page`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-settings-primitives, agent-settings-contract, mcp-agent-settings-api (not `agent-backend-uar`: the UAR form ships with honest unavailable states; W4). Scheduled in Round 5.

## 1. Tasks

- [x] 1.1 Lazy `#/settings/agent` route before `/settings*`; provider with injected `SettingsClient` (HTTP to the console origin; Desktop bridge per the contract). Acceptance: `routes.js` does not import the page module eagerly.
- [x] 1.2 OpenAI-compatible form: base URL, write-only key, test connection, model combobox. Acceptance: the key field is never re-rendered with a value; on a host that answers `secrets_not_accepted` the field is replaced by "Set on the server (Kubernetes Secret)".
- [x] 1.3 UAR form with honest unavailable states (enabled later by `agent-backend-uar` 1.6). Acceptance: copy states what is missing; no fake success.
- [x] 1.4 Feature switches and the fixed "Writes and deletes ask you first" PolicyDecision row; no default flips here (W9). Acceptance: Jest asserts toggling the chat switch writes `enabled` in `ipfs-webui:agent-chat`, the WebMCP switch writes `enabled` and `dnslink` in the WebMCP store and never persists `writes`, and the Desktop confirmation switch is absent without the bridge.
- [x] 1.5 ApplyStatus states; typed re-confirmation for backend and egress changes; admin token in memory or `sessionStorage` only; 401 means "Sign in again". Acceptance: grep shows no `localStorage` write of the admin token.
- [x] 1.6 First run: two-step list, no preselected backend; old `AgentChatSettings`/`WebMcpSettings` boxes removed with a summary row on `/settings`; `data-id` hooks kept. Acceptance: no backend preselected in the initial state.
- [x] 1.7 Jest and e2e tests for every state and both clients. Acceptance: test files exist; run at the gate.
- [ ] 1.8 Round 5 confirmation residuals (`.agent-team/kubo-ipfs-agentic/findings/review/2026-10-14-round5-ui-confirm.md`): C1 (gate Test only when a key is stored, host field above Test, reword so "confirmed" is not reused), C2 (clear the conflict status after Reload settings), C3, C4, C5, L6 (Diagnostics play button accent during the nudge), plus the owed browser captures listed there. Acceptance: Jest for C1, C2, C4, C5; captures for every item in "Still needs a browser check".
