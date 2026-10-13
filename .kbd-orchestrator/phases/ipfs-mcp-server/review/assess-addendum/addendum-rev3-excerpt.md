```
ADDENDUM: ipfs-mcp-server (G9-G12)
Project: Kubo (fork GQAdonis/kubo, 0.44.0-dev, HEAD 833e04a24)
Date: 2026-10-09
Base: assessment.md revision 2 (decision of record F4, W1+W4), plan.md revision 2 (22 changes)
Scope of this addendum: G9 AG-UI/A2A, G10 LLM backend, G11 chat UI, G12 embeddings/SurrealDB
Revision: 3 (consolidation, 2026-10-09): the decision of record follows the round-2 adjudication and the operator answers of 2026-10-09; revision 2 followed review round 1 (BLOCK)
```

Assessor: `ipfs-agent-architect` (KBD Assess stage, addendum). The only file written is this one. No code, no OpenSpec change, and no file in another repository was touched.

### Revision 2 changes (review round 1)

| Finding | How revision 2 addresses it |
|---|---|
| CRITICAL A: no falsifier; non-delivery defaulted to "ship non-conformant" | New section ASSUMPTIONS AND FALSIFIERS, per goal. The safe default on non-delivery is **defer**, never "ship non-conformant", with a dated delivery gate (end of Round 4). The N2 spike now tests the addendum's own top risks: an A2A-delegated write, setup cost against a threshold, and UAR behavior with and without the shipped profile. |
| CRITICAL B: G10/G12 guarantees delegated to a runtime that breaks them by default | New section KUBO-SIDE ENFORCEMENT. Four things Kubo ships in `ipfs mcp`: a principal policy, an egress acknowledgement, an out-of-band confirmation channel, and a `check-uar` preflight. Kubo also ships a tested UAR profile under `mcp/uar/**`. Acceptance criteria are tied to these. Guarantees Kubo cannot enforce are named and the goal is marked **partially unmet** for them. "Nothing leaves the machine by default" is removed from ADR-0006. |
| WARNING B: "the human gate holds today" over A2A | Claim removed. New evidence: UAR bypasses tool approval entirely when its governance gate is disabled. That is possible in loopback mode without JWT (`uar:src/uar/runtime/manager.rs:5469-5480`, `uar:src/uar/governance/runtime_control.rs:455-478, 662-663`). The A2A human gate is **unknown** until N2 proves it, and the Kubo-side out-of-band confirmation becomes the enforceable gate. |
| WARNING C: G10 reinterpreted | The G10 decision is **PENDING operator question 2**. L3 (thin proxy) is re-presented with its real gain (a Kubo-controlled origin/auth boundary) and its real cost. |
| WARNING D: scores without weights; (d) Security and Rules fit inflated | Weights and the aggregation rule are stated. Options with goal fit at or below 2 are excluded before aggregation. Every table is re-scored, (d) and E1 are scored "as shipped" and "with Kubo enforcement", and the margins and sensitivities are stated. |
| WARNING E: G12 silently narrowed to UAR-routed content | Listed as an explicit limitation, with an opt-in `ipfs mcp` embedding hook as the option that closes it (N9, conditional). |
| SUGGESTION: deletion checks remaining pins | Adopted. Vectors are deleted only when `pin ls --type=all <cid>` shows no direct, recursive or indirect pin. On `repo_gc`, deletion is limited to CIDs that GC reports as removed. |
| Consolidation (2026-10-09, after round 2 and the full cross-model review `findings.full.json`) | The DECISION and the ADR-0004 to ADR-0006 drafts are rewritten as the self-contained decision of record:<ul><li>G9: topology A, AG-UI and A2A in `ipfs mcp` at default `127.0.0.1:5050`, with topology B (UAR-hosted) gated on a published UAR release by the end of Round 4</li><li>G10: L3, `ipfs mcp` proxies to an operator-configured UAR or OpenAI-compatible server</li><li>Human gate: native OS prompt with a presence factor</li><li>G11: Web UI island, Promethity, talking only to `ipfs mcp`; Desktop fork in scope</li><li>G12: requirements, concrete per-mode mechanisms and acceptance tests, with the component choice left to plan revision 2</li></ul>Load-bearing assumptions and falsifiers now sit inside DECISION per goal. Stdio-spawned UAR is forbidden wherever node content reaches a model, and the general "nothing leaves the machine" claim is withdrawn and replaced by a stated partial guarantee. Plan impact is rewritten: 34 changes. The revision-2 DECISION, ADRs and plan impact are kept under "SUPERSEDED (2026-10-09)". The review sections at the end are unchanged. |
Evidence conventions (same as `assessment.md`, plus two prefixes):

- `path:line` is a Kubo working-tree fact read this session.
- `uar:path:line` is from `/Users/gqadonis/Projects/prometheus/universal-agent-runtime` (HEAD `a7cb9729`, 2026-10-01), read-only.
- `webui:path:line` is from `/Users/gqadonis/Projects/references/ipfs-webui` (HEAD `24ddaa89`), and `desktop:path:line` is from `/Users/gqadonis/Projects/references/ipfs-desktop` (HEAD `9bd9904`, origin `tribe-health/ipfs-desktop`). Both are read-only.
- **[sub]** marks facts from read-only Explore sub-investigations run this session. The lead re-read these key ones and they hold:
  - `uar:src/uar/api/a2a/types.rs:116-132`, `uar:src/uar/api/a2a/mod.rs:75`, `uar:src/uar/api/a2a/agent_card.rs:26-30`
  - `uar:src/server.rs:2040-2051, 2528-2533`
  - `uar:src/config.rs:1409-1414, 1476` (no `cors` key in `uar:src/config.rs`: `grep -n cors` returns nothing)
  - `uar:src/uar/api/adapters.rs:361-381`, `uar:src/mcp/registry.rs:140-173`
  - `uar:src/uar/runtime/manager.rs:647-652, 5450-5510`, `uar:src/uar/governance/runtime_control.rs:455-478, 533, 662-663`
  - `uar:src/uar/api/a2a/task_execution.rs:568`
  - `webui:package.json:83, 91, 100, 104, 313-314`, `desktop:src/webui/index.js:41-47`
  - the greps for `elicit`, `ui://` and `RunAgentInput` in UAR `src` (no relevant hits; the only `elicit` match is the token "felicity" in a tokenizer JSON file)
- **[external]** marks facts from outside the repositories, with the URL and the date fetched (2026-10-09). **[unverified]** marks claims that were not checked.

---


> Review excerpt: lines 506-951 (DECISION, ADR DRAFTS, IMPACT, RISKS AND OPEN QUESTIONS) of assessment-addendum.md revision 3. Option analysis (107-382), enforcement (383-447) and falsifiers (448-505) are summarized inside DECISION; superseded revision-2 text (987+) is excluded.

## DECISION

**Decision of record, revision 3 (consolidation, 2026-10-09).** This section is self-contained. It supersedes the revision-2 DECISION ("UAR-hosted primary"), which is kept unchanged under "SUPERSEDED (2026-10-09)" near the end of this file. Its basis:

- the orchestrator adjudication of the round-2 CRITICAL, applied here: the addendum's own goal-fit gate excludes UAR-hosted (d) when undelivered UAR work is not credited, which leaves (a) at 51 against (c) at 49;
- the operator's binding answers of 2026-10-09 (`operator-answers.md`, "Answers to the G9-G12 addendum");
- the orchestrator defaults for questions 3, 5 and 8-11 recorded there.

**Shared principle (replaces the revision-2 principle).** `ipfs mcp` is the agentic surface of the node:

- It serves MCP, AG-UI and A2A from one process.
- It runs a minimal, bounded agent loop over contract tools only.
- It proxies model calls to an operator-configured OpenAI-compatible endpoint: UAR or any other server.
- It reuses one `AuthVerifier`, one scope map and one confirmation gate in-process.

The daemon, `/api/v0`, the gateway, `config/` and `docs/config.md` stay unchanged (F4). UAR is a supported LLM backend in topology A, and a second, gated topology (B) in which UAR hosts the agent endpoints.

**This reverses one revision-2 claim, and the consequence is stated, not hidden:** Kubo's fork now contains an LLM HTTP client and a minimal agent loop. Both live inside `ipfs mcp`, behind the build tag `noagent` (and `nomcp`). There is no default endpoint, and nothing is mounted unless configured.

### G9: topology A, AG-UI and A2A served by `ipfs mcp`; topology B (UAR-hosted) is gated

**Topology A (primary, delivered this phase).**

- **Listener.**
  - `ipfs mcp --http` gains AG-UI and A2A on the same listener as MCP Streamable HTTP, on distinct paths: `/mcp`, `/agui`, `/a2a`, and `/.well-known/agent-card.json`.
  - The listener's default address is **`127.0.0.1:5050`**, overridable with `--http=<multiaddr>`.
  - Port 4000 is never used as a default; it is the local liter-llm gateway port in this environment and would collide.
  - Kubo's defaults 4001, 5001 and 8080 are not used either.
  - The F4 bind rules carry over: a non-loopback bind is refused without a configured verifier.
- **AG-UI.**
  - `POST /agui` accepts `RunAgentInput` and streams spec events over SSE.
  - A pending confirmation ends the run with `RUN_FINISHED` `outcome: {type: "interrupt", interrupts: [...]}` (https://docs.ag-ui.com/concepts/interrupts, fetched 2026-10-09).
  - The resume input carries `resume: [{interruptId, status, payload}]`.
- **A2A 1.0.**
  - The agent card lives at `/.well-known/agent-card.json`, with the JSON-RPC binding at `/a2a`: `message/send`, `message/stream`, `tasks/get`, `tasks/cancel`.
  - A pending confirmation is `TASK_STATE_INPUT_REQUIRED`, and a missing or insufficient credential is `TASK_STATE_AUTH_REQUIRED`.
  - The skills are generated from the contract's A2A skill metadata.
- **In-process reuse.**
  - All three protocols authenticate through the same `AuthVerifier` and authorize through the same scope map.
  - A tool call from AG-UI or A2A goes through the same confirmation gate as an MCP call.
- **Off switches.**
  - `--agui` and `--a2a` are off by default.
  - They are mounted only when an LLM backend is configured (G10).
  - `noagent` compiles them out.
- **Protocol implementation.** The plan prefers hand-written types generated from the spec schemas over `net/http`, with no new SDK. An SDK (a2a-go v2, a community AG-UI Go SDK) is taken only if plan revision 2 measures its dependency weight and the operator accepts it; operator answer 2 covered go-sdk only.

**Topology B (second supported topology, gated).**

- **What it is.** UAR hosts AG-UI and A2A, and calls `ipfs mcp --http` as a configured principal (kind `uar`) with read-only default groups, `egress_ack`, the native confirmation prompt, and `check-uar`.
- **Stdio is forbidden.** Stdio-spawned UAR is forbidden for topology B. Over stdio, `ipfs mcp` cannot identify the client or enforce egress.
  - The shipped profile never spawns `ipfs mcp`.
  - `ipfs mcp` refuses `contentEgress` tools to a stdio client whose MCP `clientInfo` identifies UAR. This is a misconfiguration guard, not a security boundary, because `clientInfo` is self-reported.
- **Delivery gate.** B is supported only if UAR's conformance items are **contained in a published, installable UAR release (tag plus image digest or native package) by the end of Round 4**: agent card at `agent-card.json`, A2A 1.0 TaskState wire values, approval surfaced as input-required, `RunAgentInput` with spec interrupts, a configurable CORS allowlist, and safe defaults.
  - **Version pin.** The shipped `mcp/uar/profile/` pins that minimum UAR version, and `check-uar` refuses older versions via the agent card.
  - **If the gate fails:** topology B is deferred and reported unmet. There is no non-conformant B release. G9 is still delivered through topology A.

**Assumptions and falsifiers (G9).** Each row's observable is measurable; the default on falsification is the safe one.

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G9-1 | AG-UI and A2A can be served conformantly from `ipfs mcp` without an SDK | Plan revision 2's conformance tests (AG-UI event schema validation against the published `@ag-ui/core` schemas; A2A JSON-RPC schema validation against `a2aproject/A2A` 1.0 JSON schema) fail and cannot be fixed within the change | Take the SDK (operator approval of the root `go.mod` addition), or ship A2A only and defer AG-UI |
| G9-2 | A2A-delegated writes cannot run without a human | A test peer agent's `message/send` asks for `pin_add`, and the pin exists before a human approves the native prompt | Block: write groups are refused to every A2A principal until fixed |
| G9-3 | Topology B's UAR fixes ship in a published release by the end of Round 4 | No UAR tag or image digest containing all listed items exists at the end of Round 4 | Topology B deferred and reported unmet; A unaffected |
| G9-4 | The single `--http` listener can carry MCP, AG-UI and A2A without cross-protocol auth confusion | A token scoped for one protocol's principal is accepted on another protocol's path for a tool outside its groups (`test/cli` matrix) | Block until fixed; separate listeners if needed |

### G10: L3, `ipfs mcp` proxies model calls to an operator-configured UAR instance or OpenAI-compatible server

- **The loop.**
  - The agent loop for topology A runs in `ipfs mcp`.
  - It is minimal: a step limit, a token budget and a wall-clock limit, all configurable with conservative defaults.
  - Its tools are only the contract tools enabled for the calling principal.
  - It has no shell, no filesystem and no network tools.
- **The model endpoint.** The model is reached over the OpenAI-compatible chat-completions API (`POST <base_url>/chat/completions` with `tools`). Both UAR (`/v1/chat/completions`, `uar:src/server.rs:1901`) [sub] and other OpenAI-compatible servers provide it.
- **Configuration.**
  - `--llm-base-url`, `--llm-model` and `--llm-api-key-env=<VAR>` (or `--llm-api-key-file`), or the same keys in the `ipfs mcp` config file.
  - **There is no default URL and no default model.**
  - The key is read from the named env var or file, never accepted as a literal.
  - Unset means G9/G10 surfaces are not mounted and no model call is ever made.
- **Egress, enforced in-process.**
  - A tool result marked `contentEgress` (`ipfs_cat`, `ipfs_get`, `dag_get`, `ipfs_ls` and others) is included in a model request only if the backend's host is listed in `egress_ack`.
  - Otherwise the loop returns a structured refusal to the model and the user ("Content not sent: `<host>` is not in egress_ack.").
  - With a UAR backend, `check-uar` also verifies UAR's effective upstream provider (re-checked per session and when UAR's settings report a change) where its settings API exposes it.
- **Remaining partial guarantee (limitation).** Kubo cannot verify what an acknowledged endpoint does with data. The cases:
  - a loopback server that forwards upstream;
  - a UAR whose effective provider is not readable;
  - stdio MCP hosts (Claude Code, Codex, OpenCode) that send tool results to their own models;
  - WebMCP tools executing in the browser.

  The operator's `egress_ack` is the control, and docs state this. **This addendum does not claim "no content leaves the machine" in general.**

**Assumptions and falsifiers (G10).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G10-1 | UAR's `/v1/chat/completions` accepts client-supplied `tools` and returns `tool_calls` without executing its own tools | The integration test against a UAR built from the pinned published release shows UAR executing a tool itself, or dropping client tools | UAR as a G10 backend is unsupported until fixed. OpenAI-compatible servers remain. |
| G10-2 | At least one local OpenAI-compatible server supports tool calling well enough for the contract tools | The real-service test with one named local server (chosen in plan revision 2) fails the tool-call round trip | G10 is local-unmet; a remote backend with `egress_ack` is still supported. Reported. |
| G10-3 | No model call happens without configuration | `test/cli`: with no `--llm-*` set, `/agui` and `/a2a` return 404, and a network capture shows zero outbound connections from `ipfs mcp` | Block |
| G10-4 | `egress_ack` is enforced for every `contentEgress` result | `test/cli` with a fake model server: content from `ipfs_cat` reaches the model request body without its host in `egress_ack` | Block |
| G10-5 | The UAR backend setup is acceptable | Measured from published artifacts on clean macOS arm64 and Linux x86_64: more than 30 minutes or more than 15 steps (threshold per operator default for question 11) | The UAR backend is documented as "advanced". OpenAI-compatible local servers are the recommended path. |

### Human gate (applies to G9, G10, G11 and topology B)

- **Priority order.** For clients that can elicit, MCP elicitation stays the primary gate. For every principal that cannot elicit, the gate is a **native OS prompt**:
  - an IPFS Desktop confirmation window, when Desktop is running and paired with `ipfs mcp` over a local authenticated channel;
  - otherwise a macOS or Linux system prompt raised by `ipfs mcp`.

  Both show the tool, the CID, path or name, and the effect.
- **Human presence.** The prompt must require presence that a same-user background process cannot simulate. Candidate mechanisms, chosen in plan revision 2 and verified by the falsifier below:
  - macOS LocalAuthentication (Touch ID or password);
  - a Linux polkit authentication agent;
  - for Desktop, a window that requires a user gesture and is not reachable through the Web UI's renderer.
- **Headless only.** `ipfs mcp confirm <id>` exists only for headless servers behind an explicit opt-in (`--confirm=cli`). It checks for a TTY and prints, at startup and per request, that same-user processes can reach it.
- **Shell-capable tools.** They are refused for principals with write scope.
  - In topology A, the loop exposes no shell tools.
  - In topology B, `check-uar` refuses write groups when UAR reports `terminal_exec` enabled (`uar:src/uar/tools/terminal_exec.rs:22-27`, opt-in at `uar:src/config.rs:2523-2525`, per the round-2 finding) or any configured MCP server other than `ipfs mcp`.
  - The shipped profile pins `terminal_exec` off.
- **AG-UI chat interrupts.** An interrupt answered in the chat island counts as the human gate for the chat principal (kind `ui`), the same way elicitation does. Destructive tools additionally raise the native prompt. This default is revisited if G11-3 is falsified.
- **Expiry.** An expired or unanswered prompt is a denial ("Deny is final.").

**Assumptions and falsifiers (human gate).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| HG-1 | A same-user process cannot approve a pending write | Test: an agent with shell access (topology B UAR with `terminal_exec` enabled in a test-only profile, and a plain same-user script) tries to approve via AppleScript/`System Events`, `xdotool`, the control socket or the Desktop channel, and succeeds | Block that mechanism. Fall back to the presence-factor mechanism or refuse writes for non-elicitation principals on that platform. |
| HG-2 | `check-uar` sees UAR's tool inventory | The UAR settings or tools API does not expose `terminal_exec` or the MCP server list from the pinned release | Write groups are refused to topology-B principals (fail closed) |

### G11: React 19 island in the Web UI fork, Promethity, talking only to `ipfs mcp`; Desktop fork in scope

- **The `mcp/chat` package.**
  - A shadow-root custom element (React 19, Tailwind 4, shadcn, the A2UI renderer from `mcp/ui`, an AG-UI client), with Promethity tokens and fonts bundled.
  - No remote assets, no `unsafe-eval`.
- **Mount.** It is mounted in the GQAdonis/ipfs-webui fork behind an off-by-default Settings toggle and loaded by dynamic import. Desktop gets it through the bundled Web UI (D1).
- **Backend.**
  - The island talks **only** to `ipfs mcp --http` `/agui`, **never** to UAR.
  - `ipfs mcp` enforces an origin allowlist: `webui://localhost`, `http://127.0.0.1:5001`, `http://localhost:5001`, plus configured subdomain-gateway origins.
  - The chat principal is kind `ui`, with its own bearer token.
  - **Desktop:** the token is held by the main process with `safeStorage` and attached via a preload bridge, so it is never in page storage.
  - **Browser `/webui`:** the user enters it once, and it is held per-origin. This is the stated admin-origin risk, accepted by the operator.
- **Desktop fork (tribe-health/ipfs-desktop), in scope this phase.**
  - Owned by ui-ux-engineer, reviewed by security-reviewer.
  - It adds a CSP for `webui://localhost` whose `connect-src` lists the RPC and `ipfs mcp` origins.
  - It re-enables `webSecurity` (`desktop:src/webui/index.js:41-47`).
  - It adds the `safeStorage` key bridge.
  - It adds the Desktop confirmation window and its local channel to `ipfs mcp` (human gate).

**Assumptions and falsifiers (G11).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G11-1 | The shadow root isolates Tailwind 4 from Tachyons in browsers and Electron 44 | Visual regression: any Web UI page differs with the island loaded and closed, or the island renders unstyled in Electron 44 | Desktop-only side view (U3) for this phase |
| G11-2 | Re-enabling `webSecurity` and adding the CSP does not break Desktop | Desktop smoke test (files, explore, peers, settings, add, pin) fails with the change | Fix allowlists. Never ship with `webSecurity: false` restored silently; report the regression. |
| G11-3 | Chat interrupts can only be resolved by a user gesture | Security review or test shows script in the page resolves an interrupt without a user gesture | Require the native prompt for all chat writes |
| G11-4 | The admin-origin exposure is bounded | Security review finds the island bundle loads any remote resource, or the `ipfs mcp` token is readable from Desktop page storage | Block |

### G12: requirements, mechanisms and falsifiers stated; the component choice is made by plan revision 2's re-score

**G12 is not decided beyond this section.** The adjudication found that revision 2's E1 goal-fit score has the same defect as (d). Plan revision 2 re-scores G12 under the goal-fit gate and chooses which component performs embedding and storage: `ipfs mcp`, or UAR in topology B.

**Requirements** (binding, from G12 and the operator defaults):

- off by default;
- operator-configured embedding provider, local or remote, never a hardcoded endpoint;
- a remote embedder needs `egress_ack`;
- both embedded and remote SurrealDB modes;
- chunking, model, dimension, content types and trigger are configurable;
- storage keyed by CID;
- pin-aware deletion;
- no daemon-wide hook (E5 refused).

**Remote mode, concrete.** Either:

- `ipfs mcp` connects with `github.com/surrealdb/surrealdb.go`;
  - v1.7.0, published 2026-09-07 per `proxy.golang.org/.../@latest`;
  - Apache-2.0 per its `LICENSE`;
  - its `go.mod` requires only pure-Go modules (`fxamacker/cbor/v2`, `gorilla/websocket`, `lxzan/gws`, `golang-jwt/jwt/v5`, `gofrs/uuid`) [external, fetched 2026-10-09; cgo-freeness of the full build is unverified until plan revision 2 builds it];
  - the URL and namespace/database come from config, and credentials come from env or file;
- or UAR's remote configuration (`config.remote.surreal.yaml`) in topology B.

**Embedded mode, concrete.** Either:

- **(i) a managed local `surreal` process.**
  - `ipfs mcp` starts `surreal start --bind 127.0.0.1:<ephemeral> surrealkv://<operator data dir>` with credentials generated per start, and stops it on exit. It connects with `surrealdb.go`.
  - It runs only when embedded mode is configured.
  - The `surreal` binary path is operator-configured. Kubo never downloads it or ships it (BSL 1.1 stays outside Kubo's artifacts).
- **or (ii) UAR-embedded surrealkv** (`uar:src/config.rs:1439-1440`) in topology B.

`surrealdb.c.go` (cgo plus a Rust native library) is excluded: Kubo has no cgo.

**Embedding computation.** Either:

- an operator-configured OpenAI-compatible `/embeddings` endpoint called by `ipfs mcp` (a local server, or UAR's endpoint if it exposes one [unverified]);
- or UAR's FastEmbed in topology B.

There is no default.

**Trigger.**

- On a successful `ipfs_add` from any `ipfs mcp` transport (stdio, HTTP, AG-UI chat, A2A), when indexing is configured.
- Subject to the MIME allowlist and a size cap.
- Plus an explicit "index this CID" tool.
- **Limitation:** WebMCP tools in the browser call the RPC directly, not through `ipfs mcp`, so content they add is not indexed unless re-indexed explicitly.

**Deletion.**

- Delete a CID's vectors only when `pin ls --type=all <cid>` reports no direct, recursive or indirect pin.
- On `repo_gc`, only for CIDs that GC reports as removed.
- Operator-triggered reconciliation for content unpinned outside `ipfs mcp`.

**Acceptance tests per mode** (plan revision 2 turns them into tasks):

| Mode | Test |
|---|---|
| Disabled (default) | No `surreal` process spawned. No connection to any store or embedder (network capture). Index tools absent. |
| Embedded (i) | Start and stop lifecycle, including crash cleanup. Loopback-only bind. Data under the configured directory. Ingest, then search, then pin-aware delete against a harness daemon. Zero outbound connections with a local embedder. |
| Embedded (ii) / remote via UAR (topology B only) | The same flow through the pinned UAR release, gated like topology B. |
| Remote (surrealdb.go) | ws/wss URL from config, credentials from env, the same flow against a real `surreal` server started by the test. Refusal when the embedder host is not in `egress_ack`. |

**Assumptions and falsifiers (G12).**

| # | Assumption | Falsifier (observable) | Consequence |
|---|---|---|---|
| G12-1 | `surrealdb.go` builds into `ipfs` without cgo and at an acceptable size | `CGO_ENABLED=0 make build` fails with it, or the measured binary delta exceeds the limit plan revision 2 sets | Remote and embedded (i) via `ipfs mcp` are unsupported; topology B only, or G12 deferred |
| G12-2 | SurrealDB vector search (HNSW) works for the configured dimension via `surrealdb.go` | A real-service test cannot define or query an HNSW index from Go | In-memory fallback is not accepted; G12 deferred |
| G12-3 | The managed process can be supervised safely | The test leaves an orphaned `surreal` process after `ipfs mcp` is killed, or it binds non-loopback | Embedded (i) unsupported until fixed |
| G12-4 | Deletion stays consistent with the pin state | A CID pinned twice (recursive root and direct) loses its vectors when one pin is removed | Block |

---

## ADR DRAFTS

The plan stage materializes these in `docs/mcp/`, after ADR-0001 to ADR-0003. The revision-2 drafts are kept under "SUPERSEDED (2026-10-09)".

### ADR-0004 draft: AG-UI and A2A are served by `ipfs mcp`, and model calls are proxied to an operator-configured endpoint

- **Status:** Proposed, decision of record per operator answers (2026-10-09). Accepted once plan revision 2 is approved.
- **Context.**
  - G9 and G10.
  - F4 keeps Kubo's agent surface in `ipfs mcp` (RPC-only, no daemon change).
  - AGENTS.md: no default endpoints, an off switch for every feature, no tracking.
  - UAR (MIT) is non-conformant to A2A 1.0/AG-UI today, and its approval can be bypassed when governance is disabled (Summary 2, 4).
  - The goal-fit gate excludes UAR-hosted endpoints while UAR's fixes are undelivered (adjudication).
  - The operator chose "Both".
- **Decision.**
  - **Topology A.** `ipfs mcp --http` (default `127.0.0.1:5050`) serves AG-UI (`/agui`, spec interrupts) and A2A 1.0 (`/.well-known/agent-card.json`, `/a2a`, input-required and auth-required). Both reuse the `AuthVerifier`, the scope map and the confirmation gate in-process.
  - **The loop and the model.** A bounded loop over contract tools calls an operator-configured OpenAI-compatible endpoint (UAR or another server). There is no default URL or model, and the key comes from env or file.
  - **Egress.** `contentEgress` results reach a model only for hosts in `egress_ack`.
  - **Human gate.** For non-eliciting clients, a native OS prompt with a presence factor. CLI confirm is headless opt-in only. Shell-capable tools are refused for write principals.
  - **Topology B.** UAR-hosted, over `--http` only, with stdio forbidden. Supported once UAR's fixes ship in a published release by the end of Round 4; otherwise deferred and reported unmet.
  - **Off switches.** `--agui`/`--a2a` are off by default and mounted only with a configured backend; `noagent` compiles them out.
- **Consequences.**
  - **Positive:** conformance is under this team's control; one gate and one auth path; works with any OpenAI-compatible server, local or remote; UAR is optional.
  - **Negative:**
    - The fork now carries an LLM client and a minimal agent loop (maintenance in this repo).
    - Possible new root `go.mod` modules if an SDK is taken (G9-1).
    - The egress guarantee is limited to what `egress_ack` and `check-uar` can see.
    - Native prompts need per-OS implementations.
- **Rejected or deferred alternatives:**

  | Alternative | Rejected or deferred because |
  |---|---|
  | (d) UAR-hosted as primary | Excluded by the goal-fit gate while UAR's fixes are undelivered. Kept as topology B. |
  | (b) daemon listener | C1 plus an LLM client in the daemon |
  | (c) separate binary | A second product in the repo |
  | L1 UAR-only backend | Fails G10's "or an OpenAI-compatible server" |
  | Token fallback or a 0600 socket as the human gate | Reachable by same-user processes (round-2 finding) |
  | Stdio-spawned UAR | Egress unenforceable |

### ADR-0005 draft: chat UI as a React 19 island in the Web UI fork, talking only to `ipfs mcp`; Desktop hardened

- **Status:** Proposed, decision of record per operator answers (2026-10-09).
- **Context.**
  - G11.
  - The Web UI is React 16 / CRA 5 / Tachyons with React-capped dependencies (Summary 6).
  - The Web UI origin holds admin RPC.
  - Desktop has no CSP and runs `webSecurity: false`.
  - The operator chose the Web UI island, Promethity, and the Desktop fork in scope.
- **Decision.**
  - `mcp/chat` is a shadow-root React 19 element (Tailwind 4, shadcn, A2UI, an AG-UI client, Promethity), mounted off by default in the Web UI fork and reaching Desktop through D1.
  - It talks only to `ipfs mcp` `/agui`, behind an origin allowlist and a `ui` principal token.
  - In the Desktop fork: CSP, `webSecurity` re-enabled, a `safeStorage` token bridge, and the native confirmation window.
  - Owner: ui-ux-engineer. Reviewer: security-reviewer.
- **Consequences.**
  - **Positive:** the fork stays near upstream; one component set; brand control; no browser credential to UAR.
  - **Negative:**
    - The chat runs in the admin origin (operator-accepted risk, bounded by G11-3/G11-4).
    - Two React copies when enabled.
    - Desktop fork divergence (CSP, `webSecurity`).
- **Rejected alternatives:**

  | Alternative | Rejected because |
  |---|---|
  | U1 upgrade the fork | Rewrite-sized |
  | U3 Desktop-only | Leaves browser users out. It is the flip if G11-1 fails. |
  | U4 iframe UAR | Not IPFS-specific |
  | Direct-to-UAR from the browser | UAR CORS is not configurable; the operator chose "never directly to UAR" |

### ADR-0006 draft: G12 requirements and mechanisms; component choice deferred to plan revision 2

- **Status:** Proposed, requirements only. The component choice (`ipfs mcp` or UAR topology B) is recorded by plan revision 2's re-score under the goal-fit gate. This ADR does not claim G12 decided.
- **Context.**
  - G12 requires both embedded and remote SurrealDB, an operator-configured embedder, off by default.
  - Embedded SurrealDB from Go via `surrealdb.c.go` needs cgo; Kubo has none.
  - `surrealdb.go` v1.7.0 is an Apache-2.0 network client.
  - The SurrealDB engine is BSL 1.1.
  - Kubo has no pin-removal event stream.
- **Decision.**
  - **Remote mode:** `surrealdb.go` from `ipfs mcp`, or UAR remote config (topology B).
  - **Embedded mode:** a managed local `surreal` process (surrealkv, loopback, operator-provided binary, started and stopped by `ipfs mcp` only when configured), or UAR-embedded surrealkv (topology B).
  - **Embedding:** an operator-configured OpenAI-compatible endpoint or UAR FastEmbed. There is no default, and a remote embedder needs `egress_ack`.
  - **Trigger:** `ipfs_add` through `ipfs mcp` or an explicit index tool.
  - **Deletion:** pin-aware.
  - **Refused:** a daemon-wide hook. Excluded: cgo.
- **Consequences.**
  - **Positive:** no cgo and no BSL code in Kubo artifacts; both modes have a concrete mechanism and per-mode acceptance tests.
  - **Negative:**
    - Kubo (`ipfs mcp`) may supervise a third-party process (embedded (i)).
    - WebMCP-added content is not indexed automatically.
    - Content unpinned outside `ipfs mcp` stays indexed until reconciliation.
    - Egress for a remote embedder is limited to the `egress_ack` control.

---

## IMPACT ON F4 AND THE CURRENT PLAN

This section replaces the revision-2 plan impact (kept under "SUPERSEDED (2026-10-09)").

### F4 itself

- **Placement unchanged.** `ipfs mcp` stays a local, RPC-only command.
- **Additions inside `ipfs mcp`:**
  - the `--http` default address `127.0.0.1:5050`;
  - AG-UI and A2A routes;
  - a bounded agent loop and an OpenAI-compatible client;
  - principal policy, `egress_ack`, the native confirmation prompt and `check-uar`;
  - optional G12 indexing with a managed `surreal` process or a remote client.
- **Build tag.** `noagent` removes everything except MCP.
- **Still unchanged:** the daemon, `config/`, `docs/config.md`, `/api/v0` and the gateway.
- **Possible new root `go.mod` modules:** `surrealdb.go` (G12) and a protocol SDK only if G9-1 requires one. Operator answer 2 covered go-sdk only, so plan revision 2 must get explicit approval for each.

### Changes to existing plan items (22)

| # | Change | Impact |
|---|---|---|
| 0 | team-ownership-grant | Add grants:<ul><li>ui-ux-engineer: `mcp/chat/**`, Web UI `src/agent-chat/**`, and the Desktop fork (`tribe-health/ipfs-desktop`: `src/webui/**`, the confirmation window, CSP)</li><li>architect: `mcp/uar/**`</li><li>mcp-server-engineer: `mcp/server/**` (already granted) and `test/cli/mcp_agent_*_test.go`</li></ul>The batched question now only confirms the go.mod additions and the remaining defaults. |
| 1 | mcp-tool-contract | Add `contentEgress`, A2A skill metadata, A2UI binding hints, principal kinds (`human-mcp`, `ui`, `a2a`, `uar`) and scopes, and the `egress_ack` and LLM-backend config schemas. |
| 2 | mcp-architecture-adrs | Add ADR-0004 and ADR-0005 (decision of record) and ADR-0006 (requirements; status completed after plan revision 2's re-score). |
| 6 | mcp-pinning-with-confirmation | The gate state machine supports external resolution (native prompt, AG-UI interrupt, A2A input-required). The token fallback is not accepted from non-eliciting configured principals. |
| 7 | mcp-http-transport-auth | Default `127.0.0.1:5050`; configured principals with kinds; path-scoped auth for `/mcp`, `/agui` and `/a2a` (G9-4). |
| 5 | mcp-ui-component-library | Chat primitives (message stream, tool-call card, interrupt/approval card, egress badge), shadow-DOM-safe. |
| 16 | webui-webmcp-integration | Sequence it before N5 (shared Settings). |
| 20 | mcp-user-docs | Agent endpoints, LLM backend config (no defaults), `egress_ack`, the native prompt, topology B, G12 modes, and the limitations. |
| 21 | phase-boundary-review | All falsifier tests from DECISION. Security review of the loop, the proxy, the prompts, the Desktop changes and the chat bundle. |
| others | 3, 4, 8-15, 17-19 | No change. |

### New changes (12, three conditional)

| New change | Owner | Depends on | Round | Notes |
|---|---|---|---|---|
| N1 `uar-conformance-request` | architect (`team-request`; no UAR edits) | team-ownership-grant | 1 | For topology B and the UAR backend (G10-1). Delivery is evaluated against **published releases** at the end of Round 4. |
| N4 `agent-chat-island` | ui-ux-engineer | mcp-ui-component-library | 3 | Island and AG-UI client, Promethity. G11-1 and G11-4 tests. |
| N8 `mcp-agent-principal-enforcement` | mcp-server-engineer | mcp-http-transport-auth, mcp-pinning-with-confirmation | 4 (merges after http-transport-auth) | Principal kinds, `egress_ack`, `check-uar` (per session and on settings change), shell-tool refusal for write principals, the stdio `clientInfo` guard. |
| N14 `native-confirmation-prompt` | mcp-server-engineer | mcp-pinning-with-confirmation | 4 | macOS and Linux presence-factor prompts, the Desktop pairing channel protocol, `--confirm=cli` headless opt-in. HG-1 tests. |
| N11 `mcp-llm-proxy-loop` | mcp-server-engineer | N8 | 5 | G10 L3: OpenAI-compatible client and bounded loop, no defaults, key from env or file. Tests G10-1 to G10-4 against UAR (pinned release) and one local server. |
| N12 `mcp-agui-endpoint` | mcp-server-engineer | N11, N14 | 6 | `/agui` with spec events and interrupts. G9-1 and G9-4. |
| N13 `mcp-a2a-endpoint` | mcp-server-engineer | N11, N14 | 6 | A2A 1.0 agent card and JSON-RPC, input-required and auth-required. G9-1, G9-2 and G9-4. |
| N6 `desktop-agent-chat` | ui-ux-engineer (security-reviewer reviews) | N4, N14 | 6 | Desktop fork: CSP, `webSecurity` re-enabled, `safeStorage` bridge, the confirmation window. G11-2. |
| N7 `mcp-embeddings-surrealdb` | per plan revision 2's re-score (mcp-server-engineer if `ipfs mcp`) | N8, N11 | 6 | G12 modes and the per-mode acceptance tests in DECISION, the trigger (it absorbs the former N9 hook), pin-aware deletion, reconciliation. G12-1 to G12-4. |
| N5 `webui-agent-chat` | ui-ux-engineer | N4, N12, webui-webmcp-integration (sequencing) | 7 | Mount, toggle, i18n, a real run against `ipfs mcp`. G11-3. |
| N2 `uar-integration-spike` (CONDITIONAL: the topology-B gate passes) | mcp-server-engineer, plus the operator for the setup measurement | gate at the end of Round 4, N8, N14 | 5 | Topology B with the pinned published UAR over `--http`: the A2A-delegated write (G9-2 analogue), the setup measurement (G10-5), defaults with and without the profile, `terminal_exec` refusal (HG-2). |
| N3 `agentic-uar-integration` (CONDITIONAL: N2 GO) | architect + mcp-server-engineer | N2 | 6 | Shipped `mcp/uar/profile/` with the pinned minimum version, and the topology-B e2e tests. |

**Removed or superseded:** revision-2 N9 (`mcp-embedding-hook`, folded into N7) and N10 (`mcp-agui-proxy`, replaced by N12).

### Count and rounds

- **Count:** 22 existing + 12 new = **34 changes**. Three are conditional: `webui-cid-delivery`, N2 and N3.
- **Rounds:**

  | Round | Additions |
  |---|---|
  | 0 | Grant |
  | 1 | N1 |
  | 3 | N4 |
  | 4 | N8, N14 (Round 4 now has 9 parallel changes; the `plan.md` T1 merge risk rises; merge order is http-transport-auth, then N8, then N14, then the tool groups) |
  | End of Round 4 | **Topology-B gate** (published UAR release) |
  | 5 | N11, N2 (conditional) |
  | 6 | N12, N13, N6, N7, N3 (conditional) |
  | 7 | N5 (alongside marketplaces and user-docs; user-docs now also depends on N11-N13 and N7) |
  | 8 | Review |

- **Critical path.** pinning (R3), then http-transport-auth and N8 (R4), then N11 (R5), then N12 (R6), then N5 (R7), then review (R8). That fits the existing 0-8 round structure, so the phase does not get longer.
- **Failure isolation.** A failed topology-B gate removes only N2 and N3. Failed G9-G12 falsifiers block or defer only their own changes. G1-G8 are unaffected.

## RISKS AND OPEN QUESTIONS

### Top risks (including risks against this addendum's own decisions)

> **Note (2026-10-09).** Risks 1, 2 and 10 were written against the revision-2 UAR-hosted primary. Under the revision-3 decision:
>
> - The main risks are the maintenance of an LLM client and agent loop in the fork, the per-OS native-prompt implementations, and possible new root `go.mod` modules.
> - UAR delivery affects only topology B and the UAR backend.
> - Risks 3-9 stand as written for topology B, with the egress limitation now restated in DECISION G10.

1. **Dependency on another team.** Conformance, CORS, elicitation, safe defaults and ANN search all need UAR changes. The default on non-delivery is now deferral, which means **G9-G12 may deliver nothing in this phase**. That is the honest cost of the safe default.
2. **Setup cost.** It is measured in N2 against an explicit threshold (A10.2). If it fails, the principle itself is in question, and L2 becomes the likely answer.
3. **UAR's unsafe and non-configurable defaults.**
   - `0.0.0.0`, the remote model, the remote memory embedder, CORS `Any` (not configurable) and the governance bypass in local mode.
   - N8 enforces the node-data boundary only. A UAR instance an operator runs raw is still unsafe for other reasons, outside Kubo's control.
4. **Out-of-band confirmation UX.**
   - One extra human step per write, on a second surface (CLI or the loopback page).
   - This is safer than relying on UAR, but it may still drive rubber-stamping.
   - The control socket and the confirmation page are new attack surface in `ipfs mcp`: CSRF on the page, socket permissions.
5. **Egress over stdio is not enforceable.** A UAR or any host spawning `ipfs mcp` over stdio sends whatever content the agent reads to the host's model. This is documented as outside the guarantee, which is the same position as every other stdio MCP host today.
6. **`check-uar` couples Kubo to UAR's settings API shape.** A UAR release that changes it makes the principal fail closed, which is safe but disruptive. A10.3 records whether verification is possible at all.
7. **Chat in the admin origin.** Through the L3 proxy, the key is no longer in the browser, but an XSS in the island is still admin RPC.
8. **Embeddings drift and coverage.** Index drift from out-of-agent unpins, and non-UAR surfaces are not indexed without N9.
9. **SurrealDB BSL 1.1** is inherited by UAR deployers.
10. **The weighted margins for G9 and G11 are not decisive.** The decisions rest on qualitative arguments (maintenance location, reach) that a reasonable operator could weigh differently.

### Questions only the operator can answer

> **Answered 2026-10-09.**
>
> - Q1, Q2, Q4, Q6 and Q7 by the operator (binding).
> - Q3, Q5 and Q8-Q11 by orchestrator defaults the operator may overrule.
>
> See `operator-answers.md` and the appended "Answers to the G9-G12 addendum". The questions are kept as asked. New open items for plan revision 2:
>
> - approval of any new root `go.mod` modules (`surrealdb.go`, any protocol SDK);
> - the G12 component choice from the re-score;
> - the local OpenAI-compatible server used for G10-2.

1. **UAR as a required component.** Do you accept UAR as the runtime for G9-G12, with Kubo adding the N8 enforcement but no LLM client, vector store or chat backend? A "no" moves to (a)+L2 for G9/G10 and to E3 or deferral for G12.
2. **G10 narrowing.** Should "OpenAI-compatible server plus key" run through UAR (L1, narrows G10), through a Go loop in `ipfs mcp` (L2, about +2 changes), or behind an `ipfs mcp` proxy (L3, +1 change, also the G11 browser boundary)?
3. **Delivery gate.** The default at the end of Round 4, if UAR has not delivered the conformance items, is to defer G9 conformance and report it unmet. Do you instead want, in advance, the Kubo fallback (a) or an explicitly non-conformant release?
4. **Out-of-band confirmation.** Is a local CLI (`ipfs mcp confirm`) plus a loopback confirmation page acceptable as the human gate for agents without elicitation, including UAR? That reverses the plan's cut of this channel for these principals.
5. **Egress.** Is per-principal `egress_ack` (naming the model and embedder hosts) the right control, and should content tools stay refused for every non-elicitation principal without it?
6. **Chat placement and brand.** Web UI island (reaching Desktop) or Desktop-only (U3)? Promethity or IPFS-family branding inside the Web UI?
7. **Desktop fork.** Is `tribe-health/ipfs-desktop` in scope for this phase (CSP, key bridge)? Which role owns it?
8. **Embedding scope.** Embed only UAR-routed agent actions, or also enable the opt-in N9 hook so content added from stdio hosts, the MCP App and WebMCP is indexed? And is the pin-aware deletion plus operator-triggered reconciliation acceptable?
9. **SurrealDB license.** Is SurrealDB's BSL 1.1 acceptable for your deployments, through UAR?
10. **Remote modes.** Are a remote SurrealDB or a remote embedder needed this phase, or is local-only enough?
11. **Setup threshold.** Is the setup-cost threshold in A10.2 (30 minutes or 15 steps from published artifacts) the right falsifier, or do you want a different number?

---

