# Spec Delta: ipfs-agent-context

## Purpose

Visible, removable context and named turn phases in the chat island, aligned with KnowMe Praxis ed. 1.2 WS-06, CX-01 and the turn anatomy.

## ADDED Requirements

### Requirement: Context strip
The chat island SHALL show a context strip above the composer listing container, files attached, model placement, memory scope and tools offered. Each item SHALL show its provenance ("From this page", "Added by you", "From settings" or "Reported by the server").

#### Scenario: Strip on a file page
- **WHEN** the island opens while the Web UI shows an MFS path and the server reports 12 offered tools out of 44
- **THEN** the strip shows the path as container labelled "From this page", "12 of 44 tools offered" labelled "Reported by the server", memory scope "None", and the model placement

#### Scenario: Server reports no tool facts
- **WHEN** the run-start payload has no `offered_tools`
- **THEN** the tools item reads "Not reported by this server" and no count is shown

### Requirement: Removable context
Container and attached files SHALL be removable before send, and a removed item SHALL NOT be included in the request. Tools offered and model placement SHALL link to agent settings instead of being removable.

#### Scenario: Remove the container
- **WHEN** the person removes the container item and sends a message
- **THEN** the request carries no container, and the item stays removed until the route changes

#### Scenario: Settings items
- **WHEN** the person activates the tools-offered item
- **THEN** the Web UI opens `#/settings/agent`, and nothing is removed

### Requirement: Context inspector
The context inspector SHALL open from the strip and SHALL contain the sections Overview, Context, Prompt plan and Events. It SHALL display server-reported facts and SHALL NOT compute or alter budgets.

#### Scenario: Inspector sections
- **WHEN** the person opens the inspector after one turn
- **THEN** Overview shows container, model placement, memory scope and egress host; Context shows the items sent with that turn and their provenance; Prompt plan shows the offered tool names and the system prompt digest; Events lists the current run's AG-UI events read-only

### Requirement: Run-start facts
The server SHALL include in the AG-UI run-start payload the offered tool names, the contract's total tool count and a short system prompt digest. It SHALL NOT include the system prompt text.

#### Scenario: Run-start payload
- **WHEN** a run starts with 12 tools offered
- **THEN** the payload has 12 entries in `offered_tools`, `catalog_size` equal to the contract tool count, a `system_prompt_digest` beginning with `sha256:`, and no system prompt text

### Requirement: Lifecycle strip
Each turn SHALL show its current phase by name, derived from AG-UI events: Planning, Calling `<tool>`, Waiting for approval, Running, then Done, Failed or Cancelled. A phase SHALL NOT be shown by animation alone.

#### Scenario: Turn with an approval
- **WHEN** a turn starts, calls `pin_rm`, and is interrupted for confirmation
- **THEN** the strip shows Planning, then "Calling pin_rm", then "Waiting for approval", each as a word

#### Scenario: Reduced motion
- **WHEN** `prefers-reduced-motion: reduce` is set
- **THEN** the strip shows the same words with no animation
