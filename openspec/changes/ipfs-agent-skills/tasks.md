# Tasks: ipfs-agent-skills

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `ipfs-agent-skills`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-tools-content`, `mcp-tools-mfs`, `mcp-tools-ipns-keys`, `mcp-tools-dag-car`, `mcp-tools-routing-diag`, `mcp-pinning-with-confirmation`. Scheduled in Round 5.

## 1. Agent skills

- [x] 1.1 Write the push/pull and pinning skills. Verify: `skills-ref validate` passes for both
- [x] 1.2 Write the IPNS and MFS skills. Verify: `skills-ref validate` passes for both
- [x] 1.3 Write the DAG/CAR and troubleshooting skills. Verify: `skills-ref validate` passes for both
- [ ] 1.4 Run `skills-ref validate` on all skills, the catalog name check and trigger evals. Verify: every check passes, and results are recorded
  - Recorded (R11, 2026-10-11): `skills-ref validate` is clean for all 8 skills, and the catalog name check passes 41/41.
  - The lexical trigger proxy (`python3 mcp/skills/scripts/trigger_check.py` against `mcp/skills/evals/trigger-evals.json`) routes 20 of 22 queries correctly (0.91), re-run on 2026-10-11. It fails two queries:
    - `read bafy... and t...` routes to `ipfs-content-search` instead of `ipfs-retrieve-verify`;
    - `confirmation_unavailable ... add something` routes to `ipfs-publish-content` instead of `ipfs-troubleshooting`.
  - Open: the live-model near-miss trigger run. The 6 near-miss queries (`expect: null`) are not scored by the lexical proxy. Until that run is recorded, and the two misroutes are fixed or accepted, this task stays open (R11-Q06).
