# Spec Delta: mcp-app-ui

## Purpose

Serves IPFS MCP App views as `ui://` resources so capable hosts can render tool results interactively.

## ADDED Requirements

### Requirement: Embedded ui resources
`ipfs mcp` SHALL serve its MCP App views as `ui://` resources from content embedded in the binary. It SHALL NOT fetch any remote origin.

#### Scenario: Read a ui resource offline
- **WHEN** a client reads a `ui://` resource with networking disabled
- **THEN** the full view content is returned

### Requirement: Tools work without UI
Every tool linked to a view SHALL still return text and structured content usable without rendering the view.

#### Scenario: Host without MCP Apps
- **WHEN** a host that does not render MCP Apps calls `pin_ls`
- **THEN** it receives the structured pin list
