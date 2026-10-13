# Tasks: agent-chat-island-integration

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `agent-chat-island-integration`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-promethity-foundation, mcp-agent-hot-reload, mcp-agui-a2ui-registry. Scheduled in Round 5.

## 1. Tasks

- [ ] 1.1 `theme` attribute from the shell theme. Acceptance: the launcher passes the resolved theme.
- [ ] 1.2 Empty state on 503 `backend_not_configured` with a link to `#/settings/agent`. Acceptance: fixture replay of the 503.
- [ ] 1.3 Launcher restyle. Acceptance: no Tachyons colour class on the launcher.
- [ ] 1.4 Rebuild `mcp/chat` and re-vendor into `public/agent-chat/` with the hash manifest. Acceptance: `MANIFEST.json` hashes match the build.
