# Tasks: ipfs-agent-skills

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `ipfs-agent-skills`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-tools-content`, `mcp-tools-mfs`, `mcp-tools-ipns-keys`, `mcp-tools-dag-car`, `mcp-tools-routing-diag`, `mcp-pinning-with-confirmation`. Scheduled in Round 5.

## 1. Agent skills

- [x] 1.1 Write the push/pull and pinning skills. Verify: `skills-ref validate` passes for both
- [x] 1.2 Write the IPNS and MFS skills. Verify: `skills-ref validate` passes for both
- [x] 1.3 Write the DAG/CAR and troubleshooting skills. Verify: `skills-ref validate` passes for both
- [x] 1.4 Run `skills-ref validate` on all skills, the catalog name check and trigger evals. Verify: every check passes, and results are recorded
  - Recorded (R11, 2026-10-11): `skills-ref validate` is clean for all 8 skills, and the catalog name check passes 41/41.
  - The lexical trigger proxy (`python3 mcp/skills/scripts/trigger_check.py` against `mcp/skills/evals/trigger-evals.json`) routes 20 of 22 queries correctly (0.91), re-run on 2026-10-11. It fails two queries:
    - `read bafy... and t...` routes to `ipfs-content-search` instead of `ipfs-retrieve-verify`;
    - `confirmation_unavailable ... add something` routes to `ipfs-publish-content` instead of `ipfs-troubleshooting`.
  - Open: the live-model near-miss trigger run. The 6 near-miss queries (`expect: null`) are not scored by the lexical proxy. Until that run is recorded, and the two misroutes are fixed or accepted, this task stays open (R11-Q06).
  - Live run (2026-10-12, `mcp/skills/evals/results-2026-10-12.md`): `skills-ref validate` valid for all 8 again; near-miss queries 6/6 clean; should-trigger 5 of 22 (23%) called the expected `ipfs:` skill, the rest used the MCP tools or the `ipfs` CLI directly. No threshold is stated in the spec, so the task stays open pending an operator decision on the bar (see "Proposed next steps" in the results file).
  - Pass bar (operator decision, 2026-10-12): at least 70% of should-trigger prompts load the expected skill and 100% of near-miss prompts stay clean, measured with `claude -p --max-turns 2 --disallowedTools Bash` against the installed plugin.
  - Round 2 (2026-10-12, `mcp/skills/evals/results-2026-10-12-r2.md`): descriptions rewritten to lead with user intent and then to say "invoke this skill first"; prompts now carry concrete payloads (24 positives, 8 near-misses). Result: should-trigger 13 of 24 (54%), near-misses 8 of 8 clean. Bar not met; the task stays open. Description wording moved the rate 50% to 54% (within noise); concrete payloads moved it 23% to 50%. Next lever is outside the skills (tool descriptions or server `instructions` pointing at the matching skill) or a lower bar for single-call requests. Lexical proxy on the new prompt set: 18 of 24 (it is only a proxy).
  - Round 3 (2026-10-12, `mcp/skills/evals/results-2026-10-12-r3.md`): with entry-tool descriptions in `mcp/contract/catalog.json` and the server `instructions` pointing at the matching skill (no skill description change), should-trigger 19 of 24 (79%), near-misses 8 of 8 clean. Bar met (>=70% and 100%); task 1.4 ticked. `skills-ref validate` valid x8 and `check_skills.py` OK 41/41 on the same tree. Margin is two prompts on one run per prompt.
