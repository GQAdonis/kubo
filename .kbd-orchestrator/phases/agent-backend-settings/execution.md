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
- Real-task trial 2026-10-14 (operator: "trial the ACP driver on a low risk task to confirm its use"): `webui-promethity-assets` 1.1 with `--kbd`, rc=0; 16 shell commands, each approved through `session/request_permission`, none denied, no question asked; acceptance PASS; the runtime closed the task. The session titled its verdict "Acceptance check", which the not-met detector did not read; the pattern now covers "Acceptance result" and "Acceptance check" (all prompt-mode logs re-checked: still only `mcp-agui-a2ui-registry` 1.1 flagged). From Round 3 task 2 on, builders run through `kimi-acp.mjs`; `kimi-task.mjs` stays as the fallback.
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

## OG-0 lifted (2026-10-14)

Operator, verbatim: "record the evidence, lift OG-0, and register the changes". All `lke-origin-isolation` 1.8 checks pass (unauthenticated by the lead, credentialed by the operator in their own terminal): evidence in `openspec/changes/lke-origin-isolation/evidence/2026-10-14-og6/README.md`. OG-0 hygiene (do not open gateway content in a browser holding the Web UI basic-auth credentials) is lifted: the gateway origin is refused by kubo (403), the console origin works (200), and every `/api/v0` and `/webui` route on both hosts is behind basic auth (401).

## Operator gate OG-16 (answered 2026-10-14): plan revision 4, whole-node vector search

Operator, verbatim: "approve the vector search plan revision".

- Approved: plan revision 4 (`plan-revision-4-proposal.md`), goal 6, ADR-0011 decisions V1-V9, and the recommendations batched into OG-16: O-5 (search off by default everywhere), O-6 (embedding settings in the config file, restart to change; hot reload deferred), O-7 (no Web UI "start indexing" button; runs start from the agent chat), O-8 (default bulk caps 1000 files, 1 GiB, 30 files per minute per run), O-10 (the candidate image does not wait for the vector chain; `lke-vector-search` moves to a Round 12 on the final image if the chain slips).
- Still open, at their own gates: OG-17 (describer per node), OG-18 (LKE GPU and embedder placement), OG-19 (artifact pins, liter-llm deployment, Desktop binaries bundled or installed, Hugging Face downloads for the LKE warm Job), OG-20 to OG-22.
- Cluster work in this goal (`lke-vector-search`) follows the know-me-cluster skill `lke-gitops-change` (live preflight posted before merge, verify after sync).

## Operator gates OG-17, OG-18, OG-19 (answered 2026-10-14)

Operator, verbatim: "go with your recommendations for OG-17, OG-18 and OG-19".

- **OG-17 (O-1, describer per node).** Desktop: a local vision model for images (an operator-configured OpenAI-compatible endpoint on loopback, for example Ollama with a vision model); video description off. LKE: describer off until the operator has read Xiaomi's data terms; afterwards hosted MiMo for images, and video only by a further explicit operator choice (a new gate when it arises). Search stays off by default everywhere (O-5).
- **OG-18 (O-2, LKE compute).** No GPU on LKE. Embeddings run on CPU inside the cluster (ClusterIP only); the describer there is hosted or off, per OG-17.
- **OG-19 (O-3, O-4, O-9, artifacts).** liter-llm is not deployed at first; if added later, the upstream image `ghcr.io/xberg-io/liter-llm` pinned by digest unless a fork-only feature is needed. Desktop uses operator-installed `xberg` and `surreal` binaries by absolute path (not bundled). Hugging Face model downloads are allowed only for the LKE xberg warm Job; Desktop stays offline and the docs state what that leaves out. Artifact pins: the xberg `v1.3.10` image digest and macOS CLI checksum, and the SurrealDB and embedder image digests, are resolved and recorded by the owning task at the time it runs (`lke-vector-search`, `desktop-search-sidecar`), never as floating tags.

## Operator gate OG-6 (answered 2026-10-14)

Operator, verbatim, in the lead session: "Approved" (OG-6: merge the cluster PR and the planned restart), then "do the restart once the policy is accepted". Policy `ipfs-webui-auth` became `Accepted=True` on both listeners after know-me-cluster#25. The restart was then held by the lead: the console listener is not Programmed (TLS), and the child phase `lke-deploy-discipline` gates it on a passing `verify.sh`, a pre-restart snapshot, a bounded wait, and a stop-and-hand-over rule if the pod hangs (no force delete; recovery on the node is an operator action).

## Console TLS and the OG-6 restart (2026-10-14, child lke-deploy-discipline)

- know-me-cluster#28 (skill `lke-gitops-change`) merged by the operator. #29 (console on the wildcard `ipfs-tls`) opened with a live preflight PASS; security read PASS WITH FINDINGS (LOW only).
- Meanwhile `console-tls` was issued by Let's Encrypt at 10:11Z through #27's HTTP-01 path. The operator answered "Close #29, keep LE cert"; the lead ran `gh pr close`, but #29 was merged at 10:41:19Z by GQAdonis at the same moment. Live result: the console serves `ipfs-tls`; Certificate `console-tls` deleted on sync; Secret `console-tls` orphaned. The lead's "closed" comment, the issue #30 update and child task 2.4 "cancelled" were wrong and are corrected (PR comment, #30 edited, task 2.6 registered for PR-B #31). Operator then chose "Keep wildcard, do PR-B".
- **The OG-6 restart overlapped #29's sync.** Restart 10:41:44Z-10:42:17Z (33 s, no hang); the pre-restart snapshot at 10:41:33Z still showed Argo at the pre-#29 revision, so the plan's "no restart while a cluster PR is open or unsynced" guard read stale state. Outcome: post-restart verify PASS after one transient timeout at 10:43:29Z; `origin-isolation.sh unauth` PASS; cross-host request (ipfs SNI, console Host) 404. Lesson: check open PRs and their merge state immediately before the restart command, not only in the snapshot.
- Skill tweak (redundant-certificate as INFO for an issued, Ready certificate) is stashed in know-me-cluster (`git stash list`), not needed after #29.

## Round 3 ledger mode while the child phase is active (2026-10-14)

- `webui-promethity-foundation` 1.2 first timed out (30 min, mostly reasoning; partial work kept), then was resumed (driver now resumes an already-started task) and completed, but `end-task` was refused ("local runtime rejected the command") because the active path is the child `lke-deploy-discipline`. Closed with `kbd-apply mark-done` and the after-boundary repaired with `prometheus kbd guard evaluate --boundary task --edge after --subject webui-promethity-foundation/1.2 --repair-projections` (outcome pass).
- Until the child exits, the Round 3 chain runs without `--kbd`: after a session passes the driver's checks (no executed outward action, remote refs unchanged, acceptance not reported unmet) the lead records the task. Hooks do not fire for these tasks; this is recorded here as the deviation.
- Correction (2026-10-14, about 10:00Z): `kbd-apply mark-done` resolves tasks against the active (child) phase. For 1.2 it updated the projection only (the runtime still showed `in_progress`), and for 1.3 it refused ("none matches backend task 1.3"). Recording now uses typed runtime transitions that name the phase explicitly: `prometheus kbd task transition --phase agent-backend-settings --change <id> --id <n> --status in-progress`, then `--status complete --summary ...`, and the lead ticks `tasks.md`. Runtime after the fix: `webui-promethity-foundation` 1.1-1.3 complete. Kimi ticked 1.3 in `tasks.md` itself despite the prompt; prompts now say "Do not edit any tasks.md".
- ACP driver: thought chunks are no longer logged (counted only); a task whose start is already recorded is resumed instead of refused.

## Operator direction 2026-10-14: vector search over all node files (scope change)

Operator, verbatim: "I need to be able to search for files in IPFS based on vector search." Answers to the lead's questions, verbatim:

- Where: "Cloud node (LKE), IPFS Desktop (local Mac)".
- Embedder host: "This should be configurable."
- File types: "Text and Markdown, PDF and Office docs, Source code and JSON/CSV, Images, I want to be able to configure Mimo v2.6 models to assist with image and video search by creating embeddings off inference. I want to use xberg open source for document processing."
- Search UI: "Agent chat / MCP clients, Search box in the Web UI".

Identified: xberg = `github.com/xberg-io/xberg` (renamed Kreuzberg); MiMo-V2.6 = Xiaomi's open-weight omnimodal family (text, image, video, audio understanding; OpenAI-compatible hosted API). Gaps against the delivered G12 feature: no bulk indexing of existing pins or MFS (only adds through `ipfs mcp` are auto-indexed), text and Markdown only, search only through agents, nothing configured on LKE. Operator, verbatim, later the same day: "I want to use our fork at git@github.com:Prometheus-AGS/xberg.git for the xberg support." Checked read-only: a fork of `xberg-io/xberg`, default branch `main`, 0 commits ahead or behind upstream `main`, latest tag `v5.0.0-rc.35`; offers CLI, REST API, MCP server and a Go binding. Superseded the same day after research showed the fork's only tag (`v5.0.0-rc.35`) is stale Kreuzberg-era code and its Docker and Go setup point at `xberg-io`. Operator, verbatim: "use the upstream xberg original repo latest version". Decision: `xberg-io/xberg` latest release, `v1.3.10` (2026-10-13) at research time, image `ghcr.io/xberg-io/xberg` pinned by digest; the Prometheus-AGS fork is not used. Operator, verbatim: "ok, continue with Round 3 and use git@github.com:GQAdonis/liter-llm.git if necessary for providing access to the various models we need". Checked read-only: a fork of `xberg-io/liter-llm` (universal LLM client and gateway, Rust core), default branch `main`, tags up to `v1.1.1`. Next: read-only research, then an architect plan revision (new goal, ADR, OpenSpec changes) for operator approval before any code.

## Dispatch

- Round 1 started 2026-10-13: `team-ownership-agent-backend` 1.1 (ipfs-agent-architect) as the PR-K1 dispatch check.
- Execute stays active until every change satisfies the completion checklist; dispatch is not completion.
