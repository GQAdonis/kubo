# Design: mcp-ci-workflow

## Context

- See proposal.md for why. The checks themselves already exist and were run by hand in R11 (`docs/mcp/progress-report.md` section 5.1). This change only schedules them.
- Upstream workflows in this repository (`gotest.yml`, `golint.yml`, `gobuild.yml`) pin `actions/checkout`, `actions/setup-go`, `actions/setup-node` and `actions/upload-artifact` by tag (`@v7` and so on); only `codecov/codecov-action` is pinned by SHA. Their jobs are guarded by `github.repository == 'ipfs/kubo'`.
- No package under `mcp/` declares `packageManager`, there is no `.nvmrc`, and `versions.toml` pins no Node or pnpm version. The packages require Node `>=22` (`engines`), and the lockfiles are `lockfileVersion: '9.0'`.
- `mcp/app` and `mcp/chat` link `@gqadonis/ipfs-mcp-ui` from `../ui` and import its built `dist/`, so `mcp/ui` must be built first.
- pnpm does not run the `electron` install script unless approved; `mcp/chat/tests/e2e/harness.ts` documents `node node_modules/electron/install.js` as the way to fetch the binary.
- The Playwright screenshot baselines are not platform-suffixed. `snapshotPathTemplate` in all three configs has no `{platform}` token; the files live under `__screenshots__/chromium/` (and `electron44/` for `mcp/chat`). They were produced on the operator's macOS 26 arm64 host (26.7.1 on 2026-10-11). Correction (R12 review REV-M2): an earlier version of this design called them darwin-suffixed.

## Goals / Non-Goals

**Goals:**

- Run the R11 verification set automatically on changes to MCP paths, in the fork only.
- Least privilege: read-only token, no secrets, SHA-pinned actions.

**Non-Goals:**

- Running the Ollama-backed tests (they need a local model; `-skip Ollama`).
- Building or publishing release archives, signing, tagging.
- Changing upstream workflows or Kubo's own CI coverage.
- Path filtering per job: every MCP job runs on any MCP-path change. Splitting Go-only and UI-only triggers is possible later if runner cost matters.

## Decisions

1. **SHAs resolved from the action repositories, not copied from upstream.** Upstream has no SHA pins for these actions. The SHAs were resolved with `git ls-remote --tags` on 2026-10-11 for the newest tag of the major version upstream uses (`checkout` v7.0.1, `setup-go` v7.0.0, `setup-node` v7.1.0, `upload-artifact` v7.0.2) and `pnpm/action-setup` v6.1.0 (annotated tag, dereferenced to its commit). Alternative: tag pins like upstream. Rejected: the operator asked for SHA pins, and a tag can be moved.
2. **Node 24.11.1 and pnpm 11.25.0.** No repository pin exists, so the workflow uses the versions the lockfiles were produced and tested with on the operator's machine, declared once as workflow `env`. Alternative: Node 22 (the `engines` floor). Rejected: untested here. Follow-up, not in this change: declare `packageManager` in the three packages so the version lives with the code.
3. **Go version from `go.mod`** through `setup-go` `go-version-file`, as upstream does.
4. **Lint the way upstream does**: `make -O test_go_lint`, which installs the pinned `golangci-lint` from `test/bin`. On Linux the upstream `fuse/node/mount_darwin.go` finding seen on the operator's Mac does not apply.
5. **`go vet` package set**: `./cmd/ipfs/... ./mcp/... ./test/cli/...` for each of `nomcp` and `noagent`. The R11 record says only that vet with both tags was clean; it does not list packages. This set covers the command, the server and the CLI tests (the earlier `noagent` vet failure was in `test/cli`).
6. **Playwright on the pinned `macos-26` image.** The baselines' paths are platform-agnostic, so any runner compares against them; macOS is chosen because it is the renderer closest to the host that produced them. The label is pinned rather than `macos-latest` (R12 REV-M3), so a GitHub image migration cannot change the renderer under the baselines. `macos-26` is GitHub's standard arm64 macOS 26 image (per the `actions/runner-images` README on 2026-10-11: arm64 labels `macos-latest`, `macos-26`, `macos-26-xlarge`; `macos-26-intel` and `macos-26-large` are x64). It matches the operator's macOS 26 arm64 host in OS major version and architecture; fonts, display and GPU path can still differ. Only Chromium is installed (`playwright install chromium`); Electron comes from `mcp/chat`'s devDependency. Unit tests, `gen:check` and `check:dist` run on `ubuntu-latest`, which is cheaper.
7. **Artifacts only on failure**: the Playwright configs use the `list` reporter, so the useful output is `test-results/` (diffs, traces); `playwright-report/` is included in case a reporter is added. `if-no-files-found: ignore`, 14-day retention.
8. **Baselines are the operator's, never CI's.** CI never runs `test:visual:update` and never commits or uploads replacement baselines. If the first run differs only by hosted-runner rendering, the remedy is an operator decision recorded in this design (task 1.4): for example, a per-spec threshold with a stated reason, adding `{platform}` to `snapshotPathTemplate` with a separate, reviewed set of CI baselines, or running the visual specs only on the operator's host. Regenerating on CI over the operator's files is not an option.
9. **Triggers (R12 REV-L6, REV-L7).** The path filter also covers `test/cli/harness/**`, `Makefile` and `mk/**`, which the Go job depends on. `push` is restricted to `master` and `feat/**`, so a pull request from a branch of this repository does not also run once per push on an unrelated branch. Both events share one path list through a YAML anchor.
10. **Node before pnpm (R12 REV-L8, REV-N3).** `setup-node` runs first, then `pnpm/action-setup`, so pnpm is installed with the pinned Node rather than the runner's preinstalled one. The pnpm store is cached by `pnpm/action-setup` (`cache: true`, `cache_dependency_path` listing each lockfile once) instead of by `setup-node`, whose `cache: pnpm` requires pnpm to be installed before it. `package_json_file` points at `mcp/ui/package.json` because the repository root has no `package.json`; no package declares `packageManager`, so the `version` input decides.
11. **No token on disk (R12 SEC-L4).** Every `actions/checkout` sets `persist-credentials: false`, so later steps, which run code from the pull request, cannot read the `GITHUB_TOKEN` from `.git/config`.

## Risks / Trade-offs

- [Hosted-runner rendering differs from the operator's Mac (fonts, display scaling, GPU path), so screenshot comparisons fail without a UI change; likely on the first run given `maxDiffPixelRatio: 0.005` and `retries: 0`] → The operator decides after the first run and the decision is recorded here (decision 8). CI never regenerates the baselines. Not loosened now.
- [`check:dist` is not byte-identical between macOS and Linux builds] → Make the build deterministic across platforms; do not skip the check.
- [The race detector and the CLI suite are slow on a 2-core runner] → 45-minute job timeout; split the job if it is exceeded.
- [The CLI suite starts real daemons] → Each test uses its own harness repo and OS-chosen ports, and `IPFS_PATH` points into `runner.temp`; no other node runs on a hosted runner.
- [Pinned SHAs age] → Bump them deliberately with a resolved SHA and version comment; never switch to a floating tag.

## Migration Plan

Add the file. Rollback: disable the workflow in the Actions settings or delete the file.
