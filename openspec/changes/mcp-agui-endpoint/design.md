# Design: mcp-agui-endpoint

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **Hand-written types, no SDK.** The AG-UI Go SDK is a community SDK and would be a new root module; G9-1 decides only if schema validation fails.
- **`ui` principal interrupts count as the gate** (addendum revision 3), revisited if G11-3 (a script in the page resolves an interrupt without a user gesture) is falsified; destructive tools always also raise the native prompt.
- **Default port 5050 on loopback** (operator, 2026-10-09).

## Binding to the implemented loop (Round 6, architect)

The loop shipped in `mcp-llm-proxy-loop` (`mcp/server/agent.go`): `srv.EnableAgent(settings)`, `srv.Agent().Run(ctx, identity, RunInput{SessionID, Message, Resume}, emit)`, events `run_started`, `text`, `tool_call`, `tool_result`, `interrupt`, `run_finished`. Its `Interrupt`, `ResumeEntry` and `Event` types were reviewed against ADR-0005 and `mcp/contract/fixtures/agui/`. They fit with the mapping below and three loop changes (L1 to L3). L1 to L3 are `mcp/server` work for `mcp-server-engineer` inside tasks 1.2 and 1.3; the loop's other behavior is unchanged.

### One interrupt per run (decided; resolves the mixed-batch question)

The loop answers the calls of one model reply in order and ends the run at the first one that needs confirmation. A run carries exactly one interrupt and a resume holds exactly one entry. A write approval and a native-prompt `{wait: true}` therefore never share a resume: a reply with `pin_add` and `repo_gc` is two runs, each ending with its own interrupt (`two-calls-write-interrupt.json`, then `two-calls-resume-then-native-interrupt.json`, whose interrupt is answered like `destructive-native-prompt-wait-*.json`). `agui.schema.json` pins `maxItems: 1` on `resume` and `outcome.interrupts`. A resume entry naming any interrupt other than the thread's open one gets HTTP 422, resolves nothing and starts no run. ADR-0004 and ADR-0005 carry the Round 6 amendments.

### Threads and history

- `threadId` is the loop's `SessionID` (both cap at 128 characters). A new message on an unknown `threadId` creates the session under that ID; a session belongs to the principal that created it.
- The server's in-memory session is the history (idle TTL 30 minutes, at most 64 sessions, 256 messages). From `RunAgentInput.messages` the adapter takes only the last message, which must have `role: "user"`, as `RunInput.Message`; client-supplied assistant and tool messages are ignored, so a client cannot forge tool results into the model's context. After eviction or a restart a thread starts over. `agui.schema.json` rule (b) now says this.

### Errors before the stream

`Run` returns these before it emits `run_started`, so the adapter writes no SSE headers until the first event:

| Loop error | HTTP | ToolError `code` (Round 7) | `retryable` |
|---|---|---|---|
| `ErrEmptyRun`, `ErrBadResume`, `ErrNoOpenInterrupt`, `ErrUnknownInterrupt`, last input message not `user`, `{approved: true}` on a `requires_native_prompt` interrupt, `{wait: true}` on an interrupt without `requires_native_prompt`, a replayed resume | 422 | `invalid_argument` | false |
| `ErrMessageTooLarge` | 413 | `size_limit_exceeded` | false |
| `ErrSessionNotFound` (also another principal's thread; the body does not say which) | 404 | `not_found` | false |
| `ErrSessionBusy` | 409 | `session_busy` | true |
| `ErrInterruptOpen`, `ErrSessionFull` | 409 | `failed_precondition` | false |
| `ErrTooManySessions` | 503 with `Retry-After` | `resource_exhausted` | true |
| any non-`ui` principal on `/agui`, including a resume that approves | 403 | `permission_denied` | false |
| policy refusal (`*ToolError` from `applyPolicy`) | 403 | the ToolError's own code | as set |
| anything else | 500 | `internal` | false |

Round 7 correction: the table above replaces the Round 6 one. Round 6 listed "`{approved: true}` from a non-`ui` principal" under 422; the implementation answers 403 earlier (see "Recorded behavior", below), which is the correct result. The Round 6 table had no code column; the first implementation used `invalid_argument` for the 409s and 413 and `internal` for the 503 because the shared ErrorCode list had no conflict, size or capacity code. `session_busy`, `failed_precondition` and `resource_exhausted` were added to `mcp/contract/schemas/common.json`; `size_limit_exceeded` already existed. ADR-0004 (Round 7 amendment) records the shared mapping that `/agui` and `/a2a` both use.

A backend or transport failure after `run_started` is `RUN_ERROR`. A limit outcome (`max_steps`, `max_tokens`, `wall_clock`) is `RUN_ERROR` with `code` set to the reason; AG-UI has no limit outcome. `RUN_ERROR.code` is the `RunErrorCode` vocabulary in `agui.schema.json` (`max_steps`, `max_tokens`, `wall_clock`, `backend_error`, `session_busy`, `session_full`, `interrupt_open`), distinct from the shared ErrorCode; its `message` is fixed per code and for `backend_error` names at most the backend's HTTP status, never its body. Fixtures: `run-error-step-limit.json`, `run-error-backend-failure.json`.

### Events

| Loop event | AG-UI events |
|---|---|
| `run_started` | `RUN_STARTED` with the input's `threadId` and `runId` |
| `text` | `TEXT_MESSAGE_START` (new `messageId`) before the first delta of a reply, then `TEXT_MESSAGE_CONTENT` per delta |
| reply boundary (L1) | `TEXT_MESSAGE_END` if a text message is open; then for every call of the reply, in order, `TOOL_CALL_START` (`parentMessageId` = the reply's assistant message ID), `TOOL_CALL_ARGS`, `TOOL_CALL_END`; then a `MESSAGES_SNAPSHOT` that introduces the assistant message with its `toolCalls` (schema rule (c); `read-pin-ls.json`) |
| `tool_result` | `TOOL_CALL_RESULT` on the call's `toolCallId` with a new `messageId` |
| `interrupt` | `MESSAGES_SNAPSHOT` (input messages, this run's assistant replies with their `toolCalls`, this run's tool results), then `RUN_FINISHED` with `outcome: {type: "interrupt", interrupts: [one]}` |
| `run_finished`, outcome success | `TEXT_MESSAGE_END` if a text message is open, then `RUN_FINISHED` `{type: "success"}` |

Interrupt mapping: `id` = `Interrupt.ID` (`int-` + confirmation ID), `reason: "tool_call"`, `message` = `Interrupt.Message`, `toolCallId`, `expiresAt`, `responseSchema` = `{approved: boolean}` for writes and `oneOf [{approved: false}, {wait: true}]` when `RequiresNativePrompt`, `metadata["ipfs-mcp"]` = `{confirmation_id, tool, class, requires_native_prompt}`. `Resolvers` is not sent.

### Resume

| Resume entry | Adapter action |
|---|---|
| `resolved`, `{approved: true}`, `ui` principal, write | Owner check (L2); `srv.ResolveConfirmation(confirmationID, ResolverAGUIInterrupt, true)`; `ErrConfirmationExpired` and `ErrConfirmationResolved` are not errors here (the run reports the real state), `ErrNativePromptRequired` is 422; then `Run` with `ResumeEntry{Status: "resolved", Approved: true}` |
| `resolved`, `{approved: false}` | `Run` with `Approved: false` (the loop records the denial) |
| `cancelled` | `Run` with `Status: "cancelled"` (a denial) |
| `resolved`, `{wait: true}` | Owner check (L2). Start the stream with `RUN_STARTED`, then wait with `srv.AwaitConfirmation` (L3) without resolving anything. When the confirmation leaves `pending`, `Run` with `Approved: true`: the call runs only if a presence resolver approved it, otherwise the loop reports `confirmation_declined` or `confirmation_expired`. The adapter drops the loop's own `run_started` because the stream already started. If the client disconnects first, stop waiting and do not call `Run`; the confirmation stays pending until it expires, and a new `{wait: true}` picks it up. |
| `{approved: false}` while a `{wait: true}` for the same interrupt is open | Only `srv.ResolveConfirmation(confirmationID, ResolverAGUIInterrupt, false)`, then this request's own stream is `RUN_STARTED`, `RUN_FINISHED` success; the open watch reports `confirmation_declined` (ADR-0005). The adapter keeps one watch per thread. |

The `{wait: true}` observation is not in the loop; this change implements it as above.

### Loop changes this change needs (`mcp/server`, `mcp-server-engineer`)

- **L1, announce at reply time.** Emit one event when a model reply completes, before any of its calls run, carrying the reply's tool calls in order (ID, name, arguments); emit `tool_call` for each call there and not from `execute()`. Today `execute()` announces a call only when it runs, so (a) the adapter cannot tell where one reply ends, and (b) a call queued behind an interrupt is announced in the next run, which breaks fixture rule (d) (`two-calls-resume-then-native-interrupt.json` fails if `repo_gc` is re-emitted).
- **L2, owner check before approval.** An accessor such as `Agent.OpenInterrupt(id Identity, sessionID string) (*Interrupt, error)` that returns the open interrupt only for the session's owner, with `ErrSessionNotFound` otherwise. The adapter must call it, and match the resume's `interruptId`, before `ResolveConfirmation`. Without it, `ResolveConfirmation` approves any pending confirmation whose ID the caller sends, including another principal's or an MCP session's, before `Run` gets to reject the resume. Severity HIGH: a `ui` token holder could approve writes it did not originate.
- **L3, wait for a decision.** `Server.AwaitConfirmation(ctx, id string) (state string, err error)` that returns when the confirmation is no longer pending (approved, denied, expired) or `ctx` ends. The broker has no wait or subscribe today; polling `broker.get` from another package is not possible.

## Recorded behavior (Round 7, architect, from the implementation)

These are decisions, pinned by the `mcp/server` tests named, not accidents of the code.

- **Path-scoped auth decides first: a non-`ui` approve is 403, not 422.** `/agui` serves the `ui` principal only. A `human-mcp` or `a2a` bearer on `/agui` is refused with 403 `permission_denied` before the body is read, so a resume that approves from such a principal never reaches the resume checks and resolves nothing (`TestAGUIResumeNeedsTheChatPrincipal`; the confirmation stays pending and the node is unchanged). The Round 6 error table said 422 for that case; 403 is correct because the principal is not allowed on the path at all, whatever the body says.
- **A replayed resume is 422 and executes nothing.** After a resume has been answered, the thread has no open interrupt. Sending the same resume again (same `interruptId`, same payload) gets 422 `invalid_argument` with no stream, and the call is not executed a second time (`TestAGUIWriteWaitsThenRunsOnceAfterTheChatApproves`, "exactly once"). The interrupt ID is the idempotency key: it is server-issued, single-use, and claimed when the resume is accepted, so a client retry after a lost response cannot run a write twice. A client that lost the response of an accepted resume recovers state from the next run's `MESSAGES_SNAPSHOT`, not by resending.
- **The `agui-interrupt` resolver is offered only to the agent loop's own calls.** A `ui` principal that calls a write tool directly over MCP (`/mcp`, not the agent loop) gets a confirmation whose resolvers do not include `agui-interrupt`; it is resolved by elicitation or the native prompt like any MCP session, and a `/agui` resume naming it gets 422 and leaves it pending (`TestAGUICannotApproveAConfirmationOfAnMCPSession`, `TestAGUIChatResolverIsListedForTheChatPrincipalOnly`). With no native resolver configured, such a call is refused with `confirmation_unavailable` rather than left waiting for a chat that is not there (`TestAGUIMountDoesNotMakeNonChatWritesWaitForNothing`).
- **Client-declared input is ignored.** `tools`, `context`, `state` and `forwardedProps` in `RunAgentInput` are accepted and ignored; real clients send `tools: []`, so `agui.schema.json` no longer pins `tools` to `maxItems: 0`. The model is offered only the contract tools enabled for the principal.
- **`Retry-After` is exposed to the browser.** For an allowlisted origin, the preflight and the POST also carry `Access-Control-Expose-Headers: Retry-After`, so the chat can read when to retry a 503 `resource_exhausted`; nothing else is exposed (`mcp/server/agui_test.go`, origin tests; config `agent.origin_allowlist` description and `cors.exposeHeaders`; ADR-0005 Round 7 amendment).
- **Origin allowlist default is empty.** See proposal.md. `webui://localhost`, `http://127.0.0.1:5001` and `http://localhost:5001` are the documented values an operator adds, never defaults.

## Open questions (operator)

- **OQ-EGRESS-ENUM: do `swarm_peers`, `routing_*` and `key_list` become `contentEgress`?** Decided 2026-10-10 by the operator: no, all three stay `contentEgress: false` and `/agui` is unchanged. Consequence: a `ui` principal with read grants on the swarm, routing or key groups can send the peer list, routing answers and key names to the model backend without `egress_ack`; withhold those groups from the principal to prevent it. Context as it stood before the decision: ADR-0004 (Round 6 amendment) made the pinset `contentEgress` and left peer and key enumerations unchanged. Today a `ui` principal with an empty `egress_ack` can send its peer list, routing answers and key names to the model backend through `/agui`. If the operator decides yes, the change is a catalog flag flip in `mcp/contract/catalog.json` (architect) plus fixture and test updates; `/agui` code needs no change because the loop already enforces `contentEgress` per result. Tracked in `mcp-tool-contract` design.md under the same ID.

## Risks / Trade-offs

- [AG-UI interrupts may still be draft] -> task 1.1 records the status; the fixtures pin the exact events this server emits.
