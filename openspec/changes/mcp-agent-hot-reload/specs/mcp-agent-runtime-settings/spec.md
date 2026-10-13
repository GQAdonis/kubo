# Spec Delta: mcp-agent-runtime-settings

## Purpose

Phase `agent-backend-settings`, change `mcp-agent-hot-reload`: the `ipfs mcp` agent backend can change while the process runs, without a manual restart. This narrows the `mcp-agui-endpoint` requirement "Off by default": `/agui` is still off unless `--agui` is given, but once enabled it is mounted with or without a configured backend.

## ADDED Requirements

### Requirement: One backend snapshot per run
The agent SHALL read its backend (LLM client, backend host, loop limits, system prompt) from one snapshot taken when a run starts, and AG-UI and A2A SHALL read the same current snapshot.

#### Scenario: Swap during a run
- **WHEN** the backend is replaced while an AG-UI run is streaming
- **THEN** that run completes on the previous backend and the next run uses the new backend

### Requirement: Interrupts survive a swap
A run resumed from an interrupt SHALL use the snapshot its run started with, or fail with a stable error code, and a pending confirmation created before a swap SHALL resolve exactly once after it.

#### Scenario: Resume after swap
- **WHEN** a write interrupt is pending, the backend is swapped, and the user approves the interrupt
- **THEN** the resumed run calls the original backend, never the new one, and the write executes once

### Requirement: /agui answers when no backend is configured
When `--agui` is enabled and no backend is configured, `/agui` SHALL be mounted and SHALL answer 503 with code `backend_not_configured` without contacting any model endpoint.

#### Scenario: No backend
- **WHEN** `ipfs mcp --http --agui` runs with no `llm` configuration and a `ui` principal posts a run
- **THEN** the response is 503 `backend_not_configured` and no outbound connection is made

#### Scenario: AG-UI disabled
- **WHEN** `ipfs mcp --http` runs without `--agui`
- **THEN** `POST /agui` returns 404

### Requirement: Fixed safety prefix on the system prompt
The system prompt SHALL be configurable, and the safety sentences about untrusted content and confirmation SHALL be prepended to every configured prompt and SHALL NOT be removable by configuration.

#### Scenario: Hostile override
- **WHEN** the configured system prompt is empty or instructs the model to skip confirmation
- **THEN** the prompt sent to the model still begins with the fixed safety prefix
