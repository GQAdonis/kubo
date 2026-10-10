# Tasks: ipfs-agent-skills

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `ipfs-agent-skills`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-tools-content`, `mcp-tools-mfs`, `mcp-tools-ipns-keys`, `mcp-tools-dag-car`, `mcp-tools-routing-diag`, `mcp-pinning-with-confirmation`. Scheduled in Round 5.

## 1. Agent skills

- [x] 1.1 Write the push/pull and pinning skills. Verify: `skills-ref validate` passes for both
- [x] 1.2 Write the IPNS and MFS skills. Verify: `skills-ref validate` passes for both
- [x] 1.3 Write the DAG/CAR and troubleshooting skills. Verify: `skills-ref validate` passes for both
- [x] 1.4 Run `skills-ref validate` on all skills, the catalog name check and trigger evals. Verify: every check passes, and results are recorded
