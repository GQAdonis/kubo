```
ASSESSMENT: ipfs-mcp-server
Project: Kubo
Date: 2026-10-09
Codebase baseline: Fork of Kubo at 0.44.0-dev (HEAD 833e04a24; tracked changes only in AGENTS.md and .gitignore; team/tool state untracked) with no MCP server, tool contract, skills, plugin or MCP App code in the tree.
Cross-tool progress: none
```

Assessor: `ipfs-agent-architect` (KBD Assess stage). Revision 2, after adversarial review round 1 (`review/assess/findings.json`, verdict BLOCK). The only file written is this one.

Evidence conventions used below:

- `path:line` means the line was read in this session from the working tree.
- **[local, uncommitted]** marks facts that come from untracked or uncommitted files (`git status --short` shows `?? .agent-team/`, `?? .agents/`, `?? .claude/`, `?? .codex/`, `?? .kimi-code/`, `?? .opencode/`, `?? openspec/`, ` M AGENTS.md`). They describe how this team works locally. They are not Kubo baseline, and no option is scored on them.
- **[external]** marks facts from outside the repo. Each gives the URL or the exact command used. **[unverified]** marks claims that were not checked.
- **[probe]** marks measurements from throwaway builds in the session scratchpad (`/private/tmp/claude-501/.../scratchpad/sdkprobe/`). They are not part of the repo, and the exact commands are listed in the APPENDIX.
- An **owned path, not yet created** is a path a team role is assigned in `.agent-team/kubo-ipfs-agentic/team.json` [local, uncommitted] that does not exist on disk. `mcp/`, `mcp/README.md`, `mcp/contract/`, `mcp/server/`, `mcp/skills/`, `mcp/plugin/`, `mcp/app/`, `mcp/ui/`, `docs/mcp/`, `.claude-plugin/` and `.agents/plugins/` are all in this state.

---

## IMPLEMENTATION STATUS

Existence check, run from the repo root:

```
$ ls mcp docs/mcp mcp/skills mcp/plugin .claude-plugin .agents/plugins
ls: .agents/plugins: No such file or directory
ls: .claude-plugin: No such file or directory
ls: docs/mcp: No such file or directory
ls: mcp: No such file or directory
ls: mcp/plugin: No such file or directory
ls: mcp/skills: No such file or directory
$ grep -rniE "modelcontextprotocol|\bmcp\b" --include=*.go -l .     # no output
$ grep -n modelcontextprotocol go.mod go.sum                        # no output
```

| Area | Status | Evidence |
|---|---|---|
| MCP server (any placement) | MISSING | No `.go` file mentions MCP. `go.mod` and `go.sum` have no `modelcontextprotocol` entry. `mcp/server/` is an owned path, not yet created. |
| Tool contract | MISSING | `mcp/contract/` is an owned path, not yet created (`ls mcp` fails). |
| `mcp/README.md` | MISSING | Owned path, not yet created; its parent `mcp/` does not exist. The review packet's "EXISTS mcp/README.md" came from a basename grep that hit unrelated `README.md` files under `.agents/skills/vercel-*`. |
| ADRs | MISSING | `docs/mcp/` is an owned path, not yet created (`ls docs/mcp`: no such directory). No ADR convention exists elsewhere in `docs/` for MCP. |
| OpenSpec changes | MISSING (initialised, empty) [local, uncommitted] | `openspec/config.yaml` holds only the default `schema: spec-driven`. `openspec/specs/` and `openspec/changes/archive/` are empty, and there is no active change. |
| Skills and plugins | MISSING | `mcp/skills/`, `mcp/plugin/`, `.claude-plugin/` and `.agents/plugins/` are owned paths, not yet created. `.agents/skills/` holds only vendored third-party skills (`.agents/skills/VENDORED.md`) [local, uncommitted]. `.codex/agents`, `.opencode/{agents,commands,skills}` and `.kimi-code/{agents,skills}` hold team role definitions, not IPFS plugins [local, uncommitted]. |
| MCP App / React library | MISSING | `mcp/app/` and `mcp/ui/` are owned paths, not yet created. No JS build step exists: `grep -n "npm\|node" Makefile Rules.mk mk/*.mk` returns nothing. |
| Kubo building blocks to reuse | PRESENT | `client/rpc/` implements `coreiface.CoreAPI` over HTTP (`client/rpc/api.go:221-257`: Unixfs, Block, Dag, Name, Key, Pin, Object, Swarm, PubSub, Routing). `HttpApi.Request` (`client/rpc/api.go:206`) is a raw command builder for commands outside CoreAPI: `files/*`, `dag import/export`, `repo gc`, `id`, `stats`. Daemon discovery goes through `$IPFS_PATH/api` (`client/rpc/api.go:59-99`). |
| RPC auth primitives | PRESENT | `API.Authorizations` (`config/api.go:13-32`) is enforced server-side by `withAuthSecrets` (`core/corehttp/commands.go:176-198`). On the client side, `--api-auth` maps to `auth.NewAuthorizedRoundTripper` (`cmd/ipfs/kubo/start.go:354-358`). |
| CLI-only command slot | PRESENT | `localCommands` (`cmd/ipfs/kubo/ipfs.go:20-26`: `daemon`, `init`, `commands`) are CLI-only. They are not part of `core/commands.Root`, so they are not in the `/api/v0` command tree and not in `TestCommands` (`core/commands/commands_test.go:18`). |
| Real-daemon test harness | PRESENT | `test/cli/harness/` (`harness.go`, `node.go`, `http_client.go`). |

## CROSS-TOOL PROGRESS

None. `.kbd-orchestrator/phases/ipfs-mcp-server/progress.json` reports `changes_total: 0`, `implementation_total: 0` and status `PENDING`. No tool has recorded work in this phase.

Team routing, all [local, uncommitted]:

- `.agent-team/kubo-ipfs-agentic/team.json` (untracked) assigns six roles:
  - architect: `openspec/**`, `mcp/contract/**`, `mcp/README.md`, `docs/mcp/**`
  - mcp-server-engineer: `mcp/server/**`
  - skills-plugin-specialist: `mcp/skills/**`, `mcp/plugin/**`, `.claude-plugin/**`, `.agents/plugins/**`
  - ui-ux-engineer: `mcp/ui/**`, `mcp/app/**`
  - two read-only reviewers
- The uncommitted AGENTS.md hunk (`git diff AGENTS.md`, lines added near the top under "Agent Team: Default for All Work") routes work on unowned paths through an architect-owned OpenSpec change. Unowned paths include `core/`, `cmd/`, `config/` and `client/rpc/`.

These facts shape who executes. They are not used to score options.

## SPEC GAP SUMMARY

| Goal requirement | Spec present? | Gap |
|---|---|---|
| Placement decision and ADR (G1) | No (this document carries the DECISION and the ADR DRAFT) | The plan stage must materialize the ADR to `docs/mcp/` (owned path, not yet created). |
| Tool catalog and per-tool JSON Schemas (G2) | No | Needed: a catalog with the prefixes `ipfs_`, `ipns_`, `mfs_`, `dag_`, `pin_`, `car_` and `routing_`; 2020-12 input and output schemas; annotations; an error model; pagination for list tools (`pin ls`, `files ls`, `key list`, `refs`). |
| Tool to `/api/v0` mapping (G2) | No | MFS, CAR import/export, `repo gc` and stats have no typed CoreAPI method. Their output shapes come from `core/commands` output types and must be pinned by contract tests against a real daemon. |
| Transport, auth and Origin/Host policy (G3) | No | Not yet specified: bind defaults, the auth scheme for HTTP, the Origin allowlist, the session model and body limits. |
| Per-capability off switches (G3, G6) | No | Undefined. The DECISION below puts them in `ipfs mcp` flags and a `nomcp` build tag, documented in the CLI help and `docs/mcp/`. |
| Destructive-operation gating (G6) | No | The DECISION below makes destructive tools opt-in. The elicitation question is open. |
| MCP App resources, `ui://` URIs, A2UI catalog (G4) | No | Not yet specified: URI conventions, CSP and sandbox policy, the asset build and embed pipeline. |
| Skills and plugins (G5) | No | See the G5 breakdown below. |
| Disabled-state tests (G6) | No | Not specified. |
| WebMCP in Web UI (G8) | No | Not yet specified: contract-to-WebMCP annotation mapping, the shared JS tool package, the Web UI toggle and default, confirmation UX for consequential tools, the delivery path (Desktop bundle versus Kubo `WebUIPath`), and host support evidence. |

### G5 breakdown: skill areas

All six areas are MISSING. `mcp/skills/` is an owned path, not yet created, and none of the vendored or local skills cover IPFS operations. The upstream candidates were rejected in `.agents/skills/VENDORED.md` [local, uncommitted].

| Skill area | Present | Kubo surface the skill drives (via MCP tools; `ipfs` CLI as fallback) | Notes |
|---|---|---|---|
| Push/pull (add, get, cat) | No | `add`, `get`, `cat` (`core/commands/add.go`, `get.go`, `cat.go`); CoreAPI `Unixfs()` | The skill must not set a chunker or CID version unless the user asks (default CID recipe rule). |
| Pinning | No | `pin add/ls/rm/update/verify` (`core/commands/pin/`); CoreAPI `Pin()` | `pin rm` is destructive and must follow the opt-in decision. |
| IPNS | No | `name publish/resolve`, `key gen/list/rename/rm` (`core/commands/name/`, `keystore.go`) | Key export is excluded from the tool catalog (see DECISION). `key rm` is destructive. |
| MFS | No | `files *` (`core/commands/files.go`; `rm` at `files.go:146`) | No CoreAPI. Goes through the raw RPC builder. `files rm` is destructive. |
| DAG/CAR | No | `dag get/put/import/export/stat` (`core/commands/dag/`) | Import pins roots by default in Kubo; the skill must expose that explicitly. |
| Troubleshooting | No | `id`, `swarm peers`, `routing findprovs`, `stats bw/provide`, `diag`, `version` | Read-only. Must not collect or report peer or user data beyond what the operator asks for (no-tracking rule). |

### G5 breakdown: plugin and marketplace targets

All four targets are MISSING. Every manifest format below is [unverified]: none was fetched in this session.

| Target | Present | Owned packaging path | Launch line under the DECISION | Unknowns |
|---|---|---|---|---|
| Claude Code plugin and marketplace | No | `.claude-plugin/` (owned path, not yet created), `mcp/plugin/` | `{"command":"ipfs","args":["mcp"]}`; remote `{"type":"http","url":...}` | Manifest schema [unverified]; MCP Apps rendering [unverified] |
| Codex | No | `.agents/plugins/` (owned path, not yet created), `mcp/plugin/` | Same stdio command | Plugin and marketplace format and MCP Apps support [unverified] |
| OpenCode | No | `mcp/plugin/` | Same stdio command; remote URL | Plugin and marketplace packaging format [unverified] |
| Kimi Code | No | `mcp/plugin/` | Same stdio command | Plugin and marketplace format entirely unknown [unverified] |

## BUILD HEALTH

- `make build`: **PASS**. Run once this session, 13.8 s wall. The tail of the output:
  ```
  go build "-trimpath" -ldflags="-X "github.com/ipfs/kubo".CurrentCommit=833e04a24-dirty ... buildOrigin=github.com/GQAdonis/kubo" -o "cmd/ipfs/ipfs" "github.com/ipfs/kubo/cmd/ipfs"
  ```
  `./cmd/ipfs/ipfs version --all` reports Kubo 0.44.0-dev-833e04a24-dirty, repo version 18, go1.27.1 darwin/arm64. The binary is 103,307,490 bytes. Afterwards `git status --short` shows no new tracked change: the build regenerates `plugin/loader/preload.go` with identical content.
- Toolchain: local Go is 1.27.1. `go.mod` declares `go 1.26.5`.
- `make test_unit`, `make -O test_go_lint`, `make test_go_fmt` and the `test/cli` gates: **UNKNOWN**. They were not run because no Go code changed in this stage, and `make test` is excluded by policy.

## CONSTRAINT CHECK

Checked against `.kbd-orchestrator/constraints.md` [local, uncommitted], which derives from the committed `AGENTS.md` Stability and User Agency sections.

| Constraint | State now | Implication |
|---|---|---|
| build-passes | PASS | Baseline. |
| go-mod-in-sync | Not evaluated (no dependency change) | `make mod_tidy` runs `find . -name go.mod -execdir go mod tidy` (`Rules.mk:69-70`), so it picks up any nested `go.mod`. It also descends into `node_modules/` once JS packages exist. |
| no-external-replace-directive | PASS | The check exempts only `github.com/ipfs/kubo =>`. A root replace such as `github.com/ipfs/kubo/mcp/server => ./mcp/server` would be flagged. This matters for the options below. |
| rpc-api-compatible | PASS (no diff) | A command in `cmd/ipfs/kubo` `localCommands` stays out of `core/commands.Root`, so `/api/v0` and `TestCommands` are untouched. A command in `core/commands` with `NoRemote` is also kept off HTTP (go-ipfs-cmds v0.16.1 `http/parse.go:55` returns `ErrNotFound`), but it does change `TestCommands`. |
| gateway-conformance | PASS (no diff) | Rules out mounting anything on the gateway listener. |
| default-cid-recipe-unchanged | PASS (no diff) | `ipfs_add` passes no recipe defaults of its own. |
| telemetry-opt-outs / no-tracking | PASS (no diff) | The MCP server logs no CIDs, paths, peer IDs or client IPs by default and has no reporting path. |
| no-silent-cid-injection | PASS (no diff) | The default `Provide.Strategy` is `"all"` (`config/provide.go:12-13`). See Risk 1. |
| configurable-endpoints | PASS | The Kubo RPC endpoint is taken from `--api`, `$IPFS_PATH/api` or `--api-auth`. Nothing is hardcoded. |
| feature-off-switch | n/a yet | See the DECISION: flags, a build tag and disabled-state tests. |
| protected-paths | PASS | No diff under `test/sharness/lib`, `.github`, `version.go` or release tooling. |
| agents-md-separate-pr (warning) | WARNING | `git diff --stat AGENTS.md` shows 260 added lines, uncommitted. The hunks are only near the top and at the end; the Stability and User Agency sections are untouched. The addition must be committed on its own before any code change. |
| mcp-tool-annotations | n/a yet | No contract exists yet. |

## GOAL PROGRESS

| Goal | Status | Notes |
|---|---|---|
| G1 Architecture decision with adversarial review, ADR | DECIDED IN THIS ASSESSMENT; ADR to materialize | See the DECISION and the ADR DRAFT. The plan stage writes it to `docs/mcp/`. |
| G2 Server, annotations, shared contract | NOT STARTED | The surface to wrap is present (`client/rpc`, `core/commands`). |
| G3 stdio + Streamable HTTP, auth, loopback, Origin/DNS rebinding, off switches | NOT STARTED | go-sdk facts are [external], from module source (APPENDIX A2). |
| G4 MCP App, React 19 A2UI v0.9, `ui://`, WCAG 2.2 AA | NOT STARTED | Neither go-sdk MCP Apps helpers nor the extension spec were checked [unverified]. |
| G5 Skills and four plugin targets | NOT STARTED | See the G5 breakdown. |
| G6 No compatibility break, no tracking, no silent injection, destructive gating, docs/tests | NOT STARTED (baseline clean) | |
| G7 Team execution with independent review | NOT STARTED | Role files exist [local, uncommitted]. |
| G8 WebMCP in the IPFS Web UI | NOT STARTED; placement DECIDED in this assessment (see G8 section and DECISION item 8) | The GQAdonis/ipfs-webui fork is cloned at `/Users/gqadonis/Projects/references/ipfs-webui` at `24ddaa89`, in sync with upstream. `grep -rn "modelContext" src` returns nothing, so the fork has no WebMCP code. No role owns the fork. |

---

## ARCHITECTURE OPTIONS

### Facts that constrain every option

1. **One command tree serves CLI and RPC, except for local commands.** `core/commands.Root` (`core/commands/root.go:128-165`) is served over `/api/v0`. `cmd/ipfs/kubo/ipfs.go:20-36` builds the CLI root from `localCommands` plus every `core/commands.Root` subcommand, and local commands are never served over RPC. In `makeExecutor`, a command marked `SetDoesNotUseRepo` and not `NoLocal` runs locally before any API lookup (`cmd/ipfs/kubo/start.go:264-268`). Otherwise the CLI becomes an RPC client when `$IPFS_PATH/api` exists (`start.go:290-372`).
2. **Repo lock.** Running locally constructs the node lazily via `fsrepo.Open` (`start.go:130-152`). That takes `repo.lock` (`repo/fsrepo/fsrepo.go:150-166`). A long-lived MCP process that opened the repo would block `ipfs daemon` for the whole session, and would deadlock any tool that shells out to `ipfs`.
3. **RPC security model.**
   - RPC is admin-level and bound to loopback by default (`docs/config.md:304-331`).
   - Origin/CORS defaults allow localhost and Companion (`core/corehttp/commands.go:39-51, 124-140`).
   - `API.Authorizations` wraps only the cmds handler (`commands.go:144-147`).
   - `AllowedPaths` uses `strings.HasPrefix` (`commands.go:186-191`), so it is not segment-aware.
   - The version check applies only when the User-Agent contains `/kubo/` or `/go-ipfs/` (`commands.go:207-226`). `client/rpc` sets no User-Agent: `grep -rn "User-Agent\|UserAgent" client/rpc/*.go` returns nothing.
4. **CLI stdio safety.**
   - go-ipfs-cmds reads stdin only for arguments marked `SupportsStdin` (v0.16.1 `cli/parse.go:233, 279`).
   - `plugin/loader/*.go` and `cmd/ipfs/kubo/start.go` contain no `fmt.Print` and no direct `os.Stdout` use, apart from passing `os.Stdout` into `cli.Run` (`start.go:214`).
   - [unverified]: whether the go-ipfs-cmds CLI emitter writes anything to stdout for a command that never emits a value.
5. **Plugins.**
   - `PluginDaemon` and `PluginDaemonInternal` run inside the daemon (`plugin/daemon.go`, `plugin/daemoninternal.go`).
   - Preloaded plugins are disable-able via `Plugins.Plugins.<name>.Disabled` (`config/plugins.go:9`).
   - External `.so` plugins are tied to the exact toolchain (`docs/plugins.md:133-175`).
6. **Build-time removal precedent.**
   - `GOTAGS` is wired into the Makefile (`mk/golang.mk:7, 33-34`).
   - The `nofuse` tag stubs out FUSE (`core/commands/mount_nofuse.go`).
   - AGENTS.md (committed) requires that builders can remove built-in destinations at build time for telemetry. The same mechanism suits an optional subsystem.
7. **Embedded assets.** One non-test `go:embed` exists (`assets/assets.go:15`). The WebUI is served by CID through the API port (`core/corehttp/webui.go:15`; `cmd/ipfs/kubo/daemon.go:870-877`), not embedded.
8. **Dependency weight.** All [external]/[probe]; see APPENDIX.
   - go-sdk v1.8.0 requires `golang-jwt/jwt/v5`, `google/jsonschema-go`, `segmentio/encoding`, `yosida95/uritemplate/v3`, `golang.org/x/{oauth2,time,tools}`, with `segmentio/asm`, `x/sync` and `x/sys` indirect.
   - Five of these are absent from Kubo's `go.mod`: jwt, jsonschema-go, segmentio/encoding, segmentio/asm, uritemplate. `x/oauth2`, `x/time` and `x/tools` are already indirect.
   - Binary sizes from the probes:

     | Build | Bytes |
     |---|---|
     | `ipfs` main | 103,257,762 |
     | same main + minimal go-sdk server | 105,994,050 (+2.74 MB, +2.7%) |
     | standalone `client/rpc` + go-sdk | 18,878,162 |
9. **A nested module cannot be linked into `ipfs` cleanly.**
   - The root module would have to require `github.com/ipfs/kubo/mcp/server`.
   - That nested module itself requires `github.com/ipfs/kubo` for `client/rpc`, so the two modules depend on each other.
   - The require resolves either through a published version or through a root `replace ... => ./mcp/server`, which the no-external-replace check flags (see CONSTRAINT CHECK).
   - Therefore "separate module plus thin `ipfs mcp` launcher" reduces in practice to one of two shapes:
     - (i) an exec-dispatch launcher that runs a separately installed binary;
     - (ii) the server as a package inside the root module.
10. **Boxo.** An MCP server is a product surface over Kubo's own RPC. "Where a change belongs" (committed AGENTS.md) places CLI, RPC and config in Kubo. A generic CoreAPI-to-MCP adapter would import `kubo/core/coreiface`, which boxo cannot import. Boxo placement is rejected for every piece.
11. **Upstream signal** [external]: `gh search issues "MCP" --repo ipfs/kubo` returns `[]`; `gh search issues "model context protocol" --repo ipfs/kubo` returns `[]`; `gh search prs "MCP" --repo ipfs/kubo` returns only two unrelated provider-test PRs. Neither the `ipfs` org nor the `ipshipyard` org has a repo matching "mcp". Maintainer appetite is unknown.

### Options

**A. `ipfs mcp` subcommand, server code inside the root module.**

- **Placement and lock:**
  - Registered in `localCommands` (`cmd/ipfs/kubo/ipfs.go:22`), next to `daemon` and `init`.
  - Marked `SetDoesNotUseRepo`, so it never constructs a node and never takes `repo.lock`.
  - It talks to the daemon via `client/rpc`, using the API address from `--api` or `$IPFS_PATH/api` and auth from `--api-auth` (the same plumbing as `start.go:290-358`).
- **Transports:** stdio by default. Streamable HTTP is opt-in in the same process.
- **Compatibility:** `/api/v0` and `TestCommands` are unchanged.
- **Costs:**
  - go-sdk in the root `go.mod`: five new modules, +2.74 MB at minimum.
  - A UI bundle embedded in `ipfs`.
  - Edits to `cmd/ipfs/kubo`.
  - It only exists in this fork's binary until upstream accepts it.

**B. Separate binary in a nested Go module `mcp/server` using `client/rpc`.**

- **Gains:**
  - Zero Kubo diff.
  - go-sdk stays out of the root module.
  - An 18.9 MB binary.
- **Costs:**
  - A second binary to install and configure.
  - `go install` is blocked. The module path `github.com/ipfs/kubo/mcp/server` cannot be fetched from the GQAdonis fork. If the module also uses the existing sub-module self-replace pattern (`docs/examples/kubo-as-a-library/go.mod:7`), Go refuses to install it at a version.
  - Releasing binaries becomes new release tooling outside Kubo's.
  - It does not deliver "MCP support in the ipfs CLI".

**C. Daemon-hosted endpoint.**

- **C1, a new listener `Addresses.MCP`:**
  - A new serve function beside `serveHTTPApi`/`serveHTTPGateway` (`daemon.go:806, 1063`).
  - Has in-process CoreAPI and node access.
  - Needs new config in `config/addresses.go:3-10` and `docs/config.md`, plus a changelog entry and disabled-state tests.
  - A second auth system inside the daemon.
  - stdio needs a bridge.
- **C2, mounted on the RPC mux:** bypasses `API.Authorizations` unless re-wrapped (`commands.go:144-147`).
- **Gateway mount:** rejected outright.

**D. Kubo plugin.**

- In-daemon like C1, but wired through `PluginDaemonInternal`.
- Its off switch is the plugin's `Disabled` flag.
- Same root `go.mod` cost as A and C.
- `.so` distribution is impractical.

**E. Out-of-tree repo.**

- **Gains:**
  - Best `go install`/`npx` distribution.
  - No Kubo coupling.
- **Costs:**
  - Loses the in-tree contract, CI against a fork-built daemon, and the "ipfs CLI" goal.
- **Reachability:** reachable later from A or B by moving a package.

**F. Hybrids.**

- **F1, C1 + A as a stdio bridge:** HTTP in the daemon, stdio via `ipfs mcp`. Pays both costs.
- **F2, B as a library package with A/C added later:** this was revision 1's provisional choice. Fact 9 shows the "add `ipfs mcp` later" step costs a module restructure or an exec launcher, not a thin import.
- **F3, B + exec-dispatch launcher:** `ipfs mcp` execs `ipfs-mcp` found on `PATH`, the way `git` dispatches subcommands.
  - Gains: no root deps.
  - Costs: a PATH-lookup exec behavior that is new in Kubo (binary-planting surface), two artifacts to version in lockstep, and the user still installs two things.
- **F4, A with a library-structured server, a `nomcp` build tag, and no daemon hosting in this phase:**
  - The server lives in package `github.com/ipfs/kubo/mcp/server` inside the root module.
  - It is wired through the `ipfs mcp` local command.
  - It is excluded from builds with `GOTAGS=nomcp` (fact 6).
  - Handlers depend on a narrow interface, so a daemon endpoint (C1) can import the same package later.

### Steelman of the strongest alternatives against the earlier F2 choice

- **The operator's stated goals.** The operator asked for "MCP server support to the kubo ipfs cli" and for remote Streamable HTTP.
- **F2/B misses one of them.** It ships a separate program, so it does not meet the first goal. Its strongest argument is that it adds no root dependency. That is a cost argument, not a goal argument.
- **Adding `ipfs mcp` later is not a thin step.** Fact 9 shows that adding it to F2 later is either an exec launcher (F3) or moving the package into the root module (F4) anyway. So F2 defers the cost; it does not avoid it.
- **F1 versus F4.** F1 (daemon HTTP + stdio bridge) is the most integrated end state. It wins only if a single daemon process must also be the remote endpoint. F4 meets the remote HTTP goal with `ipfs mcp --http` as a client process of the daemon, with:
  - no new daemon listener;
  - no `Addresses`/`docs/config.md` keys;
  - no in-daemon blast radius;
  - one auth path into Kubo (`API.Authorizations`).
- **F3 versus F4.** F3 meets the CLI goal without root deps, but at the cost of exec-from-PATH behavior and two installs. F4 meets the goal with one binary, and its dependency cost can be opted out at build time.
- **Result:** on the operator's stated goals the hybrid wins, and among the hybrids F4 meets both goals with the least new surface in Kubo itself.

### Scores

1 is worst and 5 is best. "Compat" scores high when compatibility risk is low. "Goal fit" means the operator's stated goals: CLI support plus remote Streamable HTTP. Team ownership is not scored.

| Option | Goal fit | Compat | Security | Local UX | Remote UX | Distribution | Maintenance | Upstreamability |
|---|---|---|---|---|---|---|---|---|
| A | 4 | 4 | 4 | 5 | 3 | 3 (ships in fork `ipfs`) | 3 | 2 |
| B | 2 | 5 | 4 | 3 | 4 | 2 (go install blocked; own release tooling) | 4 | 3 |
| C1 | 3 | 3 | 3 | 2 | 5 | 3 | 2 | 2 |
| C2 | 3 | 3 | 2 | 2 | 4 | 3 | 2 | 1 |
| D | 3 | 3 | 3 | 2 | 4 | 2 | 2 | 2 |
| E | 2 | 5 | 4 | 3 | 4 | 5 | 3 | n/a |
| F1 | 5 | 3 | 3 | 5 | 5 | 3 | 1 | 1 |
| F2 | 2 | 5 | 4 | 3 | 4 | 2 | 3 | 3 |
| F3 | 4 | 4 | 3 | 4 | 4 | 2 | 2 | 1 |
| **F4** | **5** | **4** | **4** | **5** | **4** | **3** | **3** | **2** |

---

## G8: WEBMCP IN THE IPFS WEB UI

### Facts

Codebase facts below were read in this session. Web UI fork files are cited as `webui:<path>:<line>`, from `/Users/gqadonis/Projects/references/ipfs-webui`, which was not modified. Desktop fork files are cited as `desktop:<path>:<line>`, from `/Users/gqadonis/Projects/references/ipfs-desktop` (HEAD `9bd9904`, origin `tribe-health/ipfs-desktop`).

1. **How Kubo serves the Web UI.**
   - `WebUIPath` is a pinned constant: `/ipfs/bafybeiciqeyipumpmhxzlxnbqdbbv6u5uij4hy4wax64dmj7kvrhusiq6y`, v4.13.0 (`core/corehttp/webui.go:15`). Past CIDs are listed at `webui.go:18-35`.
   - `WebUIOption` mounts `/webui/` on the RPC listener (`webui.go:93-106`). The handler copies `API.HTTPHeaders` (`webui.go:118-120`) and redirects to `WebUIPath` (`webui.go:147`). It refuses to serve when `Gateway.DeserializedResponses=false`, and also when `Gateway.NoFetch` is set and the CID is not local (`webui.go:122-145`).
   - The gateway on the API port serves only `WebUIPaths` unless `--unrestricted-api` is set (`cmd/ipfs/kubo/daemon.go:870-877`).
   - **Consequence:** a forked Web UI CID can be served from the RPC origin only in two ways:
     - (a) a Kubo code change that edits `WebUIPath`/`WebUIPaths`;
     - (b) `--unrestricted-api`, which opens arbitrary CIDs on the admin origin. That is rejected.
2. **The Web UI app.**
   - `webui:package.json:3` gives version 4.13.0. It uses React 16 (`webui:package.json:83`), create-react-app via `react-scripts` 5.0.1 (`:100`), and `kubo-rpc-client` ^6.1.0 (`:76`).
   - The build goes to `build/` with relative asset paths (`webui:AGENTS.md`, "Build and test" and "Hosting notes"; `"homepage": "./"` at `webui:package.json:191`).
   - It talks to the RPC through `ipfs-provider` and `kubo-rpc-client`. The API address comes from settings or localStorage (`webui:src/bundles/ipfs-provider.js:145, 377-432, 452-458`).
   - State lives in a redux-bundler store (`webui:src/bundles/index.js`, `webui:src/index.js:7`), with a `ContextBridge` for bundle-to-React context (`webui:src/helpers/context-bridge.tsx`). That store is the natural place for tool executors to reuse existing actions (files, pins, IPNS).
   - Analytics are hard-disabled: `DISABLE_ALL_ANALYTICS = true` at `webui:src/bundles/analytics.js:78`, though the `countly-sdk-web` dependency is still declared (`webui:package.json:50`).
   - The release flow gates pinning and publishing to `github.repository == 'ipfs/ipfs-webui'`, so fork CI builds but does not publish (`webui:AGENTS.md`, "Forks").
3. **How IPFS Desktop gets the Web UI.**
   - It downloads the same CID at build time (`desktop:package.json:25`: `npx ipfs-or-gateway -c bafybeiciqey... -g "https://trustless-gateway.link"`), or copies a local build via `IPFS_WEBUI_PATH` (`desktop:package.json:24`, `desktop:scripts/copy-local-webui.js:6-25`).
   - It serves the UI under the custom scheme origin `webui://localhost` (`desktop:src/webui/index.js:22-24`), on Electron ^44.3.0 (`desktop:package.json:56`).
4. **WebMCP** [external], from the W3C Web Machine Learning CG Draft Community Group Report dated 8 October 2026 (https://webmachinelearning.github.io/webmcp/, fetched with `curl -sfL` this session):
   - **The getter.** `partial interface Document { [SecureContext, SameObject] readonly attribute ModelContext modelContext; }`.
   - **Registration.** `registerTool(ModelContextTool, {exposedTo, signal})`. `ModelContextTool` has `name`, `title`, `description`, `inputSchema` (JSON Schema), `execute(input, {signal})` and `annotations`.
   - **Annotations.** `ToolAnnotations` has exactly `readOnlyHint`, `untrustedContentHint`, `consequentialHint` and `debugging`. There is no `destructiveHint`, `idempotentHint` or `openWorldHint`.
   - **Permissions policy.** Access is gated by the policy-controlled feature `tools`, default allowlist `'self'`. `Permissions-Policy: tools=()` disables it for a document and all its frames (§4.4 and the security considerations).
   - **Not in this draft:** the text has no `navigator.modelContext` and no `requestUserInteraction`.
   - **Unverified items from the coordinator:** the 2026-05-27 navigator-to-document move, the Chrome 150 deprecation of `navigator.modelContext`, the Chrome 149-156 origin trial, and the Edge/Firefox/Safari positions. The `navigator.modelContext` fallback in G8's wording targets an API this draft no longer defines.
   - **Secure context.** `http://127.0.0.1` and `*.localhost` count as potentially trustworthy, so they are secure contexts; a LAN `http://` origin is not. This is standard Secure Contexts behavior, not re-checked this session [unverified here].
   - **Host support.** Whether Kimi desktop, Codex desktop or Electron 44 (and therefore IPFS Desktop) expose `document.modelContext` is [unverified].

### Placement options for G8

- **W1: tools inside the GQAdonis/ipfs-webui fork.**
  - A new bundle (for example `src/webmcp/`) registers tools when `document.modelContext` exists and the user has enabled the feature.
  - Executors reuse `kubo-rpc-client` and redux-bundler actions.
  - Best grounding: same origin as the RPC, existing auth, existing UI for confirmations.
  - Requires shipping a new build.
- **W2: injected script or browser extension** that registers tools into the stock Web UI page.
  - Works with the stock CID and needs no Kubo or Web UI change.
  - It runs in someone else's page and depends on the app's internals, which break on every upstream release.
  - It needs a store-distributed extension.
  - It cannot cover Desktop's `webui://` origin.
- **W3: IPFS Desktop wrapper.** An Electron preload registers tools and calls Kubo via Desktop's existing node handle.
  - Desktop only.
  - Depends on Electron exposing WebMCP [unverified].
  - Does not help browser users of `/webui`.
- **W4: a shared framework-free JS package** generated from `mcp/contract`, holding tool definitions plus RPC executors over `kubo-rpc-client`.
  - It is a layer, not a placement. Consumers: W1, W2 or W3 for WebMCP, and the MCP App for definitions and labels.
  - It must not depend on React. The Web UI is on React 16 and the MCP App targets React 19, so a React-bound package cannot serve both.

**Delivery path for a changed Web UI (W1):**

| Path | Kubo change | Who gets it | Assessment |
|---|---|---|---|
| D1: Desktop fork via `IPFS_WEBUI_PATH` (`desktop:scripts/copy-local-webui.js`) | None | Users of a fork-built Desktop | Lowest blast radius, but WebMCP only works if Electron exposes it [unverified]. |
| D2: bump `WebUIPath` in this Kubo fork | `core/corehttp/webui.go:15, 18-35` | Every fork user opening `/webui` | Changes what the RPC origin serves. The first visit fetches the new CID from the network unless it is pinned (with `Gateway.NoFetch`, the handler returns 503 until it is pinned, `webui.go:127-145`). Needs a `test/cli` check and a changelog note. Not an `/api/v0` break, but it is admin-origin content chosen by this fork. |
| D3: serve the fork build from a subdomain gateway origin (`<cid>.ipfs.localhost:8080`) plus an `API.HTTPHeaders` CORS entry for that origin | None (operator config) | Operators who opt in | Works without Kubo code. The per-CID origin isolates the page. The operator must widen RPC CORS to one origin. |
| D4: path gateway `127.0.0.1:8080/ipfs/<cid>` plus RPC CORS for that origin | None | n/a | **Rejected.** Every CID on the path gateway shares that origin, so granting it RPC CORS gives admin RPC to any content the node serves. |

**One contract across MCP server, MCP App, A2UI and WebMCP:**

- `mcp/contract` remains the single source. Each tool records:
  - its name, description, and 2020-12 input and output schemas;
  - the MCP annotations;
  - one extra boolean, `returnsUntrustedContent`.
- The WebMCP annotations are derived, not authored:

  | WebMCP annotation | Derived from |
  |---|---|
  | `readOnlyHint` | the MCP `readOnlyHint` |
  | `consequentialHint` | `true` for any tool that is not read-only (writes, publishes, provides) and for every `destructiveHint` tool |
  | `untrustedContentHint` | `returnsUntrustedContent` (`ipfs_cat`, `ipfs_get`, `dag_get`, `ipns_resolve` results come from the network) |
  | `debugging` | `true` only for troubleshooting tools |
  | `idempotentHint`, `openWorldHint` | no WebMCP equivalent; dropped |
- W4 is generated from the contract. The Go server validates against the same files. A contract test checks that every implementation:
  - exposes the same names and schemas;
  - uses the same derived annotations.

**Security model:**

- **The page holds admin RPC access.** The Web UI runs on the RPC origin `http://127.0.0.1:5001` and already has admin RPC access. Registering WebMCP tools lets any agent in that browser or host call them; agents act on untrusted inputs, so prompt-injected calls are expected.
- **Off by default.** A Settings toggle (localStorage, per origin) enables WebMCP. Tools are registered only when the toggle is on and `document.modelContext` exists.
- **Initial tool set.** The read-only set ships first.
- **Writes and consequential tools.** Write and consequential tools are enabled by a second toggle. Each call requires an in-page confirmation dialog that names the operation and its CIDs or paths. The draft has no browser-mediated confirmation API, so the app must provide it.
- **Destructive tools** (`pin_rm`, `mfs_rm`, `key_rm`, `repo_gc`) are not exposed over WebMCP in this phase.
- **No cross-origin exposure.** `exposedTo` is left empty, and nothing widens the default `'self'` allowlist.
- **No silent CID injection.** Tools add, pin or provide only on an agent call that the user confirmed in the page. Executors set `pin` and `provide` explicitly rather than relying on hidden defaults.
- **Operator off switch.** `Permissions-Policy: tools=()` removes the whole feature for that origin. Whether `Gateway.HTTPHeaders` (`core/corehttp/gateway.go:426`) applies this to Web UI content served on the API port is [unverified].

**Team ownership:**

- No role owns the Web UI fork.
- **Recommendation:**
  - ui-ux-engineer owns `GQAdonis/ipfs-webui:src/webmcp/**` and the Settings toggle, plus a new kubo-repo path `mcp/webmcp/**` for the W4 package (owned path, not yet created);
  - the architect owns the annotation mapping in `mcp/contract`;
  - security-reviewer reviews the fork changes.
- This needs an update to `.agent-team/kubo-ipfs-agentic/team.json` [local, uncommitted] by the operator or agent-team-creator.
- Upstream `ipfs/ipfs-webui` has its own `AGENTS.md` and release rules (`webui:AGENTS.md`). Fork-only work is not bound by them, but any upstream PR would be.

### G8 scores

1 is worst and 5 is best.

| Option | Goal fit | Security | Maintenance | Reach | Kubo impact (5 = none) |
|---|---|---|---|---|---|
| **W1 + W4, delivered via D1 then D3** | 5 | 4 | 3 | 3 | 5 |
| W1 + W4 via D2 (`WebUIPath` bump) | 5 | 3 | 3 | 4 | 3 |
| W2 extension | 3 | 3 | 2 | 3 | 5 |
| W3 Desktop preload | 3 | 4 | 3 | 2 | 5 |

---

## DECISION

**Decision of record: F4.**

- **Entry point.** MCP support ships as an `ipfs mcp` CLI subcommand in the `ipfs` binary.
- **Placement.** It is registered in `cmd/ipfs/kubo` `localCommands`, so it is never part of `/api/v0`.
- **Code location.** The server lives in package `github.com/ipfs/kubo/mcp/server`, inside the root module. There is no nested `go.mod`.
- **Connection to Kubo.** It always connects to a running daemon through `/api/v0` via `client/rpc`. It never opens the repo.

The rest of the decision:

1. **Transports.**
   - stdio is the default.
   - `--http=<multiaddr>` enables Streamable HTTP in the same process. It defaults to loopback when given without an address.
   - The go-sdk loopback Host check stays on.
   - Explicit cross-origin protection uses an empty allowlist by default.
   - A request body limit is set.
   - A non-loopback bind is refused unless a bearer token is configured (`--http-auth` via env or a file, never a literal on the command line).
   - No daemon-hosted endpoint in this phase.
2. **Connection to Kubo.**
   - The endpoint comes from `--api`, `$IPFS_PATH/api`, or an explicit URL. Nothing is hardcoded.
   - Credentials come from `--api-auth`.
   - Docs and tests cover running the server with a dedicated `API.Authorizations` secret whose `AllowedPaths` list only the enabled tools' commands.
   - An unreachable daemon is an explicit tool error.
   - There is no offline mode.
3. **Capability switches** (`ipfs mcp` flags, plus an optional MCP config file read only by `ipfs mcp`; no Kubo config keys):
   - Each tool group can be enabled or disabled.
   - Read-only tools are on by default.
   - Content-writing tools (`ipfs_add`, `car_import`, `mfs_write`, `pin_add`, `ipns_publish`, `routing_provide`) are on for stdio and off for HTTP by default.
   - Destructive tools (`pin_rm`, `mfs_rm`, `key_rm`, `repo_gc`) are off everywhere unless enabled by an explicit flag, and are annotated `destructiveHint: true`.
   - No tool exports private keys.
4. **Build-time removal.** `GOTAGS=nomcp` compiles `ipfs mcp` out (stub pattern like `mount_nofuse.go`). The dependencies remain listed in `go.mod`.
5. **UI.**
   - `mcp/app` and `mcp/ui` build with their own JS toolchain.
   - The built bundle is committed under the server package and embedded with `go:embed`.
   - CI checks it by rebuilding and diffing.
   - It is served only as `ui://` MCP resources over the MCP channel: no new HTTP route on Kubo and no CORS change on the RPC port.
6. **Documentation.**
   - Off switches and flags are documented in `ipfs mcp --help` and `docs/mcp/`.
   - Disabled-state tests run in `test/cli` against a `make build` binary.
7. **Kubo surfaces that do not change:** `/api/v0`, the gateway, CID defaults, wire behavior, the daemon, `config/` and `docs/config.md`.
8. **G8, WebMCP in the Web UI: W1 + W4.**
   - **Placement.** WebMCP tools live in the GQAdonis/ipfs-webui fork (`src/webmcp/`). They are built from a framework-free package `mcp/webmcp` generated from `mcp/contract`, and use derived WebMCP annotations.
   - **Defaults.**
     - Off by default (Settings toggle), and registered only when `document.modelContext` exists. There is no reliance on `navigator.modelContext`, which the current draft does not define.
     - The read-only set comes first.
     - Write and consequential tools sit behind a second toggle, with an in-page confirmation per call.
     - Destructive tools are not exposed over WebMCP.
   - **Delivery.**
     - First via a fork-built Desktop (`IPFS_WEBUI_PATH`, D1) and the documented subdomain-gateway setup (D3). Neither requires a Kubo change.
     - Bumping `WebUIPath` in this Kubo fork (D2) is deferred to its own OpenSpec change, made only on operator request.
   - **Gate on host support.** No implementation work on G8 starts until a WebMCP host is confirmed. The plan stage's first G8 task is a host-support spike: Chrome origin trial, Kimi desktop, Codex desktop, Electron 44.

**Operator answers that would flip this decision:**

| If the operator answers... | ...the decision flips to |
|---|---|
| "No new dependencies in the root `go.mod`" or "keep the `ipfs` binary unchanged" (for example, to keep the fork rebase-clean or to propose upstream as a separate tool) | **B**, a nested module and separate binary, accepting the release-tooling cost; or **E** if `go install`/`npx` distribution is the priority |
| "Users on stock (non-fork) Kubo must be able to use it" | **E** (preferred) or **B**. F4 only exists in this fork's binary. |
| "The remote endpoint must be served by the daemon itself, with no second process" | **F1**: add **C1** (`Addresses.MCP`, default empty/off, `docs/config.md`, changelog, disabled-state tests) as a separate OpenSpec change that imports the same package. The F4 stdio path stays. |
| "Upstream acceptance is the goal and maintainers ask for a separate tool" | **E**, a package move |
| G8: "WebMCP must work for users of stock Kubo or stock Web UI" | **W2**, a browser extension, accepting the coupling to upstream internals |
| G8: "every fork user should get the WebMCP Web UI at `/webui`" | Add **D2**, a fork `WebUIPath` bump, as its own OpenSpec change with `test/cli` coverage and a changelog note |
| G8: the host-support spike finds no target host exposing `document.modelContext` | **Defer G8.** Keep only the contract mapping and the W4 package, and record the deferral in an ADR. |
| "Offline use without a daemon is required" | **No flip.** Holding `repo.lock` for an agent session conflicts with the lock lesson and bypasses `API.Authorizations`. The answer would be an explicit refusal plus guidance to start the daemon (`timeout`/`ipfs shutdown` per AGENTS.md). |

## ADR DRAFT

To be materialized by the plan stage as `docs/mcp/0001-mcp-server-placement.md`. `docs/mcp/` is an owned path, not yet created, and no ADR numbering exists yet, so this starts the sequence.

**Title:** ADR-0001: MCP server ships as `ipfs mcp`, a daemon RPC client inside the root module

**Status:** Proposed (accepted on operator approval at the assess/plan boundary)

**Context.**

- **Operator goals:**
  - MCP server support in the Kubo `ipfs` CLI;
  - stdio for local agents and Streamable HTTP for remote use;
  - an MCP App UI;
  - skills and plugins for Claude Code, Codex, OpenCode and Kimi Code.
- **Hard constraints** (committed AGENTS.md Stability and User Agency):
  - never break `/api/v0`, the gateway, the default CID recipe or wire compatibility;
  - no tracking;
  - no silent CID injection;
  - an off switch for every feature;
  - new modules get code-level scrutiny.
- **Codebase facts:**
  - CLI-only commands live in `localCommands` (`cmd/ipfs/kubo/ipfs.go:20-26`) and are not served over RPC.
  - Local execution of repo-using commands takes `repo.lock` (`repo/fsrepo/fsrepo.go:150-166`).
  - `client/rpc` covers most operations; `HttpApi.Request` covers the rest (`client/rpc/api.go:206`).
  - `API.Authorizations` protects only the cmds handler (`core/corehttp/commands.go:144-147`).
  - go-sdk v1.8.0 adds five modules to the root graph and at least 2.74 MB to `ipfs` [probe].

**Decision.**

- The server is implemented in package `github.com/ipfs/kubo/mcp/server` (root module) and exposed as the local command `ipfs mcp`.
- **Repo and daemon:** it never opens the repo and always connects to the daemon over `/api/v0` with `client/rpc`.
- **Transports:** stdio by default; opt-in Streamable HTTP in the same process, loopback by default, with the Host check, cross-origin protection and a body limit. A non-loopback bind requires a bearer token.
- **Capability defaults:** read-only tools on; writes on for stdio and off for HTTP; destructive tools off unless the operator enables them; no key export.
- **Build-time removal:** the `nomcp` build tag removes the command.
- **UI:** bundles are embedded and served only as `ui://` resources.
- **No daemon endpoint** in this phase.

**Consequences.**

- **Positive:**
  - One binary.
  - Plugin manifests use `{"command":"ipfs","args":["mcp"]}`.
  - No change to `/api/v0`, the gateway, CID defaults, wire behavior, daemon config or `docs/config.md`.
  - Lock contention cannot occur.
  - One auth path into Kubo.
  - Builders can compile it out.
- **Negative:**
  - go-sdk and its five new modules join the root `go.mod` and every default build (+2.74 MB minimum before tool code and UI) [probe].
  - A committed JS build artifact lives in the repo.
  - It exists only in this fork's `ipfs` unless upstreamed, so stock-Kubo users cannot use the plugins.
  - Remote use requires a second process (`ipfs mcp --http`) beside the daemon.
  - MFS/CAR/gc output shapes are untyped and must be pinned by real-daemon contract tests.
  - Edits to `cmd/ipfs/kubo` need an architect-owned OpenSpec change, because `cmd/` has no owning role [local, uncommitted].
- **Follow-ups:**
  - An optional daemon-hosted endpoint (C1) via its own OpenSpec change if the operator requires single-process remote.
  - Upstream proposal via an `ipfs/kubo` issue if desired.

**Rejected alternatives.**

| Alternative | Rejected because (evidence) |
|---|---|
| B: nested module, separate binary | Misses the "ipfs CLI" goal. `go install` is blocked by the fork module path and the self-replace pattern (`docs/examples/kubo-as-a-library/go.mod:7`). Linking it into `ipfs` later needs a root replace that the no-external-replace check flags, or a module cycle (fact 9). |
| C1: daemon listener `Addresses.MCP` | Adds a daemon feature: new `config/addresses.go` keys, `docs/config.md`, changelog, disabled-state tests. It puts a second admin-auth system inside the daemon process. Remote HTTP is achievable without it. Kept as a conditional follow-up. |
| C2: mount on the RPC mux | Bypasses `API.Authorizations`, which wraps only the cmds handler (`core/corehttp/commands.go:144-147`). Widens the admin port. |
| Gateway mount | The gateway is a spec-defined, often public interface (committed AGENTS.md Stability). |
| D: plugin | Same root-dependency and daemon blast-radius cost as C1. `.so` plugins are toolchain-locked (`docs/plugins.md:133-175`). No stdio path. |
| E: out-of-tree repo | Misses the "ipfs CLI" goal. Loses in-tree contract tests against a fork-built daemon. Remains the fallback if the operator rejects root dependencies or needs stock-Kubo users. |
| F1: daemon HTTP + stdio bridge | Pays the costs of A and C1 together. It wins only if a single daemon process must be the remote endpoint (conditional). |
| F2: B now, adapters later | Defers rather than avoids the root-module decision (fact 9). Misses the CLI goal in this phase. |
| F3: exec-dispatch launcher | Introduces PATH-lookup exec behavior into `ipfs` (binary-planting surface). Two artifacts to keep in lockstep. Two installs. |
| Offline mode (open repo in `ipfs mcp`) | Holds `repo.lock` for the session (`fsrepo.go:150-166`), blocking `ipfs daemon`. It also bypasses `API.Authorizations`. |
| Boxo placement | It is a product surface over Kubo RPC, not a reusable primitive. A CoreAPI adapter would import `kubo/core/coreiface` (committed AGENTS.md "Where a change belongs"). |

### ADR-0002 draft: WebMCP in the IPFS Web UI

To be materialized as `docs/mcp/0002-webmcp-web-ui.md` (owned path, not yet created).

- **Status:** Proposed. Implementation is gated on a confirmed WebMCP host.
- **Context.**
  - G8.
  - Kubo serves a pinned Web UI CID only, on the RPC origin (`core/corehttp/webui.go:15, 93-147`; `cmd/ipfs/kubo/daemon.go:870-877`).
  - Desktop bundles that CID or a local build (`desktop:package.json:24-25`).
  - The Web UI is React 16 with `kubo-rpc-client` and redux-bundler (`webui:package.json:76, 83`).
  - The WebMCP draft (8 Oct 2026) defines `document.modelContext.registerTool` with annotations `readOnlyHint`, `untrustedContentHint`, `consequentialHint` and `debugging`, gated by the `tools` permissions policy.
- **Decision.**
  - Tools are registered by a fork-only `src/webmcp/` bundle in GQAdonis/ipfs-webui.
  - They are built from the contract-generated, framework-free package `mcp/webmcp`, with WebMCP annotations derived from the MCP ones.
  - The feature is off by default. Writes need a second toggle and an in-page confirmation per call. There are no destructive tools.
  - Delivery is via Desktop fork bundling and opt-in subdomain-gateway use; there is no Kubo `WebUIPath` change in this phase.
- **Consequences.**
  - Kubo is untouched.
  - The fork must be kept in sync with upstream `ipfs/ipfs-webui`.
  - Two RPC client stacks implement the contract (Go `client/rpc`, JS `kubo-rpc-client`); contract tests must cover both.
  - Browser users of stock Kubo `/webui` do not get WebMCP until D2 or an upstream merge.
  - Reach depends on host support, which is unverified.
- **Rejected alternatives.**

  | Alternative | Rejected because |
  |---|---|
  | W2 extension | Couples to upstream internals; needs store distribution; does not reach Desktop |
  | W3 Desktop preload | Desktop-only; depends on Electron WebMCP support [unverified] |
  | D2 `WebUIPath` bump now | Changes the content of the admin origin for every fork user; deferred, not refused |
  | D4 path-gateway origin with RPC CORS | Gives admin RPC to every CID served on that shared origin |
  | `--unrestricted-api` | Opens arbitrary CIDs on the admin origin |
  | A React-bound shared package | The Web UI (React 16) and the MCP App (React 19) cannot share it |

### Where this decision is weak

- **Dependency cost is real.** It is the strongest objection to F4, and the reason B scores higher on compat and maintenance. The `nomcp` tag removes the code from a build, but `go.mod` still lists the modules, and `segmentio/asm` ships assembly.
- **Two processes for remote use.** An operator who expected `ipfs daemon` to serve MCP will find that it does not.
- **Fork-only.** Plugins built around `ipfs mcp` do not work on stock Kubo.
- **stdout hygiene.** One unverified point remains (fact 4) and must be pinned by a test.

---

## RISKS AND OPEN QUESTIONS

### Top risks

1. **Prompt-injected writes and CID injection.**
   - The default `Provide.Strategy` is `"all"` (`config/provide.go:13`), so agent-triggered fetches and adds are stored and announced.
   - A tool call is an operator-exposed interface, but a weaker "local cause" than a human typing `ipfs add`.
   - Mitigation: HTTP writes are off by default; destructive tools are opt-in; pin and provide behavior is explicit in tool parameters.
2. **Remote exposure of admin capability.**
   - A non-loopback bind, a weak token or broad `AllowedPaths` gives remote control of the node.
   - `AllowedPaths` prefix matching is not segment-aware (`commands.go:186-191`).
3. **Fork-only distribution and dependency cost.**
   - Plugins depend on this fork's `ipfs`.
   - The root `go.mod` grows by five modules.
   - The flip conditions in the DECISION are the escape hatches.
4. **stdio stream corruption.**
   - The CLI emitter's behavior for a never-emitting command is [unverified].
   - It needs a `test/cli` test that runs a full MCP initialize exchange over stdio.
5. **MCP Apps maturity and client support.** Unverified across all four hosts; tools must stay fully usable without the UI.
6. **JS supply chain inside a Go repo.** A committed bundle must not contain analytics, remote fonts or CDN fetches (no-tracking and configurable-endpoints gates).
7. **G8: any agent in the host drives an admin-privileged page.**
   - The Web UI origin holds admin RPC.
   - A WebMCP-enabled browser or desktop host lets every agent session there call the registered tools.
   - Prompt injection via cat/get output is the expected attack.
   - Mitigations:
     - off by default;
     - `untrustedContentHint` on content-returning tools;
     - per-call confirmation for consequential tools;
     - no destructive tools;
     - an empty `exposedTo`;
     - `Permissions-Policy: tools=()` as the operator off switch.
8. **G8: host support may not exist yet.** The draft is a CG report; Chrome support is through an origin trial, and Kimi, Codex and Electron support are unverified. G8 could ship code that no target host runs, hence the spike gate.
9. **G8: the Web UI fork drifts from upstream `ipfs/ipfs-webui`.** That upstream releases frequently via semantic-release, so the fork carries a rebase cost. The fork also depends on `countly-sdk-web` (disabled in code, `webui:src/bundles/analytics.js:78`). The fork must keep it disabled to satisfy the no-tracking gate.

### Questions only the operator can answer

1. **Upstream intent.** Is this fork-only or headed upstream? If upstream, open an `ipfs/kubo` issue before `cmd/` work.
2. **Root dependencies.** Are go-sdk's five new modules in the root `go.mod` acceptable? A "no" flips the decision to B or E.
3. **Stock Kubo.** Must users on stock Kubo be supported? A "yes" flips the decision to E or B.
4. **Remote setup.**
   - Is a separate `ipfs mcp --http` process acceptable, or must the daemon host it? Requiring the daemon flips the decision to F1.
   - Who are the remote users, and who terminates TLS?
   - Is a static bearer token enough, or is OAuth required?
5. **Writes and elicitation.** Confirm that HTTP writes are off by default and destructive tools are opt-in. Should elicitation also be required where the client supports it?
6. **Distribution.** Release binaries of the fork? Homebrew tap? Which marketplace first?
7. **AGENTS.md commit.** Commit the 260-line AGENTS.md addition on its own before code?
8. **`versions.toml`.** Record go-sdk v1.8.0 and the JS pins there? It is operator-edited only.
9. **G8 hosts.** Which WebMCP hosts must work first: the Chrome origin trial, Kimi desktop or Codex desktop? Can you confirm, or provide documentation showing, that Kimi desktop and Codex desktop expose `document.modelContext`?
10. **G8 delivery.** Is Desktop fork bundling (D1) acceptable as the first delivery path, or must `/webui` in this Kubo fork serve the WebMCP build (D2, a `WebUIPath` change)?
11. **G8 ownership.** Assign the GQAdonis/ipfs-webui fork (`src/webmcp/**`) and `mcp/webmcp/**` to ui-ux-engineer in `team.json`?
12. **G8 upstream.** Is `ipfs/ipfs-webui` an upstream target? Its `AGENTS.md` and release rules would then apply.

---

## APPLIED LESSONS

- **"Never call a CLI that takes a lock from inside an operation that already holds that lock, and never paper over such a failure with a default value."**
  - `ipfs mcp` is `SetDoesNotUseRepo`, never opens the repo and has no offline mode.
  - Tools never shell out to `ipfs`.
  - An unreachable daemon is a tool error, never an empty result.
- **"For every external integration, run at least one test against the real service before claiming done."** Contract tests run against a real `make build` daemon via `test/cli/harness`, especially for `files/*`, `dag import/export` and `repo gc`.
- **"Tests must discover their environment rather than encode one checkout's state."** Tests use an isolated `IPFS_PATH`, non-default ports (AGENTS.md "Use Non-Default Ports") and API discovery via `$IPFS_PATH/api`.
- **"The operator merges every PR... wants operator-only actions asked explicitly"** and **"a single up-front approval question at the phase boundary."** The flip conditions and the eight questions are batched for one decision point. This stage committed nothing and edited nothing besides this file.

## KNOWLEDGE GAPS

No gaps were recorded in prior context. Gaps found in this assessment:

- **MCP spec 2026-07-28 deltas** relative to 2025-11-25: the go-sdk negotiates it (APPENDIX A2), but the spec text was not read [unverified].
- **MCP Apps extension:** the MIME type, `_meta.ui` fields, CSP rules, and whether go-sdk v1.8.0 has helpers [unverified]. Only the go-sdk conformance server mentions `ui://`: `grep -rln "mcp-app\|resourceUri\|ui://"` over the module source matches only `conformance/everything-server/main.go`.
- **A2UI v0.9 catalog format** [unverified].
- **Host support matrix:** MCP Apps, Streamable HTTP and elicitation across Claude Code, Codex, OpenCode and Kimi Code [unverified].
- **Plugin and marketplace manifest formats** for all four targets [unverified]. Kimi Code's format is entirely unknown.
- **Binary cost of the full tool set plus the UI bundle:** unmeasured; only a minimal server was probed.
- **The CLI emitter's stdout behavior** for a never-emitting long-running command [unverified].
- **Upstream maintainer stance:** no signal found.
- **WebMCP browser status:** the Chrome origin trial window, the `navigator.modelContext` deprecation timeline, and Edge/Firefox/Safari positions [unverified; reported by the coordinator, not in the draft text].
- **WebMCP host support:** Kimi desktop, Codex desktop, and Electron 44 / IPFS Desktop's `webui://localhost` scheme (is it a secure context, and does it expose `document.modelContext`?) [unverified].
- **User confirmation in WebMCP:** the draft has no `requestUserInteraction` or equivalent; whether one is planned is [unverified].
- **API-port headers:** whether `Gateway.HTTPHeaders` applies a `Permissions-Policy` header to Web UI content served on the API port [unverified].

---

## APPENDIX: external sources and probe commands

- **A1, go-sdk v1.8.0 dependencies** [external]:
  `curl -sfL https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/go.mod`
  - Output requires `golang-jwt/jwt/v5 v5.3.1`, `google/go-cmp v0.7.0`, `google/jsonschema-go v0.4.3`, `segmentio/encoding v0.5.4`, `yosida95/uritemplate/v3 v3.0.2`, `golang.org/x/oauth2 v0.35.0`, `x/time v0.15.0`, `x/tools v0.42.0`; indirect `segmentio/asm v1.1.3`, `x/sync v0.20.0`, `x/sys v0.41.0`.
  - Presence check against Kubo: `grep -m1 "<module> " go.mod` for each module. Absent: jwt, jsonschema-go, segmentio/encoding, segmentio/asm, uritemplate, go-cmp (go-cmp appears only as old `/go.mod` lines in `go.sum`). Present (indirect): `x/time`, `x/tools`, `x/oauth2`.
- **A2, go-sdk behavior** [external]: the module source, fetched by `go get github.com/modelcontextprotocol/go-sdk@v1.8.0` into `$GOMODCACHE/github.com/modelcontextprotocol/go-sdk@v1.8.0/`.
  - `mcp/shared.go:50-51`: `latestProtocolVersion = protocolVersion20260728 = "2026-07-28"`.
  - `mcp/streamable.go:128-195`: `StreamableHTTPOptions` with `Stateless`, `JSONResponse`, `SessionTimeout`, `DisableLocalhostProtection`, `CrossOriginProtection`, `MaxRequestBodyBytes`.
  - `mcp/streamable.go:317-333`: loopback Host check and cross-origin check.
  - `auth/auth.go:97`: `RequireBearerToken`.
- **A3, size probes** [probe], all `go build -trimpath`, go1.27.1 darwin/arm64, under the scratchpad `sdkprobe/`:
  - `sdk/`: a minimal module importing go-sdk only gives 11,354,226 bytes, against 5,081,298 for a `net/http` hello world (`base/`).
  - `rpc/`: `client/rpc` + go-sdk, with `replace github.com/ipfs/kubo => <repo>`, gives 18,878,162 bytes; `go list -deps ./client/rpc | grep -c .` gives 479.
  - `ipfsdelta/a`: `kubo.Start(kubo.BuildDefaultEnv)` gives 103,257,762 bytes. `ipfsdelta/b`: the same plus a one-tool go-sdk server with stdio and Streamable HTTP gives 105,994,050 bytes.
- **A4, upstream search** [external]:
  - `gh search issues "MCP" --repo ipfs/kubo` returns `[]`.
  - `gh search issues "model context protocol" --repo ipfs/kubo` returns `[]`.
  - `gh search prs "MCP" --repo ipfs/kubo` returns #10940 and #10929, both unrelated.
  - `gh search repos "mcp" --owner ipfs` and `--owner ipshipyard` return `[]`.
  - `gh search repos "ipfs mcp"` returns two unrelated third-party repos with 0 stars.
- **A6, WebMCP** [external]:
  - Fetched with `curl -sfL https://webmachinelearning.github.io/webmcp/`; the header reads "WebMCP Draft Community Group Report, 8 October 2026".
  - IDL quoted in the G8 Facts above: `Document.modelContext` with `[SecureContext]`, the `ModelContextTool` dictionary, `ToolAnnotations` (`readOnlyHint`, `untrustedContentHint`, `consequentialHint`, `debugging`), `ModelContextRegisterToolOptions` (`exposedTo`, `signal`), and §4.4 permissions-policy feature `tools` with default `'self'`.
  - Searching the text: `navigator.modelContext` is absent, and `requestUserInteraction` returns 0 matches.
- **A7, the Web UI and Desktop forks** (read-only):
  - `git -C /Users/gqadonis/Projects/references/ipfs-webui log -1` gives `24ddaa89`; remotes are origin `git@github.com:GQAdonis/ipfs-webui.git` and upstream `https://github.com/ipfs/ipfs-webui.git`.
  - `git -C /Users/gqadonis/Projects/references/ipfs-desktop log -1` gives `9bd9904`; origin is `tribe-health/ipfs-desktop`.
- **A5, go-ipfs-cmds** v0.16.1 (`go list -m -f '{{.Dir}}' github.com/ipfs/go-ipfs-cmds`): `http/parse.go:55` (`NoRemote` returns `ErrNotFound`); `cli/parse.go:233, 279` (stdin read only for `SupportsStdin` arguments).

ASSESSMENT COMPLETE

## Unresolved review findings

Adversarial review (artifact mode, judge `gpt-6.1-sol`, producer `claude-opus-5-5`) returned BLOCK in round 1 and round 2. Round 2 is the last vet round, so this assessment is accepted with the CRITICAL finding below carried forward verbatim. Round 1 findings: `review/assess/findings.round1.json`; round 2: `review/assess/findings.json`.

### CRITICAL (round 2), verbatim

> **Claim:** The assessment bases the G1 placement decision on unsupported claims about a nested module being impossible or blocked, which can drive the plan to the wrong architecture.
>
> **Evidence:** The artifact claims: "A nested module cannot be linked into `ipfs` cleanly" and "The require resolves either through a published version or through a root `replace ... => ./mcp/server`, which the no-external-replace check flags" and later rejects B because "`go install` is blocked by the fork module path and the self-replace pattern." The provided file tree only shows existing root and submodule paths, while the constraints only forbid committed external replace directives and allow existing self-replaces; nothing in the packet proves a new nested module could not be given its own import path, released separately, or kept out of the root binary. This unsupported assertion is central to rejecting B/F2 and choosing F4.
>
> **Suggested fix:** Revise the assessment to separate proven repo facts from speculative distribution concerns, and evaluate B without claiming it is technically blocked unless the exact proposed module path, go.mod contents, and failing command output are provided.

### Lead adjudication (orchestrator, not the producer)

- Partly upheld. The "flagged" claim rests on `.kbd-orchestrator/constraints.md` `no-external-replace-directive`, a check written during `/kbd-init` whose intent is external modules (boxo); its grep would also flag a new in-repo self-module replace, which is a defect in the check, not a fact about Go. A separate binary under its own module path (for example `github.com/GQAdonis/kubo-mcp`) is installable with `go install`. B is not technically blocked.
- Not upheld as decisive. F4 was chosen primarily because the operator asked for "mcp server support to the kubo ipfs cli"; B misses that goal regardless of module mechanics.
- Required of the plan stage: (1) re-state the B-vs-F4 rationale on the operator goal and dependency cost only, with the module mechanics demoted to a cost note; (2) ~~fix the `no-external-replace-directive` check~~ done 2026-10-09 by the orchestrator: it now allows in-repo `github.com/ipfs/kubo(/...)` replaces to relative paths and still flags external ones; (3) put the placement decision and its flip conditions to the operator before materializing ADR-0001.

### WARNINGS (round 2), carried to the handoff

- G7 (agent-team execution with independent review) lacks a concrete gap analysis.
- Untracked team configuration is presented as execution-relevant evidence; it is local, uncommitted state.
- Baseline claims about tracked changes are not supported by the review packet.
- The explanation of the round-1 `mcp/README.md` false positive asserts a cause (basename grep) the packet does not prove.
