# Tasks: mcp-ci-workflow

Phase `ipfs-mcp-server`, follow-up to R11-Q05; operator decision 5 of 2026-10-11 (`docs/mcp/progress-report.md` section 5.1). Owner: `ipfs-agent-architect` (the operator authorized this one new file under `.github/`).

## 1. Workflow

- [x] 1.1 Write `.github/workflows/mcp.yml` with the jobs, triggers, repository guard, read-only permissions, per-ref concurrency and SHA-pinned actions described in the proposal and design. Verify: `actionlint .github/workflows/mcp.yml` exits 0, and the file parses with PyYAML into the jobs `go`, `go-lint`, `ui-unit`, `playwright`
- [x] 1.2 Apply the R12 review fixes to the workflow: platform-agnostic baseline premise corrected (REV-M2), Playwright runner pinned to `macos-26` (REV-M3), `persist-credentials: false` on every checkout (SEC-L4), `test/cli/harness/**`, `Makefile` and `mk/**` added to the paths (REV-L6), `push` limited to `master` and `feat/**` (REV-L7), Node set up before pnpm with the store cached by `pnpm/action-setup` (REV-L8, REV-N3). Verify: `actionlint .github/workflows/mcp.yml` exits 0; PyYAML shows the same path list on both events and `push.branches` `[master, feat/**]`
- [ ] 1.3 First run on GitHub after the branch is pushed to GQAdonis/kubo (push or `workflow_dispatch`; pushing needs operator approval). Verify: all four jobs complete; record the run URL and each job's result in `docs/mcp/progress-report.md`
- [ ] 1.4 If a Playwright job fails only on screenshot differences caused by hosted-runner rendering, the operator decides the remedy (a per-spec threshold with a stated reason, a separate reviewed set of CI baselines behind `{platform}` in `snapshotPathTemplate`, or visual specs only on the operator's host). CI never regenerates or overwrites the operator's baselines. If `check:dist` fails on Linux only, make the `mcp/app` build deterministic across platforms. Verify: a following run is green, and the operator's decision is recorded in `design.md` (decision 8)
- [ ] 1.5 Correct the `mcp/app/embed.go` comment if it still says `check-dist` does not run in CI (owner `ui-ux-engineer`). Verify: the comment names `.github/workflows/mcp.yml`
