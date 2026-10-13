# Tasks: team-ownership-agent-backend

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `team-ownership-agent-backend`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: none. Scheduled in Round 1.

## 1. Tasks

- [x] 1.1 Record the ownership extension in the proposal: `ui-ux-engineer` gains the whole `../ipfs-webui/**` fork (with `src/webmcp/**` writes still requiring security review), `../ipfs-desktop/src/tray.js`, Desktop branding assets (`assets/icons/**`, `assets/build/**`) and kubo `PRODUCT.md`; `mcp-server-engineer` gains `../ipfs-desktop/src/daemon/mcp-sidecar*.js`, the wiring lines in `src/daemon/index.js`, and `../ipfs-desktop/test/**/mcp-sidecar*` (W5); the architect gains `docs/config.md` (pointer entries only), `docs/changelogs/v0.44.md` (TOC and Highlights only; W5) and a know-me-cluster feature branch under `namespaces/ipfs/**`; the `WebUIPath` edit stays with the architect under D17. Acceptance: the proposal lists every path in assessment "Ownership" plus round-2 S3 and W5, and names who reviews `src/webmcp`.
- [ ] 1.2 Write the exact `team.json` patch and the native re-export command into the change folder, not applied (W5: `team.json` and the exports are unowned). Acceptance: `git apply --check` of the patch succeeds against the current `team.json`; each granted path appears once; no path is owned by two builder roles.
- [ ] 1.3 After OG-11, verify the applied grants. Acceptance: `jq '.roles[] | {id, owns}'` shows the patch's paths; a grep of `.claude/agents`, `.codex/agents`, `.kimi-code/agents`, `.opencode/agents` finds the new paths and no stale owns list.
