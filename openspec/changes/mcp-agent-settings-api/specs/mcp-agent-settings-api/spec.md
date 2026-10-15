# Spec Delta: mcp-agent-settings-api

## Purpose

Phase `agent-backend-settings`, change `mcp-agent-settings-api` (ADR-0009, decisions D2, D3, D4): a token-protected settings resource on the `ipfs mcp --http` listener that reads and changes the agent backend without a restart and never returns secrets.

## ADDED Requirements

### Requirement: Settings-scoped admin principal
Principal kind `admin` SHALL be accepted only on the settings route, and SHALL be refused on `/mcp`, `/agui` and `/a2a`. A configured principal whose token variable or file is absent SHALL be disabled with a warning and SHALL NOT stop startup.

#### Scenario: Admin token on the chat path
- **WHEN** an `admin` bearer posts to `/agui`
- **THEN** the response is 403 and no run starts

#### Scenario: Missing token
- **WHEN** the `admin` principal's token environment variable is unset at startup
- **THEN** `ipfs mcp` starts and no request authenticates as `admin`

### Requirement: Read never returns key material
A settings read SHALL return the backend configuration with `api_key` reported only as `{set, source}`, and no response or log line SHALL contain key material.

#### Scenario: Key configured
- **WHEN** a key is set and an `admin` reads the settings
- **THEN** the response contains `api_key.set: true` and no key bytes, and the log contains none

### Requirement: Atomic, revision-checked writes
A settings write SHALL be validated with the same rules as startup configuration, SHALL require the current revision in `If-Match`, and SHALL apply atomically or not at all.

#### Scenario: Stale revision
- **WHEN** a write carries an outdated revision
- **THEN** the response is 412 and the running backend is unchanged

### Requirement: Secrets on hosts that refuse them
With `agent.settings.accept_secrets: false`, a write carrying key fields SHALL fail with `secrets_not_accepted`. An absent `llm.api_key` file SHALL report `api_key.set: false` and SHALL NOT stop startup.

#### Scenario: Cloud key write
- **WHEN** `accept_secrets` is false and an `admin` writes an API key
- **THEN** the response is `secrets_not_accepted` and nothing is stored

### Requirement: Stored model key follows only its own host
A write that changes the model backend scheme, host or port SHALL NOT carry a key stored through the API to the new backend (ADR-0012). Unless the same write supplies `api_key`, the API-stored key SHALL be cleared and the response SHALL carry `key_not_carried`. A key supplied by reference (environment variable or file) SHALL follow the move unchanged, because the file or variable is the operator's. An acknowledged host SHALL NOT receive the stored key by being acknowledged.

#### Scenario: API-stored key cleared on a host move
- **WHEN** an `admin` changes the model base URL to a different host or port without `api_key` and the current key was stored through the API
- **THEN** the stored key file is cleared, the new backend is contacted without it, and the response carries `key_not_carried`

#### Scenario: Same-write api_key
- **WHEN** an `admin` changes the model host and supplies `api_key` in the same write
- **THEN** the new key is stored and used for the new backend, and the response does not carry `key_not_carried`

#### Scenario: Referenced key follows the move
- **WHEN** an `admin` changes the model host and the key is a reference (environment variable or file) rather than an API-stored key
- **THEN** the reference is unchanged and used for the new backend, and the response does not carry `key_not_carried` (operator ruling 2026-10-15)

### Requirement: Egress extension only with confirmation
A write SHALL add to a principal's `egress_ack` only the newly chosen backend host, never any other host, and SHALL do so only when the request carries the typed host confirmation.

#### Scenario: Missing confirmation
- **WHEN** a write changes the backend host and asks to extend `egress_ack` without the typed confirmation
- **THEN** the response is `egress_confirmation_required` and nothing changes

### Requirement: Test connection persists nothing
A test-connection request SHALL contact only the candidate URL, SHALL refuse URLs with credentials, plain http to a non-loopback host, or a non-http scheme, and SHALL NOT persist anything.

#### Scenario: Probe then read
- **WHEN** an `admin` tests a candidate URL and then reads the settings
- **THEN** the settings are unchanged

### Requirement: Off switch
The settings route SHALL be absent (404) when `agent.settings.enabled` is `false` or no `admin` principal is configured, and SHALL be removed by the `noagent` build tag; in that state nothing is written and nothing is dialed.

#### Scenario: Disabled
- **WHEN** `agent.settings.enabled` is `false` and any request reaches the settings route
- **THEN** the response is 404, no file in the settings directory changes, and no outbound connection is made
