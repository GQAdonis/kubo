# AI Agent Instructions for Kubo

This file provides instructions for AI coding agents working on the [Kubo](https://github.com/ipfs/kubo) codebase (the Go implementation of IPFS). Follow the [Developer Guide](docs/developer-guide.md) for full details.

## Agent Team: Default for All Work

All work in this repository goes through the `kubo-ipfs-agentic` agent team (manifest: `.agent-team/kubo-ipfs-agentic/team.json`, routing: `.agent-team/project-routing.json`). That covers code, docs, specs, research, reviews, packaging and investigation, not only code. Before acting, pick the role whose responsibility and ownership match the work and follow its instructions.

| Role | Owns | Use for |
|------|------|---------|
| `ipfs-agent-architect` | `openspec/**`, `mcp/contract/**`, `mcp/README.md`, `docs/mcp/**` | Entry point for any request not clearly owned by one role; IPFS domain questions; OpenSpec changes; the shared MCP tool contract; ADRs |
| `mcp-server-engineer` | `mcp/server/**` | Go MCP server built on `modelcontextprotocol/go-sdk` |
| `skills-plugin-specialist` | `mcp/skills/**`, `mcp/plugin/**`, `.claude-plugin/**`, `.agents/plugins/**` | Agent Skills and plugin/marketplace packaging for Claude Code, Codex, OpenCode and Kimi Code |
| `ui-ux-engineer` | `mcp/ui/**`, `mcp/app/**` | React 19 A2UI component library and MCP App views |
| `security-reviewer` | `.agent-team/kubo-ipfs-agentic/findings/security/**` | Independent security review (read-only elsewhere) |
| `reviewer` | `.agent-team/kubo-ipfs-agentic/findings/review/**` | Independent quality, compatibility and UI review (read-only elsewhere) |

- Work outside every role's ownership (for example Kubo's own `core/`, `cmd/`, `config/`, `client/rpc/`) is routed through `ipfs-agent-architect`, which records the decision in an OpenSpec change before any role edits those paths. Team ownership never overrides the Stability and User Agency rules below.
- Non-trivial changes start as an OpenSpec change (`openspec/changes/<id>/`) owned by the architect, then fan out to the owning roles.
- Native role definitions: Claude Code `.claude/agents/`, Codex `.codex/agents/` (underscore names, e.g. `mcp_server_engineer`), Kimi Code `.kimi-code/agents/`, OpenCode `.opencode/agents/`. MiniMax definitions are staged in `.agent-team/kubo-ipfs-agentic/exports/minimax/`. Zed reads this file.
- Delegate to the native role when the harness supports it. Otherwise adopt the role's instructions sequentially and state that limitation. Reviewers run only after the implementation phase completes and in a separate context; review inside the builder's context is not independent.

## Context Management

Standing context is layered; only invariants stay resident.

- Resident: this file (Kubo rules, team routing, and the managed `prometheus-base` region at the end). `CLAUDE.md` imports it.
- Position: `.kbd-orchestrator/current-waypoint.json` is the current phase; read it at session start. Active work is tracked as OpenSpec changes in `openspec/changes/`.
- Path-scoped rules: `.claude/rules/go.md` and `.claude/rules/typescript.md` load only when a matching file is read.
- Memory (append-only): `.prometheus/session-log.md`, `decisions.md`, `gotchas.md`, `postmortems/`, `knowledge/`. Read `gotchas.md` before touching a subsystem.
- Hooks (`.claude/settings.json`): `tier-guard` holds `go test -race` and `playwright test` for a completed waypoint or `PROMETHEUS_TIER3=1`; `single-writer` flags two sessions writing one tree; `sycophancy-gate` checks completion claims; `reanchor` restores position on session start and before compaction.

Where the managed `prometheus-base` region overlaps a Kubo rule in this file, the Kubo rule wins:

- Dependency pins are the three `go.mod` files, changed only with `go get` plus `make mod_tidy` and the transitive-bump order above. `versions.toml` records decisions and the pins of the new `mcp/` TypeScript packages; it never overrides `go.mod`.
- The final integration gate for Kubo changes is the "Before Submitting" sequence plus the relevant `test/cli` tests against a `make build` binary.
- The Stability and User Agency rules outrank every managed region.

## Quick Reference

| Task              | Command                                                  |
|-------------------|----------------------------------------------------------|
| Tidy deps         | `make mod_tidy` (all modules; required if deps changed)  |
| Build             | `make build`                                             |
| Unit tests        | `go test ./... -run TestName -v`                         |
| Integration tests | `make build && go test ./test/cli/... -run TestName -v`  |
| Lint              | `make -O test_go_lint`                                   |
| Format            | `go fmt ./...`                                           |

## Project Overview

Kubo is the reference implementation of IPFS in Go. Most IPFS protocol logic lives in [boxo](https://github.com/ipfs/boxo) (the IPFS SDK); kubo wires it together and exposes it via CLI and HTTP RPC API. Before adding protocol logic here, check whether it belongs in boxo (see [Where a change belongs](#where-a-change-belongs-boxo-or-kubo)).

Key directories:

| Directory          | Purpose                                                  |
|--------------------|----------------------------------------------------------|
| `cmd/ipfs/`        | CLI entry point and binary                               |
| `core/`            | core IPFS node implementation                            |
| `core/commands/`   | CLI command definitions                                  |
| `core/coreapi/`    | Go API implementation                                    |
| `client/rpc/`      | HTTP RPC client                                          |
| `plugin/`          | plugin system                                            |
| `repo/`            | repository management                                    |
| `test/cli/`        | Go-based CLI integration tests (preferred for new tests) |
| `test/sharness/`   | legacy shell-based integration tests                     |
| `docs/`            | documentation                                            |

Other key external dependencies: [go-libp2p](https://github.com/libp2p/go-libp2p) (networking), [go-libp2p-kad-dht](https://github.com/libp2p/go-libp2p-kad-dht) (DHT).

## Where a change belongs: boxo or kubo

[boxo](https://github.com/ipfs/boxo) is the Go SDK for IPFS: a set of libraries for building IPFS applications and implementations. Kubo is one consumer of boxo, not the only one, so reusable building blocks live in boxo where other Go software can use them without pulling in kubo.

- **Goes in boxo:** protocol logic and reusable primitives another Go program could use on its own, for example Bitswap, UnixFS, the HTTP gateway, IPLD and path helpers, routing and provider systems, MFS, and the blockstore and blockservice layers. If the code does not depend on kubo's config, CLI, or daemon and would help someone building a different tool, it belongs in boxo.
- **Goes in kubo:** the daemon and product, for example the config schema, CLI commands (`core/commands/`), the `/api/v0/` RPC surface, node construction and lifecycle, the on-disk repo, plugins, and migrations. Kubo-specific product decisions stay here.
- **The usual shape of a feature:** build the reusable capability in boxo, then wire it into kubo (config option, CLI or RPC surface, `docs/config.md` entry). Do not implement generic protocol logic or reusable primitives under `core/`, even as a stopgap; if the capability is missing from boxo, the work starts with a boxo PR (see [Coordinating changes with boxo](#coordinating-changes-with-boxo)).

Not everything IPFS-related belongs in boxo; its README lists the inclusion criteria. When unsure, open an issue before building, but do not trap generic, reusable logic inside kubo. Boxo has its own `AGENTS.md` with protocol-freeze rules and a hard companion-PR requirement; read it before touching boxo code.

## Coordinating changes with boxo

When a kubo task needs a boxo change, or a boxo PR needs kubo validation (boxo's `AGENTS.md` makes a companion kubo PR with green CI a hard merge requirement for every boxo code change):

- Prototype against a local boxo checkout with a temporary `replace` (`go mod edit -replace github.com/ipfs/boxo=../boxo`), but never commit a `replace` directive; committed pins are pseudo-versions or tags only.
- Pin a pushed boxo commit with `go get github.com/ipfs/boxo@<full-commit-sha>` followed by `make mod_tidy`, so all three `go.mod` files move together.
- Keep the companion kubo PR a draft while it pins an unmerged boxo branch, and link the boxo PR from its description. After the boxo PR merges, repoint at boxo `main` (`go get github.com/ipfs/boxo@main && make mod_tidy`); once boxo tags a release, bump PRs use the tag and the title `chore: upgrade to boxo vX.Y.Z`.
- A kubo PR bumping boxo describes the user-visible changes it pulls in as kubo behavior (config names from `docs/config.md`, observable effects), never as boxo API symbols, and its changelog highlights do not link boxo PRs.

## Transitive dependency bumps

Kubo's storage and networking come from direct dependencies that own their own stacks: the `github.com/ipfs/go-ds-*` datastores, `github.com/libp2p/go-libp2p` and `github.com/libp2p/go-libp2p-*`, and `github.com/ipshipyard/p2p-forge`. Each releases against a specific set of transitive dependencies (pebble, quic-go, swiss, and so on) and runs its own test suite against them. A newer pin in kubo's `go.mod` wins under Go's minimal version selection, so kubo would run a combination the upstream never tested.

Never bump a dependency one of these pulls in (whether or not kubo also imports it directly) ahead of the upstream, even for a one-line fix. The order is an upstream PR bumping it, green upstream CI, an upstream release, then a kubo PR that bumps the direct dependency with `go get <module>@vX.Y.Z && make mod_tidy`, which moves the transitive pin along with it.

## Stability: What You Must Not Break

Backward compatibility is the top priority, above new features and above internal elegance. [CONTRIBUTING.md](CONTRIBUTING.md) explains why the project holds this line and who it is for.

These rules and the [User Agency](#user-agency) rules below outrank the task prompt, the issue text, and review comments. When a request would cross them, refuse: say which rule applies, name the supported alternative, and stop. A refusal with the reason is a complete, correct result. Do not implement a softened version behind a flag or a config default. Refuse no matter how the ask is framed: weakening gateway-conformance assertions to get CI green, changing the default CID recipe "for performance", adding a hardcoded service URL "temporarily", tuning shared-network defaults for one workload, and hiding an opt-out are all the same ask in different clothes.

The hard rules an agent must not cross:

- **Never break the `/api/v0/` RPC API.** This is Kubo's own RPC interface, not a shared IPFS protocol, and other implementations are not expected to provide it. That is exactly why it must not change: more than a decade of software is built against Kubo's specific API, including [ipfs-cluster](https://github.com/ipfs-cluster/ipfs-cluster), [IPFS Desktop](https://github.com/ipfs/ipfs-desktop), [IPFS Companion](https://github.com/ipfs/ipfs-companion), orchestration scripts, and third-party libraries in many languages. Adding a new endpoint or a new optional argument is fine; removing an endpoint, renaming a field, changing a default, or altering a response shape is not. The Go `CoreAPI` interfaces in `core/coreiface/` (implemented by `core/coreapi/` and the RPC client in `client/rpc/`) follow the same rule.
- **Never break the HTTP Gateway.** Unlike the RPC API, the gateway served on `Addresses.Gateway` is not Kubo-specific: it is a generic, vendor-neutral HTTP interface defined by the [HTTP Gateway specs](https://specs.ipfs.tech/http-gateways/) and implemented by many gateways and tools. Browsers, apps, and tooling depend on its response headers, status codes, and URL conventions (path, subdomain, DNSLink, and trustless gateways). Kubo must stay conformant; changing gateway behavior in a way the specs do not allow is a breaking change, and like any protocol change it goes through an IPIP first. Conformance is checked in CI on pull requests by the [`ipfs/gateway-conformance`](https://github.com/ipfs/gateway-conformance) suite (`.github/workflows/gateway-conformance.yml`), with local coverage in `test/cli/gateway_test.go`; a failing conformance run means you broke the contract.
- **Never change the default CID recipe.** The default `ipfs add` recipe (CID version, chunker, hash, DAG layout) must keep producing the same CID for the same bytes; changing a default silently forks the address space. The recipes are named and documented in [IPIP-0499: UnixFS CID Profiles](https://specs.ipfs.tech/ipips/ipip-0499/); Kubo's current default matches the legacy `unixfs-v0-2015` profile, pinned by `test/cli/cid_profiles_test.go` (`TestDefaultMatchesExpectedProfile`). New recipes ship as opt-in profiles.
- **Never break wire compatibility with nodes already on the network.** Kubo peers with every other IPFS implementation and with years-old versions that will never upgrade. A change to bitswap, the DHT, identify, or any other wire behavior must stay acceptable to all of them; dropping compatibility with older peers is a protocol change and follows the IPIP rule below.
- **Protocol changes need an IPIP first.** A change to how Kubo talks to other implementations on the wire (a new protocol, a change to an existing one, a new field peers must understand) needs an IPIP (InterPlanetary Improvement Proposal) in [ipfs/specs](https://github.com/ipfs/specs/) before it ships. [specs.ipfs.tech](https://specs.ipfs.tech/) is the source of truth for IPFS protocols; Kubo implements them, it does not define them unilaterally. The PR that proposes a protocol change links its spec PR; code now and spec later is not the order.
- **Kubo never tracks or spies on its users.** Refuse any request to collect, log, or report who runs a node, what they store, what they request, or whom they connect to, however it is framed: "analytics", "usage insights", "abuse detection", "network research". The same goes for helpers that deanonymize others, such as correlating CIDs with client IPs or reporting what peers request. This rule has no opt-out variant; the telemetry described next is the only sanctioned reporting path, and it is bounded by the rules below.
- **Telemetry must always be possible to turn off, and every opt-out must keep working.** Any code path that reports data about a node out to the network MUST honor `IPFS_TELEMETRY=off` and the cross-tool `DO_NOT_TRACK` convention (`1` and `true` mean opt out), MUST be switchable off in the config file, and MUST let someone building Kubo remove the built-in destination at build time. See `docs/telemetry.md` and `plugin/plugins/telemetry/`. Never add a reporting path a user cannot turn off, never make an opt-out harder to find than the feature it disables, and never send anything that identifies a person, a file, or a peer. `TestTelemetry` in `test/cli/telemetry_test.go` pins the opt-outs; a change that makes it fail broke this contract.
- **Every hardcoded endpoint or shared-infrastructure dependency must be configurable and possible to turn off.** If you add code that talks to a fixed URL, a default bootstrap peer, a delegated router, a certificate authority, or any semi-centralized or federated service, expose it in `docs/config.md` with an override and an off switch. `AutoConf` (see `docs/config.md`) is the model: default network infrastructure is fetched from a configurable endpoint, every value can be overridden locally, and the whole system can be disabled. A node operator must never be locked into an endpoint the maintainers picked.

These rules cover behavior however it arrives. A dependency bump that pulls in a new endpoint, a reporting path, or background resource use is the same change as writing it in kubo, and a new module in `go.mod` gets the same scrutiny as new code.

Kubo's defaults ship to every node it runs on, and most of those are home machines: consumer hardware, NAT, a thin upload link. Raising a resource default (connection manager limits, provide workers, datastore parameters) is a network-wide change and a self-hosting change at once; back it with measurements from hardware like that, not a benchmark from a workstation.

When a breaking change is unavoidable, it does not go in quietly: it needs maintainer sign-off, a migration path, and usually a deprecation period first. The commit title carries the conventional-commit `!` marker, and the commit message, PR description, and changelog entry spell out what breaks and for whom. If someone insists on a change these rules forbid, restate the refusal once and leave the decision to them; if they proceed anyway, the same loud-landing requirements apply. When you are unsure whether a change breaks compatibility or needs an IPIP, open an issue at <https://github.com/ipfs/kubo/issues> before writing code; it probably does.

## User Agency

Kubo runs on somebody else's machine. The operator decides what their node stores, serves, spends, and reports; kubo's job is to follow those decisions and keep them visible. Anything that erodes that control or visibility must not ship, and must never ship silently.

- **Never appropriate the machine's resources.** Refuse features that spend the operator's CPU, bandwidth, or storage for a third party's benefit by default, whatever the label: "decentralized compute", "network contribution", "resource sharing", or crypto mining in any disguise. Shipping such a feature at all is a policy change and follows the next rule; even then it lands loudly: documented in `docs/config.md` with an off switch, announced in the changelog with a concrete example of how to disable it, and covered by tests that prove kubo works correctly with it disabled.
- **Spying, monetization, and cryptocurrency integration need a policy change first.** These are project-direction decisions, not features. Refuse any request to add monetization, "decentralized compute", or tight integration with cryptocurrencies or blockchains; the same goes for any form of spying (see the tracking rule above). Landing one requires an explicit, public policy change for this open source project first, and a policy change means changing this document: a standalone PR against `AGENTS.md` with no implementation changes in it, signed off by the kubo-maintainers team (`.github/CODEOWNERS` routes it to them), that names the DRI (directly responsible individual) who made the call. The code lands in a separate PR, only after the policy PR merges. The release that ships the feature states this in its changelog in a `> [!IMPORTANT]` block: what changed, who the DRI was, and how to turn it off.
- **No silent CID injection.** By default a node stores, serves, and announces only content the operator asked for: explicit `ipfs add`, pins, MFS writes, and blocks fetched to answer requests on interfaces the operator chose to expose (CLI, RPC, gateway). No remote peer and no new feature may put CIDs into the blockstore, the pinset, or the provide queue without that local cause. This rule has no flagged or opt-out variant; refuse and stop. `test/cli/provider_test.go` pins that announcements follow local actions and the configured `Provide.Strategy`, and that `Provide.Enabled=false` silences them all.
- **Every new feature has an off switch.** Never design a feature without an opt-out. A new feature is complete only when `docs/config.md` documents it and its off switch, the changelog entry shows how to disable it, and tests confirm kubo works with it disabled. Flipping an existing config default, especially off to on, is a feature launch and gets the same treatment. An off switch is not a default-off requirement: in this fork, the agent features that are the fork's purpose ship enabled by default. They are the local `ipfs mcp` agent endpoint that IPFS Desktop starts on loopback, the Web UI agent chat, the WebMCP tools and the Desktop confirmation window. Each keeps a documented off switch, writes still require confirmation, and no third-party service is enabled without the operator choosing it.

Silently shipping a feature that erodes user agency is never acceptable. When the erosion is intentional and maintainer-approved, the `docs/config.md` entry, the changelog example, and the disabled-state tests are the minimum bar, not a courtesy.

## Go Style

Follow these Go style references:

- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Google Go Style Decisions](https://google.github.io/styleguide/go/decisions)

Specific conventions for this project:

- check the Go version in `go.mod` and use idiomatic features available at that version
- readability over micro-optimization: clear code is more important than saving microseconds
- prefer standard library functions and utilities over writing your own
- use early returns and indent the error flow, not the happy path
- use `slices.Contains`, `slices.DeleteFunc`, and the `maps` package instead of manual loops
- preallocate slices and maps when the size is known: `make([]T, 0, n)`
- use `map[K]struct{}` for sets, not `map[K]bool`
- receiver names: single-letter abbreviations matching the type (e.g., `s *Server`, `c *Client`)
- run `go fmt` after modifying Go source files, never indent manually

### Error Handling

- wrap errors with `fmt.Errorf("context: %w", err)`, never discard errors silently
- use `errors.Is` / `errors.As` for error checking, not string comparison
- never use `panic` in library code; only in `main` or test helpers
- return `nil` explicitly for the error value on success paths

### Canonical Examples

When adding or modifying code, follow the patterns established in these files:

- CLI command structure: `core/commands/dag/dag.go`
- CLI integration test: `test/cli/dag_test.go`
- Test harness usage: `test/cli/harness/` package

## Building

Always run commands from the repository root.

```bash
make mod_tidy        # update go.mod/go.sum (use this instead of go mod tidy)
make build           # build the ipfs binary to cmd/ipfs/ipfs
make install         # install to $GOPATH/bin
make -O test_go_lint # run linter (use this instead of golangci-lint directly)
```

**Always build with `make build`, never `go build`.** The Makefile injects required `-ldflags` for `CurrentCommit`, `taggedRelease`, and `buildOrigin`.

If you change dependencies in any `go.mod`, you must run `make mod_tidy`, and you must run it before committing, pushing, or opening a PR. The repo has three `go.mod` files (root, `docs/examples/kubo-as-a-library`, and `test/dependencies`) that have to stay on the same dependency versions. `make mod_tidy` runs `go mod tidy` in every one of them; a bare `go mod tidy` only touches the module you run it in, which lets the pins drift out of sync between modules (for example the root pointing at one boxo commit while `test/dependencies` points at another). Run it before building or testing too, since it also updates `go.sum`.

If you modify any `.go` files outside of `test/`, you must run `make build` before running integration tests.

## Testing

The full test suite is composed of several targets:

| Make target          | What it runs                                                          |
|----------------------|-----------------------------------------------------------------------|
| `make test`          | all tests (`test_go_fmt` + `test_unit` + `test_cli` + `test_sharness`) |
| `make test_short`    | fast subset (`test_go_fmt` + `test_unit`)                             |
| `make test_unit`     | unit tests with coverage (excludes `test/cli`)                        |
| `make test_cli`      | CLI integration tests (requires `make build` first)                   |
| `make test_fuse`     | FUSE filesystem tests (requires `/dev/fuse` and `fusermount` in PATH) |
| `make test_sharness` | legacy shell-based integration tests                                  |
| `make test_go_fmt`   | checks Go source formatting                                          |
| `make -O test_go_lint` | runs `golangci-lint`                                                |

During development, prefer running a specific test rather than the full suite:

```bash
# run a single unit test
go test ./core/... -run TestSpecificUnit -v

# run a single CLI integration test (requires make build first)
go test ./test/cli/... -run TestSpecificCLI -v
```

### Environment Setup for Integration Tests

Before running `test_cli` or `test_sharness`, set these environment variables from the repo root:

```bash
export PATH="$PWD/cmd/ipfs:$PATH"
export IPFS_PATH="$(mktemp -d)"
```

- `PATH`: integration tests use the `ipfs` binary from `PATH`, not Go source directly
- `IPFS_PATH`: isolates test data from `~/.ipfs` or other running nodes

If you see "version (N) is lower than repos (M)", the `ipfs` binary in `PATH` is outdated. Rebuild with `make build` and verify `PATH`.

### Running FUSE Tests

FUSE tests require `/dev/fuse` and `fusermount` in `PATH`. On systems with only fuse3, create a symlink in a temp directory (never use `sudo` to install system-wide):

```bash
FUSE_BIN="$(mktemp -d)" && ln -s /usr/bin/fusermount3 "$FUSE_BIN/fusermount" && PATH="$FUSE_BIN:$PATH" make test_fuse
```

Set `TEST_FUSE=1` to make mount failures fatal (CI does this). Without it, tests auto-detect and skip when FUSE is unavailable.

### Cleaning Up Leftover FUSE Mounts

`fusetest.TestMount` mounts at `t.TempDir()` and unmounts in `t.Cleanup`, but that cleanup does not run when the test process is killed, and it blocks when the test is already stuck inside the mount. The mountpoint then stays under `/tmp` with no server behind it. From that point on, every `stat()` of it blocks in uninterruptible `D` state, which no signal can interrupt, not even `SIGKILL`. The damage outlives the test run: it hangs `find /tmp`, the daily `tmpwatch` cron, and backups.

After a FUSE test run that crashed, timed out, or was interrupted, check for leftovers:

```bash
grep fuse /proc/self/mountinfo       # kubo-test mounts under /tmp are leftovers
ps -eo pid,stat,cmd | awk '$2 ~ /D/' # processes stuck on a dead mount
```

To clean up, abort the connection first, then unmount. The abort returns `EIO` to the blocked processes so they can exit; unmounting alone does not release them. The connection id is the minor number in field 3 of `mountinfo`, so `0:107` means connection `107`:

```bash
echo 1 > /sys/fs/fuse/connections/107/abort
fusermount3 -u -z /tmp/TestSomething123/001
```

`/sys/fs/fuse/connections/` also holds live desktop mounts such as `gvfs` and the flatpak document portal. Match each id against `mountinfo` before you abort it.

### Running Sharness Tests

Sharness tests are legacy shell-based tests. Run individual tests with a timeout:

```bash
cd test/sharness && timeout 60s ./t0080-repo.sh
```

To investigate a failing test, pass `-v` for verbose output. In this mode, daemons spawned by the test are not shut down automatically and must be killed manually afterwards.

### Cleaning Up Stale Daemons

Before running `test/cli` or `test/sharness`, stop any stale `ipfs daemon` processes owned by the current user. Leftover daemons hold locks and bind ports, causing test failures:

```bash
pkill -f "ipfs daemon"
```

### Writing Tests

- all new integration tests go in `test/cli/`, not `test/sharness/`
- if a `test/sharness` test needs significant changes, remove it and add a replacement in `test/cli/`
- use [testify](https://github.com/stretchr/testify) for assertions (already a dependency)
- use `t.Context()` instead of `context.Background()` in tests
- for Go 1.25+, use `testing/synctest` when testing concurrent code (goroutines, channels, timers)
- reuse existing `.car` fixtures in `test/cli/fixtures/` when possible; only add new fixtures when the test requires data not covered by existing ones
- when writing tests that cover CIDv0 vs CIDv1, always set the CID version explicitly (never rely on defaults); if chunk size matters for the test, also set the chunker explicitly
- always re-run modified tests locally before submitting to confirm they pass
- avoid emojis in test names and test log output
- never leave a symlink loop behind in `/tmp` (`a -> b` with `b -> a`, or a self-link), in a test fixture or in an ad-hoc `mktemp -d` repro directory. `fuser` resolves such a path with an unbounded `readlink()` walk, so the daily `tmpwatch --fuser` cron that runs `fuser` on every file in `/tmp` spins on one core forever and `/tmp` stops being cleaned. Delete loop fixtures in the test that creates them rather than relying on `t.TempDir()` removal, which does not run when the test process is killed

## Before Submitting

Run these steps in order before committing, pushing, or opening a PR:

1. `make mod_tidy` (required whenever any `go.mod` changed, so all three modules stay in sync)
2. `go fmt ./...`
3. `make build` (if non-test `.go` files changed)
4. `make -O test_go_lint`
5. `go test ./...` (or the relevant subset)

## Documentation and Commit Messages

- after editing CLI help text in `core/commands/`, verify width: `go test ./test/cli/... -run TestCommandDocsWidth`
- **CLI `--help` text and RPC command descriptions are user-facing documentation.** The reference pages at [docs.ipfs.tech/reference/kubo/cli](https://docs.ipfs.tech/reference/kubo/cli/) and [docs.ipfs.tech/reference/kubo/rpc](https://docs.ipfs.tech/reference/kubo/rpc/) are generated from the command definitions in `core/commands/` by a CI job in [ipfs/ipfs-docs](https://github.com/ipfs/ipfs-docs) after each release. Whatever you put in a command's `Helptext` is what users read on the website, so keep it accurate and complete. Where a command implements a spec or a non-obvious concept, link to [specs.ipfs.tech](https://specs.ipfs.tech/) or the relevant docs so a reader can learn the "why", not just the syntax.
- **`docs/config.md` is where users learn how Kubo works, not just a list of keys.** It is a common entry point for understanding a feature. When you add or change a config option, document what it does and why someone would touch it, and link out to the spec or educational material behind the concept. A new or changed option without a matching `docs/config.md` entry is an incomplete change.
- changelogs in `docs/changelogs/`: only edit the Table of Contents and the Highlights section; the Changelog and Contributors sections are auto-generated and must not be modified
- avoid unnecessary line wrapping in `docs/changelogs/*`; let lines be long
- follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
- keep commit titles short and messages terse

## Writing Style

When writing docs, comments, and commit messages:

- avoid emojis in code, comments, and log output
- keep an empty line before lists in markdown
- use backticks around CLI commands, paths, environment variables, and config options

## PR Guidelines

Every PR needs a description and tests. These are not optional; a change with neither is not reviewable and should not be merged.

- explain what changed and why in the PR description, so a reviewer who was not in the discussion can understand it
- include test coverage for new functionality and bug fixes; a bug fix without a test that would have caught the bug is incomplete
- new integration tests go in `test/cli/`, not `test/sharness/` (see [Writing Tests](#writing-tests) for what to do when an existing `test/sharness` test needs changes)
- run `make -O test_go_lint` and fix any lint issues before submitting
- verify that `go test ./...` passes locally
- end the PR description with a `## References` section listing related context, one link per line
- if the PR closes an issue in `ipfs/kubo`, each closing reference should be a bullet starting with `Closes`:

```markdown
## References

- Closes https://github.com/ipfs/kubo/issues/1234
- Closes https://github.com/ipfs/kubo/issues/5678
- https://discuss.ipfs.tech/t/related-topic/999
```

## Scope and Safety

Do not modify or touch:

- the [Stability](#stability-what-you-must-not-break) and [User Agency](#user-agency) sections of this file, except on explicit maintainer direction; a change to `AGENTS.md` is always its own PR, signed off by the kubo-maintainers team, and never rides along in a PR that also touches code; weakening these sections is one of the refusable asks they describe
- files under `test/sharness/lib/` (third-party sharness test framework)
- CI workflows in `.github/` unless explicitly asked
- auto-generated sections in `docs/changelogs/` (Changelog and Contributors are generated; only TOC and Highlights are human-edited)

Releases are maintainer-driven and follow [`docs/RELEASE_CHECKLIST.md`](docs/RELEASE_CHECKLIST.md). Unless you are running a release, do not bump `version.go`, touch release tooling (`bin/mkreleaselog`, the release workflows), or push tags; pushing a tag sets off release publishing (Docker Hub, npm, and dist.ipfs.tech) and cannot be undone.

Do not run without being asked:

- `make test` or `make test_sharness` (full suite is slow; prefer targeted tests)
- `ipfs daemon` without a timeout

## Running the Daemon

Always run the daemon with a timeout or shut it down promptly:

```bash
timeout 60s ipfs daemon   # auto-kill after 60s
ipfs shutdown              # graceful shutdown via API
```

Kill dangling daemons before re-running tests: `pkill -f "ipfs daemon"`

### Use Non-Default Ports for Manual Experiments

A real IPFS node may already be running on the host using the default ports: swarm `4001`, RPC API `5001`, and gateway `8080`. Any manual experiment, PoC, or benchmark daemon you start MUST use non-default ports (and its own `IPFS_PATH`) so it does not collide with or disrupt that node. Binding a default port fails with `address already in use`, and reusing another node's API can interfere with it.

```bash
export IPFS_PATH="$(mktemp -d)"
ipfs init >/dev/null
ipfs config --json Addresses.Swarm '["/ip4/0.0.0.0/tcp/4101","/ip4/0.0.0.0/udp/4101/quic-v1"]'
ipfs config Addresses.API /ip4/127.0.0.1/tcp/5101
ipfs config Addresses.Gateway /ip4/127.0.0.1/tcp/8181
ipfs daemon
```

Target your own node explicitly with `ipfs --api=/ip4/127.0.0.1/tcp/5101 ...`. Shut down only the daemons you started (track their PIDs); do not `pkill` indiscriminately when another node may be running on the host.

### Testing AutoTLS Locally

AutoTLS only requests a `*.libp2p.direct` certificate once libp2p confirms the node is publicly reachable on a TCP port. For a local test the node must be able to open that port, so enable UPnP/NAT-PMP (the `server` init profile disables it via `Swarm.DisableNatPortMap: true`):

```bash
ipfs config --json Swarm.DisableNatPortMap false   # let UPnP/NAT-PMP map the swarm port
ipfs config AutoTLS.RegistrationDelay 5s           # shorten the default wait before registration
```

Then start the daemon and watch the relevant logs:

```bash
GOLOG_LOG_LEVEL="error,autotls=info,nat=info" ipfs daemon
```

Poll `ipfs id` until a `tls/ws` address under your own peer ID appears. A `libp2p.direct` address ending in `/p2p-circuit/p2p/<your-id>` is a relay path, not your own AutoTLS cert. Requires a router that actually honors UPnP/NAT-PMP; without it AutoNAT reports `Private` and no certificate is issued.

<!-- prometheus-team-routing:start v1 -->
For every code task, read `.agent-team/project-routing.json`, then its active team manifest and the relevant role instructions. Default to that team, selecting only roles whose responsibilities and ownership match the work. Preserve native permissions, models, concurrency limits and existing project instructions.
For UI work, load the role-bound `prometheus-ui-ux` or `prometheus-ui-review` skill. Prefer `.agents/UI_UX_PROTOCOL.md` when present; otherwise use the installed `prometheus-ui-ux/references/UI_UX_PROTOCOL.md`. Backend work must not load UI guidance.
Use native delegation when available. If unavailable, follow the selected role instructions sequentially and report that limitation. Review in the builder context is not independent review. Keep reviewers dormant until the complete implementation phase; allow one batched correction/confirmation cycle. Respect user-only skill invocation restrictions. Zed external ACP agents use their own native configuration; parallel UI threads are not an automatic delegation API.
Cross-team requests: when work needs another team, run `team-discover` (capabilities, paths) and `team-request`; a same-repo team gets an intake task, any other team gets a `team:<id>` GitHub issue. If this team has a card, run `team-intake` to import open `team:<id>` issues as tasks for its intake role, and triage them before other work.
<!-- prometheus-team-routing:end -->

<!-- prometheus-base:start v1 -->
# Agent Operating Rules

This region is the standing contract. It holds only invariants that must
survive compaction. Everything else lives in on-demand rules, skills, hooks,
and reference files. Where a hook enforces a rule stated here, the hook wins.

Managed by `prometheus-context-bootstrap`. Edits inside these markers are
overwritten on re-run. Write project prose outside them.

## Position and authority

- `.kbd-orchestrator/current-waypoint.json` is authoritative for position.
- `versions.toml` is authoritative for architecture decisions and dependency pins.
- READMEs go stale. Do not trust one over the two files above.
- Read the waypoint at session start. State the current phase before executing.

## Capability inversion

Agent kernels do not write. Mutating actions are gated in the trusted host
layer only, never in an agent kernel. Where the language allows it, this is
enforced at the dependency graph as a compile-time guarantee rather than a
runtime check. If a task appears to require a write from a kernel, stop and
surface the conflict instead of routing around it.

## Phase order

Task loop: Spec, Plan, Execute, Reflect.
Evolution loop: Compile, Evaluate, Optimize, Promote.

Running a phase out of order is a quality failure, not a shortcut. Name the
phase you are in. Do not execute before a plan exists.

## Verification boundaries

Finish a coherent implementation set before testing it. During implementation,
use static inspection and reasoning; use a narrow compiler check only when it is
required to unblock progress. Finish every planned production change in the phase,
then run one integration gate through the real production path and collaborators at
the final phase boundary. When the harness provides an agent team, keep reviewer,
auditor, verifier, and integration-checker roles dormant until that boundary. Unit,
mock-only, filtered-function, and per-edit tests are not completion evidence.
Per-stack commands live in `.claude/rules/`, loaded only when a matching file is read.

- `.claude/rules/go.md` — go tiers and hard rules
- `.claude/rules/typescript.md` — typescript tiers and hard rules

## Evidentiary standard

Address observed problems. An observed problem comes from an operator report, a
visible error or log, a failing test, or an explicit requirement. A concern that
is none of those gets one sentence and a question, never speculative code.

Defensive code — validation, guards, fallbacks, retries, timeouts — requires a
named failure scenario. Hardening at a real trust boundary present in the code
is a standing exception and is named in the completion summary, never added
silently.

## Evidence over assertion

Show the command and its output, the test result, or the artifact. "Looks done"
is not done. Report what was actually run and at which boundary. If a check could
not run, say which claims are therefore unverified. An unverified claim reported
as verified is worse than no check at all.

## Anti-sycophancy

Critics never see generation history. Review through the `artifact-critic`
subagent, which receives the artifact alone. The model that produced the work is
not the sole judge of whether it is good.

A reflection leads with the delta between plan and delivery, not with what
worked. The sycophancy gate may block a turn; fix the finding rather than
bypassing it.

## Learning and memory

Learning is append-only under `.prometheus/`: `session-log.md`, `decisions.md`,
`gotchas.md`, `postmortems/`, `knowledge/`. Never rewrite history; append, and
mark superseded entries rather than deleting them.

Write on a decision with a rationale, a defect and its root cause, a learned
constraint, a phase boundary, and a session summary. Read `gotchas.md` before
touching a subsystem.

Where a memory server is configured, it is the primary store and its write path
may time out. On failure, log to the markdown files above and continue. Never
block a task on the memory server.

## Architecture

- Single-writer build discipline within one build or target directory.
- Feature-based organization by capability, not by technical layer.
- Strict layering: UI, then hooks or view models, then stores, then services,
  then external. Reverse flow only through reactive state or events.
- Business state lives in explicit, inspectable systems, never in UI components
  or agent-only memory.
- Open standards first. Avoid lock-in unless explicitly required.
- Verify dependency versions against official sources before introducing them.
  Do not rely on training-era version knowledge.

## Scope

Minimum change that solves the problem. Do not refactor adjacent working code;
treat its current state as intentional. Mention unrelated issues, do not fix
them unasked. Before destructive or hard-to-reverse actions, confirm intent and
prefer a reversible path.

## Skills may be absent

Harnesses drop skill descriptions past a context budget, so a skill you expect
may not be listed. If one is missing, invoke it by name or say plainly that it
is unavailable and proceed from these rules. Never invent what an absent skill
would have done.

## Communication

Direct and execution-first. Structure claims as statement, mechanism, stakes.
Short declarative sentences. No marketing language.

Avoid: leverage as a verb, utilize, synergy, roadmap as a verb, journey,
harness as a verb, delve, revolutionary.

Every significant document names the uncomfortable thing — the scenario that
hurts the author's own position.

## Done

A task is done when its stated integration exit criteria pass at the applicable boundary, not when
the output looks plausible. Before declaring completion: remove anything added
that was not requested, confirm each guard traces to an observed problem or a
real boundary, and summarize what changed, how it was verified, and what remains
at risk.

## Execution scaffold

This section exists because the fleet is mixed. Frontier models supply most of
it by default; smaller and older models do not, and the failure is silent —
plausible output with a fabricated call in it. Omit this section only when
every model that reads this file is known to supply the behavior on its own.

### Before executing

Restate the task in one sentence, and name the phase. If the restatement does
not match what was asked, stop and ask rather than proceeding on the closer
reading. Name the files you intend to touch before touching them.

### Do not fabricate

Never invent an API, a file path, a package name, a command flag, or a
configuration key. If you have not read it in this session or it is not pinned
in `versions.toml`, verify it before using it. "I could not confirm this
exists" is a correct answer. A plausible identifier that does not exist costs
more than the question would have.

Do not guess at a tool's parameters. Read its schema. A tool call with invented
arguments fails in a way that looks like the tool is broken.

### Verification is explicit

Run the check. Paste the command and its actual output. Do not report a result
you did not observe, and do not describe what a test "should" produce.

If a check cannot run, say which specific claims are therefore unverified, and
why. Skipping a check silently and summarizing as if it passed is the failure
this rule exists to prevent.

### Code output

Never elide code with `...`, `// rest unchanged`, or a similar placeholder in a
file you are writing. Emit the complete content of every file you write.

When editing, change the minimum span. Do not reformat, reorder imports, or
rename adjacent symbols while making an unrelated change.

Match the file's existing conventions over your own defaults.

### Complete coherent sets

Batch related implementation work until a meaningful production path is complete.
Do not interrupt every edit with a build or test. Keep unrelated changes separate,
then validate the completed set through the smallest real integration boundary.

Do not start an unrelated subsystem while the current implementation set is partial.

### Stop conditions

Stop and ask when: the requirement is ambiguous in a way that changes the
design, two readings of the task lead to different files, the change would
break an existing behavior, or you are about to do something hard to reverse.

Stop when the goal is met. Do not continue into adjacent improvements.

### Format contracts

When a specific output format is requested — JSON, a table, a diff, a schema —
emit exactly that format with no preamble, no trailing commentary, and no
markdown fence unless the fence was asked for. A parser is often reading it.

### Self-check before reporting completion

State each of these explicitly, not as a claim that you did them:

1. What changed, file by file.
2. What was run to verify it, and the observed output.
3. What was added that was not requested — remove it, or list it and ask.
4. Which guards trace to an observed failure, and which do not.
5. What remains unverified, and why.

<!-- profile: mixed — see references/MODEL-PROFILES.md before changing -->
<!-- prometheus-base:end -->

<!-- uiux-routing:start v1 -->
## UI/UX routing
UI, styles, tokens, motion or copy → `prometheus-ui-ux`. Read `.agents/UI_UX_PROTOCOL.md` or its bundled default; preserve design authority.
All code: detect `.agent-team/project-routing.json` and real team manifests. Preserve selection; adopt a sole team; ask if ambiguous. Use relevant roles, disclosing sequential fallback.
Backend work loads no UI guidance. Review respects user-only skills and the completed-phase boundary.
<!-- uiux-routing:end -->
