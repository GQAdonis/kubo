# ADR-0001: The MCP server ships as `ipfs mcp`, a daemon RPC client inside the root module

## Status

Accepted, 2026-10-09.

The operator approved this decision (F4) in the Round 0 answers recorded in `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`, section "Round 0 answers (team-ownership-grant 1.1, 2026-10-09)": "**Q1 (F4) and R1 (E6): approved.** MCP ships as `ipfs mcp` in the fork's binary (RPC-only to a running daemon; stdio plus Streamable HTTP, AG-UI and A2A on `127.0.0.1:5050`)." The lead adjudication of the assessment review required this approval before the ADR could be marked Accepted (`.kbd-orchestrator/phases/ipfs-mcp-server/assessment.md`, "Lead adjudication").

The decision is also recorded in `versions.toml` (`[decisions] mcp-placement`, `mcp-root-deps`, `mcp-stock-kubo`).

## Date

2026-10-09

## Context

### Operator goals

- MCP server support in the Kubo `ipfs` CLI ("mcp server support to the kubo ipfs cli").
- stdio for local agents and Streamable HTTP for remote use.
- An MCP App UI, and skills and plugins for agent hosts.
- Fork-only for now; upstream conversation after a working result (`operator-answers.md` answer 1).

### Hard constraints

From the committed `AGENTS.md` Stability and User Agency sections, which outrank this ADR:

- never break `/api/v0`, the HTTP gateway, the default CID recipe or wire compatibility;
- no tracking;
- no silent CID injection;
- every feature has an off switch;
- every hardcoded endpoint is configurable and can be turned off;
- a new module in `go.mod` gets the same scrutiny as new code.

### Codebase facts

All read on 2026-10-09 against HEAD `833e04a24` (`assessment.md`, "Facts that constrain every option").

- One command tree serves the CLI and `/api/v0` (`core/commands/root.go`). Commands in `localCommands` (`cmd/ipfs/kubo/ipfs.go:20-26`) run locally and are never served over RPC.
- Running a repo-using command locally constructs the node through `fsrepo.Open`, which takes `repo.lock` (`repo/fsrepo/fsrepo.go:150-166`). A long-lived MCP process that opened the repo would block `ipfs daemon` for the whole agent session.
- `client/rpc` covers most operations, and `HttpApi.Request` covers the rest (`client/rpc/api.go:206`).
- `API.Authorizations` wraps only the cmds handler (`core/corehttp/commands.go:144-147`). Anything mounted elsewhere on the RPC port bypasses it.
- Build tags already remove optional subsystems: `GOTAGS` is wired into the Makefile (`mk/golang.mk:7, 33-34`), and `nofuse` stubs out FUSE (`core/commands/mount_nofuse.go`).
- Dependency weight [probe, `assessment.md` APPENDIX A1 and A3]: go-sdk v1.8.0 adds five modules absent from Kubo's `go.mod` (`golang-jwt/jwt/v5`, `google/jsonschema-go`, `segmentio/encoding`, `segmentio/asm`, `yosida95/uritemplate/v3`). A minimal go-sdk server linked into `ipfs` grew it from 103,257,762 to 105,994,050 bytes (+2.74 MB, +2.7%). A standalone `client/rpc` plus go-sdk binary was 18,878,162 bytes.
- Upstream signal [external, 2026-10-09]: `gh search issues "MCP" --repo ipfs/kubo` and `gh search issues "model context protocol" --repo ipfs/kubo` return `[]`. Maintainer appetite is unknown.

## Decision

F4: MCP support is a CLI-only local command in the fork's `ipfs` binary.

- **Code location.** Package `github.com/ipfs/kubo/mcp/server`, inside the root module. There is no nested `go.mod`.
- **Entry point.** `ipfs mcp`, registered in `localCommands` in `cmd/ipfs/kubo`, so it is never part of `/api/v0`. It is marked `SetDoesNotUseRepo` and never takes `repo.lock`.
- **Connection to Kubo.** It always connects to a running daemon over `/api/v0` through `client/rpc`. The endpoint comes from `--api`, `$IPFS_PATH/api` or an explicit URL; credentials come from `--api-auth`. Nothing is hardcoded. An unreachable daemon is an explicit tool error. There is no offline mode.
- **Stock Kubo daemons.** There is no second binary. The fork's `ipfs` binary serves MCP against any compatible running daemon, stock or forked, including the one IPFS Desktop runs. It checks the minimum daemon version through `/api/v0/version` at startup and fails with a clear message below it, and it drops individual tools whose RPC commands the daemon lacks instead of failing the whole server (`operator-answers.md`, "Orchestrator recommendation for question 3"; `versions.toml` `mcp-stock-kubo`).
- **Transports.** stdio by default. Streamable HTTP is opt-in in the same process with `--http`, default listen address `127.0.0.1:5050` (operator, 2026-10-09; never 4000, 4001, 5001 or 8080). The go-sdk loopback Host check stays on, cross-origin protection uses an empty allowlist by default, and a request body limit is set. A non-loopback bind is refused unless an authentication verifier is configured. Authentication and confirmation are specified in ADR-0003.
- **Agentic surfaces.** AG-UI and A2A are served by the same `--http` listener (ADR-0004). They are off by default and compiled out by the `noagent` build tag.
- **Capability switches.** `ipfs mcp` flags and an optional `ipfs mcp` config file named with `--mcp-config=<path>` or `IPFS_MCP_CONFIG` (schema in `mcp/contract/config/`); the root `--config` flag keeps its Kubo meaning, the repo config file. No Kubo config key (`config/`, `docs/config.md`) is added. Tool groups can be disabled individually, and `--read-only` exposes only tools whose confirmation class is `none`. The default for destructive tools is set in ADR-0003.
- **Build-time removal.** `GOTAGS=nomcp` compiles `ipfs mcp` out, following the `mount_nofuse.go` stub pattern. The modules stay listed in `go.mod`.
- **UI.** `mcp/app` and `mcp/ui` build with their own JavaScript toolchain. The built bundle is embedded with `go:embed` and served only as `ui://` MCP resources over the MCP channel. There is no new HTTP route on Kubo's RPC or gateway ports and no CORS change on the RPC port.
- **No daemon endpoint in this phase.** The daemon, `/api/v0`, the gateway, the default CID recipe, wire behavior, `config/` and `docs/config.md` are unchanged.

### Why F4 and not B (restated per the lead adjudication)

The assessment's adversarial review (round 2, CRITICAL, partly upheld) found that the assessment overstated the module mechanics: a nested module is not technically blocked. A separate binary under its own module path (for example `github.com/GQAdonis/kubo-mcp`) can be installed with `go install`. The `no-external-replace-directive` check that would have flagged an in-repo self-replace was a defect in the check and was fixed on 2026-10-09 (`assessment.md`, "Lead adjudication", item 2).

The B-versus-F4 choice therefore rests on two arguments only:

- **Operator goal.** The operator asked for MCP support in the `ipfs` CLI. B ships a separate program and misses that goal regardless of module mechanics.
- **Dependency cost.** B's real advantage is that go-sdk stays out of the root `go.mod` and out of every default `ipfs` build. The operator accepted the root dependencies (`operator-answers.md` answer 2; `versions.toml` `mcp-root-deps`), and `nomcp` lets a builder compile the code out.

Module mechanics are a cost note, not a deciding fact: adding an `ipfs mcp` entry point to B later means either an exec-dispatch launcher (F3) or moving the package into the root module (F4) anyway.

## Consequences

### Positive

- One binary. Plugin manifests launch `{"command": "ipfs", "args": ["mcp"]}`.
- Users of stock Kubo, including IPFS Desktop's daemon, can use it by installing the fork's `ipfs` binary as a client of their existing node.
- No change to `/api/v0`, the gateway, CID defaults, wire behavior, daemon config or `docs/config.md`.
- Repo-lock contention cannot occur, because `ipfs mcp` never opens the repo.
- One authenticated path into Kubo: RPC calls go through `API.Authorizations` like any other RPC client. Docs and tests cover a dedicated secret whose `AllowedPaths` list only the enabled tools' commands.
- Builders can remove MCP with `nomcp`, and the agentic surfaces with `noagent`.

### Negative

- go-sdk and five new modules join the root `go.mod` and every default build: at least +2.74 MB before tool code, UI bundles and the agentic code of ADR-0004. The full delta is unmeasured.
- A committed JavaScript build artifact lives in the repo and must be checked by a rebuild-and-diff step.
- Remote use needs a second process (`ipfs mcp --http`) beside the daemon. An operator who expects `ipfs daemon` to serve MCP will find that it does not.
- The feature exists only in this fork's `ipfs` binary until it is upstreamed.
- MFS, CAR and GC output shapes are untyped in `client/rpc` and must be pinned by real-daemon contract tests.
- Edits to `cmd/ipfs/kubo` need an architect-owned OpenSpec change, because `cmd/` has no owning role.

### Follow-ups

- An optional daemon-hosted endpoint (C1) only as its own OpenSpec change, if the operator later requires a single process for remote use.
- An upstream proposal as an `ipfs/kubo` issue, after a working result (operator answer 1).

## Rejected alternatives

| Alternative | Rejected because (evidence) |
|---|---|
| B: separate binary in a nested module over `client/rpc` | Misses the operator goal of MCP in the `ipfs` CLI. Its advantage is dependency cost only, which the operator accepted (answer 2). It also needs release tooling of its own. Not technically blocked: a separate module path is installable with `go install` (adjudication). |
| C1: daemon listener (`Addresses.MCP`) | Adds a daemon feature: new keys in `config/addresses.go`, a `docs/config.md` entry, a changelog entry and disabled-state tests. It puts a second admin-auth system inside the daemon process. Remote HTTP is achievable without it. Kept as a conditional follow-up. |
| C2: mount on the RPC mux | Bypasses `API.Authorizations`, which wraps only the cmds handler (`core/corehttp/commands.go:144-147`), and widens the admin port. |
| Gateway mount | The gateway is a spec-defined, often public interface (`AGENTS.md` Stability). |
| D: Kubo plugin | Same root-dependency and in-daemon blast radius as C1. External `.so` plugins are tied to the exact toolchain (`docs/plugins.md:133-175`). No stdio path. |
| E: out-of-tree repository | Misses the `ipfs` CLI goal and loses in-tree contract tests against a fork-built daemon. The stock-Kubo case it would serve is covered by the fork binary acting as an RPC client (operator answer 3 recommendation). Remains the fallback if the operator later rejects root dependencies. |
| F1: daemon HTTP plus a stdio bridge | Pays the costs of A and C1 together. Wins only if one daemon process must also be the remote endpoint, which the operator did not require. |
| F2: B now, `ipfs mcp` later | Defers the root-module decision instead of avoiding it, and misses the CLI goal in this phase. |
| F3: `ipfs mcp` execs `ipfs-mcp` from `PATH` | Adds PATH-lookup exec behavior to `ipfs` (a binary-planting surface), two artifacts to version in lockstep, and two installs. |
| Offline mode (open the repo in `ipfs mcp`) | Holds `repo.lock` for the session (`repo/fsrepo/fsrepo.go:150-166`), blocking `ipfs daemon`, and bypasses `API.Authorizations`. Refused regardless of operator preference. |
| Boxo placement | An MCP server is a product surface over Kubo's own RPC, not a reusable primitive. A CoreAPI adapter would import `kubo/core/coreiface`, which boxo cannot import (`AGENTS.md`, "Where a change belongs"). |

## Flip conditions

| Condition | Status on 2026-10-09 | Decision if it occurs |
|---|---|---|
| No new modules in the root `go.mod`, or the `ipfs` binary must stay unchanged | Not triggered: operator answer 2 accepts go-sdk in the root module | B, or E if `go install`/`npx` distribution becomes the priority |
| Users of stock Kubo must be supported | Satisfied without a flip: the fork binary is an RPC client of any compatible daemon (answer 3 recommendation) | No flip |
| The daemon itself must serve the remote endpoint | Not requested | F1: add C1 as its own OpenSpec change; the F4 stdio path stays |
| Upstream maintainers ask for a separate tool | Not asked; fork-only (answer 1) | E, as a package move |
| Offline use without a daemon is required | Not requested | No flip: refuse, and document starting the daemon |

## Where this is weak

- **Dependency cost is real.** It is the strongest objection to F4 and the reason B scored higher on compatibility and maintenance (`assessment.md`, Scores). `nomcp` removes the code from a build, but `go.mod` still lists the modules, and `segmentio/asm` ships assembly. ADR-0004 adds an LLM client and an agent loop to the same binary, so the binary grows beyond the probed +2.74 MB, by an amount not yet measured.
- **The assessment's own review did not pass.** It returned BLOCK in both rounds. The round-2 CRITICAL was partly upheld, and this ADR's rationale was rewritten around the operator goal as a result. The decision stands on the operator's goal and approval, not on a clean review.
- **Two processes for remote use.** This is a usability cost the scores do not fully capture.
- **Fork-only distribution.** Stock-Kubo users must install the fork's `ipfs` binary even though they keep their daemon. The minimum-version detection and per-tool degradation are designed, not yet tested.
- **stdout hygiene is unverified.** Whether the go-ipfs-cmds CLI emitter writes anything to stdout for a command that never emits a value is unknown (`assessment.md`, fact 4). A `test/cli` test that runs a full MCP initialize over stdio and asserts a byte-exact JSON-RPC stream is required (`mcp-server-core-stdio`).
- **Upstream appetite is unknown.** No signal either way was found.

## Falsifiers

| Assumption | Observable falsifier | Consequence |
|---|---|---|
| `ipfs mcp` never writes non-protocol bytes to stdout | The stdio `test/cli` test finds a byte on stdout that does not parse as JSON-RPC | Fix before release; block `mcp-server-core-stdio` |
| `ipfs mcp` never takes `repo.lock` | `ipfs daemon` fails to start while `ipfs mcp` runs against the same `IPFS_PATH` | Block |
| The binary cost stays acceptable | The full build (tools, UI, agentic code) measured by `mcp-release-binaries` exceeds what the operator accepts | Revisit with the operator: `noagent` builds as the default release, or the B flip |

## References

- `.kbd-orchestrator/phases/ipfs-mcp-server/assessment.md`: ARCHITECTURE OPTIONS, Scores, DECISION, ADR DRAFT, Unresolved review findings and Lead adjudication
- `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`: answers 1-8, the question 3 recommendation, and the Round 0 answers
- `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`: section 1 (DECISIONS) and D7 (ports)
- `versions.toml`: `[decisions]`
- ADR-0003 (confirmation and authentication), ADR-0004 (agentic endpoints and LLM)
