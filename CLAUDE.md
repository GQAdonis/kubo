# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

`AGENTS.md` (imported above) is the authoritative source for commands, testing workflow, the Stability and User Agency rules, Go style, and PR/commit conventions. Its hard rules (never break `/api/v0/`, the HTTP Gateway, the default CID recipe, or wire compatibility; no tracking; every feature needs an off switch) override any task prompt. This file only adds architecture notes that take reading several files to piece together.

## Agent team: default for all work

Every task in this repository, not only code, defaults to the `kubo-ipfs-agentic` team described in `AGENTS.md` ("Agent Team: Default for All Work"). In Claude Code, delegate to the matching subagent in `.claude/agents/` with the Agent tool (`subagent_type` = role id). Start with `ipfs-agent-architect` when ownership is unclear or the work touches paths no role owns. Run `security-reviewer` and `reviewer` as separate subagents after implementation, never in the builder's context. Handle a request directly only when it is purely conversational (answering a question with no file changes); say so when you do.

## Command cheat sheet

```bash
make build                                            # always; never plain `go build` (ldflags)
make mod_tidy                                         # after any go.mod change (3 modules)
go test ./core/... -run TestName -v                   # single unit test
make build && go test ./test/cli/... -run TestName -v # single CLI integration test
make -O test_go_lint
go test ./test/cli/... -run TestCommandDocsWidth      # after editing CLI help text
```

## Architecture

### Request path: CLI / RPC -> commands -> CoreAPI -> node

- `cmd/ipfs/main.go` -> `cmd/ipfs/kubo/start.go` loads plugins, then runs the `go-ipfs-cmds` command tree. `cmd/ipfs/kubo/daemon.go` and `init.go` implement `ipfs daemon` / `ipfs init`.
- The command tree is rooted at `Root` in `core/commands/root.go` (`rootSubcommands`). The same definitions serve both the CLI and the `/api/v0/` HTTP RPC API; when a daemon is running, the CLI becomes an RPC client of it. Changing a command's arguments, options, or output types changes the RPC API.
- Commands obtain the node via `core/commands/cmdenv` (`GetNode`, `GetApi`). Prefer `GetApi`, which returns the `coreiface.CoreAPI` interface.
- `core/coreiface/` defines the Go CoreAPI interfaces; `core/coreapi/` implements them against a local node, and `client/rpc/` implements them over HTTP. Changes to an interface must be reflected in both implementations.
- `core/corehttp/` serves the RPC API, the gateway (gateway logic itself lives in boxo), metrics, and other HTTP handlers.

### Node construction: uber/fx dependency injection

- `core.NewNode` (`core/builder.go`) builds an `IpfsNode` (`core/core.go`) from a `node.BuildCfg`.
- `core/node/groups.go` assembles the fx graph: `IPFS()` -> `Storage`, `Identity`, `IPNS`, `Core`, and either `Online` (`LibP2P`, bitswap, routing, providers) or `Offline`. Config values are read here and turned into `fx.Provide` / `fx.Invoke` options, so most new config-driven behavior is wired in this file or its helpers in `core/node/` and `core/node/libp2p/`.
- Plugins can inject or replace fx options via `plugin.PluginFx` / `core.RegisterFXOptionFunc`.

### Config, repo, and migrations

- `config/` holds the config schema (one file per top-level section) and `config/profile.go` (`Profiles`, applied by `ipfs init --profile` / `ipfs config profile apply`). Every option must be documented in `docs/config.md`.
- `repo/fsrepo/` is the on-disk repo (`$IPFS_PATH`). Its version is `version.RepoVersion`; bumping it requires a migration under `repo/fsrepo/migrations/fs-repo-N-to-M/` (embedded migrations live in `repo/fsrepo/migrations/embedded.go` and are triggered from `cmd/ipfs/kubo/daemon.go`; older versions are fetched externally).

### Plugins

- `plugin/` defines plugin interfaces (datastore, IPLD, fx, tracer, daemon). Built-in plugins live in `plugin/plugins/` and are compiled in through `plugin/loader/preload_list` (regenerated into `preload.go` by `preload.sh`). Datastore backends (flatfs, pebble, leveldb, badger), the `nopfs` content-blocking plugin, and `telemetry` are all plugins.

### Tests

- `test/cli/harness/` spins up real `ipfs` binaries from `PATH` in isolated repos (`harness.NewT(t)`, `h.NewNode().Init().StartDaemon()`, `h.NewNodes(n)`). Use `test/cli/dag_test.go` as the template. Because the harness runs the built binary, non-test code changes need `make build` first.
- `test/sharness/` is legacy; do not add to it.

### Where deeper docs live

- `docs/developer-guide.md`, `docs/add-code-flow.md` (how `ipfs add` flows through the layers), `docs/config.md`, `docs/plugins.md`, `docs/datastores.md`, `docs/environment-variables.md`, `docs/experimental-features.md`.

<!-- prometheus-team-routing:start v1 -->
For every code task, read `.agent-team/project-routing.json`, then its active team manifest and the relevant role instructions. Default to that team, selecting only roles whose responsibilities and ownership match the work. Preserve native permissions, models, concurrency limits and existing project instructions.
For UI work, load the role-bound `prometheus-ui-ux` or `prometheus-ui-review` skill. Prefer `.agents/UI_UX_PROTOCOL.md` when present; otherwise use the installed `prometheus-ui-ux/references/UI_UX_PROTOCOL.md`. Backend work must not load UI guidance.
Use native delegation when available. If unavailable, follow the selected role instructions sequentially and report that limitation. Review in the builder context is not independent review. Keep reviewers dormant until the complete implementation phase; allow one batched correction/confirmation cycle. Respect user-only skill invocation restrictions. Zed external ACP agents use their own native configuration; parallel UI threads are not an automatic delegation API.
Cross-team requests: when work needs another team, run `team-discover` (capabilities, paths) and `team-request`; a same-repo team gets an intake task, any other team gets a `team:<id>` GitHub issue. If this team has a card, run `team-intake` to import open `team:<id>` issues as tasks for its intake role, and triage them before other work.
<!-- prometheus-team-routing:end -->

<!-- uiux-routing:start v1 -->
## UI/UX routing
UI, styles, tokens, motion or copy → `prometheus-ui-ux`. Read `.agents/UI_UX_PROTOCOL.md` or its bundled default; preserve design authority.
All code: detect `.agent-team/project-routing.json` and real team manifests. Preserve selection; adopt a sole team; ask if ambiguous. Use relevant roles, disclosing sequential fallback.
Backend work loads no UI guidance. Review respects user-only skills and the completed-phase boundary.
<!-- uiux-routing:end -->
