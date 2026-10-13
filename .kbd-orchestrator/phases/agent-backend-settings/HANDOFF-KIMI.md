# Handoff to Kimi Code: agent-backend-settings

Written 2026-10-13 by the Claude Code lead session at commit `a93d72b44` (branch `feat/ipfs-mcp-server`, pushed). Read this file first, then the files in "Read in order". This file records position and rules; it does not override AGENTS.md, the plan, or canonical KBD state.

## Position

- Repo: `/Users/gqadonis/Projects/references/kubo`. Forks: `../ipfs-webui` (GQAdonis/ipfs-webui, branch `feat/ipfs-mcp-server`), `../ipfs-desktop` (tribe-health/ipfs-desktop, branch `feat/ipfs-mcp-server`). Cluster repo: Prometheus-AGS/know-me-cluster (`namespaces/ipfs/manifests.yaml`, Argo app `ipfs`, kubectl context `know-me`).
- Phase `agent-backend-settings`: Assess complete, Plan complete, **Execute not started**. 28 changes and 139 tasks are registered in the KBD runtime; implementation 0 of 28.
- Waypoint next: `team-ownership-agent-backend` task 1.1 (`.kbd-orchestrator/current-waypoint.json`).
- Previous phase `ipfs-mcp-server` is still open in Execute (31 of 37); its carry-over is folded into this plan. Do not close it from here.

## Read in order

1. `AGENTS.md` (resident rules; Stability and User Agency outrank every task; `5f019f0df` default-on rule).
2. `.prometheus/gotchas.md` (shell hang, KBD close repair, the 2026-10-13 LKE outage lessons).
3. `.kbd-orchestrator/phases/agent-backend-settings/plan.md`: section 1 (assumed decisions D0-D18), section 2 (operator gates), section 4 (changes and acceptance), section 5 (rounds), section 10 (Task model assignments: launch commands WK-1, reviewer route RK-R), section 12 (round-2 review findings not re-vetted).
4. `assessment.md` (synthesis and cloud threat model), then `openspec/changes/<id>/` for the change you run.

## Start of Execute: what must happen first

1. Run `/kbd-execute agent-backend-settings`: it writes `execution.md` and `execute-dispatch.json`. Record the backend as Kimi Code per plan section 10 and copy the operator's OG-1 answers verbatim into `execution.md`.
2. **OG-1 is not answered yet.** The operator said "proceed to plan" without confirming D0-D18. Ask the operator once, as one batched question, before Round 1 work that depends on a decision. Do not change a decision on your own.
3. **OG-0 is in force now:** the production node has a live RPC exposure through the shared gateway origin. The operator must not open gateway content in a browser that holds the Web UI basic-auth credentials until the OG-6 checks pass. Remind the operator; do not touch the cluster.

## Task execution rules

- Drive every task through `kbd-apply` (`~/.kimi-code/skills/kbd-apply/kbd-apply.sh begin-task <change> <id> <i> <n> "<title>"`, then the work, then `end-task` with the same arguments). Tick `[x]` in `openspec/changes/<id>/tasks.md` only through that path; a checkbox alone does not move the runtime (gotcha 2026-10-10).
- Run every `kbd-apply` and `prometheus kbd` call under `timeout` with `</dev/null`. After an interrupted close, check `boundaryObligations` in `prometheus kbd status --json` and repair with `prometheus kbd guard evaluate --boundary <change|task> --edge after --subject <change|change/id> --json --repair-projections`.
- One Kimi session per role and change, launched as in plan section 10 WK-1 (`--agent-file .kimi-code/agents/<role>.md`, `-m kimi-code/k3`, `--auto`, prompt mode, under `timeout`, stdin closed). Builders get no push, `gh`, cluster or registry credentials; the driver scans each session's output for outward actions and rejects the task on any hit.
- Reviewers (`reviewer`, `security-reviewer`) run on Claude Code (claude-opus-5-5), never on Kimi, and never in a builder's context. They run only at the points the plan names (the pre-LKE `settings-security-review`, the security read of each know-me-cluster PR, and `phase-integration-gate`).
- Verification boundary: no per-task test loops. Finish each round's production changes, then the named checks; one integration gate at the phase boundary. Exceptions are listed in plan section 4.
- Ownership: edit only paths your role owns in `.agent-team/kubo-ipfs-agentic/team.json`. The unowned paths become owned only after OG-11 applies the `team-ownership-agent-backend` patch.

## Never do (operator-only or forbidden)

- Push, tag, merge, publish, file issues, `kubectl apply`, Argo sync, image push, or DNS: these are operator gates (OG-1 through OG-15). Prepare the PR or command and stop.
- Read or print `.env`, `.pem`, Kubernetes Secret values, or Keychain secrets.
- Touch the operator's running IPFS node, `~/.ipfs`, IPFS Desktop's daemon, Ollama (11434), or ports 4001, 5001, 8080, 4000. Tests use non-default ports and their own `IPFS_PATH`.
- `pkill` or `killall`: stop processes by PID only.
- Force-delete a pod, or add a blocking `lifecycle` hook (2026-10-13 outage).
- Large here-documents or here-strings in shell (bash 5.3 pipe hang); write files with the editor tool.
- Edit `AGENTS.md` (the D12 clarification is the operator's own PR, OG-5), `versions.toml` (operator-directed only), CI workflows, or the managed `prometheus-base` region.
- Install Desktop builds anywhere but `/Applications` (quit the running app first).

## Open prerequisites before Round 1

- PR-K1: `/kbd-apply` dispatching to Kimi sessions is unverified. Do one dry run on `team-ownership-agent-backend` 1.1 and record the result in `execution.md`.
- PR-K3: skills missing from Kimi Code (`~/.kimi-code/skills`): `golang-patterns`, `golang-testing`, `security-review` (Claude Code only; the security skills are covered by the Claude reviewers), and `mcp-builder`, `code-review-and-quality`, `documentation-and-adrs`, `api-and-interface-design` (only in the old `~/.kimi/skills`). `go-mcp-server-generator` and `mcp-implementation-security-review` are in the repo's `.agents/skills/`; whether Kimi discovers that folder is unverified. Installing skills into `~/.kimi-code/skills` changes the operator's machine and needs approval. `~/.kimi-code/skills/kbd-handoff` is a broken symlink.
- PR-K4: the launch-environment hardening and the post-session outward-action scan are not built yet; build them before the first `--auto` session.
- Effort: everything runs at the configured `high` (`~/.kimi-code/config.toml`); 40 hard-tier tasks are below their planned `max`. Changing the global effort needs operator approval and must not race parallel sessions.

## Reporting

- After each round: update `docs/mcp/progress-report.md` (living document; append to its change log), and give the operator the full KBD status (phase, stage, N of 28, next task, pending gates).
- Ask operator-only actions explicitly, one batch at a time, with what, why, and why it cannot be automated.
- Show commands and their real output; say what was not verified.
