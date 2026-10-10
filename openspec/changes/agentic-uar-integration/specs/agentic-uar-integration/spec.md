# Spec Delta: agentic-uar-integration

## Purpose

N3 (CONDITIONAL on N2 GO): shipped UAR profile and topology-B end-to-end tests.

## ADDED Requirements

### Requirement: Safe profile
The shipped profile SHALL bind UAR to loopback, require JWT, disable `terminal_exec`, use local embedders, set no default model provider and connect to `ipfs mcp` over HTTP only.

#### Scenario: Profile review
- **WHEN** the profile is loaded
- **THEN** each of those settings is present with the stated value and no stdio server entry exists

### Requirement: Minimum UAR version
`ipfs mcp` SHALL refuse a `kind: uar` principal whose agent card reports a version below the pinned minimum.

#### Scenario: Old UAR
- **WHEN** a UAR older than the pinned minimum connects
- **THEN** the principal is refused with a message naming both versions

### Requirement: Human gate in topology B
Writes requested through UAR-hosted AG-UI or A2A SHALL execute only after the native prompt in `ipfs mcp` approves.

#### Scenario: UAR-hosted write
- **WHEN** a UAR-hosted A2A task asks `ipfs mcp` to `pin_add`
- **THEN** nothing is pinned until the prompt approves, and nothing after deny

### Requirement: UAR backend upstream provider verified per session
When the topology-A LLM backend is a UAR instance, `ipfs mcp` SHALL read UAR's effective settings through the pinned `check-uar` path for each agent session and SHALL NOT include a `contentEgress` result in a model request unless UAR's `llm.provider_host` is `local`, `disabled` or covered by the calling principal's `egress_ack`.

#### Scenario: Remote upstream not acknowledged
- **WHEN** an agent session on a UAR backend whose effective `llm.provider_host` is a remote host missing from `egress_ack` calls `ipfs_cat`
- **THEN** the request body sent to UAR contains the structured egress refusal and no file content

#### Scenario: Settings unreadable
- **WHEN** UAR's effective settings cannot be read or do not validate against the contract schema under `check: verify`
- **THEN** no `contentEgress` result is sent to the backend for that session
