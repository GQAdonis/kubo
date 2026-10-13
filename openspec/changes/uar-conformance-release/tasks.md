# Tasks: uar-conformance-release

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `uar-conformance-release`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: none. Scheduled in Round 1.

## 1. Tasks

- [x] 1.1 Draft the request as a follow-up to `Prometheus-AGS/universal-agent-runtime#379`: commit the working tree and release `v1.1.0` with an OCI digest; bind an external MCP server to an agent with a run-time credential; remote skill upload; elicitation for globally registered servers (and `shared_transports_enabled`); an AG-UI agent path and interrupt shapes; verify #379 items 2, 3, 7. Acceptance: `mcp/uar/requests/2026-10-13-v1.1.md` with one falsifiable check per item.
- [x] 1.2 After the operator files it (OG-3), record the URL and poll with read-only `gh issue view` until it is visible. Acceptance: URL in the request file; `gh issue view` output saved.
- [ ] 1.3 When OG-4 confirms a release, record tag and digest. Acceptance: tag and digest present; the change stays open until then.
