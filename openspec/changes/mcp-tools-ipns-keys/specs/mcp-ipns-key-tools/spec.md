# Spec Delta: mcp-ipns-key-tools

## Purpose

Lets agents publish and resolve IPNS records and manage signing keys of the connected node, with no way to export private keys.

## ADDED Requirements

### Requirement: No private key export
The server MUST NOT expose any tool that returns private key material.

#### Scenario: Tool list
- **WHEN** `tools/list` is called
- **THEN** no tool exports or imports keys

### Requirement: Publish and resolve
`ipns_publish` SHALL publish a path under a named key after write confirmation. `ipns_resolve` SHALL return the resolved path.

#### Scenario: Round trip
- **WHEN** a CID is published under key `k` (confirmed) and the name is resolved
- **THEN** the resolved path is that CID

### Requirement: Key removal confirmed
`key_rm` SHALL require destructive confirmation.

#### Scenario: Unconfirmed key removal
- **WHEN** `key_rm` is called without confirmation
- **THEN** the key is still listed by `key_list`
