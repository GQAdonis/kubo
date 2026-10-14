# Tasks: webui-shell-redesign

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-shell-redesign`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-promethity-foundation, webui-promethity-assets. Scheduled in Round 4.

## 1. Tasks

- [x] 1.1 `App.js` shell (surface rail, ground top bar, no inline hex) and `NavBar` (active `accent-text` + 600, no bars). Acceptance: no hex literal in `App.js` or `NavBar.css`.
- [x] 1.2 `Box`, `Title`, `button`, `Notify`, `Connected`, tour skin on tokens; fills, no borders, no shadows. Acceptance: grep shows no `b--`/`shadow` classes in these files.
- [x] 1.3 Lucide icons for nav and settings. Acceptance: nav icons import from one icon module.
- [ ] 1.4 Round 4 UI confirmation residuals (`.agent-team/kubo-ipfs-agentic/findings/review/2026-10-14-round4-ui-confirm.md`): N1 WebMCP dialog decision row at 375 and 320 (expiry and both disclosures visible at first view), N2 shared modal scrim and no shadow, N3 Peers count and empty state, N4 Files header at 1440, N5 Files row rules (fill or recorded exception), N6 modal buttons at 375, M11 remaining upstream glyphs, L1 remaining Title Case strings. Acceptance: recaptures listed in that file show each item fixed in both themes; the WebMCP expiry line is visible at scrollTop 0 at 320 and 375.
