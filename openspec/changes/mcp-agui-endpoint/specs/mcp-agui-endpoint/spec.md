# Spec Delta: mcp-agui-endpoint

## Purpose

N12: AG-UI endpoint served by `ipfs mcp --http` (G9, topology A).

## ADDED Requirements

### Requirement: Off by default
`/agui` SHALL NOT be mounted unless `--agui` is given and an LLM backend is configured.

#### Scenario: Disabled
- **WHEN** `ipfs mcp --http` runs without `--agui`
- **THEN** `POST /agui` returns 404 and no run starts

### Requirement: Authenticated and origin-checked
`/agui` SHALL require a bearer accepted by the AuthVerifier for a principal allowed on this path, and SHALL reject origins outside the allowlist with 403.

#### Scenario: Foreign origin
- **WHEN** a request with `Origin: https://evil.example` reaches `/agui`
- **THEN** the response is 403 and no run starts

### Requirement: Interrupt instead of silent writes
A write or destructive call in an AG-UI run SHALL end the run with an interrupt outcome and SHALL NOT execute before approval; a destructive call SHALL also require the native prompt.

#### Scenario: Destructive call approved only in chat
- **WHEN** a `ui` principal resumes a `pin_rm` interrupt with approval but the native prompt is denied
- **THEN** the pin remains

### Requirement: Empty origin allowlist by default
`agent.origin_allowlist` SHALL default to empty, and with it every `/agui` request that carries an `Origin` header SHALL get 403 with no CORS headers; no origin, including `webui://localhost` or the RPC origin, SHALL be allowed unless the operator lists it.

#### Scenario: Web UI origin not configured
- **WHEN** the allowlist is unset and a request with `Origin: webui://localhost` and a valid `ui` bearer reaches `/agui`
- **THEN** the response is 403 with no `Access-Control-Allow-Origin` header and no run starts

### Requirement: Only the ui principal on /agui
`/agui` SHALL refuse every principal other than kind `ui` with 403 `permission_denied` before reading the body, so a resume from another principal SHALL resolve nothing.

#### Scenario: Approve from a human-mcp bearer
- **WHEN** a `human-mcp` principal sends a resume approving the open interrupt of a `ui` thread
- **THEN** the response is 403, the confirmation stays pending and the tool does not run

### Requirement: A resume is single-use
An interrupt ID SHALL be accepted in at most one resume; a later resume naming it SHALL get 422 `invalid_argument`, start no run and execute nothing.

#### Scenario: Replayed approval
- **WHEN** a client resends an approval that was already accepted and executed
- **THEN** the response is 422 and the write has run exactly once

### Requirement: Errors carry the shared error codes
An error before the stream SHALL be an HTTP status with a ToolError body whose `code` is from the shared ErrorCode list: 409 `session_busy` (retryable) for a run in progress on the thread, 409 `failed_precondition` for an open interrupt or a full thread, 503 `resource_exhausted` with `Retry-After` at the session cap, 413 `size_limit_exceeded` for an oversized message. A failure after `RUN_STARTED` SHALL be `RUN_ERROR` with a `RunErrorCode` and SHALL NOT include the model backend's response body.

#### Scenario: Second message while a run streams
- **WHEN** a new user message arrives on a thread whose run is still streaming
- **THEN** the response is 409 with `error.code` `session_busy` and `retryable` true

#### Scenario: Backend fails mid-run
- **WHEN** the model backend answers HTTP 500 with a body after `RUN_STARTED`
- **THEN** the stream ends with `RUN_ERROR` code `backend_error` whose message names the status and not the body

### Requirement: Client-declared tools are ignored
The endpoint SHALL accept and ignore `tools`, `context`, `state` and `forwardedProps` in `RunAgentInput` and SHALL offer the model only the contract tools enabled for the principal.

#### Scenario: Client declares a tool
- **WHEN** a `RunAgentInput` lists a tool named `shell_exec` in `tools`
- **THEN** the run proceeds and the model is not offered `shell_exec`
