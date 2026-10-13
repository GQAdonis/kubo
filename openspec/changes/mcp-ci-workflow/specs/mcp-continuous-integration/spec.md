# Spec Delta: mcp-continuous-integration

## Purpose

Automated checks for the fork's `ipfs mcp` server, its contract-derived docs and skills, and the `mcp/ui`, `mcp/app` and `mcp/chat` packages, run on GitHub-hosted runners for the GQAdonis/kubo fork only.

## ADDED Requirements

### Requirement: Runs only in the fork
Every job of the `MCP` workflow SHALL run only when the repository is `GQAdonis/kubo`. The workflow SHALL NOT modify or replace any upstream workflow.

#### Scenario: Branch proposed upstream
- **WHEN** the branch is opened as a pull request against `ipfs/kubo`
- **THEN** every `MCP` job is skipped and no runner time is spent

### Requirement: Least privilege and pinned actions
The workflow SHALL request only `contents: read`, SHALL use no repository secrets, SHALL NOT persist the job token in the checked-out repository, and SHALL pin every third-party action by full commit SHA.

#### Scenario: Action reference audit
- **WHEN** the workflow file is inspected
- **THEN** every `uses:` names a 40-character commit SHA, and the top-level permissions are `contents: read`

### Requirement: Go checks
On a change to the MCP paths, the workflow SHALL build `ipfs` with `make build`, vet the `nomcp` and `noagent` build variants, run the `mcp` and `cmd/ipfs/kubo` unit tests with the race detector, run the MCP CLI integration tests against the built binary without contacting a model backend, check the config reference against the schema, the sidecar and `--help`, check the skills against the tool contract, and run the Go linter.

#### Scenario: Stale config reference
- **WHEN** a pull request changes `mcp/contract/config/ipfs-mcp-config.schema.json` without regenerating `docs/mcp/config-reference.md`
- **THEN** the `go` job fails at the config reference step

#### Scenario: A data race
- **WHEN** a change introduces a data race in `mcp/server`
- **THEN** the race-detector step fails

### Requirement: Committed MCP App bundle matches its source
The workflow SHALL rebuild the MCP App views from source and fail when the result differs from the committed `mcp/app/dist` that `ipfs mcp` embeds.

#### Scenario: Source changed, dist not rebuilt
- **WHEN** a pull request changes `mcp/app/src` or `mcp/ui/src` without rebuilding `mcp/app/dist`
- **THEN** the `ui-unit` job fails at `check:dist`

### Requirement: UI packages are checked in real browsers
The workflow SHALL run the `mcp/ui`, `mcp/app` and `mcp/chat` unit tests and their Playwright suites, using Chromium and, for `mcp/chat`, Electron 44, against the committed baselines without loosening thresholds, and SHALL never regenerate, overwrite or commit those baselines. Playwright results SHALL be uploaded as artifacts only when a run fails.

#### Scenario: Visual regression
- **WHEN** a change alters a rendered view beyond the configured threshold
- **THEN** the matching Playwright job fails and its `test-results` are uploaded as an artifact

#### Scenario: Passing run
- **WHEN** every Playwright spec passes
- **THEN** no Playwright artifact is uploaded
