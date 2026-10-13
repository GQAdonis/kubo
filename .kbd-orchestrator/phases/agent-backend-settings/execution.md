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
- Reviewers: Claude Code native `reviewer` and `security-reviewer` (claude-opus-5-5), separate context, only at the points the plan names.
- Pushes, PRs, merges, cluster and registry actions: operator gates; the lead pushes branches under OG-1.

## Operator gate OG-1 (pending)

The operator directed "proceed to plan" and then "keep going as if [kbd-handoff] was already complete" without answering D0-D18. Work proceeds on the assumed defaults in plan section 1 for changes whose decisions are not contested; OG-1 answers are to be recorded here verbatim when given. Round 1 change `team-ownership-agent-backend` depends only on D10 (assumed default).

## Dispatch

- Round 1 started 2026-10-13: `team-ownership-agent-backend` 1.1 (ipfs-agent-architect) as the PR-K1 dispatch check.
- Execute stays active until every change satisfies the completion checklist; dispatch is not completion.
