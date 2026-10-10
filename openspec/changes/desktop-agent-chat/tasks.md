# Tasks: desktop-agent-chat

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `desktop-agent-chat`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `agent-chat-island`, `native-confirmation-prompt`. Scheduled in Round 5.

## 1. Desktop hardening and confirmation window

- [x] 1.1 Record what breaks with `webSecurity: true`, the CORS configuration that keeps the Web UI working, and a smoke baseline, in the fork `docs/agent/HARDENING.md`. Verify: each smoke item has a pass or fail before the change
- [x] 1.2 Add the CSP for `webui://localhost` and re-enable `webSecurity`. Verify: an external fetch is blocked and the smoke test passes (G11-2)
- [x] 1.3 Add the `safeStorage` token bridge in main and preload. Verify: the token is in no renderer storage and survives restart (G11-4)
- [ ] 1.4 Implement pairing and the native confirmation window over the N14 channel. Verify: an unpaired instance cannot list or resolve; a scripted renderer cannot reach the window
- [ ] 1.5 Add Electron e2e tests: a pending write appears in the modal with its CIDs; approve runs it once; deny does not; and the disabled state (chat off, unpaired) exposes no bridge call and no confirmation channel. Verify: the Electron e2e suite passes
- [ ] 1.6 The operator runs a paired Desktop on macOS and approves and denies one write each. Verify: screenshots and node state are recorded in the fork change notes
