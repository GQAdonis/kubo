# Tasks: webui-agent-chat

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `webui-agent-chat`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `agent-chat-island`, `mcp-agui-endpoint`, `native-confirmation-prompt`, `webui-webmcp-integration`. Scheduled in Round 8.

## 1. Web UI chat mount

- [x] 1.1 With N4 and N12 complete, add `src/agent-chat/**` with the launcher, the Settings toggle (off by default), endpoint and token settings, and the `agentChat` i18n namespace. Verify: Jest tests show no dynamic import while the toggle is off, and with the toggle on the island connects to the configured `/agui` URL
- [x] 1.2 With N4 and N12 complete, run the fork's Jest, `npm run eslint` and typecheck per the fork `AGENTS.md`, including the toggle-off test (no island request, no `/agui` request). Verify: all three pass
- [ ] 1.3 With N4, N12 and N14 complete, the operator runs the fork Web UI in a browser and through a Desktop build bundling it (D1) against `ipfs mcp --http --agui` with a local backend, performing a read and a confirmed write in chat. Verify: transcripts and node state are recorded in the fork change notes
- [x] 1.4 (operator decision 2026-10-10: the chat token lives in browser storage) ui-ux-engineer: persist the `ui` principal token in `localStorage` in the fork's `src/agent-chat/settings-store.js` under its own key, cleared when the chat is switched off; add a "Forget token" control in Settings; state in Settings copy that the token is stored in this browser profile; update the module comment and tests. Verify: the fork's agent-chat unit tests pass, and a test proves switching off and "Forget token" both remove the stored token
