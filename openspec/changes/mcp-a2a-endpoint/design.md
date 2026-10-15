# Design: mcp-a2a-endpoint

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **A2A peers never approve.** The task's input-required state is informational. Approval is only through the native prompt, the paired Desktop window or (opt-in, writes only) the headless CLI; a destructive call needs a presence resolver (Round 7).
- **Read-only by default.** An A2A principal is granted read groups unless the operator names write groups for it.

## Binding to the implemented loop (Round 6, architect)

Reviewed against `mcp/server/agent.go` (`Agent.Run`, `Interrupt`, `ResumeEntry`, `Event`), ADR-0005 and `mcp/contract/fixtures/a2a/`. The loop has no AG-UI-specific state, so A2A can reuse it; the gaps are listed as loop changes.

- **Wire names.** Methods and enums follow `mcp/contract/fixtures/a2a.schema.json` (A2A 1.0.0: `SendMessage`, `SendStreamingMessage`, `GetTask`, `CancelTask`, `TASK_STATE_*`, `ROLE_*`). The `message/send` style names in the proposal and task 1.3 are the pre-1.0 spellings; task 1.1 confirms the 1.0 names from the specification text.
- **Contexts and tasks.** `contextId` is the loop's `SessionID`. One task per run; a task that ends in an interrupt stays open in `TASK_STATE_INPUT_REQUIRED`. The loop allows one open interrupt per session, so a context has at most one input-required task.
- **Text parts** go through `Agent.Run` with `RunInput.Message`. The model is offered only tools the principal may use, so it cannot request an ungranted tool; it says so in text instead.
- **Data parts `{tool, arguments}`** (fixture `write-input-required.json`) call the named contract tool directly, with no model. They go through the same handler as MCP `tools/call` with the A2A identity and a per-context session key, so input validation, the principal's scope, the MCP-side `egress_ack` rule and the confirmation gate apply unchanged; the loop's model-host egress check does not apply because the result goes to the peer, not to a model. A tool outside the principal's grants gives `TASK_STATE_AUTH_REQUIRED` and runs nothing (spec "Auth-required state"). This needs loop change L4.
- **Input-required.** An `Interrupt` (from `Run`) or a `ConfirmationRequired` (from a direct call) becomes `TASK_STATE_INPUT_REQUIRED` with the agent message "Waiting for approval on the node's machine: <summary>" and `metadata["ipfs-mcp"] = {confirmation_id, tool, class, expires_at}`, plus `requires_native_prompt: true` for a destructive call (Round 7). No A2A message ever calls `ResolveConfirmation` with `approve = true`, whatever it says; a follow-up message on that task returns the same state with "Approval cannot be given over A2A" (fixture). There is no AG-UI-style `ui` resolver for A2A.
- **Completion without a peer message.** The fixture's `GetTask` shows `TASK_STATE_COMPLETED` after the native prompt approved, with no further peer request. For each input-required task the endpoint waits with `Server.AwaitConfirmation` (L3 of `mcp-agui-endpoint`) in a goroutine bounded by the confirmation TTL (120 s) and the session cap (64). When the confirmation leaves `pending` it continues: a text task with `Run` and `ResumeEntry{Status: "resolved", Approved: true}` (runs only if a presence resolver approved), a data-part task by calling the tool again with the same identity, session key and arguments (the gate then claims the approval exactly once). Approved gives `TASK_STATE_COMPLETED` with the result as an artifact; declined or expired gives `TASK_STATE_REJECTED` with the ToolError as an artifact. Task 1.1 confirms `REJECTED` is the right 1.0 state; `FAILED` is the fallback.
- **`CancelTask`** on an input-required task records a denial and gives `TASK_STATE_CANCELED`. Denial is always safe; approval never comes from A2A. (Round 7: implemented as `ResolveConfirmation(id, "", false)` for text and data-part tasks alike; see "Recorded behavior".)
- **Errors.** Loop errors before a run (`ErrSessionBusy`, `ErrTooManySessions`, `ErrMessageTooLarge`, `ErrSessionNotFound`) are JSON-RPC errors, not task states; a policy refusal of the whole principal is `TASK_STATE_AUTH_REQUIRED`. Limit outcomes are `TASK_STATE_FAILED` with the reason. Every JSON-RPC `error` carries an `error.data` **array** (A2A 1.0.0 section 9.5: each detail is an object with an `@type`): first a `google.rpc.ErrorInfo` `{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": <UPPER_SNAKE>, "domain": "a2a-protocol.org"}`, then `{"@type": "type.googleapis.com/ipfs.mcp.ToolError", "error": {code, message, retryable}}` with the ToolError object of `mcp/contract/schemas/common.json`, so a peer gets the same machine code as an MCP or `/agui` client. Peers select the ToolError by `@type`, not by position. (Round 7 correction: the Round 6/7 text said `data: {"error": ...}`, which 9.5 does not allow.) Numeric codes are cited in `mcp/server/a2a/SOURCES.md`. Mapping, as implemented in `mcp/server/a2a_wire.go` (`rpcErrorOf`, `a2aErr`) and `a2a.go`:

| Cause | JSON-RPC `code` | `reason` | ToolError `code` | `retryable` |
|---|---|---|---|---|
| unparseable body | -32700 | `PARSE_ERROR` | `invalid_argument` | false |
| a batch, or a body that is not a JSON-RPC 2.0 request | -32600 | `INVALID_REQUEST` | `invalid_argument` | false |
| unknown method, including pre-1.0 `message/send` style names | -32601 | `METHOD_NOT_FOUND` | `invalid_argument` | false |
| invalid params (missing `messageId`, a role other than `ROLE_USER`, no parts or more than 16, text mixed with a data part, a malformed data part, no text, a `contextId` that does not match the named task) | -32602 | `INVALID_PARAMS` | `invalid_argument` | false |
| a raw or URL part, or a data part whose `mediaType` is not `application/json` | -32005 | `CONTENT_TYPE_NOT_SUPPORTED` | `invalid_argument` | false |
| oversized message (`ErrMessageTooLarge`) | -32602 | `MESSAGE_TOO_LARGE` | `size_limit_exceeded` | false |
| `A2A-Version` other than `1` or `1.x` | -32009 | `VERSION_NOT_SUPPORTED` | `invalid_argument` | false |
| `GetTask` or `CancelTask` on an unknown task or another principal's task (not told apart, 3.3.2) | -32001 | `TASK_NOT_FOUND` | `not_found` | false |
| a `contextId` that is unknown, another principal's, or whose agent session is gone (`ErrSessionNotFound`) | -32001 | `CONTEXT_NOT_FOUND` | `not_found` | false |
| a message for a context with a task still running or waiting for approval (`ErrSessionBusy` or an open task in the store) | -32603 | `CONTEXT_BUSY` | `session_busy` | true |
| `ErrInterruptOpen` | -32603 | `INTERRUPT_OPEN` | `failed_precondition` | false |
| `ErrBackendChanged` (a continuation of a task whose run started on a backend whose endpoint or key has since changed; Round 7 settings L5) | -32603 | `BACKEND_CHANGED` | `failed_precondition` | false |
| `ErrSessionFull` | -32603 | `CONTEXT_FULL` | `failed_precondition` | false |
| `ErrTooManySessions`, or the task store's context cap | -32603 | `TOO_MANY_CONTEXTS` | `resource_exhausted` | true |
| a message naming a task in a terminal state (3.1.2) | -32004 | `TASK_FINAL` | `failed_precondition` | false |
| a message naming a task that is not waiting for input | -32004 | `TASK_NOT_WAITING` | `failed_precondition` | false |
| `CancelTask` on a task in a terminal state | -32002 | `TASK_NOT_CANCELABLE` | `failed_precondition` | false |
| `ListTasks`, `SubscribeToTask` (not declared by the card) | -32004 | `UNSUPPORTED_OPERATION` | `tool_unavailable` | false |
| push-notification methods | -32003 | `PUSH_NOTIFICATION_NOT_SUPPORTED` | `tool_unavailable` | false |
| `GetExtendedAgentCard` | -32007 | `EXTENDED_AGENT_CARD_NOT_CONFIGURED` | `tool_unavailable` | false |
| anything else | -32603 | `INTERNAL` | `internal` | false |

`-32603` for busy, open-interrupt, full and the session cap is deliberate: A2A defines no closer error, and the ToolError code carries the meaning (ADR-0004, Round 7 A2A amendment). A JSON-RPC error never carries a task; a per-call refusal inside a task (egress, a missing grant, a declined, expired or unavailable confirmation) stays a task state with the ToolError as an artifact. The error `message` is fixed text per cause and never quotes the model backend. The JSON-RPC `code` and the ToolError `code` are the contract; the `reason` strings are recorded for reference and may gain values, but an existing one is not renamed.

## Recorded behavior (Round 7, architect, from the implementation)

Recorded from `mcp/server/a2a*.go` and `mcp/server/a2a/SOURCES.md`. Each item is a decision; "accepted" means the implementation stands, "change" means a follow-up for `mcp-server-engineer`.

- **Destructive calls need a presence resolver (accepted).** A destructive call from an `a2a` principal sets `requires_native_prompt` and can be approved only by `desktop`, `macos-localauth` or `linux-polkit`. With only `cli` available it is refused with `confirmation_unavailable`, and the task ends `TASK_STATE_REJECTED` with the ToolError as an artifact; nothing runs. Write-class calls are unchanged. ADR-0003 and ADR-0004, Round 7 amendments.
- **`error.data` is an array (accepted).** See "Errors" above; the earlier object form is withdrawn.
- **Streams begin with the Task (accepted; fixture corrected).** `SendStreamingMessage` sends the Task in `TASK_STATE_SUBMITTED` first, then every status and artifact update in the order they happened, and closes after the terminal status update (A2A 1.0.0 section 3.1.2: "the stream MUST begin with the Task object, followed by zero or more TaskStatusUpdateEvent or TaskArtifactUpdateEvent objects"). No Task is sent after the first event. A stream of a task waiting for approval stays open until the task is terminal. `fixtures/a2a/stream-read.json` had a status update first and a Task last; it now has the specification's order, and `fixtures/a2a.schema.json` rejects any other (`TestA2AStreamOrder`).
- **`CancelTask` on input-required resolves the confirmation as denied (accepted).** The handler sets `TASK_STATE_CANCELED` first, so an approval that lands concurrently finds the task cancelled and does not run, then calls `ResolveConfirmation(id, "", false)` for text and data-part tasks alike, instead of `Run` with `Status: "cancelled"`. The effect is the same denial: the broker records it, and the loop session's interrupt is cleared by the next message (`acceptMessage` sees the confirmation is no longer pending and answers the held call as not run). A confirmation already approved keeps its state but cannot run, because the task that would run it is cancelled.
- **Results are kept until the context is evicted, not until first read (accepted).** `GetTask` is a read that A2A clients poll and retry; deleting a result on first read would turn a lost response into a lost result. The bound is in memory only: at most 64 contexts (the loop's session cap), 32 tasks per context (oldest dropped first), and a context with no open task is dropped after 30 minutes untouched, or earlier when the cap needs room. Nothing is written to disk and no content is logged. This replaces the Round 6 task-store rule (results kept only until the task is read or its context is evicted).
- **No text-delta streaming (accepted).** A text task produces one artifact with the final reply, sent as one `artifactUpdate` with `lastChunk: true` when the run ends. A2A does not require chunked artifacts, and the loop's text events are not exposed as partial artifacts in this change. A peer that needs incremental text uses `/agui`.
- **The card's interface URL is derived from the request (accepted; no config key).** The card advertises `<scheme>://<Host>/a2a`, where the scheme is `https` only when the listener terminated TLS. Loopback listeners accept loopback `Host` values only, and a client that sends another `Host` to a non-loopback listener changes only its own copy (caches key on host). No `agent.public_url` key is added this phase: no supported deployment needs it, and a key would be a new configured endpoint with its own validation. The case that needs it is a reverse proxy that terminates TLS or rewrites the host in front of `ipfs mcp`; there the card would advertise the internal address. If that deployment becomes supported, the key is added to `mcp/contract/config/ipfs-mcp-config.schema.json` with no default (absent means "derive from the request", which is its off state) and an `https` or loopback-`http` URL rule.
- **A context whose agent session is gone is rejected, not restarted (change).** Today, if the loop session behind a context is evicted (idle TTL, or the shared 64-session cap, which `/agui` threads also count against) while the A2A context survives, the next text message silently starts a new loop session under the same `contextId`, and the model loses the conversation without the peer knowing. A2A 1.0.0 section 3.4 requires a server to reject a `contextId` it cannot honor. Required behavior: a text message for a context that already ran a text task, whose loop session no longer exists, gets `-32001` `CONTEXT_NOT_FOUND` (`not_found`, not retryable), runs nothing, and the A2A context is dropped, so the peer starts a new one by omitting `contextId`. A first text task in a context that so far had only data-part tasks still creates the session. This needs loop change L5 below.
- **Input-required metadata.** `metadata["ipfs-mcp"]` is `{confirmation_id, tool, class, expires_at}`, plus `requires_native_prompt: true` for a destructive call. `expires_at` is RFC 3339 UTC.

### Loop change this change needs (`mcp/server`, `mcp-server-engineer`)

- **L4, direct tool call for transports.** An exported `Server.CallTool(ctx, id Identity, sessionKey, name string, args json.RawMessage) (*mcp.CallToolResult, error)` that runs the registered handler exactly as MCP `tools/call` does (`confirm_token` stripped, never accepted from this path). Transports outside the package cannot reach the registry today.
- L2 (owner check) and L3 (`AwaitConfirmation`) from `mcp-agui-endpoint` are shared; whichever change lands first adds them.
- **L5, continue-only runs (Round 7).** A way to run a new message on an existing loop session without creating one (for example a `RunInput` field that makes `acquire` use `create = false`), returning `ErrSessionNotFound` when the session is gone. The A2A adapter uses it for every text message after the first text task of a context, so the check and the run are one step and cannot race an eviction.

## Open questions (operator)

- **OQ-EGRESS-ENUM: do `swarm_peers`, `routing_*` and `key_list` become `contentEgress`?** Decided 2026-10-10 by the operator: no, all three stay `contentEgress: false` and `/a2a` is unchanged (recorded in `mcp-tool-contract` design.md). Consequence: an `a2a` peer with read grants on the swarm, routing or key groups receives the peer list, routing answers and key names, and the agent loop may send them to the model backend, without `egress_ack`; withhold those groups from `a2a` principals to prevent it. Context as it stood before the decision: It matters here because an `a2a` peer is a remote party: with the current flags, a peer with read grants for the swarm, routing or key groups can enumerate the node's peers and key names regardless of `egress_ack`. Whatever is decided is a catalog flag; `/a2a` needs no code change because the loop and the direct-call path enforce `contentEgress` per result.

## Risks / Trade-offs

- [A2A 1.0 details were partly LLM-extracted in the addendum] -> task 1.1 re-verifies from the specification text.
