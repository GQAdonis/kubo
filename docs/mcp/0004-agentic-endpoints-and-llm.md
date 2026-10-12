# ADR-0004: AG-UI and A2A are served by `ipfs mcp`, model calls are proxied to an operator-configured endpoint, and non-eliciting writes need a human-presence prompt

## Status

Accepted, 2026-10-09, per the operator's binding answers in `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`:

- "Answers to the G9-G12 addendum", agent architecture (addendum Q1, Q2): "**Both.** AG-UI and A2A endpoints are served by `ipfs mcp` now (option (a)), reusing its AuthVerifier, scope map and confirmation gate in-process; when an LLM is needed `ipfs mcp` proxies to an operator-configured UAR instance or an OpenAI-compatible server with a key from the environment (L3; no default, off unless configured). UAR-hosted AG-UI/A2A calling `ipfs mcp` as a tool server is a second supported topology once UAR's conformance fixes ship in a published release."
- Same section, human gate (Q4): native OS prompt; CLI confirm only for headless servers behind an explicit opt-in; shell-capable agent tools refused for principals with write scope.
- Orchestrator defaults the operator may overrule: Q3 (topology-B delivery gate), Q5 (`egress_ack`), Q11 (setup threshold).
- "Round 0 answers": Q1 approved (`127.0.0.1:5050`); R3 fail-closed human gate accepted; R5 local models `qwen3:8b` (tool-calling chat) and `bge-m3` for real-service tests.

## Date

2026-10-09

## Context

- Goals G9 (AG-UI and A2A endpoints for the IPFS services) and G10 (an LLM backend behind them: an operator-configured Universal Agent Runtime (UAR) instance or an OpenAI-compatible server, no default endpoint, off unless configured, credentials never hardcoded) in `.kbd-orchestrator/phases/ipfs-mcp-server/goals.md`.
- ADR-0001 (F4) keeps Kubo's agent surface in `ipfs mcp`: RPC-only, no daemon listener, no Kubo config key.
- `AGENTS.md`: no default endpoints, an off switch for every feature, no tracking, no silent CID injection, no appropriation of the operator's resources.
- UAR (MIT, read-only at HEAD `a7cb9729`, 2026-10-01) is not conformant today (`assessment-addendum.md`, Summary 2-4; spot-checked again on 2026-10-09):
  - the A2A agent card is served at `/.well-known/agent.json` (`uar:src/uar/api/a2a/mod.rs:75`), not the A2A 1.0 path `/.well-known/agent-card.json`;
  - `TaskState` serializes with `rename_all = "lowercase"` (`uar:src/uar/api/a2a/types.rs:118`), so `InputRequired` goes on the wire as `"inputrequired"`;
  - tool approval goes out as the AG-UI `CUSTOM` event `uar.tool.approval_required` (`uar:src/uar/api/adapters.rs:373`), not a spec interrupt;
  - CORS is `allow_origin(Any)` (`uar:src/server.rs:2530`) and has no configuration key;
  - defaults: bind `0.0.0.0` (`uar:src/config.rs:1412`), model `openai/gpt-4o` (`:1476`), memory embedder `openai` (`:1464`);
  - approval is bypassed when the governance gate is not effectively enabled (`uar:src/uar/runtime/manager.rs:647-652, 5469-5480`; `uar:src/uar/governance/runtime_control.rs:455-478, 662-663`).
- Protocols [external, fetched 2026-10-09]: A2A 1.0.0 (`a2aproject/A2A` `docs/specification.md`: agent card at `/.well-known/agent-card.json`; task states include input-required and auth-required). AG-UI interrupts (https://docs.ag-ui.com/concepts/interrupts): `RUN_FINISHED` carries `outcome: {type: "interrupt", interrupts: [...]}`, and the client resumes with a new run whose input carries `resume: [...]`.

### Goal-fit re-scoring that excluded UAR-hosted-only

The addendum's scoring method (`assessment-addendum.md`, "Scoring method") excludes any option with goal fit at or below 2 before aggregation, and does not credit work that needs UAR-team delivery. Revision 2 nonetheless gave UAR-hosted endpoints (d) goal fit 4 and selected it. The round-2 review flagged this as CRITICAL, and the orchestrator upheld it (`assessment-addendum.md`, "Orchestrator adjudication" item 1):

- Without the UAR conformance items, the addendum itself reports G9 unmet (its A9.1 consequence and its "What Kubo cannot enforce" table). Scored as UAR is, (d) has goal fit 2 and is **excluded**.
- Of the remaining options, (a) AG-UI and A2A in `ipfs mcp` scores 51 and (c) a separate binary 49 (weights: goal fit 3, Kubo surface 2, rules fit 3, security 3, maintenance 2, time 1, interop 1; maximum 75). (b) daemon listener scores 33. (e1), an LLM-free A2A layer, was already excluded at goal fit 2.
- The 2-point margin between (a) and (c) is within the method's 3-point "not decisive" band. (a) is chosen on the qualitative argument that (c) is a second product with its own release tooling (ADR-0001 rejected the same shape as B), and because the operator chose (a).

The same re-scoring applies to the LLM backend. In the addendum's G10 table, L1 (UAR is the only loop) scored 42, L3 (proxy in `ipfs mcp`) 40 and L2 (a Go loop in `ipfs mcp`) 39: not decisive. L1 narrows G10, which names an OpenAI-compatible server as an alternative to UAR. The operator chose the loop-in-`ipfs mcp` shape with proxying (recorded as "L3" in the operator answer and the revision-3 decision). Note what that label carries: the bounded loop is L2's cost, and the proxy to UAR or another OpenAI-compatible server is L3's boundary.

## Decision

### Topology A (primary, delivered this phase)

**One listener.**

- `ipfs mcp --http` serves, on distinct paths:
  - `/mcp`: MCP Streamable HTTP (ADR-0001);
  - `/agui`: AG-UI;
  - `/a2a`: the A2A 1.0 JSON-RPC binding;
  - `/.well-known/agent-card.json`: the A2A agent card.
- Default address `127.0.0.1:5050`, overridable with `--http=<multiaddr>` and an environment variable. No component defaults to 4000 (the local liter-llm gateway), 4001, 5001 or 8080, or to the other reserved ports in `plan.md` D7. The contract config schema rejects reserved default ports.
- The ADR-0001 bind rules carry over: a non-loopback bind is refused without a configured verifier.
- All three protocols authenticate through the same `AuthVerifier` and authorize through the same scope map (ADR-0003), with path-scoped authorization.

**AG-UI.**

- `POST /agui` accepts `RunAgentInput` and streams spec events over SSE.
- A pending confirmation ends the run with `RUN_FINISHED` and `outcome: {type: "interrupt", interrupts: [...]}`. The client resumes with `resume: [{interruptId, status, payload}]`.

**A2A 1.0.**

- The agent card is at `/.well-known/agent-card.json`; its skills are generated from the contract's A2A skill metadata.
- The JSON-RPC binding at `/a2a` supports sending and streaming messages, getting tasks and cancelling tasks, under the A2A 1.0 method names.
- A pending confirmation puts the task in the input-required state; a missing or insufficient credential puts it in the auth-required state.
- No A2A message can approve a write. An A2A caller is an agent, so its writes are resolved by the native prompt.

**Protocol implementation.** Hand-written types generated from the published spec schemas, over `net/http`, with no new SDK. An SDK (`a2a-go/v2`, a community AG-UI Go SDK) is taken only if schema validation fails and cannot be fixed (falsifier G9-1) and the operator approves the root `go.mod` addition after a measured dependency review. Operator answer 2 covered go-sdk only.

### The bounded loop and the model proxy (G10)

- **The loop.**
  - Runs in `ipfs mcp` for topology A.
  - Has a step limit, a token budget and a wall-clock limit, all configurable with conservative defaults.
  - Its tools are only the contract tools enabled for the calling principal. It has no shell, filesystem or network tools.
  - No memory, planning or multi-agent features this phase (`plan.md` section 3).
- **The model endpoint.** The OpenAI-compatible chat-completions API with `tools` (`POST <base_url>/chat/completions`). UAR's `/v1/chat/completions` (`uar:src/server.rs:1901`) and any other OpenAI-compatible server qualify.
- **Configuration.**
  - `--llm-base-url`, `--llm-model`, and `--llm-api-key-env=<VAR>` or `--llm-api-key-file`, or the same keys in the `ipfs mcp` config file (`--mcp-config=<path>` or `IPFS_MCP_CONFIG`; not the root `--config`, which names Kubo's repo config).
  - **There is no default URL and no default model.** A literal key fails validation.
  - Unset means `/agui` and `/a2a` are not mounted, and no model call is ever made.
- **Off switches.** `--agui` and `--a2a` are off by default and are mounted only when a backend is configured. The `noagent` build tag compiles out AG-UI, A2A, the loop and the LLM client while keeping MCP. `nomcp` compiles out all of it.

**Egress, enforced in-process.**

- A tool result whose contract entry has `contentEgress: true` (`ipfs_cat`, `ipfs_get`, `dag_get`, `ipfs_ls` and others) is put into a model request only if the backend's host is listed in the principal's `egress_ack`.
- Otherwise the loop sends the model, and shows the user, a structured refusal: "Content not sent: `<host>` is not in egress_ack." It never substitutes an empty result.
- With a UAR backend, `check-uar` (below) also verifies UAR's effective upstream provider, per session, where UAR's settings API exposes it.

**Stated limitation.** Kubo cannot see what an acknowledged endpoint does with data it receives. The cases are: a loopback server that forwards upstream; a UAR whose effective provider is not readable; stdio MCP hosts (Claude Code, Codex, OpenCode) that send tool results to their own models; WebMCP tools running in the browser. The operator's `egress_ack` is the control, and the docs say so. **This ADR does not claim that no content leaves the machine in general.**

### Principals

Every HTTP client is a configured principal in the `ipfs mcp` config file named by `--mcp-config` (`mcp-agent-principal-enforcement`). The one exception is a loopback listener with no credential configured at all: it serves the read-only `anonymous-loopback` principal (ADR-0003, Round 4 amendment section 3).

- Kinds: `human-mcp`, `ui` (the chat island, ADR-0005), `a2a`, `uar`.
- Read-only by default; write and destructive groups granted by name (ADR-0003). A token that maps to no configured principal gets nothing.
- `contentEgress` tools are refused to a principal without `egress_ack`, with a structured error.

### Human gate (applies to topology A, topology B and the chat)

- **Order.** Clients that can elicit keep MCP elicitation (ADR-0003). For every principal that cannot elicit, writes and destructive calls wait in the confirmation broker (expiry is denial, execution exactly once) for a **native OS prompt with a human-presence factor** that a same-user background process cannot simulate:
  - the paired IPFS Desktop confirmation window, when Desktop runs and is paired with `ipfs mcp` over a local authenticated channel (pairing key in Desktop's `safeStorage`);
  - otherwise a macOS LocalAuthentication prompt (Touch ID or password) or a Linux polkit authentication-agent prompt raised by `ipfs mcp`.

  Each names the tool, the CID, path or name, and the effect.
- **Fail closed (Round 0 R3).** Linux needs a one-time operator-installed polkit action, documented and never installed by Kubo; without it, non-eliciting writes are refused. Windows refuses unless Desktop is paired. The refusal is `confirmation_unavailable`.
- **Default and off switch (Round 4).** The gate runs by default with `native,desktop` (`--confirm`, then `IPFS_MCP_CONFIRM`, then config `confirmation.modes`). `IPFS_MCP_CONFIRM=none` turns it off; per-call confirmation still applies, so only eliciting clients can then confirm writes and every other write is refused with `confirmation_unavailable`. A mechanism the host cannot offer is skipped.
- **Native prompt requirement (Round 4).** `requires_native_prompt` is satisfied only by presence resolvers: `macos-localauth`, `linux-polkit` or `desktop`. `cli` does not satisfy it (ADR-0003, Round 4 amendment section 1).
- **Headless only.** `ipfs mcp confirm <id>` and `deny <id>` exist only behind `--confirm=cli`. They need a TTY and print, at startup and per request, that same-user processes can reach them. Under `--confirm=cli`, every write principal needs the operator assertion `shell_capable: false`; otherwise its write groups are refused.
- **Shell-capable tools.** They are refused for principals with write scope. The topology-A loop has none. In topology B, `check-uar` refuses write groups when UAR reports `terminal_exec` enabled (`uar:src/config.rs:2525`) or any configured MCP server other than `ipfs mcp`.
- **Chat interrupts.** An AG-UI interrupt answered by a user gesture in the chat island counts as the gate for the `ui` principal, as elicitation does. Destructive tools additionally raise the native prompt. This is revisited if falsifier G11-3 (ADR-0005) fails.

### Topology B (second supported topology, gated)

- **What it is.** UAR hosts AG-UI and A2A and calls `ipfs mcp --http` as a configured `kind: uar` principal: read-only default groups, `egress_ack`, the native prompt for writes, and `check-uar`.
- **Stdio is forbidden.** Over stdio, `ipfs mcp` cannot identify the client or enforce egress. The shipped profile (`mcp/uar/profile/`, `agentic-uar-integration`) never spawns `ipfs mcp`. `ipfs mcp` refuses `contentEgress` tools to a stdio client whose MCP `clientInfo` identifies UAR. That is a misconfiguration guard, not a security boundary, because `clientInfo` is self-reported.
- **`check-uar`.** `ipfs mcp check-uar <url>` runs at startup, at every session initialize and when UAR reports a settings change. It checks: the agent-card version is at or above the pinned minimum from a published release; authentication is required; the effective model, embedders and governance state; shell-tool refusal. Unreadable settings fail closed unless the operator records `check: assert`. A failed check disables that principal only. A UAR bound to `0.0.0.0` answers identically on loopback, so `check-uar` does not claim to detect the bind; the shipped profile is the supported configuration.
- **Delivery gate.** Topology B is supported only if UAR's conformance items are **contained in a published, installable UAR release (a tag plus an image digest or a native package) by the end of Round 4**. Merged-but-unreleased items do not count. The requested items and their checks are in `mcp/uar/requests/CONFORMANCE.md`. The gate result is recorded by `uar-integration-spike` task 1.1. If it fails, topology B is deferred and reported unmet; there is no non-conformant B release, and topology A is unaffected.

### The cost of two topologies

Topology B accounts for `uar-conformance-request`, `uar-integration-spike` and `agentic-uar-integration`, plus the `check-uar` and stdio-guard parts of `mcp-agent-principal-enforcement`: 3 changes and about 14 tasks, and a standing coupling to the shape of UAR's settings API (`check-uar` fails closed when it changes). The plan keeps B because the operator chose "Both", and makes it conditional so it cannot delay topology A (`plan.md` D1).

## Amendment (2026-10-09, Round 6: egress scope, UAR backend check, loop interrupts)

Raised by `mcp-server-engineer` after implementing `mcp-llm-proxy-loop`.

- **What `contentEgress` covers.** A result is content egress when it carries stored bytes, names taken from stored data, or an enumeration of what the node stores (the pinset, an MFS listing, index hits). A result about one item the caller already named (`mfs_stat`, `dag_stat`) or about node health is not. `pin_ls` stays `contentEgress: true`: the pinset and the operator's pin names say what the operator stores, which is exactly what must not reach a model host the operator did not acknowledge. So a `ui` principal with an empty `egress_ack` cannot list pins through the agent loop; that is intended, and the fix for an operator is to add the backend host (or `local`) to that principal's `egress_ack`. `pin_verify` returned the same CIDs with `contentEgress: false`, which let the pinset out through a second tool; it is now `true` (`mcp/contract/catalog.json`, rule in `catalog.schema.json`, pinned by `TestPinsetEnumerationIsContentEgress`). `swarm_peers`, `routing_*` and `key_list` are unchanged here; whether peer and key enumerations need the same treatment is an open question for the operator, not decided by this amendment.
- **UAR as the topology-A backend.** The per-session check of UAR's effective upstream provider, stated under "Egress, topology A", is moved out of `mcp-llm-proxy-loop` into `agentic-uar-integration` (task 1.5). It needs the pinned UAR settings endpoint (ADR-0007), which no published UAR release provides, so it is deferred with topology B (ADR-0008). Until then a UAR backend is an ordinary backend: its host must be in `egress_ack`, and the limitation in "Where this is weak" applies.
- **One interrupt per run.** The loop answers the tool calls of one model reply in order and ends a run at the first call that needs confirmation; later calls of that reply wait in the session queue. A run therefore carries at most one interrupt, and a resume answers exactly that interrupt. AG-UI and A2A adapters follow this; mixed resume batches cannot arise (`mcp-agui-endpoint` design, fixtures `two-calls-*.json`).

## Amendment (2026-10-09, Round 7: endpoint error codes)

The shared `ErrorCode` list (`mcp/contract/schemas/common.json`) had no code for a busy thread, a request against the wrong state, or a server-wide cap, so the first `/agui` build reported its 409s as `invalid_argument` and its 503 as `internal`. Both are wrong for an agent: one says "fix your input", the other "the server broke". Three codes are added, following the gRPC canonical names the list already uses (`invalid_argument`, `not_found`, `permission_denied`, `cancelled`, `internal`), plus one domain code because "busy" is the case an agent must handle by waiting:

| Code | Meaning | `/agui` | `/a2a` (JSON-RPC `error.data.error.code`) | Retryable |
|---|---|---|---|---|
| `session_busy` | another run is in progress on the same thread, session or A2A context | 409 | a message for a context with a run in progress | yes |
| `failed_precondition` | the target is not in a state that accepts the request | 409: an open interrupt, a thread at its message cap | `CancelTask` on a terminal task; an open interrupt or full context | no |
| `resource_exhausted` | a server-wide cap is reached (64 sessions) | 503 with `Retry-After` | same cap | yes |

Existing codes cover the rest: 413 is `size_limit_exceeded`, an unknown or foreign thread or task is `not_found`, a principal not allowed on the path is `permission_denied`. AG-UI `RUN_ERROR.code`, for failures after `RUN_STARTED`, keeps its own vocabulary (`RunErrorCode` in `fixtures/agui.schema.json`: the three limit reasons, `backend_error`, and the thread conflicts), because AG-UI defines `code` as free text and a limit reason is not an error class. A2A errors carry the ToolError object in `error.data` so a peer reads the same code an MCP client does (Superseded in part by the Round 7 A2A amendment below: `error.data` is an array, and the ToolError is its second element); the numeric JSON-RPC code is JSON-RPC 2.0's or the A2A-defined one recorded in `mcp/server/a2a/SOURCES.md`. Per-endpoint tables: `openspec/changes/mcp-agui-endpoint/design.md` ("Errors before the stream") and `openspec/changes/mcp-a2a-endpoint/design.md` ("Errors").

Still open for the operator, unchanged from Round 6: whether `swarm_peers`, `routing_*` and `key_list` become `contentEgress` (OQ-EGRESS-ENUM, `openspec/changes/mcp-tool-contract/design.md`).

## Amendment (2026-10-09, Round 7: the A2A endpoint as implemented)

Recorded from `mcp-a2a-endpoint` (`mcp/server/a2a*.go`, `mcp/server/a2a/SOURCES.md`) and accepted by the architect. Details and the per-cause table are in `openspec/changes/mcp-a2a-endpoint/design.md` ("Recorded behavior (Round 7)").

- **Destructive calls from A2A peers need a presence resolver.** `requires_native_prompt` applies to kind `a2a` as to kind `ui`; `cli` alone fails closed with `confirmation_unavailable` (ADR-0003, Round 7 amendment). This makes "an A2A caller is an agent, so its writes are resolved by the native prompt" hold for the destructive class without exception. The "Chat interrupts" bullet above is unchanged for `ui`.
- **`error.data` is an array.** A2A 1.0.0 section 9.5 requires every `error.data` entry to be an object with an `@type`. Each `/a2a` error therefore carries `[{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason", "domain": "a2a-protocol.org"}, {"@type": "type.googleapis.com/ipfs.mcp.ToolError", "error": {code, message, retryable}}]`. A peer finds the shared ErrorCode at `error.data[@type=ipfs.mcp.ToolError].error.code`; it must select by `@type`, not by index. The Round 7 error-code table above stays valid for the codes; only the location changes.
- **Numeric codes.** A busy context (a task still running or waiting for approval), an open interrupt, a full context and the session cap are `-32603` with `session_busy`, `failed_precondition`, `failed_precondition` and `resource_exhausted` in the ToolError: the specification has no closer error, and the ToolError carries the machine meaning. A message to a task in a terminal state is `-32004` (UnsupportedOperationError, section 3.1.2) with `failed_precondition`. `CancelTask` on a finished task is `-32002` (TaskNotCancelableError) with `failed_precondition`.
- **Streams follow section 3.1.2.** `SendStreamingMessage` begins with the Task in `TASK_STATE_SUBMITTED`, replays every status and artifact update in order, and closes after the terminal status update. The contract fixture `fixtures/a2a/stream-read.json` is corrected to this order, and `fixtures/a2a.schema.json` now rejects a stream that does not begin with the Task (`TestA2AStreamOrder`).

## Amendment (2026-10-10: OQ-EGRESS-ENUM closed)

The operator decided that `swarm_peers`, `routing_*` and `key_list` stay `contentEgress: false`. The catalog, its schema rule, the fixtures and the tests are unchanged. The open-question sentences in the Round 6 and Round 7 amendments above are answered by this one.

Consequence, accepted by the operator: a principal with read grants on the swarm, routing or key groups can send the node's peer list, routing answers and key names to the model backend (through `/agui` or the agent loop behind `/a2a`) or to an `a2a` peer, without the backend host being in its `egress_ack`. The definition of content egress in the Round 6 amendment (stored bytes, names taken from stored data, enumerations of what the node stores) is unchanged; peer, routing and key enumerations are about the node, not its stored content. An operator who does not want this withholds those tool groups from `ui` and `a2a` principals. Recorded in `openspec/changes/mcp-tool-contract/design.md`, `mcp-agui-endpoint/design.md` and `mcp-a2a-endpoint/design.md`.

## Amendment (R11, 2026-10-11: per-principal agent quotas, and the gate changes)

Recorded from the R11 phase-boundary review (`.agent-team/kubo-ipfs-agentic/findings/security/r11-findings.md` M4, H1, H4) and the batched fix pass; accepted by the architect.

- **Session quota.** The agent loop holds at most 64 sessions in total, and at most 16 per principal (`maxSessions`, `maxSessionsPerPrincipal`, `mcp/server/agent_sessions.go`).
  - A principal at its quota evicts its own least recently used idle session.
  - When the table is full, a principal may evict only its own oldest idle session. If it has none, the new session is refused with `resource_exhausted`: 503 with `Retry-After` on `/agui`, and the retryable `TOO_MANY_CONTEXTS` on `/a2a`.
  - Another principal's conversation is never evicted to make room.
- **Run rate.** Each principal has a token bucket of 120 runs, refilled at 120 runs per minute (`defaultRunBurst`, `defaultRunsPerMinute`).
  - Over the rate, `/agui` answers 429 with `Retry-After`.
  - Over the rate, `/a2a` answers JSON-RPC `-32603` with reason `RATE_LIMITED` and a retryable ToolError `resource_exhausted`, whose message gives the wait in seconds.
  - A rate-limited run creates no session.
  - Pins: `TestPrincipalSessionQuotaEvictsOnlyItsOwn`, `TestFullTableRefusesInsteadOfEvictingAnotherPrincipal`, `TestRunRateIsPerPrincipalWithRetryAfter`, `TestRateLimitedRunDoesNotCreateASession`.
- **Why.** Before this change, one `a2a` peer could evict the chat's sessions, or spend the operator's model key without limit.
- **Limit (open).** The quota covers agent-loop sessions. The A2A context and task store (`a2aStore`, `mcp/server/a2a_store.go`) still has one 64-context cap, and when it is full it evicts the oldest idle context of any owner. One A2A peer can therefore still push out another peer's idle context, though not a running one. This remains open until the store is scoped per owner.
- **Human gate, as amended in ADR-0003 (R11).**
  - The "Native prompt requirement (Round 4)" bullet under "Human gate" now reads: presence resolvers are `macos-localauth` and `linux-polkit` only. `desktop` does not qualify until a Desktop confirmation window consumes the pairing key.
  - Elicitation confirms calls only for `human-mcp` principals (and stdio). `ui` and `a2a` principals cannot approve through elicitation, even when they declare it.
  - At most two out-of-band confirmations may wait per principal and per session.
- **Operator decisions of 2026-10-11.** The Web UI chat token stays in `localStorage` (R11 security M6, accepted risk) and the Desktop `openExternal` path is left as upstream has it (L6, won't fix); both are recorded in ADR-0005's amendment of 2026-10-11. The same-user Desktop pairing verifier risk and the three confirmation keys are recorded in ADR-0003's amendment of 2026-10-11.

## Amendment (2026-10-12: peer-supplied data is untrusted unless the peer ID is trusted)

Operator decision of 2026-10-12 on R13 security finding L5 (`.agent-team/kubo-ipfs-agentic/findings/security/r13-webmcp.md`). Change: `openspec/changes/mcp-trusted-peers/`. This does not reopen the 2026-10-10 OQ-EGRESS-ENUM amendment above: `swarm_peers`, `routing_findpeer` and `routing_findprovs` stay `contentEgress: false`.

- **Decision.** Addresses that remote peers supply are marked untrusted, per entry, unless the entry's peer ID is in the new `ipfs mcp` config key `trusted_peers` (default `[]`). Trust is by peer ID because libp2p authenticates peer IDs. Agent versions are self-reported and spoofable, and are never a trust signal.
- **Contract marker.**
  - Catalog field `peerSuppliedFields`: `swarm_peers` `/peers/*/addr`, `routing_findpeer` `/addrs`, `routing_findprovs` `/providers/*/addrs`.
  - Each object holding such a field has an optional boolean `untrusted` (`common.json` `PeerSuppliedUntrusted`): `false` only for a listed peer, otherwise `true`; absent is read as `true`.
  - A result with any untrusted entry also carries `_meta["ai.ipfs/untrusted-content"] = true`, so the agent loop frames it as data.
  - `ipfs_node_info` returns this node's own addresses and is not marked.
- **WebMCP.** The browser has no `ipfs mcp` config, so `mcp/webmcp` marks every peer-supplied entry untrusted, and the three tools carry `untrustedContentHint` (derivation `any:returnsUntrustedContent,peerSupplied`).
- **Off switch.** None is needed: the default marks everything untrusted, and listing a peer is the only way to trust its addresses.
- **Where this is weak.** For `routing_findpeer` and `routing_findprovs`, the addresses of a listed peer come from the routing system, possibly relayed by other DHT nodes. "Trusted" therefore means "a peer ID the operator trusts", not "addresses proven to come from that peer". Kubo's RPC does not report whether addresses came from a signed peer record.

## Consequences

### Positive

- Conformance to AG-UI and A2A is under this team's control, independent of another team's release.
- One authentication path, one scope map and one confirmation gate across MCP, AG-UI and A2A.
- Works with any OpenAI-compatible server, local or remote. UAR is optional.
- Non-eliciting writes need a human-presence factor, independent of any agent runtime's own approval or governance settings.
- With no `--llm-*` configuration, nothing is mounted and no model is ever called.

### Negative

- The fork now carries an LLM HTTP client and a minimal agent loop. This reverses a revision-2 claim of the addendum, and it is maintenance in this repository (`plan.md` trade-off T7).
- Native prompts need per-OS implementations, and Windows has none this phase (T9).
- An SDK, if G9-1 forces one, adds root `go.mod` modules.
- The egress guarantee is limited to what `egress_ack` and `check-uar` can see.
- Two topologies cost about 14 tasks and an ongoing coupling to UAR.

## Rejected and deferred alternatives

| Alternative | Rejected or deferred because (evidence) |
|---|---|
| (d) UAR-hosted AG-UI and A2A as the primary | Excluded by the goal-fit gate while UAR's fixes are undelivered (goal fit 2; adjudication item 1). Kept as topology B behind the published-release gate. |
| (b) daemon listener | ADR-0001's C1 plus an LLM client inside the daemon; scored 33/75. Changes the daemon, `config/` and `docs/config.md`. |
| (c) separate binary in this repository | A second product with its own release tooling; scored 49/75, within 2 points of (a), and rejected on that qualitative ground (ADR-0001, B). |
| (e1) LLM-free A2A layer in `ipfs mcp` | Goal fit 2: no agentic behavior, so G9/G10 are not met. |
| L1: UAR is the only loop and backend | Narrows G10, which names an OpenAI-compatible server as an alternative to UAR. |
| L4: MCP sampling only | Works only inside an MCP host; not a backend for AG-UI or A2A clients. |
| The two-step token, a 0600 socket or a loopback page as the human gate | Reachable by same-user processes, including UAR's opt-in `terminal_exec` (`uar:src/uar/tools/terminal_exec.rs:22-27`); addendum review WARNING, upheld. |
| Stdio-spawned UAR in topology B | `ipfs mcp` cannot identify the client or enforce egress over stdio. |
| A non-conformant topology-B release | Refused by the gate rule; the safe default on non-delivery is deferral. |

## Where this is weak

- **The weighted margins are not decisive.** (a) over (c) is 2 points, and the G10 options are within 3 points. The decisions rest on qualitative arguments and the operator's choice.
- **The A2A 1.0 details are partly unverified.** The release date, the exact JSON-RPC method names and the wire casing of task states came partly from an LLM-extracted summary (`assessment-addendum.md`, Knowledge gaps). Conformance is therefore defined as validation against the published 1.0 schema (G9-1), not against names written in this ADR.
- **AG-UI interrupt status** (stable or draft) is unverified.
- **Native-prompt mechanisms are unbuilt.** The macOS path without cgo is unverified, and Accessibility-permitted processes can click dialogs even though they cannot supply the presence factor.
- **Fail-closed has a user cost.** Some operators (Windows without Desktop, Linux without the polkit action) cannot let non-eliciting agents write at all.
- **Topology B may deliver nothing this phase.** It depends on another team's published release by a fixed round.
- **Egress is bounded by acknowledgement, not verification.** A misconfigured but acknowledged endpoint leaks by design of the operator.

## Falsifiers

| ID | Assumption | Observable falsifier | Consequence |
|---|---|---|---|
| G9-1 | AG-UI and A2A can be served conformantly without an SDK | Events fail validation against the published `@ag-ui/core` schemas, or A2A messages fail validation against the A2A 1.0 JSON schema, and cannot be fixed within the change | SDK with operator approval of the `go.mod` addition, or ship A2A only and defer AG-UI |
| G9-2 | A2A-delegated writes cannot run without a human | A test peer agent asks for `pin_add`, and the pin exists before the native prompt is approved | Block: write groups refused to every A2A principal |
| G9-3 | Topology B's UAR fixes ship in a published release by the end of Round 4 | No UAR tag plus image digest or native package containing every item in `mcp/uar/requests/CONFORMANCE.md` | Topology B deferred and reported unmet; A unaffected |
| G9-4 | One listener carries three protocols without cross-protocol auth confusion | A token for one path's principal is accepted on another path for a tool outside its groups (`test/cli` matrix) | Block; separate listeners if needed |
| G10-1 | UAR's `/v1/chat/completions` accepts client `tools` and returns `tool_calls` without running its own tools | The test against the pinned published UAR release shows UAR executing a tool or dropping client tools | UAR unsupported as a backend; other OpenAI-compatible servers remain |
| G10-2 | A local OpenAI-compatible server does tool calling well enough | The real-service test with Ollama and `qwen3:8b` (Round 0 R5) fails the tool-call round trip | G10 local reported unmet; a remote backend with `egress_ack` stays supported |
| G10-3 | No model call happens without configuration | With no `--llm-*` set, `/agui` or `/a2a` answers other than 404, or a network capture shows an outbound connection from `ipfs mcp` | Block |
| G10-4 | `egress_ack` is enforced for every `contentEgress` result | With a fake model server, `ipfs_cat` content reaches a request body for a host not in `egress_ack` | Block |
| G10-5 | The UAR backend setup is acceptable | Measured from published artifacts on clean macOS arm64 and Linux x86_64: more than 30 minutes or more than 15 steps (operator default for Q11) | UAR documented as an advanced backend; local OpenAI-compatible servers recommended |
| HG-1 | A same-user process cannot approve a pending write | An automated IPC attempt (no pairing key, a forged key, `ipfs mcp confirm` without `--confirm=cli`) or operator-run synthetic input (`osascript` System Events, `xdotool` on X11) performs the write | Block that mechanism; fail closed on that platform |
| HG-2 | `check-uar` sees UAR's tool inventory | The settings or tools API of the pinned release does not expose `terminal_exec` or the MCP server list | Write groups refused to every topology-B principal |

## References

- `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`: "Answers to the G9-G12 addendum" and "Round 0 answers"
- `.kbd-orchestrator/phases/ipfs-mcp-server/assessment-addendum.md`: Summary, G9 and G10 options and scores, DECISION (revision 3), ADR DRAFTS (ADR-0004), Unresolved review findings, Orchestrator adjudication
- `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`: D1-D4, D7, sections 3, 4 and 4a
- `mcp/uar/requests/CONFORMANCE.md`: the topology-B request and its gate rule
- ADR-0001 (placement), ADR-0003 (confirmation and authentication), ADR-0005 (chat UI), ADR-0006 (embeddings)
