# Spec Delta: webui-webmcp

## Purpose

Lets agents in WebMCP-capable hosts use IPFS tools through the Web UI fork, with the operator in control.

## ADDED Requirements

### Requirement: Off by default
The Web UI SHALL NOT register any WebMCP tool unless the user has enabled WebMCP in Settings.

#### Scenario: Fresh profile
- **WHEN** the Web UI loads in a new browser profile
- **THEN** no tool is registered with `document.modelContext`

### Requirement: Feature detection
The Web UI SHALL register tools only when `document.modelContext` exists. It SHALL NOT fail when it is absent.

#### Scenario: Browser without WebMCP
- **WHEN** WebMCP is enabled in Settings but `document.modelContext` is undefined
- **THEN** the Web UI works normally and shows WebMCP as unavailable

### Requirement: Per-call confirmation for writes
Every write tool call MUST show an in-page confirmation naming the operation and its CIDs or paths, and MUST NOT run unless the user confirms.

#### Scenario: User cancels
- **WHEN** the user cancels the confirmation for `mfs_write`
- **THEN** no RPC request is sent, and the tool returns a declined result
