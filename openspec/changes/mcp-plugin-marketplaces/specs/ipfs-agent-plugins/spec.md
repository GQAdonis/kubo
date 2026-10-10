# Spec Delta: ipfs-agent-plugins

## Purpose

Distributes the IPFS skills and the `ipfs mcp` server registration to Claude Code, Codex and OpenCode through their plugin marketplaces.

## ADDED Requirements

### Requirement: Server registration
Each plugin SHALL register an MCP server that launches `ipfs mcp` over stdio, with no hardcoded RPC endpoint.

#### Scenario: Plugin install
- **WHEN** the plugin is installed in its harness and the harness lists MCP servers
- **THEN** an IPFS server launched as `ipfs mcp` is listed

### Requirement: Skills included
Each plugin MUST include the six IPFS skills from `mcp/skills`.

#### Scenario: Skill availability
- **WHEN** the plugin is installed
- **THEN** the harness lists all six skills
