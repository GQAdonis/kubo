# Spec Delta: ipfs-mcp-app-a2ui-seam

## Purpose

One catalog behind both the MCP App views and A2UI surfaces, with the iframe path declared as an exception, aligned with KnowMe Praxis ed. 1.2 UI-01, UI-02, UI-08 and UI-10.

## ADDED Requirements

### Requirement: View-to-component mapping
Each MCP App view SHALL map to exactly one A2UI catalog component: `content` to `ContentCard`, `pins` to `PinList`, `mfs` to `MfsTree`, `dag` to `DagViewer`, `node-status` to `NodeStatusPanel`.

#### Scenario: Mapping complete
- **WHEN** the views test compares the contract's `ui.appView` values, `VIEW_IDS` and the catalog components
- **THEN** every `appView` has a view, every view maps to a catalog component, and the test fails if any link is missing

### Requirement: A2UI delivery to first-party hosts
When an AG-UI client advertises the IPFS catalog id, the server SHALL deliver results of tools with an `appView` as A2UI surface updates bound to the tool's structured output, and SHALL NOT send a `ui://` HTML resource link for those results.

#### Scenario: Web UI asks for pins
- **WHEN** the chat island, advertising the IPFS catalog id, runs a turn that calls `pin_ls`
- **THEN** the stream carries an A2UI surface update for `PinList` bound to the `pin_ls` output, and no `ui://ipfs-mcp/pins` link

#### Scenario: External MCP host
- **WHEN** an MCP host that supports MCP Apps and does not advertise the catalog id calls `pin_ls`
- **THEN** the result links `ui://ipfs-mcp/pins` with MIME `text/html;profile=mcp-app` as before

### Requirement: Declared iframe exception
Each MCP App view resource SHALL declare that it is an iframe exception, and the view SHALL accept host messages only from the origin of the first host message and only with the correlation id it issued for that render.

#### Scenario: Message from another origin
- **WHEN** a view receives a message from an origin other than the first host message's origin
- **THEN** the view ignores it and changes no state

#### Scenario: Wrong correlation id
- **WHEN** a view receives a message from the right origin with a correlation id it did not issue
- **THEN** the view ignores it and changes no state

### Requirement: Inert labelled fallback
A surface that names an unknown component, a component outside the pinned catalog version, or output that fails the tool's output schema SHALL render a fallback card that states "This result can't be shown here. Nothing in it runs.", names the tool, shows the result's text content, and offers no actions.

#### Scenario: Unknown component
- **WHEN** the renderer receives a surface naming a component not in the catalog
- **THEN** it renders the fallback card with the tool name and text, and the card has no buttons or links other than copy text

#### Scenario: Output fails schema
- **WHEN** a `PinList` surface is bound to data that fails the `pin_ls` output schema
- **THEN** the fallback card renders instead of a partial pin list
