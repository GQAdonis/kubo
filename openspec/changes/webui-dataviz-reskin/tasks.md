# Tasks: webui-dataviz-reskin

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-dataviz-reskin`, backend task IDs 1.1, 1.2. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-promethity-foundation. Scheduled in Round 4.

## 1. Tasks

- [x] 1.1 `tokens.js` resolving CSS variables at render and on theme change; Chart.js series from it. Acceptance: no hex in the three chart files.
- [x] 1.2 d3 map flat fills from tokens. Acceptance: no gradient or hex in `WorldMap.js`.
