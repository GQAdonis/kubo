# Spec Delta: agent-chat-island

## Purpose

N4: `<ipfs-agent-chat>`, a React 19 shadow-root chat island with a right-side sliding shadcn sidebar.

## ADDED Requirements

### Requirement: Only talks to ipfs mcp
The island SHALL send requests only to the configured `ipfs mcp` endpoint and SHALL load no remote assets.

#### Scenario: Network audit
- **WHEN** the island runs a chat turn
- **THEN** every request goes to the configured `ipfs mcp` origin and no other host

### Requirement: Style isolation
Loading the island SHALL NOT change the appearance of host pages while the sidebar is closed.

#### Scenario: Closed island
- **WHEN** a Web UI page is rendered with the island loaded and closed
- **THEN** its screenshot matches the page without the island
