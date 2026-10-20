# Spec Delta: ipfs-agent-context

## Purpose

Visible, removable reference context and truthful run facts help the operator understand what each agent turn receives and which tools and model it actually uses.

## ADDED Requirements

### Requirement: Provenance-labelled context strip
The composer SHALL show route, CID and IPFS-path references, model placement, memory scope and offered-tool facts with provenance. Labels SHALL distinguish From this page, Added by you, From settings and Reported by the server. Missing server facts SHALL say Not reported by this server rather than inventing values.

#### Scenario: Page reference and reported tools
- **WHEN** the current page supplies a valid IPFS-path reference and the run reports 12 offered tools from a catalog of 44
- **THEN** the strip shows the reference as From this page and 12 of 44 tools offered as Reported by the server

#### Scenario: Missing tool or memory facts
- **WHEN** a backend supplies neither an effective tool list nor a memory scope
- **THEN** those items say Not reported by this server without asserting a fabricated count or None

### Requirement: Removable references without uploads
The user SHALL be able to remove page and user-added references before sending. Removed page context SHALL remain removed across sends until the route changes. This feature SHALL accept references only, with no local attachment upload or implicit local file reading. Model and tools settings SHALL link to settings rather than masquerade as removable input.

#### Scenario: Remove page context
- **WHEN** the person removes a page reference and sends two messages without changing route
- **THEN** neither request contains that reference

#### Scenario: Route changes
- **WHEN** the route changes after removal
- **THEN** the strip visibly presents the new page's valid reference for the user to retain or remove before sending

#### Scenario: Settings link
- **WHEN** the tools or model settings item is activated
- **THEN** the application opens its agent settings without removing or inventing run facts

### Requirement: Bounded untrusted reference ingestion
Both built-in and UAR agent paths SHALL validate bounded route, CID and IPFS-path context and treat it as untrusted reference data, never system instructions. Unknown or oversized input SHALL be rejected or omitted visibly according to the shared contract. References SHALL not themselves trigger reads, network fetches or writes; access uses existing authorized tools.

#### Scenario: Valid references reach both backends
- **WHEN** the same valid bounded reference payload is sent through either configured agent backend
- **THEN** that turn receives the chosen references with provenance and untrusted-data framing

#### Scenario: Malicious or unsupported reference
- **WHEN** a reference contains instruction-like text, an unsupported local path or exceeds the contract limit
- **THEN** it cannot change system authority, open a local file or cause an implicit fetch, and invalid input is reported honestly

#### Scenario: Reference removed before send
- **WHEN** the strip removes a reference
- **THEN** neither backend receives it through a hidden fallback field

### Requirement: Inspector sections and retention
The inspector SHALL show Overview, Context, Prompt plan and Events for the current run with bounded event retention. It SHALL display reported facts and sent reference provenance without altering budgets. Events SHALL be read-only, bounded and safe to display, without secrets, raw system prompt text or unsafe raw tool arguments.

#### Scenario: Inspect a run
- **WHEN** the person opens the inspector after a turn
- **THEN** Overview shows reported placement, memory and egress; Context shows references actually sent; Prompt plan shows actual offered tools and available prompt digest; Events shows safe current-run events and reports any truncation

#### Scenario: Backend omitted a section's facts
- **WHEN** a backend provides no prompt digest or egress fact
- **THEN** the section remains present with Not reported by this server rather than a guessed value

### Requirement: Facts describe the actual run
Run-start facts SHALL describe that run's effective offered tools and actual selected prompt snapshot, with catalog size and prompt digest only when known. Built-in and UAR backends SHALL use the same optional contract. A full server tool registry or a newly reconstructed prompt SHALL NOT stand in for unavailable run facts.

The canonical transport SHALL be standard CUSTOM named ipfs.run.facts with RunFactsEvent from mcp/contract/agentic/run-context.schema.json, after RUN_STARTED and optional immediate a2ui.catalog and before tool/text. Standard RUN_STARTED fields SHALL remain compatible. Each supplied fact SHALL have its declared source provenance. Consumers SHALL bind thread_id and run_id to the inspected run. UAR tool/prompt/catalog/memory facts SHALL require a validated backend report and SHALL otherwise remain absent.

#### Scenario: Independent reference and A2UI validation
- **WHEN** valid bounded forwardedProps.ipfs.context accompanies an incompatible A2UI offer
- **THEN** A2UI alone is disabled and the accepted turn receives its untrusted references
- **WHEN** invalid context accompanies a valid A2UI offer
- **THEN** HTTP 422 invalid_argument is returned before either backend receives the turn

#### Scenario: Logical MFS and bounded syntax
- **WHEN** a typed mfs reference is supplied
- **THEN** it refers only to the connected node's logical MFS and causes no host file read or implicit tool call
- **WHEN** references exceed 16 entries or 16,384 encoded UTF-8 bytes, duplicate IDs or contain invalid CID/path syntax
- **THEN** the turn is rejected visibly before forwarding and no hidden generic context fallback is used

#### Scenario: Configuration off switches
- **WHEN** agent.context_references is false and the turn carries nonempty references
- **THEN** those turns receive HTTP 422 while ordinary messaging remains available
- **WHEN** agent.run_facts is false
- **THEN** no run-facts projection/event is emitted and existing runs and authority checks remain available

#### Scenario: Restricted tool selection
- **WHEN** the run is offered a subset of tools
- **THEN** offered_tools names exactly that subset rather than every registered server tool

#### Scenario: Retained prompt snapshot
- **WHEN** the backend retains the selected prompt snapshot
- **THEN** system_prompt_digest identifies that snapshot with a sha256 prefix and no prompt text is disclosed

#### Scenario: UAR facts unavailable
- **WHEN** UAR does not report the actual prompt or effective tool set
- **THEN** the corresponding fields remain unreported and the interface displays that limitation truthfully

### Requirement: Event-derived lifecycle words
Each turn SHALL show lifecycle words derived from actual events: Planning, Calling a named tool, Waiting for approval, Running, then an evidenced Done, Failed or Cancelled outcome. It SHALL not manufacture phases or treat an approval interruption as failure. Reduced motion SHALL preserve all semantic words without animation.

#### Scenario: Approval interruption
- **WHEN** a turn starts, calls pin_rm and interrupts for confirmation
- **THEN** it shows Planning, Calling pin_rm and Waiting for approval as those events occur, without claiming completion

#### Scenario: Missing final event
- **WHEN** a connection ends without a reliable terminal outcome
- **THEN** the interface communicates missing or uncertain status and does not invent Done

#### Scenario: Reduced motion
- **WHEN** reduced motion is requested
- **THEN** the same lifecycle words remain visible without animation
