# Spec Delta: mcp-mfs-tools

## Purpose

Exposes the connected node's MFS as MCP tools, with writes and removals confirmed.

## ADDED Requirements

### Requirement: MFS operations
The server SHALL provide list, stat, read, write, mkdir, copy, move and remove operations on MFS paths, with outputs matching the contract schemas.

#### Scenario: Write then read
- **WHEN** `mfs_write` (confirmed) writes text to `/notes/a.txt` and `mfs_read` reads it
- **THEN** the read returns the written text

### Requirement: Destructive removal confirmed
`mfs_rm` SHALL require destructive confirmation.

#### Scenario: Unconfirmed removal
- **WHEN** `mfs_rm` is called without confirmation
- **THEN** the path still exists
