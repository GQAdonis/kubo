# Proposal: mcp-ci-workflow

Phase `ipfs-mcp-server`, follow-up to the R11 phase-boundary review (reviewer finding R11-Q05). Operator decision of 2026-10-11 (`docs/mcp/progress-report.md` section 5.1, decision 5): add CI under `.github/` for the MCP work. The operator explicitly authorized editing `.github/` for this one new workflow; upstream workflows are not modified.

## Why

No CI job runs the `mcp/` checks. The committed `mcp/app/dist` bundle that `ipfs mcp` embeds can drift from its sources, the config reference can go stale against the schema and `--help`, and the race, CLI, skills and Playwright checks run only when someone remembers to run them by hand (R11-Q05). Every R11 verification was a manual run on the operator's Mac.

## What Changes

- New workflow `.github/workflows/mcp.yml` (name `MCP`), triggered by `pull_request` and by `push` to `master` or `feat/**`, on `mcp/**`, `cmd/ipfs/kubo/mcp*`, `test/cli/mcp*`, `test/cli/harness/**`, `docs/mcp/**`, `go.mod`, `go.sum`, `Makefile`, `mk/**` and the workflow file, plus `workflow_dispatch`.
- Every job is guarded by `github.repository == 'GQAdonis/kubo'`, so nothing runs if the branch is proposed upstream.
- `permissions: contents: read` at the top level; one concurrency group per ref with cancel-in-progress.
- Every third-party action pinned by full commit SHA with a version comment; every checkout sets `persist-credentials: false`.
- Jobs:
  - `go` (ubuntu): `make build`; `go vet` with the `nomcp` and `noagent` tags; `go test -race ./mcp/... ./cmd/ipfs/kubo/`; the MCP CLI integration tests (`-run 'TestMCP|TestCommandDocsWidth' -skip Ollama`) against the built binary with a temporary `IPFS_PATH`; `config_reference.py --check` against the built binary; `check_skills.py`.
  - `go-lint` (ubuntu): `make -O test_go_lint`, the way upstream `golint.yml` runs it.
  - `ui-unit` (ubuntu): frozen installs; in `mcp/ui` `gen:check`, typecheck, unit tests, build, URL scan; in `mcp/app` typecheck, unit tests, `check:dist`, bundle scan; in `mcp/chat` typecheck, unit tests, build, URL scan.
  - `playwright` (pinned `macos-26` image, matrix `ui`, `app`, `chat`): Chromium only, plus Electron 44 for `mcp/chat` through its devDependency; reports uploaded as artifacts only on failure.
- No change to upstream workflows, to `/api/v0`, the gateway, the CID recipe, wire behavior or Kubo config. No secrets are used. The workflow sends nothing anywhere except what GitHub-hosted runners already do (checkout, action and package downloads).

## Capabilities

### New Capabilities

- `mcp-continuous-integration`: the fork's automated checks for the `ipfs mcp` server, its contract-derived docs and skills, and the `mcp/ui`, `mcp/app` and `mcp/chat` packages.

### Modified Capabilities

None.

## Impact

- New file `.github/workflows/mcp.yml`. Nothing else under `.github/` changes.
- Runner cost: one ubuntu Go job (about 20-30 minutes with the race detector and the CLI suite), one ubuntu lint job, one ubuntu UI job, and three macOS Playwright jobs per triggering push or pull request.
- Off switch: disable the `MCP` workflow in the repository's Actions settings, or delete the file. Nothing in Kubo depends on it.

## Known risk: screenshot baselines on hosted macOS runners

The committed Playwright baselines have platform-agnostic paths (no `{platform}` in `snapshotPathTemplate`) and were produced on the operator's macOS 26 arm64 host. (Corrected in R12, REV-M2: this proposal first called them darwin-suffixed.) The Playwright jobs run on the pinned `macos-26` arm64 image, the closest hosted renderer, but font rasterization, display scaling and the GPU path may still differ, so the visual comparisons (`maxDiffPixelRatio: 0.005`, `retries: 0`) may fail on the first CI run without any UI change.

Remedy, decided by the operator after the first run and recorded in `design.md`: for example a per-spec threshold with a stated reason, a separate reviewed set of CI baselines behind `{platform}`, or visual specs only on the operator's host. CI never regenerates or overwrites the operator's baselines. Thresholds are not loosened in this change.

A second, smaller risk: `mcp/app` `check:dist` runs on Linux, while the committed `dist/` was built on macOS. If the build is not byte-identical across platforms, `check:dist` fails; the remedy is to make the build deterministic, not to skip the check.
