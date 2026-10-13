# Spec Delta: mcp-routing-diag-tools

## Purpose

Exposes routing, provide and diagnostic operations of the connected node to agents, while respecting operator provide settings.

## ADDED Requirements

### Requirement: Provide respects node configuration
`routing_provide` SHALL NOT announce content when the node has `Provide.Enabled=false`. It SHALL report that providing is disabled.

#### Scenario: Provide disabled
- **WHEN** `routing_provide` is confirmed on a node with `Provide.Enabled=false`
- **THEN** no announcement is made, and the result states providing is disabled

### Requirement: Garbage collection confirmed
`repo_gc` SHALL require destructive confirmation.

#### Scenario: Unconfirmed gc
- **WHEN** `repo_gc` is called without confirmation
- **THEN** no garbage collection runs
