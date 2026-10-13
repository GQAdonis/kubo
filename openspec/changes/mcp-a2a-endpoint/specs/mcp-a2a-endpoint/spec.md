# Spec Delta: mcp-a2a-endpoint

## Purpose

N13: A2A 1.0 endpoint and agent card served by `ipfs mcp --http` (G9, topology A).

## ADDED Requirements

### Requirement: Agent card
When A2A is enabled, the server SHALL serve an agent card at `/.well-known/agent-card.json` whose skills are derived from the contract and whose security scheme is HTTP bearer.

#### Scenario: Card served
- **WHEN** A2A is enabled and a client fetches the agent card
- **THEN** the card lists only skills for the tool groups the default A2A principal may use

### Requirement: No self-approval over A2A
A message from an A2A peer SHALL NOT resolve a pending confirmation, whatever its content.

#### Scenario: Peer says approve
- **WHEN** a task is input-required for `pin_rm` and the peer sends a message saying "approved"
- **THEN** the pin remains, and the task stays input-required until the out-of-band resolver decides or the confirmation expires

### Requirement: Off by default
A2A routes and the agent card SHALL NOT be served unless `--a2a` is given and an LLM backend is configured.

#### Scenario: Disabled
- **WHEN** `ipfs mcp --http` runs without `--a2a`
- **THEN** `/.well-known/agent-card.json` and `/a2a` return 404

### Requirement: Auth-required state
A task whose credential lacks the scope for a requested tool SHALL enter `TASK_STATE_AUTH_REQUIRED` and SHALL NOT run the tool.

#### Scenario: Insufficient scope
- **WHEN** a read-only A2A principal asks for `pin_add`
- **THEN** the task is auth-required and nothing is pinned

### Requirement: Destructive calls need a presence resolver
A destructive call from an A2A principal SHALL be approved only by a presence resolver (`desktop`, `macos-localauth` or `linux-polkit`); with none available it SHALL be refused with `confirmation_unavailable` and SHALL NOT run.

#### Scenario: Only the CLI resolver is configured
- **WHEN** `ipfs mcp` runs with `--confirm=cli` only and an A2A principal granted the destructive pin group sends a data part for `pin_rm`
- **THEN** the task ends `TASK_STATE_REJECTED` with a `confirmation_unavailable` ToolError artifact, no confirmation is opened for `ipfs mcp confirm`, and the pin remains

### Requirement: Streams begin with the task
`SendStreamingMessage` SHALL send the Task as the first event, followed only by status and artifact updates, and SHALL close the stream after the terminal status update.

#### Scenario: Streamed read
- **WHEN** a peer streams a data part for `ipfs_node_info`
- **THEN** the first event is the Task in `TASK_STATE_SUBMITTED`, no later event is a Task, and the last event is a status update in `TASK_STATE_COMPLETED`

### Requirement: Errors carry the shared code
Every JSON-RPC error from `/a2a` SHALL carry an `error.data` array that contains a `google.rpc.ErrorInfo` object and an object with `@type` `type.googleapis.com/ipfs.mcp.ToolError` holding the shared ToolError `{code, message, retryable}`.

#### Scenario: Busy context
- **WHEN** a peer sends a message for a context whose task is waiting for approval
- **THEN** the response is JSON-RPC error `-32603` whose ToolError detail has code `session_busy` and `retryable: true`, and no task is created

### Requirement: A lost context is rejected
A text message for a context whose agent session no longer exists SHALL be rejected with `-32001` and SHALL NOT start a new session under the same `contextId`.

#### Scenario: Session evicted
- **WHEN** the agent session behind a context that already ran a text task is evicted and the peer sends another text message with that `contextId`
- **THEN** the response is error `-32001` with ToolError code `not_found`, the model is not called, and a later `GetTask` for the context's tasks returns `-32001`
