# Spec Delta: ipfs-mcp-app-a2ui-seam

## Purpose

One shared component catalog supports first-party agent surfaces and compatible external MCP Apps while preserving host transport interoperability and safe inert fallbacks.

## ADDED Requirements

### Requirement: View-to-component mapping
Each supported MCP App view SHALL derive its shared catalog default and allowlist from `mcp/contract/derivations/a2ui-components.json`: content defaults to ContentCard, pins to PinList, mfs to MfsTree, dag to DagViewer and node-status to NodeStatusPanel. The mfs allowlist SHALL also retain ContentCard for existing mfs_read. Per-tool catalog metadata SHALL select the actual component within that allowlist; a default SHALL NOT overwrite a tool's declared component. Tool view metadata, exposed view identifiers and catalog mapping SHALL agree.

#### Scenario: Mapping complete
- **WHEN** contract metadata, exposed views and catalog mappings are compared
- **THEN** each supported tool view maps to an existing view and catalog component, with missing or conflicting links reported as validation failures

#### Scenario: MFS content differs from directory rendering
- **WHEN** mfs_read and mfs_ls render in the mfs view
- **THEN** their contract-derived components are respectively ContentCard and MfsTree, rather than a handwritten view default applied to both

### Requirement: Validated catalog negotiation
First-party AG-UI delivery SHALL require the exact catalogId/catalogVersion/protocolVersion tuple in the bounded forwardedProps.ipfs.a2ui.catalogs branch under the shared contract. At most four offers and 4096 encoded namespace bytes SHALL be accepted for capability selection. Claimed support SHALL not bypass component or output validation. Missing, malformed, oversize or incompatible declarations SHALL disable A2UI only, retaining ordinary run output and independent standard MCP App negotiation or a labelled inert fallback.

#### Scenario: Supported first-party catalog
- **WHEN** a first-party client advertises the compatible IPFS catalog and pin_ls returns schema-valid output
- **THEN** the stream carries the PinList surface bound to that output without requiring an HTML iframe resource for that result

#### Scenario: Incompatible declaration
- **WHEN** a client advertises an unknown version or malformed catalog capability
- **THEN** the server does not assume support for that catalog or send executable content based solely on the claim

### Requirement: External MCP App compatibility
MCP hosts using the standard app transport SHALL retain resource-linked rendering with the established MIME and view identifiers. The iframe exception SHALL be explicitly declared in the agreed resource metadata. Interoperability SHALL not require private correlation fields unsupported by conforming hosts.

#### Scenario: External pins view
- **WHEN** a compatible external MCP App host without the first-party catalog calls pin_ls
- **THEN** it can load ui://ipfs-mcp/pins as text/html;profile=mcp-app using the standard host transport

#### Scenario: Declaration present
- **WHEN** a supported app resource is inspected
- **THEN** its metadata identifies the iframe exception without altering required standard transport behavior

### Requirement: Trusted host message boundary
App views SHALL bind messages to the trusted parent source window and origin established through the host bootstrap contract, and use standard request correlation. They SHALL reject foreign sources, invalid origins and unmatched responses. Trust SHALL not be established by accepting the first arbitrary message.

#### Scenario: Foreign source window
- **WHEN** another window sends a plausible host message
- **THEN** the app ignores it without changing state or making a tool call

#### Scenario: Invalid origin or correlation
- **WHEN** a message has an untrusted origin or a response does not match an outstanding standard request
- **THEN** it is ignored and cannot mutate application state

#### Scenario: Valid standard host
- **WHEN** the trusted parent completes the agreed bootstrap and exchanges valid standard correlated messages
- **THEN** the app renders and communicates normally without a private per-render field requirement

### Requirement: Inert labelled fallback
Unknown components, incompatible versions and schema-invalid structured output SHALL render the label This result can't be shown here. Nothing in it runs. The fallback SHALL name the tool, display safe plain text and offer no actions except copying text. It SHALL never execute embedded markup, links or partial component actions.

#### Scenario: Unknown component
- **WHEN** a surface names a component outside the pinned catalog
- **THEN** the fallback shows tool name and plain text with no navigation or mutation actions

#### Scenario: Invalid structured output
- **WHEN** PinList data fails the pin_ls output schema
- **THEN** the fallback renders instead of a partly functional pin list

#### Scenario: Active content in text
- **WHEN** fallback text contains HTML, scripts or link-like instructions
- **THEN** those bytes remain inert text and trigger no fetches, navigation or execution

### Requirement: Shared consent and disabled behavior
Both delivery paths SHALL retain the same confirmation and decision-model rules for writes and destructive operations. A disabled app or first-party UI SHALL honor its documented off switch without enabling an alternate approval authority or changing Kubo RPC behavior.

#### Scenario: Write initiated from a rendered result
- **WHEN** a supported view offers a write action
- **THEN** it goes through the existing authorized confirmation path rather than performing a silent write

#### Scenario: App rendering disabled
- **WHEN** the operator disables app rendering
- **THEN** ordinary supported tool results remain available and no iframe or hidden replacement UI is started
