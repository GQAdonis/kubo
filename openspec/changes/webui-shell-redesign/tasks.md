# Tasks: webui-shell-redesign

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-shell-redesign`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-promethity-foundation, webui-promethity-assets. Scheduled in Round 4.

## 1. Tasks

- [ ] 1.1 `App.js` shell (surface rail, ground top bar, no inline hex) and `NavBar` (active `accent-text` + 600, no bars). Acceptance: no hex literal in `App.js` or `NavBar.css`.
- [ ] 1.2 `Box`, `Title`, `button`, `Notify`, `Connected`, tour skin on tokens; fills, no borders, no shadows. Acceptance: grep shows no `b--`/`shadow` classes in these files.
- [ ] 1.3 Lucide icons for nav and settings. Acceptance: nav icons import from one icon module.
