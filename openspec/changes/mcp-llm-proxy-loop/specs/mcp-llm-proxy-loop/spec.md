# Spec Delta: mcp-llm-proxy-loop

## Purpose

N11: G10 L3, an OpenAI-compatible model proxy and a bounded agent loop inside `ipfs mcp`.

## ADDED Requirements

### Requirement: No model call without configuration
`ipfs mcp` SHALL NOT make any model call, and SHALL NOT mount `/agui` or `/a2a`, unless a base URL and model are configured. There SHALL be no default endpoint or model.

#### Scenario: Unconfigured
- **WHEN** `ipfs mcp --http --agui --a2a` starts with no `--llm-*` settings
- **THEN** `/agui` and `/a2a` return 404 and a network capture shows no outbound connection from `ipfs mcp`

### Requirement: Credentials never literal
The model API key MUST come from a named environment variable or file; a literal key SHALL fail validation.

#### Scenario: Literal key
- **WHEN** the config file contains a literal key value
- **THEN** `ipfs mcp` refuses to start and names the field

### Requirement: In-process egress acknowledgement
A `contentEgress` tool result SHALL be included in a model request only when the backend host is in `egress_ack`; otherwise the model receives a structured refusal instead of the content.

#### Scenario: Unacknowledged backend
- **WHEN** the loop calls `ipfs_cat` while the backend host is not in `egress_ack`
- **THEN** the request body sent to the model contains no file content and contains the refusal text

### Requirement: Bounded loop over granted contract tools
Each run SHALL stop at its step, token or wall-clock limit, and SHALL offer the model only contract tools enabled for the calling principal, never shell, filesystem or network tools.

#### Scenario: Read-only principal
- **WHEN** a read-only principal starts a run
- **THEN** no write, destructive, shell, filesystem or network tool definition is sent to the model

### Requirement: Writes wait for confirmation
A write or destructive tool call requested by the model SHALL NOT execute until the confirmation gate approves it; denial or expiry SHALL return a denied result.

#### Scenario: Denied write
- **WHEN** the model requests `pin_rm` and the confirmation is denied
- **THEN** the pin remains and the model receives a denied result
