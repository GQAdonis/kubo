# AG-UI sources (G9-1)

Task 1.1 of `mcp-agui-endpoint`. The endpoint emits only the events below. Each is taken from the AG-UI documentation and checked against the schemas that the `@ag-ui/core` package publishes. Kubo does not depend on the package or on node: the check runs only when you point it at an install.

## Primary sources (fetched 2026-10-09)

| Source | What it settles |
| --- | --- |
| https://docs.ag-ui.com/concepts/interrupts | `RunFinished.outcome` is `{type: "success"}` or `{type: "interrupt", interrupts: [...]}` with a non-empty array. `Interrupt` fields: `id`, `reason`, `message`, `toolCallId`, `responseSchema`, `expiresAt`, `metadata`. `reason: "tool_call"` requires `toolCallId`. `RunAgentInput.resume` is `[{interruptId, status: "resolved" \| "cancelled", payload?, metadata?}]`. Denial is expressed inside `payload` (`{approved: false}`), `cancelled` carries no payload. Contract rules: same thread; resume addresses the interrupt; pending interrupts block new input (an input without `resume` is an error); a stale resume is an error. `StateSnapshot` and `MessagesSnapshot` go out before the `RunFinished` that carries the interrupt. A tool-bound interrupt is not announced again: the resumed run emits `ToolCallResult` on the original `toolCallId`. |
| https://docs.ag-ui.com/concepts/events | Event shapes and fields: `RunStarted` (`threadId`, `runId`), `RunFinished`, `RunError` (`message`, optional `code`), `TextMessageStart` (`messageId`, `role`), `TextMessageContent` (`messageId`, non-empty `delta`), `TextMessageEnd`, `ToolCallStart` (`toolCallId`, `toolCallName`, optional `parentMessageId`), `ToolCallArgs` (`toolCallId`, `delta`), `ToolCallEnd`, `ToolCallResult` (`messageId`, `toolCallId`, `content`, optional `role`), `MessagesSnapshot` (`messages`). `timestamp` is optional and not sent. |
| https://docs.ag-ui.com/concepts/architecture | `HttpAgent` posts a `RunAgentInput` to an endpoint and reads a stream of events; SSE is the text transport. The endpoint answers `text/event-stream` with `data: <event JSON>` frames. |
| https://www.npmjs.com/package/@ag-ui/core, version 1.0.2 (published 2026-10-05), `agui.protocolVersion` 1.0 | The zod schemas `EventSchemas` and `RunAgentInputSchema` used to validate. Its `RunFinishedOutcomeSchema` accepts the success and interrupt variants (and a newer cancelled one that this server does not emit). |

## Events this server emits, with their source

| Event | Source |
| --- | --- |
| `RUN_STARTED`, `RUN_FINISHED` (`outcome` success or interrupt), `RUN_ERROR` | events (Lifecycle), interrupts (Run outcomes) |
| `TEXT_MESSAGE_START`, `_CONTENT`, `_END` | events (Text Message Events) |
| `TOOL_CALL_START`, `_ARGS`, `_END`, `TOOL_CALL_RESULT` | events (Tool Call Events), interrupts (Tool-bound interrupts) |
| `MESSAGES_SNAPSHOT` | events (State Management Events), interrupts (State at the interrupt boundary) |

Not emitted: state, activity, reasoning, step, subagent, raw and custom events, and `TOOL_CALL_CHUNK`/`TEXT_MESSAGE_CHUNK`.

Where this server goes beyond the documents, on purpose:

- A run carries exactly one interrupt and a resume exactly one entry (ADR-0004, Round 6 amendment). The documents allow several; one is a valid case of "non-empty array" and "address every open interrupt".
- `responseSchema` for a native-prompt interrupt is `oneOf [{approved: false}, {wait: true}]`; `{wait: true}` is this server's own payload and is declared in the schema so a client does not have to guess.
- A stale or foreign resume is an HTTP status (404, 409, 422) before any event, not `RUN_ERROR`, because no run began (the documents say agents "emit `RunError`"; a run that never starts has no stream to carry one).
- A loop limit has no AG-UI outcome, so it is `RUN_ERROR` with the reason as `code`.

## Validating

Install the schemas somewhere outside the repository (they are not a Kubo dependency):

```bash
mkdir /tmp/agui-core && cd /tmp/agui-core
npm init -y && npm install --ignore-scripts @ag-ui/core@1.0.2 zod@3.25.76
```

Then run the replay test with the directory named. It sends every contract fixture's input to the real handler and passes the inputs and the events the handler produced through `agui/validate.cjs`:

```bash
AGUI_CORE_DIR=/tmp/agui-core go test ./mcp/server -run TestAGUIFixturesReplay -v
AGUI_CORE_DIR=/tmp/agui-core go test ./test/cli -run TestMCPAGUI -v   # needs make build
```

Recorded 2026-10-09: all 11 fixture inputs and the events of all 11 replays validate against `@ag-ui/core` 1.0.2 (`protocol 1.0`, 0 problems).
