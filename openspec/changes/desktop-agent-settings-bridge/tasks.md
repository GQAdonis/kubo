# Tasks: desktop-agent-settings-bridge

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `desktop-agent-settings-bridge`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: agent-settings-contract, desktop-mcp-sidecar, team-ownership-agent-backend. Scheduled in Round 5.

## 1. Tasks

- [x] 1.1 Write bridge `agentSettings.*` answered only for the trusted main frame; setters in the main process. Acceptance: channels reuse `isTrusted`.
- [x] 1.2 Key write-only into `safeStorage`; push events replace the read-once at window creation; window recreated when the sidecar port changes. Acceptance: `get` returns `{keySet}` only.
- [x] 1.3 Tray entries (W9). Acceptance: a unit test of the tray template finds a status line, an "Agent settings" item whose click sends navigation to `#/settings/agent`, and a sidecar toggle bound to `mcpSidecar.enabled`.
- [x] 1.4 Touch ID on backend change and egress extension. Acceptance: the setter path calls the existing Touch ID helper.
- [x] 1.5 Hardening tests (renderer cannot widen CSP; untrusted frames refused). Acceptance: test files exist.
- [ ] 1.6 Round 5 security confirmation residuals (`.agent-team/kubo-ipfs-agentic/findings/security/2026-10-14-round5-webui-desktop-confirm.md`): N1 (one pending host confirmation at a time; refuse repeats for a declined host for the window lifetime), N2 (`restart()` returns when a stop or quit ran during the halt), N4 (Touch ID reason names the key; drop or narrow the test-connection prompt), N5 (parent the native dialog only to a visible window). Acceptance: specs for N1 and N2.
