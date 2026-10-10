# Spec Delta: mcp-dag-car-tools

## Purpose

Exposes IPLD DAG reads and writes and CAR import/export as MCP tools with explicit pinning behavior.

## ADDED Requirements

### Requirement: Explicit root pinning on import
`car_import` SHALL require the caller to state `pin_roots` explicitly. It SHALL be confirmed as a write.

#### Scenario: Import without pin_roots
- **WHEN** `car_import` is called without `pin_roots`
- **THEN** the call fails with a validation error, and nothing is imported

### Requirement: Bounded export
`car_export` SHALL return a resource link, or base64 content no larger than the configured cap.

#### Scenario: Large DAG
- **WHEN** the CAR for a root exceeds the cap
- **THEN** a resource link is returned instead of inline content
