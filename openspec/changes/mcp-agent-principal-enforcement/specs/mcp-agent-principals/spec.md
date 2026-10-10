# Spec Delta: mcp-agent-principals

## Purpose

N8: principal kinds, egress acknowledgement, check-uar and the stdio guard.

## ADDED Requirements

### Requirement: Unconfigured principals get nothing
A bearer token that maps to no configured principal SHALL be rejected for every tool and endpoint.

#### Scenario: Unknown token
- **WHEN** a request carries a valid-format token that no principal owns
- **THEN** the response is 401 and no tool is listed

### Requirement: Egress acknowledgement
A tool marked `contentEgress` SHALL be refused to a principal whose config has no `egress_ack`, with a structured error naming the missing acknowledgement.

#### Scenario: Content tool without acknowledgement
- **WHEN** a principal without `egress_ack` calls `ipfs_cat`
- **THEN** the call returns an `egress_not_acknowledged` error and no content

### Requirement: UAR re-checked per session
For principals of kind `uar`, the preflight SHALL run at every session initialize; a failure SHALL disable that principal's session.

#### Scenario: Remote model switched in after startup
- **WHEN** the UAR instance's effective model is changed to a remote provider not in `egress_ack` after `ipfs mcp` started
- **THEN** the next session initialize for that principal is refused

### Requirement: No write scope with shell tools
A `uar` principal SHALL NOT receive write or destructive groups while its UAR reports `terminal_exec` enabled or a non-Kubo MCP server configured.

#### Scenario: Shell tool enabled
- **WHEN** check-uar reads `terminal_exec` enabled
- **THEN** the principal is downgraded to read-only and startup prints why
