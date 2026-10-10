# Spec Delta: mcp-pin-tools

## Purpose

Exposes the connected node's pinning operations as MCP tools with contract-defined schemas and confirmation classes.

## ADDED Requirements

### Requirement: Pin listing with pagination
`pin_ls` SHALL return pins with cursor pagination as defined in the contract.

#### Scenario: Paginated listing
- **WHEN** the node has more pins than the page size
- **THEN** `pin_ls` returns one page and a cursor that yields the rest

### Requirement: Pin writes and removal are confirmed
`pin_add` and `pin_update` SHALL be confirmed as writes. `pin_rm` SHALL be confirmed as destructive.

#### Scenario: Confirmed removal
- **WHEN** `pin_rm` is called with a valid confirm token for that CID
- **THEN** the pin is removed and the result reports the CID
