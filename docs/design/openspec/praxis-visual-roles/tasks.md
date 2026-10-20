# Tasks: praxis-visual-roles

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-visual-roles`. Decision D6 in `analysis.md`; brand assumption PX-02 in `decision-log.md`.
Change-level prerequisites (child plan r1): none. Scheduled in Round 1.

## 1. Tasks

- [ ] 1.1 Confirm PX-02 (Praxis roles, Promethity values) with the operator before code starts, and record the answer in `decision-log.md`. Verify: the decision log has a dated row confirming or replacing PX-02
- [ ] 1.2 Add `../ipfs-webui/src/promethity/roles.css` defining the fourteen `--role-*` properties for light and dark, each aliasing an existing Promethity token; `--role-ai` aliases `info` (#7FB2D9 dark, #1F5E8C light). Verify: a test parses the file and finds all fourteen roles in both themes, each a `var(--…)` reference, no hex literal
- [ ] 1.3 Mirror the same fourteen role names on `[data-ipfs-ui]` in `mcp/ui/src/styles/tokens.css`. Verify: a test asserts the role name sets in the Web UI and `mcp/ui` are equal, and each role resolves to the same computed value in both themes
- [ ] 1.4 Contrast check every text role on every surface role it can sit on, in both themes. Verify: all text pairings reach WCAG 2.2 AA (4.5:1 body, 3:1 large and UI)
- [ ] 1.5 Role-coverage lint: files under new Praxis surface directories (`src/shell/`, `src/activity/`) may not contain hex literals or raw Promethity token names. Verify: the lint fails on a planted hex literal and passes on the clean tree
- [ ] 1.6 No visual change on existing screens. Verify: before/after screenshots of Status, Files, Pins, Explore, Peers, Settings at 375 and 1440 in both themes show no pixel difference beyond antialiasing
- [ ] 1.7 Document the role table and the Flat 2.0 deviation (values and fonts not adopted, AI role → `info`) in `DESIGN.md`. Verify: the table lists all fourteen roles with light and dark values; no banned word
