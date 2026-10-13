# A2A sources (G9-1)

Task 1.1 of `mcp-a2a-endpoint`. Every wire value below is taken from the A2A specification and checked against the JSON schema the A2A project publishes. Hand-written types over `net/http` are the implementation decision (ADR-0004): no `a2a-go` module is proposed, so there is no dependency to review and no operator answer is needed.

## Primary sources (fetched 2026-10-09)

| Source | What it settles |
| --- | --- |
| https://github.com/a2aproject/A2A/blob/main/docs/specification.md (also https://a2a-protocol.org/v1.0.0/specification); latest release `v1.0.1`, published 2026-05-28; the front matter names `1.0.0` as the latest released version | Agent card at `/.well-known/agent-card.json` (8.2); the card declares `supportedInterfaces` in preference order and each entry its `protocolBinding` and `protocolVersion` (8.3.1); JSON-RPC 2.0 over HTTP, PascalCase method names, SSE for streaming (9.1); methods `SendMessage`, `SendStreamingMessage`, `GetTask`, `ListTasks`, `CancelTask`, `SubscribeToTask`, the push-notification methods and `GetExtendedAgentCard` (5.3, 9.4); JSON names are camelCase and enums are their proto string names (5.5). Blocking is the default for `SendMessage`: it waits for a terminal or interrupted state unless `returnImmediately` is true (3.2.2). A streaming call begins with the `Task` and closes when the task is terminal (3.1.2). A task ID is server-generated; a client `contextId` the server cannot accept must be rejected (3.4); a message to a terminal task is `UnsupportedOperationError` (3.1.1); an unknown task and another client's task are not told apart (3.3.2). |
| https://raw.githubusercontent.com/a2aproject/A2A/v1.0.1/specification/a2a.proto | The normative message definitions. `TaskState` values `TASK_STATE_SUBMITTED`, `_WORKING`, `_COMPLETED`, `_FAILED`, `_CANCELED`, `_INPUT_REQUIRED`, `_REJECTED`, `_AUTH_REQUIRED` (input-required and auth-required are "interrupted" states; rejected is "the agent decided not to perform the task", terminal). `Role` is `ROLE_USER` and `ROLE_AGENT`. `Part` is one of `text`, `raw`, `url`, `data` with `mediaType`. `StreamResponse` is one of `task`, `message`, `statusUpdate`, `artifactUpdate`. |
| https://a2a-protocol.org/latest/spec/a2a.json (JSON Schema 2020-12 bundle, `version: v1`, SHA-256 `1ead11c093cd0ea18d5a88f19a74107c887cffce2b6ca6988056066e7d933753`). https://a2a-protocol.org/v1.0.0/spec/a2a.json has the same shapes with refs that do not resolve inside one file (SHA-256 `6b6560c7...bf43b8d62`) | The schema the tests validate against. It is generated from the proto and marked non-normative; a copy of the `latest` bundle is kept here as `a2a.schema.json` so the check runs offline. |
| Specification 5.4 and 9.5 | JSON-RPC error codes: `-32700`, `-32600`, `-32601`, `-32602`, `-32603` (JSON-RPC 2.0); `TaskNotFoundError` `-32001`, `TaskNotCancelableError` `-32002`, `PushNotificationNotSupportedError` `-32003`, `UnsupportedOperationError` `-32004`, `ContentTypeNotSupportedError` `-32005`, `InvalidAgentResponseError` `-32006`, `ExtendedAgentCardNotConfiguredError` `-32007`, `ExtensionSupportRequiredError` `-32008`, `VersionNotSupportedError` `-32009`. `error.data` is an array of objects, each with an `@type` (`google.rpc.ErrorInfo` recommended). |
| Specification 4.5.3, 4.4.1 | Bearer authentication is `HTTPAuthSecurityScheme{scheme: "Bearer"}` inside `securitySchemes`, referenced by `securityRequirements[].schemes.<name>.list`. |

## What this server implements

- Methods: `SendMessage`, `SendStreamingMessage`, `GetTask`, `CancelTask`. `ListTasks` and `SubscribeToTask` answer `-32004`; the push-notification methods `-32003`; `GetExtendedAgentCard` `-32007`; any other name (including the pre-1.0 `message/send` style) `-32601`. `A2A-Version` other than `1` or `1.x` is `-32009`.
- Task states produced: submitted, working, completed, failed, canceled, input-required, rejected, auth-required.
- Pending confirmation is `TASK_STATE_INPUT_REQUIRED`. A declined or expired confirmation is `TASK_STATE_REJECTED` (the agent did not perform the task), with the ToolError as an artifact. A tool outside the principal's grants is `TASK_STATE_AUTH_REQUIRED`. Limit outcomes and failures are `TASK_STATE_FAILED`.
- Not implemented: history (`historyLength` is accepted and ignored; no `history` is sent), push notifications, the extended card, `tenant`.

## Where this server goes beyond or differs from the documents, on purpose

- A streaming call starts with the `Task` in `TASK_STATE_SUBMITTED` and replays every update (status and artifact events), ending with the terminal status update; the contract fixture `stream-read.json` starts with a `statusUpdate` and ends with a `task`, which the specification does not allow ("the stream MUST begin with the Task object"). The specification wins; the fixture's states and artifact still match.
- A stream of a task that waits for approval stays open until the task is terminal, so the peer sees the outcome of the out-of-band decision without polling.
- `error.data` is an array with a `google.rpc.ErrorInfo` and a second object `{"@type": "type.googleapis.com/ipfs.mcp.ToolError", "error": {code, message, retryable}}`. The architect's table writes `data: {"error": ...}`; the array form is what 9.5 requires of every detail object.
- Messages to an input-required task are answered with the same task and a notice; they never approve (the fixture's second exchange).
- `-32603` is used for a busy context, an open interrupt, a full context and the session cap: the specification has no closer error. The shared ErrorCode in `data.error.code` (`session_busy`, `failed_precondition`, `resource_exhausted`) carries the machine meaning.
- The card is public, so its skills cannot depend on the caller. It lists the skills for which at least one registered tool may be called by at least one configured A2A principal.

## Validating

`go test ./mcp/server -run 'TestA2A' -count=1` validates, against `a2a.schema.json`: the card, the contract fixture requests and card, every task and stream event the handler returns in the replay tests.

Recorded 2026-10-09: all of them validate against the `latest` bundle (spec release v1.0.1; the shapes are those of 1.0.0).
