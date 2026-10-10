# Progress report: KBD phase `ipfs-mcp-server`

Last updated: 2026-10-10

This document records what the `ipfs-mcp-server` phase has delivered in this Kubo fork, why each decision was made, what is verified, and what is still open. It is a summary for readers who were not in the sessions. The detailed, append-only log stays in [`execution.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/execution.md); the plan is [`plan.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/plan.md).

How it is maintained: the `ipfs-agent-architect` role updates this file at the end of every execution round and at the phase boundary, and adds one dated line to the change log at the bottom. Status figures come from the canonical KBD runtime (`prometheus kbd --path . status --json`), not from task checkboxes.

Position on 2026-10-10: stage Execute, rounds R0-R10 dispatched, 21 of 35 changes complete in the runtime. Round 11 (phase-boundary review) has not started. All finished work is committed and pushed to branch `feat/ipfs-mcp-server` in GQAdonis/kubo, GQAdonis/ipfs-webui and tribe-health/ipfs-desktop (operator request, 2026-10-10).

## 1. Goals

The goals are in [`goals.md`](../../.kbd-orchestrator/phases/ipfs-mcp-server/goals.md). Status in one line each:

| Goal | Summary | Status on 2026-10-10 |
|---|---|---|
| G1 | Choose where MCP lives, by assessment with adversarial review; record it as an ADR | Met: [ADR-0001](0001-mcp-server-placement.md), operator-approved |
| G2 | MCP tools for add/get/cat, pins, IPNS and keys, MFS, DAG, CAR, routing, node status, from one contract | Met: 41 tools in [`mcp/contract/catalog.json`](../../mcp/contract/catalog.json) |
| G3 | stdio and Streamable HTTP, auth, loopback default, Origin protection, off switch per capability | Met in code and tests; independent review pending |
| G4 | React 19 component library that is an A2UI v0.9 catalog, `ui://` resources, WCAG 2.2 AA | Built; real-host render, visual baselines and real-browser axe pending |
| G5 | agentskills.io skills, packaged as plugins and marketplaces | 8 skills; plugins for Claude Code, Codex, OpenCode; logged-in harness run pending |
| G6 | No break to `/api/v0`, gateway, CID recipe, wire; no tracking; no silent CID injection; gated writes | Held by design; the blocking regression suite runs in R11 |
| G7 | Team execution with independent security and quality review | Team execution done; review is R11, not started |
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
- ADR-0002 (WebMCP in the Web UI): not written. It waits for the WebMCP host spike report.
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
- Verified: `go test ./mcp/...` and `go test ./test/cli -run TestMCP` against real daemons on non-default ports, re-run at several rounds (latest recorded R7: `TestMCP` with Ollama skipped, 53.7 s, ok; `make build` ok; `go vet` including `nomcp` and `noagent`). 8 real-daemon HTTP subtests pass.
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
- Not verified: rendering in a real host (task 1.4, operator); Playwright visual baselines and real-browser axe (held by the `tier-guard` hook until R11). Each view is about 750 KB (about 305 KB gzip, about 200 KB fonts).

### 3.9 Chat island

- What: `<ipfs-agent-chat>`, a shadow-root React 19 + Tailwind 4 + shadcn sliding sidebar that talks only to `ipfs mcp` `/agui`, renders A2UI results and needs a trusted user gesture to answer an interrupt.
- Where: [`mcp/chat/`](../../mcp/chat/).
- Verified (agent-reported): 189 tests, 11 fixtures replay, closed bundle 2.3 kB gzip, first open 364 kB gzip including 145 kB fonts.
- Not verified: Playwright and Electron e2e specs are written but run in R11. `@a2ui/react` 0.12.0 injects a head style that the build strips; an upstream report is drafted in [`mcp/chat/EVIDENCE.md`](../../mcp/chat/EVIDENCE.md), not filed.

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
- Verified: `skills-ref validate` clean for all 8; catalog check 41/41 tool names; lexical trigger proxy 20/22.
- Not verified: the live-model near-miss trigger run (R11).

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
| 3 | mcp-architecture-adrs | pending | 5/6 | ADR-0002 waits for the WebMCP spike report |
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
| 22 | ipfs-agent-skills | complete | 4/4 | |
| 23 | webui-webmcp-integration | pending | 0/4 | Not started; blocked on the WebMCP spike |
| 24 | mcp-agui-endpoint | complete | 7/7 | |
| 25 | mcp-a2a-endpoint | complete | 7/7 | |
| 26 | desktop-agent-chat | pending | 3/6 | Pairing and native confirmation window not built; operator run |
| 27 | mcp-embeddings-surrealdb | complete | 5/5 | |
| 28 | agentic-uar-integration | pending | 0/4 | Deferred with topology B (ADR-0008) |
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

- G8 WebMCP: no tool package, no Web UI integration, no ADR-0002. The host probes have not been run.
- Web UI CID delivery: Kubo does not pin or serve a new Web UI CID. `core/corehttp/webui.go` is unchanged.
- Topology B (UAR hosts AG-UI and A2A): deferred. No published UAR release contains the conformance items, and the conformance request has not been filed.
- Desktop pairing and the native confirmation window.
- Kimi Code plugin packaging (out of scope by operator decision).

Known gaps and defects:

- Three confirmation config keys are validated but never read: `confirmation.resolvers`, `confirmation.cli` and `confirmation.ttl`. The TTL is fixed at 120 s. They are marked in [config-reference.md](config-reference.md).
- `ipfs mcp` exits at start when no daemon is reachable, so a harness shows the server as failed. This matches ADR-0001 (no offline mode) but is a usability cost.
- Codex starts stdio servers with an allowlisted environment; an exported `IPFS_PATH` did not reach the server in testing. A node at a non-default repo path needs the HTTP alternative.
- Binaries are unsigned and not notarized. Only darwin-arm64 was executed. Binaries report `-dirty` because the tree was uncommitted at build time; R11 rebuilds on the final tree.
- `TestMCPDAG` subtest "pin=true pins the node" failed twice and passed on rerun. Not investigated.
- The `pkill` incident: on 2026-10-09 a subagent ran `pkill -f "ipfs daemon"`, copied from the upstream "Cleaning Up Stale Daemons" section of `AGENTS.md`. It killed every daemon by name, including the operator's IPFS Desktop daemon on the default ports. Fix: the rule "never use it here; stop only harness daemons by PID" is recorded in `.prometheus/gotchas.md`; tests stop only the processes they started, and `test/cli/mcp_procgroup_*_test.go` kills only the process group a test started (used for Ollama runner children). The upstream `AGENTS.md` text is unchanged and is still unsafe on a host that runs another node.
- Independent review has not happened. The security and quality reviewers and the cross-family judge run in R11. Until then every claim here rests on the builders' and the orchestrator's checks.
- `go test -race` and Playwright runs are held by the `tier-guard` hook until R11.
- The LLM backend was verified with Ollama only.
- The binary size delta of the full feature set is unmeasured (an ADR-0001 falsifier).
- The Web UI token in `localStorage` is readable by any script in the Web UI origin. The chat runs in the admin-privileged Web UI origin (carried warning G11-3/G11-4).
- OpenCode has no marketplace format; the plugin is a file URL or a private npm module. The marketplaces sit at the repository root, so installing one clones all of Kubo.
- The skills spec named 6 skills; 8 were delivered.
- Contract item open since R6: no fixture for the `{wait: true}` outcome after a two-calls resume.
- Elicitation was exercised only on protocol 2026-07-28.

## 6. Operator actions pending, and next steps

Operator actions:

1. Run the WebMCP probe in Codex desktop and Kimi desktop (`webmcp-host-spike` 1.2, 1.3; runbook in [`mcp/webmcp/spike/RUNBOOK.md`](../../mcp/webmcp/spike/RUNBOOK.md)).
2. After the spike report, choose D1, D2 or D3 for Web UI delivery (`webui-cid-delivery` 1.1). D2 also needs approval for the Web UI CID bump in `core/corehttp/webui.go`.
3. File the UAR conformance request ([`mcp/uar/requests/CONFORMANCE.md`](../../mcp/uar/requests/CONFORMANCE.md); `uar-conformance-request` 1.2), or confirm topology B stays deferred.
4. Run the real native prompt (`native-confirmation-prompt` 1.5, `mcp/server/confirm/hg1/attempt.sh`).
5. Check the MCP App views in a real Claude Code host (`mcp-app-views` 1.4).
6. Run the plugin in a logged-in Claude Code session (`mcp-plugin-marketplaces` 1.5).
7. Run the fork Web UI chat live (`webui-agent-chat` 1.3).
8. Run a paired Desktop on macOS once pairing is built (`desktop-agent-chat` 1.6).
9. At the end: approve and commit the phase including `AGENTS.md`, and commit the Web UI and Desktop forks (`phase-boundary-review` 1.8). Tags, pushes and publishing are separate approvals.

Next step: Round 11, `phase-boundary-review`. It covers:

- 1.1: confirm the cross-family judge route works; if not, the phase is recorded `pending_review` and does not complete.
- 1.2: run every blocking constraint on the final tree: build, fmt, lint, unit tests, module sync, replace check, `TestGateway`, `TestDefaultMatchesExpectedProfile`, `TestTelemetry`, `TestProvider`, `TestCommandDocsWidth`, and all MCP `test/cli` suites on free ports; rebuild the release.
- 1.3: the G9-G12 acceptance set: unconfigured principals get nothing, `egress_ack` refusals, no self-confirm, deny and expiry final, check-uar refusals, both SurrealDB modes, embedding off by default, pin-aware deletion, chat isolation, and the recorded topology-B result.
- 1.4: independent security review (server, auth, broker and resolvers, agent endpoints, egress, supervised SurrealDB, both forks, plugins), including a re-test of the R3 handle defect.
- 1.5: independent quality, compatibility and UI review, including the carried warnings.
- 1.6: the cross-family adversarial judge over the phase diff.
- 1.7: one batched correction cycle by the owning roles.
- 1.8: operator approval and commit.

## Change log

- 2026-10-10: created after Round 10.
- 2026-10-10: runtime and checkbox mismatches reconciled (uar-integration-spike 1.1, webui-agent-chat 1.4 closed in the runtime; mcp-tools-routing-diag 1.2 checkbox ticked); lint fixes in `mcp/server`; first commit and push to `feat/ipfs-mcp-server` in all three repositories.
