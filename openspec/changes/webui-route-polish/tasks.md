# Tasks: webui-route-polish

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-route-polish`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-shell-redesign, webui-dataviz-reskin. Scheduled in Round 5.

## 1. Tasks

- [ ] 1.1 Files, Pins, Peers, Diagnostics on tokens (W9). Acceptance: grep of `src/files/**`, `src/pins/**`, `src/peers/**`, `src/diagnostics/**` finds no hex literal, `b--` class or shadow class outside the allowlist recorded in 1.4.
- [ ] 1.2 Welcome illustrations replaced (W9). Acceptance: grep of `src/welcome/**` finds no import of an `ipfs-*` image asset and no cloud, padlock or shield illustration file.
- [ ] 1.3 `/explore` token-level theming only (W9). Acceptance: the wrapper sets the token CSS variables on its container; `git diff` shows no file under `node_modules/ipld-explorer-components` changed and no version change of that package in `package.json` or the lockfile.
- [ ] 1.4 Straggler sweep across fork-edited `src`. Acceptance: grep output attached with the allowlist and a reason per entry.
