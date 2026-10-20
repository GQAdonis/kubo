# Tasks: praxis-context-inspector

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-context-inspector`. Decisions D7, D8 in `analysis.md`.
Change-level prerequisites (child plan r1): every task in this change also requires completion of: `praxis-shell-modes`. Scheduled in Round 3.

## 1. Kubo server (mcp-server-engineer)

- [ ] 1.1 Add `offered_tools`, `catalog_size` and `system_prompt_digest` to the AG-UI run-start payload schema (additive) and regenerate `mcp/ui/src/contract/generated/agui.ts`. Verify: contract tests pass, and a payload without the fields still validates
- [ ] 1.2 Emit the fields from `agui_stream.go` using `Server.ToolNames()` and the prompt built in `agent_frame.go`. Verify: a Go test starts a run and finds the offered names, the contract count, a `sha256:` digest, and no prompt text in the payload

## 2. Chat island (ui-ux-engineer)

- [ ] 2.1 Add `ContextStrip.tsx` with container, files attached, model placement, memory scope ("None") and tools offered, each with a provenance label. Verify: component tests render a fixture and find each item and its label; a payload without tool facts shows "Not reported by this server"
- [ ] 2.2 Removal: container and files removable, removed items absent from the next request; tools and model items link to `#/settings/agent`. Verify: a test removes the container, sends, and inspects the outbound request for no container
- [ ] 2.3 Add `ContextInspector.tsx` with Overview, Context, Prompt plan and Events; Events is read-only and current run only, virtualized above 200 rows. Verify: component tests render each section from fixtures; keyboard can reach every section
- [ ] 2.4 Add `LifecycleStrip.tsx` mapping AG-UI events to Planning, Calling `<tool>`, Waiting for approval, Running, Done, Failed, Cancelled. Verify: a timeline test feeds an AG-UI fixture with an interrupt and asserts the phase sequence
- [ ] 2.5 WCAG 2.2 AA (axe, keyboard, reduced motion) in light and dark; no horizontal overflow at 320 and 375. Verify: no AA violation; `scrollWidth <= clientWidth` on the island root

## 3. Web UI bridge (ui-ux-engineer)

- [ ] 3.1 Send `{route, cid?, path?}` for the current route through `../ipfs-webui/src/agent-chat/bridge.js`; re-send on route change only. Verify: a test navigates Files → Pins and sees two bridge messages with the right container
- [ ] 3.2 Respect removal: after the person removes the container, no container is sent until the route changes. Verify: a test removes the item, sends twice, and sees no container on either request
