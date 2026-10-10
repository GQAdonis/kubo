# Spec Delta: ipfs-agent-skills

## Purpose

Packages IPFS task knowledge as agentskills.io-compliant skills aligned with the MCP tool contract.

## ADDED Requirements

### Requirement: Coverage of six areas
The skill set SHALL cover push/pull, pinning, IPNS, MFS, DAG/CAR and troubleshooting.

#### Scenario: Skill inventory
- **WHEN** `mcp/skills` is listed
- **THEN** one skill exists per area

### Requirement: Contract alignment and validity
Every tool name a skill mentions MUST exist in `mcp/contract/catalog.json`. Every skill MUST pass `skills-ref validate`.

#### Scenario: Unknown tool name
- **WHEN** a skill references a tool absent from the catalog
- **THEN** the skill check fails
