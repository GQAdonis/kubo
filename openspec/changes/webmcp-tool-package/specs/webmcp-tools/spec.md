# Spec Delta: webmcp-tools

## Purpose

Provides WebMCP tool definitions and executors derived from the shared IPFS MCP contract for use in browser pages.

## ADDED Requirements

### Requirement: Generated from the contract
Tool names, descriptions, input schemas and WebMCP annotations SHALL be generated from `mcp/contract`. They SHALL NOT be authored separately.

#### Scenario: Contract change
- **WHEN** a contract tool description changes and the generator is not re-run
- **THEN** the package's drift test fails

### Requirement: Confirmation hook required for writes
An executor for a tool whose confirmation class is not `none` MUST call the host's confirmation hook. It MUST NOT perform the RPC call unless the hook confirms.

#### Scenario: Hook declines
- **WHEN** the confirmation hook returns false for `pin_add`
- **THEN** no RPC request is sent

### Requirement: No destructive tools over WebMCP
The package SHALL NOT export destructive tools in this phase.

#### Scenario: Exported tool set
- **WHEN** the generated tool list is inspected
- **THEN** it contains no tool with confirmation class `destructive`
