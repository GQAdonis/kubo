# Tasks: praxis-decision-inbox

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-decision-inbox`. Decision D4 in `analysis.md`.
Change-level prerequisites (child plan r1): every task in this change also requires completion of: `praxis-decision-model-v2`, `praxis-shell-modes`. Scheduled in Round 3.

## 1. Kubo server (mcp-server-engineer)

- [ ] 1.1 Add the decision list item schema (additive) under `mcp/contract/schemas` and the route to `mcp/contract/routes/index.json`. Verify: contract tests pass, and the route index validates against `index.schema.json`
- [ ] 1.2 Add a resolved-history ring to the broker (200 entries or 7 days, in memory). Verify: a Go test resolves 201 decisions and reads back the latest 200; a test with a fake clock drops entries older than 7 days
- [ ] 1.3 Register the read-only decisions endpoint through `h.protect` in `mcp/server/http.go`, scoped to the calling principal. Verify: a two-principal Go test finds each principal's list contains only its own decisions; a POST is refused
- [ ] 1.4 Assign `activity_id` at the first run and carry it through interrupt and continuation in `agui_resume.go`. Verify: a Go test interrupts, approves, and finds the same `activity_id` on both runs and on the decision item
- [ ] 1.5 Emit `ipfs.decision.changed` (id and state only) on create and on every state change. Verify: a Go test resolving through `desktop` sees the event on the owner's stream, the event JSON has no `arguments` or `targets` key, and another principal's stream sees nothing

## 2. Kubo UI (ui-ux-engineer)

- [ ] 2.1 Build `DecisionInbox` (Waiting and Recent groups) and `ActivityRow` in `mcp/ui/src/components/`. Verify: component tests render all ten decision states in the right group with their badge words, and the empty state copy
- [ ] 2.2 Add both to the generated A2UI catalog and extend validation. Verify: catalog validation passes, and fails on a dangling entry
- [ ] 2.3 WCAG 2.2 AA (axe, keyboard, reduced motion) in light and dark. Verify: no AA violation

## 3. Web UI (ui-ux-engineer)

- [ ] 3.1 Add `../ipfs-webui/src/activity/` with the Home inbox (count plus waiting decisions) and the `#/activity` route (Activity list: state, last run, linked decisions). Verify: a test with a mocked endpoint renders the count and the list
- [ ] 3.2 Subscribe to `ipfs.decision.changed` and re-read changed items, coalesced per frame. Verify: a test fires 20 events in one frame and sees one re-read per changed id
- [ ] 3.3 WebMCP modal, chat `InterruptItem` and inbox render the same `ApprovalCard`; answering one moves the others to `resolved_elsewhere` naming the surface. Verify: an e2e test approves in the inbox and finds the chat card showing "Answered on another screen"
- [ ] 3.4 Activity states render as words: running, waiting for approval, waiting for input, paused, completed, failed, cancelled, unknown (reconciling); waiting never uses the error tone. Verify: a test renders each state and checks the word and tone
- [ ] 3.5 With agent features off, the inbox and `#/activity` are absent. Verify: a test with the off switch set finds neither
