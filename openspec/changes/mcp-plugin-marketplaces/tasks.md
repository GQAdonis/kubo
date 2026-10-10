# Tasks: mcp-plugin-marketplaces

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-plugin-marketplaces`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `ipfs-agent-skills`, `mcp-release-binaries`, `mcp-http-transport-auth`. Scheduled in Round 10.

## 1. Plugins and marketplaces

- [x] 1.1 Verify the manifest formats for Claude Code, Codex and OpenCode from primary docs, and record sources and dates in `mcp/plugin/SOURCES.md`. Verify: each manifest field used later cites a source
- [x] 1.2 Build the Claude Code plugin and marketplace. Verify: `claude plugin` validation (or the documented equivalent) passes
- [x] 1.3 Build the Codex plugin and marketplace. Verify: Codex loads the manifest without errors
- [x] 1.4 Build the OpenCode plugin and marketplace. Verify: OpenCode loads the configuration without errors (OpenCode has no marketplace format; the package is a plugin module plus config snippet, see `mcp/plugin/SOURCES.md`)
- [ ] 1.5 Install and load each plugin in the real harness (Claude Code end to end; Codex and OpenCode at least config load). Verify: the transcripts and outputs are recorded (`mcp/plugin/evidence/`; Claude Code stops short of a model session because it needs the operator's login: validate, install, details and MCP health check are recorded)
