# Spec Delta: mcp-tool-contract

## Purpose

Defines the single machine-readable contract for every IPFS MCP tool, consumed by the MCP server, MCP App, A2UI catalog, skills and WebMCP package.

## ADDED Requirements

### Requirement: Tool catalog completeness
The contract SHALL list every tool exposed by `ipfs mcp`. Each entry SHALL give its name with one of the prefixes `ipfs_`, `pin_`, `ipns_`, `key_`, `mfs_`, `dag_`, `car_`, `routing_`, `swarm_`, `stats_`, `repo_`, plus a description, MCP annotations, a confirmation class, a `returnsUntrustedContent` flag, required RPC command paths and a minimum Kubo version.

#### Scenario: Tool registered without a contract entry
- **WHEN** a tool name is used by a consumer but is absent from the catalog
- **THEN** contract validation fails and names the missing tool

### Requirement: Schemas per tool
Each catalog entry MUST reference a JSON Schema 2020-12 input schema and output schema that validate against the 2020-12 metaschema.

#### Scenario: Invalid schema
- **WHEN** a referenced schema is not valid JSON Schema 2020-12
- **THEN** contract validation fails and names the schema file

### Requirement: Annotation consistency
A tool whose confirmation class is `destructive` SHALL have `destructiveHint` true and `readOnlyHint` false. A tool with confirmation class `none` SHALL have `readOnlyHint` true.

#### Scenario: Destructive tool marked read-only
- **WHEN** a catalog entry has confirmation class `destructive` and `readOnlyHint` true
- **THEN** contract validation fails

### Requirement: Derived WebMCP annotations
The contract SHALL define the derivation of the WebMCP annotations `readOnlyHint`, `consequentialHint`, `untrustedContentHint` and `debugging` from catalog fields. It SHALL NOT author those values independently.

#### Scenario: Content-returning tool
- **WHEN** a tool has `returnsUntrustedContent` true
- **THEN** its derived WebMCP `untrustedContentHint` is true

### Requirement: Content egress flag
Every tool whose output can carry node content SHALL be marked `contentEgress` in the catalog.

#### Scenario: Content tool without flag
- **WHEN** a tool has `returnsUntrustedContent` true and `contentEgress` false
- **THEN** contract validation fails

### Requirement: Config schema rejects literal secrets
The `ipfs mcp` config schema SHALL reject literal credential values and SHALL require credentials by environment variable or file reference.

#### Scenario: Literal key in example
- **WHEN** an example config contains a literal API key
- **THEN** contract validation fails and names the field
