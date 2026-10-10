# Tasks: mcp-ui-component-library

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS (plan revision 2), phase path `ipfs-mcp-server`, change ID `mcp-ui-component-library`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 2.1, 2.2, 2.3. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-tool-contract`. Scheduled in Round 2.

## 1. Component library

- [x] 1.1 Verify the A2UI v0.9 catalog format and MCP Apps extension fields against primary sources, and record the URLs and dates in `mcp/ui/SOURCES.md`. Verify: every format assumption used later cites a source line
- [x] 1.2 Scaffold `mcp/ui` with React 19 + TypeScript and light/dark tokens. Verify: the package build and typecheck pass
- [x] 1.3 Build the seven components bound to contract output schemas. Verify: component tests render fixture outputs validated against the contract schemas
- [x] 1.4 Generate and validate the A2UI catalog manifest. Verify: the catalog validation test passes and fails on a dangling entry
- [x] 1.5 Add WCAG 2.2 AA (axe, keyboard) checks in light and dark, plus a no-remote-assets check. Verify: the checks pass, and a build scan finds no external URLs

## 2. Chat primitives (plan revision 2)

- [x] 2.1 Build the chat primitives: message stream, tool-call card, interrupt/approval card (pending, approved, denied, expired, waiting for the system prompt) and egress badge, bound to contract schemas and AG-UI fixtures, shadow-DOM safe. Verify: component tests render each fixture, and the approval card shows no approve control when approval is out-of-band
- [x] 2.2 Add A2UI catalog entries for the chat primitives and extend the catalog validation. Verify: validation passes and fails on a dangling entry
- [x] 2.3 Run WCAG 2.2 AA checks (axe, keyboard, reduced motion) in light and dark and the no-remote-assets scan on the chat primitives. Verify: no AA violation and no external URL
