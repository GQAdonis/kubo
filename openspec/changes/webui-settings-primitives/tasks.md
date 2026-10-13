# Tasks: webui-settings-primitives

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-settings-primitives`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-promethity-foundation. Scheduled in Round 4.

## 1. Tasks

- [ ] 1.1 Field, Switch (`role=switch`), SegmentedControl (radio group). Acceptance: each wires label, help and error via `aria-describedby`.
- [ ] 1.2 Combobox per the APG pattern (W9). Acceptance: Jest keyboard tests assert: ArrowDown opens the list and moves `aria-activedescendant`; Enter selects; Escape closes and restores the input; typing filters options; free text is accepted on Enter.
- [ ] 1.3 StatusLine, ApplyBar, PolicyDecision row (W9). Acceptance: a test renders StatusLine for ok, warn, err and info and asserts each shows a word and an icon; an ApplyBar test asserts the form's `scroll-padding-bottom` equals the bar height (WCAG 2.4.11).
- [ ] 1.4 Unit and axe tests for both themes. Acceptance: test files exist; run at the gate.
