# Tasks: mcp-app-views

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-app-views`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-ui-component-library`, `mcp-tools-content`, `mcp-pinning-with-confirmation`. Scheduled in Round 5.

## 1. MCP App views

- [x] 1.1 Build the views and bundle in `mcp/app` with a CSP that allows no remote origins. Verify: the bundle build passes, and a scan finds no external URLs
- [x] 1.2 Embed the bundle in `mcp/server` with `go:embed`, register the `ui://` resources and add the `_meta.ui` links. Verify: a unit test reads every `ui://` resource
- [x] 1.3 Add the rebuild-and-diff check script. Verify: the script exits non-zero after a deliberate source change without a rebuild
- [ ] 1.4 Check rendering in a real host (Claude Code), and check that tools stay usable without the UI. Verify: the screenshot and transcript are recorded in the change
