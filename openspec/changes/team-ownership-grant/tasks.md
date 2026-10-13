# Tasks: team-ownership-grant

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS (plan revision 2), phase path `ipfs-mcp-server`, change ID `team-ownership-grant`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: none (Round 0). Scheduled in Round 0.

## 1. Approval and ownership

- [x] 1.1 Ask the operator the batched Q1/Q2/Q3 question and record the answers in `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`. Verify: Q1 and Q2 answers are recorded with the date
- [x] 1.2 Write the complete grant table into `team.json`. Existing grants stay; the new grants are root `go.mod`/`go.sum` for the engineer and architect (only via `go get` + `make mod_tidy`), `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*_test.go`, `mcp/webmcp/**` plus fork `src/webmcp/**` for ui-ux-engineer, and `mcp/release/**` for skills-plugin-specialist. Verify: a JSON diff of `team.json` shows exactly these additions
- [x] 1.3 Rewrite the role instructions superseded by F4 (root-module package, confirmation-gated writes and destructive tools on every transport). Verify: no role instruction still contains "its own Go module" or "opt-in" for destructive tools
- [x] 1.4 Re-export the native role definitions with agent-team-creator. Verify: every export lists the same owned paths as `team.json`
- [x] 1.5 Write the plan-revision-2 grants into `team.json`: architect `mcp/uar/**` except `mcp/uar/spike/**`, which goes to mcp-server-engineer; ui-ux-engineer `mcp/chat/**`, Web UI fork `src/agent-chat/**` plus the launcher mount in `src/App.js`, Settings and `agentChat` locale files, and Desktop fork `/Users/gqadonis/Projects/references/ipfs-desktop` paths `src/webui/**`, `src/agent/**`, `test/**` and `package.json` scripts; security-reviewer review scope extended to both forks. Verify: a JSON diff of `team.json` shows exactly these additions on top of 1.2
- [x] 1.6 Re-run the native export and consistency check after 1.5. Verify: every export (`.claude`, `.codex`, `.kimi-code`, `.opencode`) lists the same owned paths as `team.json`
