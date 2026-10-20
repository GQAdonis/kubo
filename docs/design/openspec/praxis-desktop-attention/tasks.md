# Tasks: praxis-desktop-attention

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-desktop-attention`. Decision D12 in `analysis.md`.
Change-level prerequisites (child plan r1): every task in this change also requires completion of: `praxis-decision-model-v2`, `praxis-decision-inbox`. Scheduled in Round 4.

## 1. Tray

- [ ] 1.1 Reorder the template in `../ipfs-desktop/src/tray.js`: status line, pending decisions, Open Promethity, Home, Files, Activity, node controls, Preferences, Advanced, About, Quit. Verify: a unit test of the tray template asserts the top-level order by item id and group separators
- [ ] 1.2 Add `src/agent/decisions-tray.js`: "N decisions waiting" plus the top 3 by expiry as "<class in words>: <object>", each opening the native window for that decision; "No decisions waiting." when empty. Verify: template tests with 0, 1 and 5 pending decisions find the right count, at most 3 entries ordered by expiry, and a click handler that opens the window with that decision id
- [ ] 1.3 Read decisions from the `praxis-decision-inbox` endpoint through the sidecar and rebuild the menu on `ipfs.decision.changed`. Verify: a test fires a change event and sees one menu rebuild with the new count
- [ ] 1.4 Node controls group holds Restart, Start, Stop, Peers, Take screenshot, node settings, provide strategy, CID profile, the agent endpoint items and the Desktop confirmation submenu. Verify: the template test finds each id inside the group, and the existing tests for those items pass unchanged
- [ ] 1.5 Preferences holds app options only; node settings are no longer in it. Verify: the template test finds no `webuiNodeSettings` under Preferences
- [ ] 1.6 With agent features off, the pending decisions group and Activity are absent and the remaining order holds. Verify: a template test with the off switch set
- [ ] 1.7 Every tray function in the assessment's "must survive" list is still reachable. Verify: the template test finds start, stop, restart, Home, Files, Peers, take screenshot, node settings, every Advanced id, About and Quit

## 2. Native confirmation window

- [ ] 2.1 Render decision card v2 in `src/agent/confirm/ui/confirm.html` and `confirm.js`: ten states with their badge words, and object, destination, scope, class in words, named authority, egress badge and short effect digest. Verify: window tests render a fixture per state and find the word and every field
- [ ] 2.2 Keep the Touch ID gate: Approve calls the existing `touchid.js` prompt; without Touch ID the window says this app cannot approve and offers Deny only. Verify: a test with `canPromptTouchID` false finds no Approve control; a test with it true finds that Approve calls `prompt` before any approval is sent
- [ ] 2.3 Resolved elsewhere: when the decision is answered on another surface, the window shows "Answered on another screen", names the surface, and closes after 5 seconds or on Escape. Verify: a test with a fake clock closes the window at 5 seconds; Escape closes it at once
- [ ] 2.4 Do not claim HG-1. Verify: grep of new strings finds no claim that other processes cannot approve; the confirm README states HG-1 is unproven on hardware
- [ ] 2.5 WCAG 2.2 AA (axe, keyboard, reduced motion) on the confirmation window in light and dark; initial focus on Deny. Verify: no AA violation; the focused element on open is Deny
