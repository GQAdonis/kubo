# Spec Delta: mcp-agui-a2ui

## Purpose

Phase `agent-backend-settings`, change `mcp-agui-a2ui-registry` (decision D15): the AG-UI stream of `ipfs mcp` advertises the A2UI catalog and carries server-built A2UI surfaces. Model-composed surfaces are out of scope.

## ADDED Requirements

### Requirement: Catalog discovery
`ipfs mcp` SHALL serve the generated A2UI catalog (`mcp/ui/catalog/ipfs-mcp.catalog.json`, embedded) at the route named in the contract and SHALL advertise it in the AG-UI run stream.

#### Scenario: Client discovers the catalog
- **WHEN** an AG-UI client starts a run
- **THEN** the stream names the catalog route and fetching it returns the embedded catalog unchanged

### Requirement: Server-built surfaces only from the catalog
When `agent.a2ui` is on, `ipfs mcp` SHALL emit A2UI v0.9 surface messages as AG-UI `CUSTOM` events built from tool results by the rules in `mcp/contract/derivations/a2ui-components.json`, and SHALL NOT emit any component that is not in the catalog.

#### Scenario: Tool result rendered
- **WHEN** a read tool returns a result during a run
- **THEN** the emitted surface uses only component names present in the catalog

### Requirement: Off switch
`agent.a2ui: false` SHALL stop all A2UI `CUSTOM` events while the rest of the AG-UI stream is unchanged.

#### Scenario: A2UI disabled
- **WHEN** `agent.a2ui` is `false` and a tool result is produced
- **THEN** the stream contains no A2UI `CUSTOM` event
