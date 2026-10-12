# Progress report: KBD phase `ipfs-mcp-server`

Last updated: 2026-10-11

This document records what the `ipfs-mcp-server` phase has delivered in this Kubo fork, why each decision was made, what is verified, and what is still open. It is a summary for readers who were not in the sessions. The detailed, append-only log stays in [`execution.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/execution.md); the plan is [`plan.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/plan.md).

How it is maintained: the `ipfs-agent-architect` role updates this file at the end of every execution round and at the phase boundary, and adds one dated line to the change log at the bottom. Status figures come from the canonical KBD runtime (`prometheus kbd --path . status --json`), not from task checkboxes.

Position on 2026-10-11: stage Execute, rounds R0-R10 dispatched, 21 of 35 changes complete in the runtime. Round 11 (phase-boundary review) is in progress: the reviews and the judge ran, the batched correction pass is applied, and re-verification is pending (section 5.1). All finished work is committed and pushed to branch `feat/ipfs-mcp-server` in GQAdonis/kubo, GQAdonis/ipfs-webui and tribe-health/ipfs-desktop (operator request, 2026-10-10).

## 1. Goals

The goals are in [`goals.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/goals.md). Status in one line each:

| Goal | Summary | Status on 2026-10-10 |
|---|---|---|
| G1 | Choose where MCP lives, by assessment with adversarial review; record it as an ADR | Met: [ADR-0001](0001-mcp-server-placement.md), operator-approved |
| G2 | MCP tools for add/get/cat, pins, IPNS and keys, MFS, DAG, CAR, routing, node status, from one contract | Met: 41 tools in [`mcp/contract/catalog.json`](../../mcp/contract/catalog.json) |
| G3 | stdio and Streamable HTTP, auth, loopback default, Origin protection, off switch per capability | Met in code and tests (full CLI suite re-run on the final tree in R11, ok); R11 review findings fixed in one batched pass, see [section 5.1](#51-r11-phase-boundary-review) |
| G4 | React 19 component library that is an A2UI v0.9 catalog, `ui://` resources, WCAG 2.2 AA | Built; real-host render, visual baselines and real-browser axe pending |
| G5 | agentskills.io skills, packaged as plugins and marketplaces | 8 skills; plugins for Claude Code, Codex, OpenCode; logged-in harness run pending |
| G6 | No break to `/api/v0`, gateway, CID recipe, wire; no tracking; no silent CID injection; gated writes | Held by design; the R11 reviews found no change to `/api/v0`, the gateway, the CID recipe or wire behavior |
| G7 | Team execution with independent security and quality review | Team execution done; R11 reviews and the cross-family judge ran, one batched correction pass applied and re-verified (race, CLI suite, Playwright, lint) ([section 5.1](#51-r11-phase-boundary-review)) |
| G8 | WebMCP tools in the IPFS Web UI | Not delivered: blocked on the operator's host probes |
| G9 | AG-UI and A2A endpoints | Met through `ipfs mcp` (topology A); UAR-hosted topology B deferred |
| G10 | LLM backend behind the agentic endpoints, no default endpoint | Met for an OpenAI-compatible server (verified with local Ollama); UAR leg not run |
| G11 | Chat sidebar in IPFS Desktop and the Web UI | Island and Web UI integration built; Desktop pairing not built; no live operator run |
| G12 | Optional embeddings into SurrealDB, embedded and remote | Met, with live `bge-m3` runs |

Operator scope changes:

- 2026-10-09, G5: plugin packaging covers Claude Code, Codex and OpenCode only. Kimi Code plugin packaging is out of scope; Kimi Code still reads the skills directly.
- 2026-10-09, G7: the phase-boundary review has two blocking parts: workspace review by the `security-reviewer` and `reviewer` subagents, and a cross-model-family adversarial judge.
- 2026-10-09: G9-G12 were added during planning. The assessment was extended by [`assessment-addendum.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/assessment-addendum.md) and the plan was redone as plan revision 2 (35 changes, rounds R0-R11).

## 2. Key decisions and why

### 2.1 Placement: `ipfs mcp` (F4)

MCP ships as a local command, `ipfs mcp`, in this fork's `ipfs` binary. The code is the package `github.com/ipfs/kubo/mcp/server` in the root module. The command is registered in `localCommands`, so it is never part of `/api/v0`. It never opens the repo and never takes `repo.lock`. It reaches the node only through a running daemon's `/api/v0` RPC API via `client/rpc`.

Why:

- The operator asked for MCP support in the `ipfs` CLI.
- Opening the repo would hold `repo.lock` for the whole agent session and block `ipfs daemon`. It would also bypass `API.Authorizations`. An offline mode is refused for that reason.
- A daemon-hosted endpoint would add Kubo config keys and a second admin-auth system inside the daemon. Mounting on the RPC mux would bypass `API.Authorizations`, which wraps only the commands handler.
- The fork's binary can act as an MCP client of any compatible daemon, stock or forked, including the daemon IPFS Desktop runs. So stock-Kubo users need no second artifact. The server checks the minimum daemon version (Kubo 0.29.0) at startup and drops individual tools whose RPC commands the daemon lacks.

Rejected alternatives (full table in [ADR-0001](0001-mcp-server-placement.md)): a separate binary in a nested module (misses the CLI goal; its only gain is dependency cost, which the operator accepted); a daemon listener; a mount on the RPC mux; a gateway mount; a Kubo plugin; an out-of-tree repository; daemon HTTP plus a stdio bridge; "separate binary now, `ipfs mcp` later"; `ipfs mcp` exec-ing a second binary from `PATH`; an offline repo-opening mode; placement in boxo.

The cost is real: go-sdk v1.8.0 and four new modules (`google/jsonschema-go`, `segmentio/asm`, `segmentio/encoding`, `yosida95/uritemplate/v3`) join the root `go.mod`. A minimal probe measured +2.74 MB on the binary. The full delta with tools, UI bundles and agentic code has not been measured.

### 2.2 Build tags

- `GOTAGS=nomcp` compiles `ipfs mcp` out entirely, following the existing `nofuse` stub pattern.
- `GOTAGS=noagent` keeps MCP and compiles out the agent loop, AG-UI and A2A.

Both builds compile (R9 evidence). The modules stay listed in `go.mod` either way.

### 2.3 How the Stability and User Agency rules shaped the design

- **No `/api/v0` change.** No command, argument, default or response shape changed. No Kubo config key was added; `docs/config.md` and `config/` are unchanged (checked with `git diff --stat docs/config.md config/` in R10: empty). `ipfs mcp` has its own config file, named by `--mcp-config` or `IPFS_MCP_CONFIG`; the root `--config` flag keeps its Kubo meaning.
- **No gateway, CID or wire change.** `ipfs_add` uses the default recipe, and tests check its CIDs equal `ipfs add`.
- **Off by default, with an off switch for everything.** Streamable HTTP, AG-UI, A2A, the LLM backend, embeddings and MCP App views are each off unless configured, and each has a documented off switch in [config-reference.md](config-reference.md). Destructive tools are off by default.
- **No hardcoded endpoints.** There is no default LLM URL, model, embedder or SurrealDB URL. The HTTP listener defaults to `127.0.0.1:5050` (never 4000); the config schema rejects ports of other local services. Credentials are `{"env": ...}` or `{"file": ...}`, never literals.
- **No silent CID injection.** Only explicit tool calls add content. Embedding is opt-in. Deleting vectors is pin-aware. A daemon-wide hook was refused.
- **Confirmation gate.** Every write and destructive call needs human confirmation on every transport: MCP elicitation where the client supports it, otherwise a native OS prompt with a presence factor that a background process cannot click. Where no such prompt exists, non-eliciting writes are refused. A confirm token is a binding handle, never consent.
- **No tracking.** Nothing reports usage anywhere. Content leaves the machine only to model or embedder hosts the operator lists in a principal's `egress_ack`.

### 2.4 ADRs

- [ADR-0001](0001-mcp-server-placement.md): `ipfs mcp` is a daemon RPC client in the root module (F4).
- [ADR-0002](0002-webmcp-web-ui.md): WebMCP tools in the Web UI. Status Deferred (2026-10-12, operator decision) until the WebMCP host probe yields a spike report with GO for a host.
- [ADR-0003](0003-confirmation-and-auth.md): confirmation of writes and destructive calls; replaceable auth verifier (static bearer now, OAuth2 later).
- [ADR-0004](0004-agentic-endpoints-and-llm.md): AG-UI and A2A served by `ipfs mcp`; model calls proxied to an operator-configured endpoint; human-presence prompt for non-eliciting writes.
- [ADR-0005](0005-agent-chat-ui.md): the chat is a React 19 shadow-root island in the Web UI fork that talks only to `ipfs mcp`; the Desktop fork is hardened.
- [ADR-0006](0006-embeddings-surrealdb.md): `ipfs mcp` embeds and stores vectors in SurrealDB, embedded through a supervised operator-supplied `surreal` process or remote by URL.
- [ADR-0007](0007-uar-preflight-and-grant-limits.md): `check-uar` reads a contract-pinned settings shape; grant limits under `--confirm=cli`; MCP App view off switch.
- [ADR-0008](0008-topology-b-gate-no-go.md): the UAR-hosted topology B gate is NO-GO; topology B deferred, topology A unaffected.

### 2.5 Operator decisions (dated)

Binding answers are in [`operator-answers.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md) and [`.prometheus/decisions.md`](../../.prometheus/decisions.md).

- 2026-10-09: fork-only; talk to upstream after a working result. go-sdk in the root `go.mod` accepted.
- 2026-10-09: HTTP can do everything stdio can, including writes and destructive tools; every such call requires confirmation. Static bearer token now, designed to move to Ory Kratos and Hydra later.
- 2026-10-09: distribution as release binaries plus marketplaces for Claude Code, Codex and OpenCode. Kimi Code plugin packaging is out of scope this phase.
- 2026-10-09: WebMCP hosts Codex desktop and Kimi desktop first, gated on a host spike.
- 2026-10-09: agentic architecture "Both": AG-UI and A2A in `ipfs mcp` now; UAR-hosted as a second topology once UAR ships conformance fixes in a published release.
- 2026-10-09: human gate is a native OS prompt; CLI confirm only for headless servers behind `--confirm=cli`.
- 2026-10-09 (Round 0): F4 and E6 approved; SurrealDB through the standard-library HTTP client (no SurrealDB Go module); fail-closed human gate; Desktop repository confirmed; local models `qwen3:8b` and `bge-m3` for real-service tests.
- 2026-10-09 (R1): destructive tools off by default, enabled explicitly, always confirmed.
- 2026-10-09 (R6): topology B NO-GO (ADR-0008); no published UAR release contains the conformance items.
- 2026-10-10: OQ-EGRESS-ENUM closed. `swarm_peers`, `routing_*` and `key_list` stay `contentEgress: false`. Consequence: a principal granted those groups can send peer lists, routing answers and key names to a model backend without `egress_ack`; operators withhold those groups to prevent it.
- 2026-10-10: the Web UI chat token is stored in browser storage (`localStorage`) instead of memory only. Trade-off: any script in the Web UI origin can read it. The token authenticates only the `ui` principal, and writes still need confirmation.
- 2026-10-10: release before gated items. The operator overrode the plan order and ran R9 (release) and R10 (docs, plugins) before the operator-gated items. The release notes and docs state what is not yet delivered. "Release" means local archives only; pushing, tagging and publishing need separate approval.
- 2026-10-10: the 11 orphaned `llama-server` processes from earlier Ollama test runs were stopped by PID on operator instruction.
- 2026-10-11: decisions on the six open R11 items (section 5.1): the same-user pairing verifier forgery is accepted as residual risk; the chat token stays in `localStorage`; `confirmation.resolvers`, `confirmation.cli` and `confirmation.ttl` are to be implemented; `SHA256SUMS` signing awaits the operator's answer (SSH signing recommended); CI is added under `.github/` as one new workflow; the Desktop `openExternal` path stays as upstream has it.
- 2026-10-11 (R12): the `cli` resolver never approves confirmations from the stdio local principal (security review SEC-M1); fix directed by the lead, recorded in ADR-0003. `confirmation.ttl` minimum raised to 1 s.

## 3. What was delivered, by area

Evidence below is what was recorded in `execution.md` and the change task files. "Agent-reported" means the owning agent ran the check and the orchestrator did not re-run it.

### 3.1 Tool contract

- What: the single source of truth for every tool. 41 tools (22 read, 14 write, 5 destructive), JSON Schema 2020-12 inputs and outputs, MCP annotations, confirmation classes, `contentEgress` and untrusted-content flags, required RPC paths, resource URIs, AG-UI and A2A fixtures, derivation rules for WebMCP and HTTP scopes, and the `ipfs mcp` config schema in which every key declares a default and an off switch.
- Where: [`mcp/contract/`](../../mcp/contract/), described in [`mcp/README.md`](../../mcp/README.md).
- Verified: `go test ./mcp/contract/...` validates every schema against the vendored meta-schema, the cross-field rules, fixtures and config annotations. Consumers (server, UI, skills, docs) check against it.
- Why it matters: one contract keeps the Go server, the TypeScript UI, the skills and the docs from drifting.

### 3.2 MCP server: tools and transports

- What: `ipfs mcp` with tool groups content, pin, names (IPNS and keys), MFS, DAG and CAR, routing and diagnostics, node status, and index. `ipfs://` and `ipns://` resources. stdio by default; Streamable HTTP with `--http` on `127.0.0.1:5050`, loopback Host check, empty cross-origin allowlist, body limit, and refusal of a non-loopback bind without an auth verifier. An unauthenticated loopback listener serves a read-only `anonymous-loopback` principal. Principals (`human-mcp`, `ui`, `a2a`, `uar`) are read-only by default and get write or destructive groups by name.
- Where: [`mcp/server/`](../../mcp/server/), [`cmd/ipfs/kubo/mcp*.go`](../../cmd/ipfs/kubo/), tests in `test/cli/mcp_*_test.go`.
- Verified: `go test ./mcp/...` and `go test ./test/cli -run TestMCP` against real daemons on non-default ports, re-run at several rounds (latest recorded R7: `TestMCP` with Ollama skipped, 53.7 s, ok; `make build` ok; `go vet` including `nomcp` and `noagent`). 8 real-daemon HTTP subtests pass. R11 re-run on the final tree: the `TestMCP` CLI suite (Ollama skipped) ok in 55.5 s, and the race-detector run over `./mcp/...` and `./cmd/ipfs/kubo/` with 0 races.
- Not verified: elicitation from clients on protocols older than 2026-07-28; the full binary size; behavior against a daemon below the minimum version.

### 3.3 Confirmation

- What: the gate, a pending-confirmation broker (expiry is denial, exactly-once execution), multi-round-trip elicitation, signed handles with distinct `use` claims, and native presence prompts: macOS LocalAuthentication through an embedded Swift script (no cgo), a Linux polkit action, and a Windows refusal unless paired with Desktop. `ipfs mcp confirm` exists only under `--confirm=cli`. The out-of-band gate is on by default (`native,desktop`); `IPFS_MCP_CONFIRM=none` turns it off, after which only eliciting clients can confirm.
- Where: [`mcp/server/gate.go`](../../mcp/server/gate.go), [`broker.go`](../../mcp/server/broker.go), [`elicit.go`](../../mcp/server/elicit.go), [`mcp/server/confirm/`](../../mcp/server/confirm/).
- A CRITICAL defect was found and fixed in R3. The elicitation request state and the confirm token were minted with the same key and claims, so a non-eliciting client could replay its token as a request state with a forged accept and approve its own write. The fix added distinct `use` claims, per-use derived HMAC keys, mechanism binding and an acceptance order. `handles_test.go` (9 tests) passes, re-run by the orchestrator. The R11 security review must re-test this path.
- Verified: unit tests and real-daemon tests; the automated HG-1 IPC test passes.
- Not verified: the real native prompt with a human (HG-1, task 1.5, operator). Residual by design: a client that advertises elicitation can answer its own prompt.

### 3.4 Agent loop and LLM proxy

- What: a bounded loop (step, token and wall-clock limits) over the caller's enabled contract tools only, with no shell, filesystem or network tools. It calls an operator-configured OpenAI-compatible chat-completions endpoint. There is no default URL or model, a literal API key fails validation, and with nothing configured no agent surface is mounted.
- Where: [`mcp/server/agent.go`](../../mcp/server/agent.go), [`llm_client.go`](../../mcp/server/llm_client.go), [agentic.md](agentic.md).
- Verified: G10-1 with local Ollama: `qwen3:8b` 5/5, `llama3.2:1b` 3/6.
- Not verified: the UAR backend leg (deferred with topology B).

### 3.5 AG-UI

- What: `/agui` takes `RunAgentInput` and streams spec events over SSE. A pending confirmation ends the run with an interrupt; one interrupt per run; resume answers it; an interrupt resolution is owner-checked. Browser origins need `agent.origin_allowlist`.
- Where: [`mcp/server/agui*.go`](../../mcp/server/), fixtures in [`mcp/contract/fixtures/agui/`](../../mcp/contract/fixtures/).
- Verified (agent-reported, R7): all 11 fixtures replay against the real handler and validate against `@ag-ui/core` 1.0.2; `TestMCPAGUI` 8 subtests plus a live `qwen3:8b` run.

### 3.6 A2A

- What: A2A 1.0 agent card at `/.well-known/agent-card.json` and JSON-RPC (`message/send`, `message/stream`, `tasks/get`, `tasks/cancel`). Pending confirmations become `TASK_STATE_INPUT_REQUIRED`; no A2A message can approve a write; destructive calls from `a2a` principals need a presence resolver (`cli` alone fails closed). A context whose loop session was evicted is rejected, not silently restarted.
- Where: [`mcp/server/a2a*.go`](../../mcp/server/), fixtures in [`mcp/contract/fixtures/a2a/`](../../mcp/contract/fixtures/).
- Verified (agent-reported, R7): `go test ./mcp/...` including `noagent`; A2A and AG-UI CLI tests ok.

### 3.7 Embeddings and index lifecycle

- What: embedded mode supervises an operator-supplied `surreal` binary on an ephemeral loopback port with `--deny-guests`, per-start credential rotation and a watchdog; remote mode uses an operator URL. The client is the standard library over SurrealDB's HTTP API (`surrealdb.go` was declined: +5.3 MB, 4 modules). The embedder is an operator-configured endpoint checked against `egress_ack`. Indexing is opt-in (`embeddings.auto` off) with a MIME allowlist and size cap. Tools `ipfs_index_search`, `ipfs_index_cid`, `ipfs_index_status`, `ipfs_index_reconcile`. Vectors are deleted only when no pin of any type remains.
- Where: [`mcp/server/embeddings/`](../../mcp/server/embeddings/), [`index_lifecycle.go`](../../mcp/server/index_lifecycle.go), [embeddings.md](embeddings.md).
- Verified: R7 live `bge-m3` run; process list identical before and after the tests. The index lifecycle change is closed in the runtime; `execution.md` records no observed output for it, so its test evidence (`TestMCPEmbeddingIndex`) is agent-reported.
- Not verified: remote SurrealDB against a separately operated server outside the tests; SurrealDB is BSL 1.1 and is accepted for self-hosted use only.

### 3.8 UI component library and MCP App views

- What: a React 19 component library that is also an A2UI catalog (catalog id is a URI, `ipfs-mcp` is an alias), with chat primitives and bundled fonts registered as `IPFS UI *`. Five single-file MCP App views served as `ui://ipfs-mcp/<view>` resources with a CSP that names no remote origin; off switch `tools.app_views` / `--no-app-views`.
- Where: [`mcp/ui/`](../../mcp/ui/), [`mcp/app/`](../../mcp/app/).
- Verified (agent-reported): `mcp/ui` 247 tests, axe WCAG 2.2 AA in dark and light, no external URLs; `mcp/app` 185 tests, deterministic rebuild check, `go test ./mcp/app` ok.
- Not verified: rendering in a real host (task 1.4, operator); real-browser checks ran in R11 (`mcp/ui` 90/90, `mcp/app` 104/104, axe included; darwin baselines committed). Each view is about 750 KB (about 305 KB gzip, about 200 KB fonts).

### 3.9 Chat island

- What: `<ipfs-agent-chat>`, a shadow-root React 19 + Tailwind 4 + shadcn sliding sidebar that talks only to `ipfs mcp` `/agui`, renders A2UI results and needs a trusted user gesture to answer an interrupt.
- Where: [`mcp/chat/`](../../mcp/chat/).
- Verified (agent-reported): 189 tests, 11 fixtures replay, closed bundle 2.3 kB gzip, first open 364 kB gzip including 145 kB fonts.
- Verified in R11: Playwright and Electron e2e 133/133. `@a2ui/react` 0.12.0 injects a head style that the build strips; an upstream report is drafted in [`mcp/chat/EVIDENCE.md`](../../mcp/chat/EVIDENCE.md), not filed.

### 3.10 Web UI fork (GQAdonis/ipfs-webui, branch `feat/ipfs-mcp-server`)

- What changed: `src/agent-chat/**` (launcher, Settings toggle off by default, endpoint and token handling, a loader for the vendored island, a Desktop bridge hook), the vendored island in `public/agent-chat/` with a hash manifest, `public/locales/en/agentChat.json`, and small edits to `src/App.js`, `src/i18n.js`, `src/settings/SettingsPage.js`, `.eslintignore`. The token is stored in `localStorage` under `ipfs-webui:agent-chat:token`; turning chat off or "Forget token" removes it.
- Verified (agent-reported): agent-chat Jest 37/37, eslint and typecheck clean.
- Not verified: a live run of the fork Web UI against `ipfs mcp` (task 1.3, operator). Copy gap: "saved in this browser profile" is wrong when storage is blocked.

### 3.11 Desktop fork (tribe-health/ipfs-desktop, branch `feat/ipfs-mcp-server`)

- What changed: the Web UI window now runs with `webSecurity: true` and a CSP (`src/webui/security.js`); a `safeStorage` token bridge (`src/agent/`); a config migration (revision 9) that adds `webui://localhost` to `API.HTTPHeaders["Access-Control-Allow-Origin"]`, because re-enabled CORS otherwise breaks every RPC call from the Web UI. A pre-existing defect where the sandboxed preload never loaded was found and fixed. Details in `docs/agent/HARDENING.md` in that fork.
- Verified (agent-reported): hardening probe 27/27 and 17/17 with Electron 44.3.0 against a throwaway Kubo on OS-assigned ports; `npm test` 145.
- Not verified: the installed Desktop app was not launched; the real macOS Keychain was not exercised; pairing and the native confirmation window (tasks 1.4-1.6) are not built. The pre-existing `isWebUiUrl` / `open-external` origin bug is documented and unfixed.

### 3.12 Skills

- What: 8 skills in [`mcp/skills/`](../../mcp/skills/): publish content, retrieve and verify, pin management, IPNS names, MFS workspace, DAG and CAR, content search, troubleshooting.
- Verified: `skills-ref validate` clean for all 8; catalog check 41/41 tool names; lexical trigger proxy 20/22 (re-run 2026-10-11; the misses route a CID read to `ipfs-content-search` and a `confirmation_unavailable` question to `ipfs-publish-content`).
- Not verified: the live-model near-miss trigger run. Task 1.4 is unticked in `openspec/changes/ipfs-agent-skills/tasks.md` until it is recorded (R11-Q06); the spec delta now names the 8 skills (R11-Q13).

### 3.13 Plugins and marketplaces

- What: one plugin root [`mcp/plugin/ipfs/`](../../mcp/plugin/) with Claude Code and Codex manifests, the 8 skills, an OpenCode plugin module and opt-in HTTP alternatives. Marketplaces `.claude-plugin/marketplace.json` and `.agents/plugins/marketplace.json` (`kubo-ipfs`). The plugin launches `ipfs mcp` from `PATH` and names no URL, token or service.
- Verified: isolated loads with temporary home and config directories against a private daemon on non-default ports. Claude Code 2.1.296: validate, install, MCP connected. Codex 0.160.1: installed, 32 tools. OpenCode 1.18.35: config hook, 8 skills, MCP connected. Evidence in [`mcp/plugin/evidence/`](../../mcp/plugin/evidence/).
- Not verified: a logged-in Claude Code model session using the plugin (task 1.5, operator).

### 3.14 Release

- What: [`mcp/release/`](../../mcp/release/) scripts build archives for darwin arm64/amd64, linux amd64/arm64 and windows amd64, with `SHA256SUMS` and release notes. Output in `mcp/release/dist/` is gitignored. Version `0.44.0-dev-833e04a24`.
- Verified: the orchestrator re-ran `verify.sh`: 5/5 OK. Smoke on darwin-arm64 only, offline daemon on non-default ports: 32 tools listed, `ipfs_node_info` ok. `nomcp` and `noagent` builds compile.
- Not verified: the other four archives were not executed. Nothing is tagged, published or uploaded.

### 3.15 Docs

- What: [user-guide.md](user-guide.md), [config-reference.md](config-reference.md) (generated by [`config_reference.py`](config_reference.py) from the config schema), [agentic.md](agentic.md), [embeddings.md](embeddings.md), [chat-setup.md](chat-setup.md), the ADRs, and the `docs/changelogs/v0.44.md` TOC and Highlights (28 lines added, none removed).
- Verified: the orchestrator re-ran `config_reference.py --check` against the built binary: OK.
- Not verified: the AG-UI, A2A and chat guides were written from code and tests, not from a live run.

## 4. Status of all 35 changes

Source: canonical runtime, revision 832, 2026-10-10. "Done" is the runtime's count of completed tasks.

| # | Change | Runtime | Done | Why open |
|---|---|---|---|---|
| 1 | team-ownership-grant | complete | 6/6 | |
| 2 | mcp-tool-contract | complete | 9/9 | |
| 3 | mcp-architecture-adrs | pending | 5/6 | ADR-0002 waits for the WebMCP spike report. 2026-10-12: ADR-0002 written as Deferred, task 1.2 ticked; runtime close pending |
| 4 | webmcp-host-spike | pending | 2/5 | Operator probes in Codex desktop and Kimi desktop; then the report |
| 5 | mcp-server-core-stdio | complete | 7/7 | |
| 6 | mcp-ui-component-library | complete | 8/8 | |
| 7 | mcp-pinning-with-confirmation | complete | 5/5 | |
| 8 | mcp-http-transport-auth | complete | 7/7 | |
| 9 | uar-conformance-request | pending | 1/2 | Operator files the request with the UAR team |
| 10 | agent-chat-island | complete | 4/4 | |
| 11 | mcp-agent-principal-enforcement | complete | 5/5 | |
| 12 | native-confirmation-prompt | pending | 4/5 | Operator real-prompt run (HG-1) |
| 13 | mcp-tools-content | complete | 3/3 | |
| 14 | mcp-tools-mfs | complete | 2/2 | |
| 15 | mcp-tools-ipns-keys | complete | 2/2 | |
| 16 | mcp-tools-dag-car | complete | 2/2 | |
| 17 | mcp-tools-routing-diag | complete | 2/2 | Done |
| 18 | webmcp-tool-package | pending | 0/3 | Not started; blocked on the WebMCP spike |
| 19 | mcp-llm-proxy-loop | complete | 6/6 | |
| 20 | uar-integration-spike | pending | 1/5 | Deferred (ADR-0008). 1.1 done and recorded in ADR-0008 |
| 21 | mcp-app-views | pending | 3/4 | Real-host render check (operator) |
| 22 | ipfs-agent-skills | complete | 4/4 | `tasks.md` 1.4 unticked in R11 (live near-miss eval pending, R11-Q06); the runtime cannot reopen a complete task, so a blocker records the pending live eval |
| 23 | webui-webmcp-integration | pending | 0/4 | Not started; blocked on the WebMCP spike |
| 24 | mcp-agui-endpoint | complete | 7/7 | |
| 25 | mcp-a2a-endpoint | complete | 7/7 | |
| 26 | desktop-agent-chat | pending | 3/6 | Pairing and native confirmation window not built; operator run |
| 27 | mcp-embeddings-surrealdb | complete | 5/5 | |
| 28 | agentic-uar-integration | pending | 0/4 | Deferred with topology B (ADR-0008). `tasks.md` has 5 tasks, all deferred: 1.5 moved in from `mcp-llm-proxy-loop` in R6 and is not registered in the runtime, which counts 4 (0/5 by checkbox) |
| 29 | mcp-release-binaries | complete | 3/3 | |
| 30 | webui-cid-delivery | pending | 0/3 | Not started; operator picks D1, D2 or D3 after the spike report |
| 31 | webui-agent-chat | pending | 3/4 | 1.3 operator live run (1.4 done: token in browser storage) |
| 32 | mcp-embedding-index-lifecycle | complete | 5/5 | |
| 33 | mcp-plugin-marketplaces | pending | 4/5 | Logged-in harness session (operator) |
| 34 | mcp-user-docs | complete | 6/6 | |
| 35 | phase-boundary-review | pending | 0/8 | Round 11, not started |

Summary: 21 complete and 14 open. Of the open ones, 6 wait on an operator action (4, 9, 12, 21, 31, 33); 4 are blocked on the WebMCP spike (3 for ADR-0002, 18, 23, 30); 2 are deferred UAR changes (20, 28); 1 is partly built (26); 1 is the not-started review (35). The runtime and the checkboxes disagree on changes 17, 20 and 31; the runtime is authoritative ([`.prometheus/gotchas.md`](../../.prometheus/gotchas.md), 2026-10-10).

## 5. Not delivered and known gaps

Not delivered:

- G8 WebMCP: no tool package, no Web UI integration. ADR-0002 records the decision as Deferred (2026-10-12). The host probes have not been run.
- Web UI CID delivery: Kubo does not pin or serve a new Web UI CID. `core/corehttp/webui.go` is unchanged.
- Topology B (UAR hosts AG-UI and A2A): deferred. No published UAR release contains the conformance items, and the conformance request has not been filed.
- Desktop pairing and the native confirmation window.
- Kimi Code plugin packaging (out of scope by operator decision).

Known gaps and defects:

- The three confirmation config keys `confirmation.resolvers`, `confirmation.cli` and `confirmation.ttl` are implemented (R12, `native-confirmation-prompt` task 1.6): `resolvers` narrows the effective modes, `cli: true` enables the `cli` mode, and `ttl` (default 120 s, at least 1 s, at most 10 minutes) sets the expiry. History: before R11 they were accepted and ignored; in R11 a config that set one was rejected at validation (R11-Q02), with the TTL fixed at 120 s; the operator chose on 2026-10-11 to implement them (section 5.1, decision 3). Since R12 the `cli` resolver never approves stdio calls (section 5.2, SEC-M1).
- `ipfs mcp` exits at start when no daemon is reachable, so a harness shows the server as failed. This matches ADR-0001 (no offline mode) but is a usability cost.
- Codex starts stdio servers with an allowlisted environment; an exported `IPFS_PATH` did not reach the server in testing. A node at a non-default repo path needs the HTTP alternative.
- Binaries are not code-signed or notarized. The checksum file is SSH-signed since `a36a99219` (section 5.1, decision 4), but the release key does not exist yet, so the archives built so far have no `SHA256SUMS.sig`. Only darwin-arm64 was executed. Binaries report `-dirty` because the tree was uncommitted at build time; R11 rebuilds on the final tree.
- `TestMCPDAG` subtest "pin=true pins the node" failed twice and passed on rerun. R11-Q01 explains it: `dag_put` returned after the first streamed object, cancelling the request before the daemon committed and pinned. It now reads the stream to EOF. `TestMCPDAG` passes 8/8 in R11 (not reproduced on the old code first).
- The `pkill` incident: on 2026-10-09 a subagent ran `pkill -f "ipfs daemon"`, copied from the upstream "Cleaning Up Stale Daemons" section of `AGENTS.md`. It killed every daemon by name, including the operator's IPFS Desktop daemon on the default ports. Fix: the rule "never use it here; stop only harness daemons by PID" is recorded in `.prometheus/gotchas.md`; tests stop only the processes they started, and `test/cli/mcp_procgroup_*_test.go` kills only the process group a test started (used for Ollama runner children). The upstream `AGENTS.md` text is unchanged and is still unsafe on a host that runs another node.
- The independent reviews ran in R11, and their findings were fixed in one batched pass (section 5.1). The fixes were re-verified by the integration gate (section 5.1); the reviewers did not re-review.
- The LLM backend was verified with Ollama only.
- The binary size delta of the full feature set is unmeasured (an ADR-0001 falsifier).
- The Web UI token in `localStorage` is readable by any script in the Web UI origin. The chat runs in the admin-privileged Web UI origin (carried warning G11-3/G11-4). The operator re-confirmed this on 2026-10-11 (R11 security M6 accepted; section 5.1, decision 2).
- OpenCode has no marketplace format; the plugin is a file URL or a private npm module. The marketplaces sit at the repository root, so installing one clones all of Kubo.
- The skills spec named 6 skills; 8 were delivered. Corrected in R11: the spec delta now lists the 8 skills (R11-Q13).
- Contract item open since R6: no fixture for the `{wait: true}` outcome after a two-calls resume.
- Elicitation was exercised only on protocol 2026-07-28.

### 5.1 R11 phase-boundary review

Findings files:

- Security review: [`.agent-team/kubo-ipfs-agentic/findings/security/r11-findings.md`](../../.agent-team/kubo-ipfs-agentic/findings/security/r11-findings.md). PASS-WITH-FINDINGS: 4 HIGH, 6 MEDIUM, 6 LOW, no CRITICAL.
- Quality, compatibility and UI review: [`.agent-team/kubo-ipfs-agentic/findings/review/r11-findings.md`](../../.agent-team/kubo-ipfs-agentic/findings/review/r11-findings.md). PASS-WITH-FINDINGS: 2 HIGH (R11-Q01, R11-Q02), no CRITICAL.
- Cross-family judge (`gpt-6.1-sol` over the diff): [`.kbd-orchestrator/phases/ipfs-mcp-server/review/r11/`](../../.kbd-orchestrator/phases/ipfs-mcp-server/review/r11/), `findings.confirmation.json` and `findings.transport-auth.json`. Verdict BLOCK: 5 CRITICAL, 2 WARNING.

All findings were fixed in one batched correction pass by the owning roles, followed by two short judge confirmation rounds. Re-verification on the final tree, 2026-10-11, run by the lead:

- `go test -race -count=1 ./mcp/... ./cmd/ipfs/kubo/`: 0 data races, all 6 packages ok. The first R11 race run had found 14 races in 13 AG-UI and A2A tests: tests changed a package-level recheck interval while server goroutines from earlier tests still read it. The interval is now a per-server option, and `Serve` joins the A2A task goroutines before it returns.
- `make build`, then `go test ./test/cli/ -run 'TestMCP|TestCommandDocsWidth' -skip Ollama`: ok (55.5 s). Ollama-backed tests were skipped so the operator's Ollama was not touched.
- `make test_go_lint`: clean for `mcp/`. The one remaining finding is in upstream `fuse/node/mount_darwin.go`.
- `go vet` with the `nomcp` and `noagent` tags: clean. `python3 docs/mcp/config_reference.py --check`: OK. `pnpm gen:check` in `mcp/ui`: clean.
- Playwright, first real-browser run: `mcp/ui` 90/90, `mcp/app` 104/104, and `mcp/chat` 133/133 (Chromium and Electron 44). Baselines are committed for darwin. The first runs exposed real UI defects that were fixed: duplicate landmark names, 22 px disclosure targets, a 52 px overflow at 320 px, and a default border on the chat composer. They also exposed test-harness bugs that had kept the MCP App and chat specs from ever exercising the views, also fixed.

Judge rounds after the correction pass (`findings.*.r2.json`, `findings.*.r3.json`) each found new, smaller items rather than repeats. These were fixed:

- `egress_ack: ["local"]` no longer covers a host literally named `local`.
- UAR startup and preflight output no longer prints the card URL or other endpoints.
- Two confirmation channels in one process coexist.
- The CLI answer read returns on cancel or expiry.
- `--confirm=` with an empty value means no resolvers.
- The CLI never approves when it cannot print the operation.
- Request bodies get a 30 s read deadline, and streaming responses are not affected.

Rounds were stopped after round 3. These items were adjudicated by the lead, not fixed:

- Principal ids in UAR diagnostics. These are operator-configured names on the operator's own stderr, which is not tracking.
- A read goroutine left behind after CLI cancel. The process exits immediately afterwards.
- A same-user process forging the stored Desktop pairing verifier. This is proposed as accepted residual risk: same-user processes are outside the write-gate threat model, and `desktop` is no longer a presence resolver. It is pending operator acceptance.

Judge findings:

- **CRITICAL.** `ipfs mcp confirm` sanitizes every server-provided field before printing it, so ANSI sequences and carriage returns could not disguise the operation.
- **CRITICAL.** Polkit helpers run only from root-owned absolute paths (security H3).
- **CRITICAL.** A replaced pairing revokes the old key on running servers (security H4).
- **CRITICAL.** HTTP rejection logs carry only a fixed reason; no host, origin or path is logged.
- **CRITICAL.** `--http=false` overrides `http.enabled: true`.
- **WARNING.** The macOS probe is bounded at 30 s.
- **WARNING.** The `--http` address is held per invocation, not in a package-level variable.

Security HIGH findings:

- **H1.** Elicitation is used only for `human-mcp` principals and stdio.
- **H2.** `resources/read` follows the `ipfs_cat` grant and `egress_ack` policy.
- **H3.** Polkit helpers run from absolute paths, root-owned and not group- or other-writable, with a minimal environment.
- **H4.** `desktop` is no longer a presence resolver; `ipfs mcp pair` needs a TTY and refuses while a server holds a socket.

Security MEDIUM and LOW findings:

- **M1.** Helper environments come from an allowlist.
- **M2.** `--api-auth-env` and `--api-auth-file` were added, and `--api-auth` warns.
- **M3.** At most 2 open out-of-band confirmations per principal and per session.
- **M4.** Agent sessions are capped at 16 per principal of 64, and each principal gets a run rate of burst 120, 120 per minute.
- **M5.** The anonymous listener refuses forwarded requests, and the docs require a credential behind a proxy.
- **L1.** The agent card is served with `Cache-Control: private` and `Vary: Host`.
- **L2.** The untrusted-content closing tag is neutralized case-insensitively.
- **L3.** The LLM and embedder require https for a key unless the host is loopback.
- **L4.** The owner of the confirmation directory is checked.

Reviewer HIGH findings:

- **R11-Q01.** `dag_put` reads the stream to EOF before it reports `pinned`.
- **R11-Q02.** The three unread confirmation keys are rejected at validation.

Reviewer MEDIUM and LOW findings (code fixes are listed by the lead):

- **R11-Q03.** The confirmation test now fails on a build failure.
- **R11-Q04.** `TestMCPNoAgentBuild` was added.
- **R11-Q05.** The `mcp/app/embed.go` comment was corrected.
- **R11-Q06.** Skills task 1.4 was unticked.
- **R11-Q08 and R11-Q09.** The ApprovalPanel role and label were fixed.
- **R11-Q10 and R11-Q12.** A catalog implementation test and skills contract tests were added.
- **R11-Q13.** The skills spec now lists 8 skills.
- **R11-Q15.** The `panic` was removed from `token.go`.
- **R11-Q16.** `agent.go` was split.
- **R11-Q20.** The chat changelog item is marked fork-only.
- **R11-Q21.** The `agentic-uar-integration` task count was reconciled.

ADRs amended for these changes: [ADR-0003](0003-confirmation-and-auth.md) and [ADR-0004](0004-agentic-endpoints-and-llm.md), both dated R11, 2026-10-11.

Still open after the pass:

- Closed later in R11: the A2A context store now has a per-owner quota of 16 within 64. It evicts only the same owner's idle contexts, never one with an open approval, and otherwise refuses with `TOO_MANY_CONTEXTS`. The `desktop` presence text in the confirmation schema, the config schema and the generated UI catalog was corrected.
- R11-Q19: `zod-v3` stays in the `./a2ui` entry, because `@a2ui/react` 0.12.0 requires zod 3. The MCP App view bundles contain one zod and no v3 code. The views are about 305 KB gzip, about 200 KB of which is fonts.
- Closed on 2026-10-11 by operator decision: the same-user pairing verifier (decision 1 below).

Operator decisions on the open R11 items, 2026-10-11:

| # | Item | Decision | Status |
|---|---|---|---|
| 1 | Same-user pairing verifier forgery (judge round 2) | Accepted as residual risk. Bounds: write-class calls only (destructive calls need a presence resolver, which `desktop` is not); same OS user only. Off switch: leave `desktop` out of `--confirm` / `IPFS_MCP_CONFIRM` / `confirmation.modes`, or out of `confirmation.resolvers` | Recorded in [ADR-0003](0003-confirmation-and-auth.md), amendment of 2026-10-11, item 1 |
| 2 | Chat token in browser `localStorage` (security M6) | Keep it in `localStorage`; no opt-in or path restriction. No code change | Recorded in [ADR-0005](0005-agent-chat-ui.md), amendment of 2026-10-11, item 1; M6 closed as accepted risk |
| 3 | `confirmation.resolvers`, `confirmation.cli`, `confirmation.ttl` | Implement them with the semantics in ADR-0003 (amendment of 2026-10-11, item 2): `resolvers` narrows after mode resolution and can only make the gate stricter; `cli: true` enables the `cli` mode unless `--confirm` or `IPFS_MCP_CONFIRM` is given; `ttl` is at least 1 s and at most 10 minutes, default 120 s (minimum raised from "greater than 0" in R12) | Implemented in R12 (`native-confirmation-prompt` task 1.6, `mcp-server-engineer`) and reviewed (section 5.2); config reference regeneration by the lead pending |
| 4 | Signing `SHA256SUMS` (security L5) | Decided: SSH signing (`ssh-keygen -Y sign`, namespace `kubo-mcp-release`) with a new dedicated ed25519 release key, not the operator's login key. Verified with an `allowed_signers` file | Implemented in `a36a99219` (`skills-plugin-specialist`): `mcp/release/signing.sh` (key from `KUBO_MCP_SIGNING_KEY`, opt-out `KUBO_MCP_UNSIGNED=1`), `verify.sh` (`--allowed-signers`, `--allow-unsigned`, `DIST_DIR`), `mcp/release/allowed_signers`; user guide "Verifying a release". L5 closed. The operator generated the key on 2026-10-12 (fingerprint `SHA256:R3LaYr91QVNgSdTfwWESKqbkjZrhwW7aZM07wkGQkL4`); its public line is committed in `b4c7b4fd9`. Exercised end to end by the operator on 2026-10-12: `KUBO_MCP_SIGNING_KEY=~/.ssh/kubo_mcp_release_signing mcp/release/build.sh` built `0.44.0-dev-17c5cf4a9` for 5 targets and wrote `SHA256SUMS.sig`; `mcp/release/verify.sh` reported `Good "kubo-mcp-release" signature ... SHA256:R3LaYr91QVNgSdTfwWESKqbkjZrhwW7aZM07wkGQkL4` and all 5 archives OK. The archives were not published |
| 5 | CI under `.github/` (R11-Q05) | Yes, for one new workflow; upstream workflows unchanged | Added: `.github/workflows/mcp.yml`, OpenSpec change `mcp-ci-workflow`; reviewed and corrected in R12 (section 5.2). First run 38182586841 on `eae28f59a`: Go ok (7m4s), lint ok, UI unit ok, Playwright `ui` and `app` ok, Playwright `chat` 101 passed and 32 failed (all `electron44` screenshots: classic scroll bars on the runner, 753 px against 768 px); fixed in `a36a99219` by setting overlay scroll bars on the chat job, no baseline or threshold change. Re-run 38200585039: chat 108 passed, 25 failed (Electron captures 1 px larger at device scale factor 1 on the runner against 2 on the baseline host, plus one Chromium target-size check on a control inside a closed `<details>`); fixed in `4f79a0933` (Electron test launch forces scale factor 2; closed `<details>` contents skipped). Run 38230221369 on `4f79a0933`: all six jobs green (Go with race and the CLI suite, Go lint, UI unit, Playwright `ui`, `app`, `chat`), no baseline or threshold changed |
| 6 | Desktop `openExternal` allowlist (security L6) | No restriction; it is upstream IPFS Desktop code | Recorded in [ADR-0005](0005-agent-chat-ui.md), amendment of 2026-10-11, item 2; L6 closed as won't-fix by operator decision. No change |

### 5.2 R12 review: the three confirmation keys and the CI workflow

Independent reviews of decision 3 (the keys) and decision 5 (CI), 2026-10-11, each in a separate read-only context. Nothing was executed by the reviewers.

- Security review: [`.agent-team/kubo-ipfs-agentic/findings/security/r12-confirm-keys-ci.md`](../../.agent-team/kubo-ipfs-agentic/findings/security/r12-confirm-keys-ci.md). 0 CRITICAL, 0 HIGH, 1 MEDIUM, 4 LOW, 5 INFO.
- Quality and compatibility review: [`.agent-team/kubo-ipfs-agentic/findings/review/r12-confirm-keys-ci.md`](../../.agent-team/kubo-ipfs-agentic/findings/review/r12-confirm-keys-ci.md). PASS-WITH-FINDINGS: 0 CRITICAL, 0 HIGH, 5 MEDIUM, 8 LOW, 4 NIT. No `/api/v0`, gateway, CID-recipe or wire impact; with the three keys absent, behavior equals the old code.

Fixes, one batched pass by the owning roles:

- **SEC-M1.** Under the `cli` mode, a shell-capable stdio host could approve its own calls through the confirmation socket (the gap predates R12; `confirmation.cli` would have added a persistent way to open it). Now the `cli` resolver never approves a confirmation from the stdio local principal; a non-eliciting stdio client with only `cli` gets `confirmation_unavailable`, with a startup warning. Recorded in ADR-0003 (amendment of 2026-10-11, item 3) and as a spec scenario in `native-confirmation-prompt`.
- **TTL minimum.** `confirmation.ttl` is now at least 1 s (was "greater than 0"), because token expiry is kept in whole seconds (SEC-I2, REV-L2). The maximum stays 10 minutes.
- **SEC-L1.** The narrowing warning no longer claims refusal when the `cli` resolver remains.
- **Docs and spec (architect).** REV-M1: this report no longer says the keys are unimplemented. REV-M5: the spec names every way to enable `cli` (`--confirm`, `IPFS_MCP_CONFIRM`, `confirmation.modes`, `confirmation.cli`) and the TTY rule. REV-L3: with `confirmation.modes` absent, `cli: true` adds `cli` to the default modes (`native`, `desktop`); stated in the schema, ADR-0003 and the spec. REV-N4: `TestMCPConfirmationConfigKeys` is cited for the three keys in the config reference sidecar.
- **CI (architect).** REV-M2: the baselines are not darwin-suffixed (no `{platform}` in `snapshotPathTemplate`); the workflow comment, the design and task 1.4 are corrected, and CI never regenerates the operator's baselines. REV-M3: the Playwright runner is pinned to `macos-26` (GitHub's arm64 macOS 26 image, matching the operator's macOS 26 arm64 host) instead of `macos-latest`. SEC-L4: `persist-credentials: false` on every checkout. REV-L6: `test/cli/harness/**`, `Makefile` and `mk/**` added to the paths. REV-L7: `push` limited to `master` and `feat/**`. REV-L8 and REV-N3: Node is set up before pnpm, and the pnpm store is cached once per lockfile by `pnpm/action-setup`. `actionlint` exits 0.
- **Code (mcp-server-engineer).** Other code findings (REV-M4 end-to-end resolvers test, L1/L2 single config parse, L3 message wording, L4 stale comments, L5, N1, N2) are in the engineer's pass and reported there.

Re-verified after the R12 pass: `go test ./mcp/... ./cmd/ipfs/kubo/` ok; `go vet` with `nomcp` and `noagent` clean; `make build && go test ./test/cli/... -run 'TestMCP|TestCommandDocsWidth' -skip Ollama` ok (80.0 s); `make test_go_lint` reports only the known upstream `fuse/node/mount_darwin.go` ST1005; `config_reference.py --check` OK after regeneration; `actionlint` clean; gitleaks clean on `.github`. Not run locally: the race detector and Playwright (held by the tier-guard hook); the new workflow runs both on GitHub. Still open after R12: the first CI run (`mcp-ci-workflow` task 1.3) and the operator's decision on Playwright baselines if it differs only by rendering (task 1.4). Update 2026-10-11: the first run (38182586841) is done and recorded in section 5.1, decision 5; only the chat Electron screenshots failed, from a runner scroll-bar setting fixed without a baseline change, and re-run 38200585039 is pending.

## 6. Operator actions pending, and next steps

Operator actions:

1. Run the WebMCP probe in Codex desktop and Kimi desktop (`webmcp-host-spike` 1.2, 1.3; runbook in [`mcp/webmcp/spike/RUNBOOK.md`](../../mcp/webmcp/spike/RUNBOOK.md)). [ADR-0002](0002-webmcp-web-ui.md) is Deferred until then (2026-10-12); a GO result per host moves it to Accepted for that host and unparks `webmcp-tool-package`, `webui-webmcp-integration` and `webui-cid-delivery`.
2. After the spike report, choose D1, D2 or D3 for Web UI delivery (`webui-cid-delivery` 1.1). D2 also needs approval for the Web UI CID bump in `core/corehttp/webui.go`.
3. File the UAR conformance request ([`mcp/uar/requests/CONFORMANCE.md`](../../mcp/uar/requests/CONFORMANCE.md); `uar-conformance-request` 1.2), or confirm topology B stays deferred.
4. Run the real native prompt (`native-confirmation-prompt` 1.5, `mcp/server/confirm/hg1/attempt.sh`).
5. Check the MCP App views in a real Claude Code host (`mcp-app-views` 1.4).
6. Run the plugin in a logged-in Claude Code session (`mcp-plugin-marketplaces` 1.5).
7. Run the fork Web UI chat live (`webui-agent-chat` 1.3).
8. Run a paired Desktop on macOS once pairing is built (`desktop-agent-chat` 1.6).
9. At the end: approve and commit the phase including `AGENTS.md`, and commit the Web UI and Desktop forks (`phase-boundary-review` 1.8). Tags, pushes and publishing are separate approvals.

`phase-boundary-review` status on 2026-10-12: 6 of 8 tasks complete.

- 1.1, 1.4, 1.5, 1.6, 1.7: done in R11 and R12 (judge route, security and quality reviews, cross-family judge, batched corrections).
- 1.2: re-run on the final tree on 2026-10-12. Build, fmt, `test_unit` (3106 tests, 0 failed), module sync, replace check, `TestGateway`, `TestDefaultMatchesExpectedProfile`, `TestTelemetry`, `TestProvider`, `TestCommandDocsWidth` and the MCP `test/cli` suite pass. Lint reports only the upstream macOS-only `fuse/node/mount_darwin.go` ST1005 (CI lint on Linux is clean). One `TestGateway` subtest failed once on its fixed port and passed on rerun. `check_plan_deps.py` passes (176 tasks, 176 rows) after `mcp-ci-workflow` and 15 missing rows were added.
- 1.3: open. The G9-G12 acceptance set needs the `uar-integration-spike` report and the topology-2 gate result (item 3 above).
- 1.8: open. Operator approval and commit (item 9 above).

The 34 stale "bottleneck" blockers in the KBD runtime (records of failed boundary evaluations on tasks that are complete or task IDs that do not exist) were cleared on 2026-10-12. The one real blocker kept is `ipfs-agent-skills-1.4-live-eval`.

## Change log

- 2026-10-10: created after Round 10.
- 2026-10-10: runtime and checkbox mismatches reconciled (uar-integration-spike 1.1, webui-agent-chat 1.4 closed in the runtime; mcp-tools-routing-diag 1.2 checkbox ticked); lint fixes in `mcp/server`; first commit and push to `feat/ipfs-mcp-server` in all three repositories.
- 2026-10-11: stuck shells stopped (bash 5.3 here-document write deadlock on a 16 KB macOS pipe; see `.prometheus/gotchas.md`). Nine half-closed KBD boundaries left by interrupted closes were closed with `prometheus kbd guard evaluate --edge after`; the 6 remaining open boundaries belong to changes still in progress. Phase-boundary review R11 started.
- 2026-10-11: R11 reviews and judge recorded (section 5.1). Docs and specs were brought in line with the correction pass: ADR-0003 and ADR-0004 amended; config reference regenerated; user guide; `MECHANISMS.md`; skills task 1.4 reopened and the spec moved to 8 skills; the changelog chat item marked fork-only. Re-verification on the final tree passed: race detector 0 races, `TestMCP` CLI suite ok, lint clean for `mcp/`, Playwright `mcp/ui` 90/90, `mcp/app` 104/104, `mcp/chat` 133/133. Judge rounds 2 and 3 fixed 7 further items; 3 adjudicated. `tier3.allow` removed.
- 2026-10-11: operator decisions on the six open R11 items recorded (section 5.1): pairing-verifier forgery accepted as residual risk (ADR-0003); chat token kept in `localStorage`, M6 accepted (ADR-0005); the three confirmation keys to be implemented (`native-confirmation-prompt` 1.6, contract descriptions updated, config reference regeneration pending the code); `SHA256SUMS` signing pending the operator's answer (SSH signing recommended); CI added as `.github/workflows/mcp.yml` (OpenSpec `mcp-ci-workflow`, `actionlint` clean, first run pending); Desktop `openExternal` left as upstream, L6 won't-fix (ADR-0005).
- 2026-10-11: R12 reviews of the three confirmation keys and the CI workflow recorded (section 5.2): security 1 MEDIUM, 4 LOW, 5 INFO; quality 5 MEDIUM, 8 LOW, 4 NIT, no CRITICAL or HIGH. Architect fixes: progress report, ADR-0003 (item 3, SEC-M1; TTL 1 s to 10 m; cli enable paths), spec scenario, schema descriptions, sidecar citations; CI pinned to `macos-26`, `persist-credentials: false`, wider paths, push limited to `master` and `feat/**`, Node before pnpm, baseline premise corrected; `actionlint` exit 0. Engineer code fixes and config reference regeneration pending re-verification by the lead.
- 2026-10-11: `SHA256SUMS` signing decided and implemented (section 5.1, decision 4): SSH signing with a new dedicated ed25519 key, namespace `kubo-mcp-release`, in `a36a99219`; user guide "Verifying a release" added; the key and its `allowed_signers` line are pending. First CI run 38182586841 recorded (decision 5): all jobs ok except 32 `electron44` screenshots in Playwright `chat`, caused by classic scroll bars on the runner; overlay scroll bars set on the chat job in `a36a99219`, re-run 38200585039 pending.
- 2026-10-12: release signing key committed (`b4c7b4fd9`); chat CI fixed (`4f79a0933`); run 38230221369 all six jobs green; `mcp-ci-workflow` complete.
- 2026-10-12: signed release build verified end to end by the operator (`0.44.0-dev-17c5cf4a9`, 5 archives, good signature); not published.
- 2026-10-12: `phase-boundary-review` 1.1, 1.2 and 1.4-1.7 closed (1.2 re-run on the final tree); `check_plan_deps.py` fixed; 34 stale KBD bottleneck blockers cleared; 1.3 and 1.8 remain open.
- 2026-10-12: ADR-0002 (WebMCP in the Web UI) written with status Deferred by operator decision, because the WebMCP host probe has not run (`webmcp-host-spike` 1.2, 1.3 open; no `REPORT.md`). A GO result per host moves it to Accepted. `webmcp-tool-package`, `webui-webmcp-integration`, `webui-cid-delivery` and `webui-agent-chat`'s WebMCP prerequisite stay parked. `mcp-architecture-adrs` 1.2 ticked; runtime close pending.
