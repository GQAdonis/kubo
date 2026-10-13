# Tasks: agent-chat-island

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `agent-chat-island`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-ui-component-library`. Scheduled in Round 3.

## 1. Chat island

- [x] 1.1 Build `<ipfs-agent-chat>` with a shadow-root React 19 + Tailwind 4 + shadcn sliding sidebar from `mcp/ui` (including its chat primitives), bundled Promethity tokens and fonts, a dynamic-import entry, no `unsafe-eval`. Verify: the build passes and a scan finds no external URL (G11-4)
- [x] 1.2 Implement the AG-UI client to `ipfs mcp` `/agui` only, A2UI rendering of tool results, and interrupt answers that require a user gesture; the bearer arrives by property only. Verify: tests against contract AG-UI fixtures; a scripted, gesture-free resolve attempt fails (G11-3); no token in page storage
- [x] 1.3 Add isolation tests: host-page screenshots unchanged with the island closed, styled rendering under Electron 44 (G11-1). Verify: Playwright visual tests pass in Chromium and Electron 44
- [x] 1.4 Record the bundle size and replay recorded contract AG-UI fixtures (including an interrupt) through the client with a local fixture server on a free port; no live `ipfs mcp` run here, because `/agui` (N12) lands in Round 7 and the live run is `webui-agent-chat` 1.3. Verify: size and replay results are saved in `mcp/chat/EVIDENCE.md`
