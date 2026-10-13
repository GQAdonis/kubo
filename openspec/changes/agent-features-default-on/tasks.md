# Tasks: agent-features-default-on

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `agent-features-default-on`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: mcp-agent-settings-api, desktop-mcp-sidecar, webui-agent-settings-page, agent-chat-island-integration, desktop-agent-settings-bridge. Scheduled in Round 6.

## 1. Tasks

- [ ] 1.1 Web UI: chat `enabled` and WebMCP `enabled` default true; WebMCP `writes` stays per-load and off; absent-keys-only migration (D18) through app boot; disabled-state tests; fork docs. Acceptance: a test asserts a stored `enabled:false` survives boot.
- [ ] 1.2 Desktop: `agentChat.enabled`, `desktopConfirm.enabled` (never win32), `mcpSidecar.enabled` default true; absent-keys-only migration at app start; disabled-state tests; fork docs. Acceptance: the same survival test through app start.
- [ ] 1.3 Kubo `test/cli` regression test pinning stock defaults (no `/agui` without `--agui`; no settings route without an admin principal). Acceptance: test exists.
- [ ] 1.4 Defaults table in `docs/mcp/chat-setup.md` and the config reference (W6). Acceptance: one row per feature in the `5f019f0df` list plus rows for the settings API (`agent.settings.enabled`) and server-emitted A2UI (`agent.a2ui`), each with where it is on, its off switch and its disabled-state test; idle RSS from `desktop-mcp-sidecar` 1.5.
- [ ] 1.5 `v0.44.md` Highlights (W6). Acceptance: a disable example per feature, including `agent.settings.enabled: false` and `agent.a2ui: false`, plus the resource note; only Highlights and TOC change.
