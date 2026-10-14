# Execution: agent-backend-settings

Dispatch contract for the Execute stage. Plan: `plan.md` (28 changes, 139 tasks, 11 rounds, 16 operator gates). Handoff rules: `HANDOFF-KIMI.md`.

## Prior context cited

- "The operator approves merges, tags and machine-level changes explicitly. A single up-front approval question at the phase boundary keeps the run unblocked." OG-1 is asked once, batched (below).
- "Tests must discover their environment ..." The driver passes repo paths explicitly and runs builders with no credentials; tests use non-default ports and their own `IPFS_PATH`.
- `.prometheus/gotchas.md` 2026-10-11 and 2026-10-13: timeouts with closed stdin, boundary repair, no blocking lifecycle hooks, no force-delete.

## Backend

- Change backend: OpenSpec (`openspec/changes/<id>/`), driven task by task through `kbd-apply` begin-task and end-task.
- Builders: Kimi Code 2.1.1, model `kimi-code/k3` at the configured effort `high`, launched by `driver/kimi-task.mjs` (WK-1 and PR-K4):
  - non-interactive prompt mode (`kimi -p ... --output-format stream-json`). Kimi rejects `--auto` with `--prompt`; prompt mode already runs tools without asking, so `--auto` is not passed (verified 2026-10-13 in a scratch repo: `pwd` and `git status` ran unattended).
  - launch environment without push, GitHub, cluster or registry credentials (`GH_CONFIG_DIR` empty, `GH_TOKEN`/`GITHUB_TOKEN`/`SSH_AUTH_SOCK` unset, `KUBECONFIG=/dev/null`, empty `DOCKER_CONFIG`, git credential helper cleared, `GIT_SSH_COMMAND=false`).
  - after the session the driver parses each `Bash` tool call in the stream-json log and rejects the task on `git push`, remote or credential changes, `gh` writes, `kubectl`, `argocd`, `helm`, `docker push/login`, Kimi config edits, or non-loopback mutating `curl`; it compares `git ls-remote origin` before and after for every repo in scope. `end-task` runs only when the session succeeded and passed both checks.
  - logs: `driver/logs/<change>-<task>-<UTC>.jsonl` (gitignored by convention: not committed).
- Planned (operator, 2026-10-13, verbatim: "yes, build the ACP driver after Round 2"): `driver/kimi-acp.mjs`, an ACP client over `kimi acp` (stdio) that streams session events live, answers tool permission requests by policy (denying outward actions before they run), and surfaces Kimi's questions to the lead, which escalates only operator decisions. Built after Round 2's builders finish; trialled on one low-risk task; `kimi-task.mjs` stays the default until it proves out. Unverified: how Kimi Code 2.1.1 sends questions over ACP.
- Built 2026-10-13 (18:30Z-18:55Z): `driver/kimi-acp.mjs` plus `driver/common.mjs`, now shared by both drivers (rules, credential-free env, remote-ref check, kbd-apply, prompt, acceptance check). Probe of `kimi acp`: ACP v1; `session/request_permission` arrives before every shell command with the command text; `session/new` reports model `kimi-code/k3`, thinking `high`, mode `default` (manual approvals); no agent-file option over ACP, so the role file opens the prompt. Questions use a driver convention: a turn ending with `QUESTION: ...` pauses the session, the question goes to `<log>.question-N.md`, the lead answers in `<log>.answer-N.md`, and the same session continues. Self-test in a scratch repo, rc=0: a command ran, `git push origin HEAD` was denied before execution, the question was answered and the session finished "Met". First self-test run exposed a driver bug (a denied call's `failed` update was counted as executed); fixed. Real-task trial still pending.
- Reviewers: Claude Code native `reviewer` and `security-reviewer` (claude-opus-5-5), separate context, only at the points the plan names.
- Pushes, PRs, merges, cluster and registry actions: operator gates; the lead pushes branches under OG-1.

## Operator gate OG-1 (answered 2026-10-13)

Operator, verbatim: "accept all, and yes copy the skills".

- D0-D18: accepted as recommended in plan section 1 (and assessment.md), including D0 (prepare the CORS-reduction PR now), D1 (dedicated console hostname; the name itself is still OG-2), D17 (the architect's single end-of-phase `WebUIPath` edit, so OG-7 is satisfied), and the plan-made assumptions.
- Pre-approved: the driver's feature-branch pushes and draft PRs in the four repos, the Desktop install into `/Applications`, and progress-report commits.
- Container registry for candidate images (PR-12): `ghcr.io/gqadonis/kubo`, the registry already used for the deployed image (plan-made assumption, accepted).
- PR-K3: builder skills copied into `~/.kimi-code/skills` on 2026-10-13: `golang-patterns`, `golang-testing`, `mcp-builder`, `documentation-and-adrs`, `api-and-interface-design` (from `~/.claude/skills`), `go-mcp-server-generator` (from the repo's `.agents/skills`). Each has a `SKILL.md`.
- ADR-0009 may move to `Accepted`.

## Operator gate OG-2 (answered 2026-10-13)

Operator, verbatim: "use console.prometheusags.ai for the console hostname".

- Console host: `console.prometheusags.ai` (D1). DNS for it is the operator's part of OG-2; not yet confirmed.

## Operator gate OG-3 (2026-10-13)

Operator, verbatim: "filed the UAR issue, and the current UAR code base fixes that issue (check that and update any references)".

- No issue newer than #379 is visible in `Prometheus-AGS/universal-agent-runtime` (`gh issue list`, 15:0xZ); the filed issue is taken to be #379. The v1.1 follow-up (`mcp/uar/requests/2026-10-13-v1.1.md`) is not filed.
- Read-only audit of the UAR working tree at `b4244aea`: #379 items 1, 4, 5, 6, 8 done; 3 and 7 mostly; 2 partial; 9 code only. Nothing committed, no tag above `v1.0.0`. The operator's report holds for the working tree only; OG-4 (published release) is not met.

## Defect: a blocked task closed as done (2026-10-13)

- `mcp-agui-a2ui-registry` 1.1 ran 16:29Z-16:49Z as `mcp-server-engineer`, wrote no code, and reported "Acceptance result: Not met — blocked": the contract names no A2UI catalog route or advertisement event (architect-owned `mcp/contract/**`), and embedding the catalog without a copy needs a Go file under `mcp/ui/catalog/` (ui-ux-owned). The driver closed the task anyway because it ran `end-task` on any clean exit. The runtime shows 1.1 done; the work is not.
- Driver fix: `kimi-task.mjs` now reads the last "Acceptance result" section and exits 7 (task left open, batch stops) on not met, partially met, unmet or blocked. Checked against all 20 session logs of the day: only this task matches.
- Repair, after the Round 2 chain: (1) `ipfs-agent-architect` adds the catalog route and run-stream advertisement event to `mcp/contract`; (2) `ui-ux-engineer` adds `mcp/ui/catalog/catalog.go` embedding the generated catalog; (3) `mcp-server-engineer` re-runs 1.1 without `--kbd` (already closed in the runtime). Acceptance is checked by the lead before the round is reported.
- Repair done 17:56Z-18:15Z (all three sessions rc=0, acceptance Met): contract `derivations/a2ui-components.json` names `serve.route: /agui/catalog` and `aguiEvents` (`a2ui`, `a2ui.catalog`); `mcp/ui/catalog/catalog.go` embeds the generated file; `mcp/server/http.go` `RouteAGUICatalog`, advertisement once after `RUN_STARTED`, `agent.a2ui: false` disables both; `test/cli/mcp_agui_catalog_test.go`. The work now matches the runtime's closed state.
- Second driver false positive (18:01Z): `lke-origin-isolation` 1.2 was rejected because the `helm` rule matched the path `helm-values/` in a read-only grep. Cluster-tool rules now match only in command position (11 test cases); 1.2 closed by the lead after re-running its targeting check (PASS).

## Dispatch

- Round 1 started 2026-10-13: `team-ownership-agent-backend` 1.1 (ipfs-agent-architect) as the PR-K1 dispatch check.
- Execute stays active until every change satisfies the completion checklist; dispatch is not completion.
