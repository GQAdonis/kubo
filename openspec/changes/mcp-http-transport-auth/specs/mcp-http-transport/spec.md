# Spec Delta: mcp-http-transport

## Purpose

Lets remote MCP clients use every `ipfs mcp` tool over Streamable HTTP with authentication, while keeping loopback-only operation as the default.

## ADDED Requirements

### Requirement: Off by default, loopback by default
HTTP SHALL be disabled unless `--http` is given. When it is given without an address, the listener SHALL bind to loopback only.

#### Scenario: Default start
- **WHEN** `ipfs mcp` is started without `--http`
- **THEN** no TCP listener is opened

### Requirement: Authentication off loopback
The server MUST refuse to start an HTTP listener on a non-loopback address unless an auth verifier is configured. It MUST reject requests without a valid bearer token with 401.

#### Scenario: Missing token
- **WHEN** a request without an Authorization header reaches an authenticated listener
- **THEN** the response is 401, and no tool runs

### Requirement: Request hardening
The server SHALL reject requests whose Host header does not match a loopback listener (DNS rebinding) and cross-origin browser requests, with 403. It SHALL enforce a maximum request body size.

#### Scenario: DNS rebinding attempt
- **WHEN** a request reaches a loopback listener with Host `evil.example`
- **THEN** the response is 403

### Requirement: Tool parity and scoped authorization
Over HTTP, the server SHALL expose the same tools as stdio, subject to the principal's scopes. Write and destructive tools SHALL still require confirmation.

#### Scenario: Parity
- **WHEN** an authorized client with all scopes calls `tools/list` over HTTP
- **THEN** the list equals the stdio list for the same flags

### Requirement: Default listen address
When `--http` is given without an address, the listener SHALL bind `127.0.0.1:5050`. The address SHALL be overridable by flag and environment variable.

#### Scenario: Default bind
- **WHEN** `ipfs mcp --http` starts with no address and no override
- **THEN** the listener is on 127.0.0.1 port 5050 only

### Requirement: Mounted routes inherit protections
Every additional route mounted on the listener SHALL pass through the same AuthVerifier, Host check, Origin policy and body limit.

#### Scenario: Mounted route without token
- **WHEN** a request without a bearer reaches a mounted agent route
- **THEN** the response is 401
