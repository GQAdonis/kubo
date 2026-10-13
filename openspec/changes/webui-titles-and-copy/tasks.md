# Tasks: webui-titles-and-copy

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-titles-and-copy`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: promethity-design-scope, webui-promethity-foundation. Scheduled in Round 4.

## 1. Tasks

- [ ] 1.1 Title helper from `brand.js` in the five `| IPFS` sites plus `src/files/FilesPage.js:327`. Acceptance: grep finds no hard-coded `IPFS` product suffix in titles.
- [ ] 1.2 `en` locale rewrites for product-name uses and banned words (`explore.json:48`, `settings.json:57`). Acceptance: the diff touches `en` only.
- [ ] 1.3 e2e title assertions updated. Acceptance: assertions use the brand name.
- [ ] 1.4 Attribution in About and footer (D8); `LICENSE` kept, fork notice beside it. Acceptance: MIT notice text unchanged.
