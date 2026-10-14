# Tasks: desktop-display-branding

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `desktop-display-branding`, backend task IDs 1.1, 1.2. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: webui-promethity-assets, team-ownership-agent-backend. Scheduled in Round 4.

## 1. Tasks

- [x] 1.1 Tray, dock and build icons from the p-icon set. Acceptance: icon files replaced at existing paths.
- [x] 1.2 Window titles and visible display name; `productName` and `io.ipfs.desktop` unchanged. Acceptance: `package.json` productName still "IPFS Desktop"; a test asserts the userData path is unchanged.
