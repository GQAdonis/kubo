# Tasks: webui-promethity-assets

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-promethity-assets`, backend task IDs 1.1, 1.2. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: promethity-design-scope. Scheduled in Round 3.

## 1. Tasks

- [ ] 1.1 Favicon set, `manifest.json` (192, 512, maskable; colours), `index.html` (two `theme-color` metas, title, description, generator). Acceptance: every icon path in the manifest exists.
- [ ] 1.2 Wordmark and p-icon components; remove IPFS logos from NavBar and Welcome; never wordmark and icon side by side. Acceptance: no `ipfs-logo*.svg` import remains in `src`.
