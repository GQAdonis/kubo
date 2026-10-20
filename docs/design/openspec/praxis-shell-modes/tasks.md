# Tasks: praxis-shell-modes

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-shell-modes`. Decisions D1, D2, D3 in `analysis.md`.
Change-level prerequisites (child plan r1): every task in this change also requires completion of: `praxis-visual-roles`. Scheduled in Round 2.

## 1. Layout modes

- [ ] 1.1 Add `../ipfs-webui/src/shell/` with an application-forward layout (navigator, IPFS app, assistant pane on the right) and a conversation-forward layout (navigator, thread, canvas). Verify: a component test renders both modes and finds the three regions with landmark roles
- [ ] 1.2 Pane states closed, docked (480–560px), expanded (about 760px) and full width; `[` and `]` step between them when focus is not in a text field; the state persists per device. Verify: a test presses `]` three times from closed and reaches full width, reloads, and finds the state restored
- [ ] 1.3 Remove `AgentChatLauncher`, `launcher.css` and `--ac-launcher-clearance`; mount `<ipfs-agent-chat>` in the pane. Verify: grep finds no `--ac-launcher-clearance` and no launcher import; the island renders inside the pane

## 2. Navigator and Home

- [ ] 2.1 Navigator order: Home, Files, Pins, Explore, Peers, Diagnostics, Activity, Settings. Verify: a NavBar test asserts the order and that every IPFS app is one click from any route
- [ ] 2.2 Home replaces Status: node identity, bandwidth, repo and peers count become cards with distinct loading, empty, offline, stale, denied and error states; `#/status` redirects to Home. Verify: a test renders each card in each of the six states with a distinct word, and `#/status` lands on Home
- [ ] 2.3 Reserve the decision inbox slot on Home and the Activity route placeholder for `praxis-decision-inbox`. Verify: the slot renders an empty state "No decisions waiting." and `#/activity` resolves

## 3. One composer

- [ ] 3.1 Merge the top-bar CID box, the search entry and the chat input into one composer with intents "Go to CID or path", "Search my content", "Ask the agent". Verify: grep finds one composer component; the old top-bar input and the separate search input are gone
- [ ] 3.2 Infer the intent (a CID or `/ipfs/` or `/ipns/` path selects Go to), show it, and let the person change it before submit. Verify: tests cover a CIDv0, a CIDv1, an `/ipns/` path and free text, and each shows the inferred intent with a working change control
- [ ] 3.3 Suggestions are drafts: activating one fills the composer and does not submit. Verify: a test activates a suggestion and finds no request sent and the text in the composer
- [ ] 3.4 With agent features off, the composer offers only Go to and Search, and the pane is absent. Verify: a test with the off switch set finds two intents and no pane

## 4. Mobile and checks

- [ ] 4.1 Below 540px: navigator as a drawer, canvas full screen, Back restores the thread scroll position and the unsent draft. Verify: a Playwright test at 375 opens the canvas, goes Back, and finds the same scroll offset and draft text
- [ ] 4.2 No page-level horizontal overflow at 320, 375, 768, 1024, 1440 and 1920 in both themes, in both modes and all pane states. Verify: a Playwright check asserts `scrollWidth <= clientWidth` on `document.documentElement` for every combination
- [ ] 4.3 WCAG 2.2 AA: axe, keyboard order (navigator, app, pane), focus rings, 44px targets, reduced motion. Verify: no AA violation in both themes
- [ ] 4.4 Every IPFS function in the assessment's "must survive" list still works from the new shell. Verify: the existing Files, Pins, Explore, Peers, Diagnostics and Settings e2e tests pass unchanged except for navigation selectors
