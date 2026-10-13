# Tasks: settings-security-review

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `settings-security-review`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: agent-settings-contract, mcp-agent-hot-reload, mcp-agent-settings-api, desktop-mcp-sidecar, webui-agent-settings-page, agent-chat-island-integration, desktop-agent-settings-bridge, agent-features-default-on. Scheduled in Round 7.

## 1. Tasks

- [ ] 1.1 Review: the settings API, `admin` principal, test-connection (SSRF), egress extension, overlay and secret storage, admin-token storage in the page, the Desktop bridge, the cloud console design (ADR-0009), and per W7: `mcp-agent-hot-reload` secret lifetime and interrupt resume across swaps, `desktop-mcp-sidecar` token generation and environment passing, and the chat `ui` token kept in `localStorage` (`src/agent-chat/settings-store.js`) on the console origin. Acceptance: findings with file:line and severity, one section per listed area.
- [ ] 1.2 One batched correction of CRITICAL and HIGH findings by the owning roles. Acceptance: each finding linked to a commit or a written rebuttal.
- [ ] 1.3 Confirmation pass. Acceptance: no open CRITICAL; open HIGHs carried to the gate in writing.
