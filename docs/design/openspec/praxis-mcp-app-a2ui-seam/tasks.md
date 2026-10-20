# Tasks: praxis-mcp-app-a2ui-seam

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-mcp-app-a2ui-seam`. Decision D9 in `analysis.md`.
Change-level prerequisites (child plan r1): every task in this change also requires completion of: `praxis-decision-model-v2`. Scheduled in Round 2.

## 1. Mapping and delivery

- [ ] 1.1 Add `VIEW_COMPONENT` next to `VIEW_IDS` in `mcp/app/src/view-specs.ts` and extend `mcp/app/tests/views.test.ts` to check contract `appView` → view → catalog component. Verify: the test passes, and fails when one mapping entry is removed
- [ ] 1.2 In `mcp/server/agui_a2ui.go`, deliver `appView` results as A2UI surface updates when the run input advertises the IPFS catalog id; keep the `ui://` link for other MCP hosts. Verify: Go tests cover both clients for `pin_ls` and find a `PinList` surface for one and the `ui://ipfs-mcp/pins` link for the other
- [ ] 1.3 A2UI fixtures for all five views drive both the A2UI renderer test and the iframe view test. Verify: one fixture directory, used by both test suites

## 2. Iframe exception

- [ ] 2.1 Add the exception declaration (`exception: "iframe"`, reason, correlation rule) to each view resource's metadata. Verify: a resources/list test finds the declaration on all five views
- [ ] 2.2 In `mcp/app/src/host/port.ts`, issue a per-render correlation id, pin the host origin from the first host message, and drop messages that do not match. Verify: tests post from a second origin and with a wrong correlation id, and the view state is unchanged in both

## 3. Fallback and mount

- [ ] 3.1 Inert labelled fallback in `mcp/ui` for unknown component, off-catalog version and schema-failing output, with the copy "This result can't be shown here. Nothing in it runs." Verify: component tests cover the three cases and find no button or link other than copy text
- [ ] 3.2 Register the `mcp/ui` A2UI renderer in the Web UI pane and canvas (`../ipfs-webui/src/shell/**`, `src/agent-chat/**`). Verify: an e2e test asks for the pin list in conversation-forward mode and finds `PinList` rendered with no request for `ui://` HTML
- [ ] 3.3 WCAG 2.2 AA and the no-remote-assets scan on the five surfaces and the fallback, light and dark. Verify: no AA violation and no external URL
