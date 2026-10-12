# Spec Delta: webui-webmcp

## Purpose

Lets agents in WebMCP-capable hosts use IPFS tools through the Web UI fork, with the operator in control.

## ADDED Requirements

### Requirement: Off by default
The Web UI SHALL NOT register any WebMCP tool unless the user has enabled WebMCP in Settings.

#### Scenario: Fresh profile
- **WHEN** the Web UI loads in a new browser profile
- **THEN** no tool is registered with `document.modelContext`

### Requirement: Feature detection
The Web UI SHALL register tools only when `document.modelContext` exists. It SHALL NOT fail when it is absent.

#### Scenario: Browser without WebMCP
- **WHEN** WebMCP is enabled in Settings but `document.modelContext` is undefined
- **THEN** the Web UI works normally and shows WebMCP as unavailable

### Requirement: Per-call confirmation for writes
Every write tool call MUST show an in-page confirmation naming the operation and its CIDs or paths, and MUST NOT run unless the user confirms.

#### Scenario: User cancels
- **WHEN** the user cancels the confirmation for `mfs_write`
- **THEN** no RPC request is sent, and the tool returns a declined result

### Requirement: The in-page approval is an intent check, hardened against accidental and scripted approval
The write approval dialog SHALL be presented as an intent and accident check, not a security boundary (ADR-0002, R13 amendment, decision A). It MUST act on Approve only for a trusted activation (`event.isTrusted` true and `event.detail >= 1`), and only after the request has been visible for at least 1 second and a fresh pointerdown has followed. While a request is open, the rest of the page MUST be inert and the document title MUST show a pending-approval cue.

#### Scenario: Scripted click
- **WHEN** page script calls `.click()` on Approve, or dispatches a synthetic click
- **THEN** the request stays pending, no RPC request is sent, and the tool result is not approved

#### Scenario: Double-click on a queued request
- **WHEN** a request is approved with a double-click and a second request is queued behind it
- **THEN** the second request is not approved by the second click, because its Approve arms only after 1 second visible plus a fresh pointerdown

#### Scenario: Background inert
- **WHEN** an approval is open
- **THEN** controls outside the dialog cannot be focused or activated, and the document title shows the pending cue until the request is answered

### Requirement: Bounded queue and a way out
At most 3 write requests SHALL wait at once; a further request MUST be declined immediately with no RPC request. The dialog MUST offer "Deny all and turn off agent writes", which declines every pending request and turns the writes toggle off. When the host passes no `AbortSignal`, a request MUST expire sooner than when it does; expiry is denial.

#### Scenario: Flood of writes
- **WHEN** the agent sends a fourth write while three are pending
- **THEN** the fourth is declined at once and no prompt is added

#### Scenario: Deny all
- **WHEN** the person chooses "Deny all and turn off agent writes"
- **THEN** every pending request returns a declined result, no RPC request is sent, and the writes toggle is off

### Requirement: Live off switch
Every tool `execute` MUST check the live Settings (WebMCP enabled, agent writes on for write tools, DNSLink resolution on for DNSLink names) and the current registration generation, and MUST return a declined result with no RPC request when any check fails, whether or not the host honored unregistration.

#### Scenario: Host ignores unregistration
- **WHEN** the person turns WebMCP off and the host still calls a previously registered tool
- **THEN** the call returns a declined result and no RPC request is sent

### Requirement: Honest toggle text
The agent-writes toggle SHALL state that an agent which controls the browser can approve its own writes, and that tool results go to the agent's AI provider. It SHALL recommend keeping agent writes off while the host runs in a browser-control or fully automatic mode.

#### Scenario: Reading the toggle
- **WHEN** the person opens Settings
- **THEN** the writes toggle text names both risks before the toggle is turned on

### Requirement: DNSLink resolution behind its own toggle
Read tools SHALL NOT resolve a DNSLink name (an `/ipns/` name or `ipns_resolve` argument that is not a libp2p-key CID) unless a separate Settings toggle, off by default, is on. IPNS names that are libp2p-key CIDs SHALL resolve regardless of that toggle. Paths with `.` or `..` segments SHALL be rejected before any RPC request.

#### Scenario: DNSLink off
- **WHEN** the DNSLink toggle is off and the agent calls `ipfs_cat` with `/ipns/example.com/a`
- **THEN** the call returns a declined result naming the DNSLink setting, and no RPC request is sent

#### Scenario: Key name
- **WHEN** the DNSLink toggle is off and the agent resolves a libp2p-key IPNS name
- **THEN** the name resolves

#### Scenario: Dot segments
- **WHEN** the agent calls `mfs_stat` with `/a/../ipns/example.com`
- **THEN** the call is rejected by input validation and no RPC request is sent
