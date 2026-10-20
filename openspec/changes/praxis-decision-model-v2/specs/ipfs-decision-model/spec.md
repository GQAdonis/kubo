# Spec Delta: ipfs-decision-model

## Purpose

One shared decision model makes approval, actual execution outcomes and the authorized answering surface visible without weakening write confirmation or inventing success.

## ADDED Requirements

### Requirement: Safe outcome transport to ordinary MCP App hosts
The authorized current winning node invocation MAY include ToolOutcomeMetadata at CallToolResult._meta["ipfs-mcp"], containing confirmation_id, effect_digest and execution. Existing output schemas and content/structuredContent/isError SHALL remain unchanged. The standard host tool-result channel SHALL suffice without HTTP inbox credentials or a private handshake. Only execution.receipt contains success evidence and SHALL match the current decision, tool, digest and completion time. Raw arguments/results, BindingKey, tokens and grants SHALL NOT be included.

#### Scenario: External host without inbox credentials
- **WHEN** the standard MCP App tool-result notification supplies a valid matching successful execution receipt
- **THEN** the shared decision model can show the actual operation outcome without accessing an authenticated HTTP inbox

#### Scenario: Legacy result and delivery failure
- **WHEN** a legacy result omits outcome metadata
- **THEN** ordinary tool output remains available without a fabricated decision receipt
- **WHEN** a recorded successful operation is followed by a result-delivery error
- **THEN** its successful receipt remains truthful and the delivery error is shown separately without automatic retry

#### Scenario: Equal observed timestamps
- **WHEN** the clock reports the same start and completion instant
- **THEN** started_at equals completed_at is valid, the receipt uses that completion instant and no extra nanosecond is fabricated

### Requirement: Eleven decision presentations
The shared model SHALL expose the following eleven presentations with the stated labels and controls. Authorization SHALL further restrict controls; display state alone SHALL NOT grant authority. Web approval SHALL arm after one second; the native Desktop window SHALL preserve its existing longer arming interval.

#### Scenario: Complete state and control matrix
- **WHEN** the approval card, confirmation dialog and inbox render each supported presentation
- **THEN** their labels and possible controls match this matrix:

| Presentation | Label | Possible controls |
|---|---|---|
| `pending` | Awaiting a person | Deny (initial focus), Approve after arm |
| `waiting_system_prompt` | Waiting for system prompt | Deny |
| `approved` | Approved, not yet run | none |
| `committed` | Done | View receipt |
| `denied` | Denied. Nothing was changed. | none |
| `expired` | Expired. Nothing was changed. | Ask again |
| `invalidated` | Changed since you saw it | Review again |
| `resolved_elsewhere` | Answered on another screen | none |
| `uncertain` | Outcome unknown, checking | Check now |
| `needs_resubmit` | Can't resume. Ask again to create a new request. | Ask again |
| `failed_after_approval` | Failed after approval | none |

#### Scenario: Native approval remains native
- **WHEN** a card represents a native-presence-required decision
- **THEN** it identifies the system prompt as the answering surface and exposes no web approval path

#### Scenario: Arm restarts on changed content
- **WHEN** the displayed payload or digest changes while approval is armed
- **THEN** approval becomes inactive until fresh review and the full applicable arming interval have completed

### Requirement: Card fields
Every card SHALL show the prepared tool and human description, destination, CID or path scope, Write or Destructive class, named authority, egress and short effect digest. Missing facts SHALL be identified as unreported rather than invented. The short digest SHALL retain its `sha256:` prefix and first four and last two hex characters.

#### Scenario: Destructive pin removal
- **WHEN** a pin removal decision supplies targets, authority, destination and digest
- **THEN** the card shows those facts, the word Destructive and a digest such as `sha256:7f3a…c1`

#### Scenario: Missing digest
- **WHEN** a server supplies no effect digest
- **THEN** the card states that the server supplied no digest and does not manufacture one or weaken the existing confirmation checks

### Requirement: Approval consumption is not execution success
The model SHALL keep confirmation approval and one-time consumption distinct from handler outcome. Legacy `executed` alone SHALL NOT mean Done. Only a confirmed successful outcome SHALL produce `committed` and a receipt. Reads, reconnects and repeated confirm attempts SHALL NOT execute the write again.

#### Scenario: Claim before handler
- **WHEN** the server consumes approval and reports legacy `executed` before the handler completes
- **THEN** the surface does not show Done or offer a successful receipt

#### Scenario: Successful handler
- **WHEN** the handler reports confirmed success
- **THEN** the surface shows Done and View receipt naming the tool, completion time and actual returned CID or path when present, with absent result facts explicitly unreported

#### Scenario: Once-only consumption
- **WHEN** the same approval is submitted twice or a receipt is reread
- **THEN** the write is executed at most once and the receipt read is read-only

### Requirement: Honest failure and ambiguity
A known unsuccessful outcome after approval SHALL show Failed after approval and available safe error detail. It SHALL NOT claim zero effects. Missing, timed-out, partial or transport-ambiguous outcomes SHALL show Outcome unknown, checking rather than success or definite failure. No such state SHALL automatically retry a write.

#### Scenario: Known unsuccessful outcome
- **WHEN** a handler supplies a definite failure after approval
- **THEN** the card shows Failed after approval, offers no implicit retry and makes no claim that nothing changed

#### Scenario: Missing outcome
- **WHEN** an approved operation reaches the bounded outcome timeout without a reliable final outcome
- **THEN** Check now reads the current outcome without re-running the call and the card remains visibly uncertain until reliable evidence arrives

#### Scenario: Partial effect or interrupted transport
- **WHEN** a handler may have changed data before an uncertain termination
- **THEN** the card remains uncertain and does not substitute the pre-execution Denied or Expired no-change labels

### Requirement: Digest-bound confirmation
Approval SHALL bind to the reviewed effect digest and existing ownership, expiry and resolver checks. A changed digest or rejected stale confirmation SHALL invalidate the displayed decision. New optional metadata SHALL preserve older-client compatibility and SHALL NOT introduce an approval bypass.

#### Scenario: Changed arguments
- **WHEN** a newer payload has a different digest or the server rejects a digest mismatch
- **THEN** the earlier approval cannot execute the changed call and the surface shows Changed since you saw it with Review again

#### Scenario: Older client
- **WHEN** a client omits newly optional presentation metadata
- **THEN** existing authorization and once-only confirmation behavior remain effective without requiring invented surface attribution

### Requirement: Resolver and answering surface are separate
The model SHALL distinguish the authorized resolver mechanism from descriptive answering-surface attribution. A remote answer SHALL identify its actual surface when known. Confirmed terminal success or failure SHALL take precedence over Answered on another screen; attribution SHALL remain visible without hiding the outcome.

Automatic expiry or withdrawal SHALL NOT invent a human resolver/surface. Genuine prior human approval attribution SHALL remain as historical provenance when an approved-but-unclaimed record later expires or is automatically withdrawn; legacy Resolver remains unchanged. The current terminal approval state/outcome SHALL determine presentation and disable actions. Retained historical attribution SHALL NOT imply that the automatic transition was performed by that person or that approval is still usable.

#### Scenario: Approved but unclaimed record expires
- **WHEN** a human approved on a known surface and the record expires before invocation
- **THEN** the observation retains the actual prior approver attribution, displays Expired with no approval actions and no execution record, and does not display still-approved or Answered on another screen

#### Scenario: Pending record expires without human answer
- **WHEN** an unanswered pending record expires automatically
- **THEN** resolver and surface attribution remain absent and no fictional human actor is reported

#### Scenario: Desktop answer
- **WHEN** another surface answers and the server reports the Desktop confirmation window
- **THEN** the waiting card names that window and does not gain Desktop resolver authority

#### Scenario: Terminal outcome after another surface answers
- **WHEN** the operation then succeeds or definitively fails
- **THEN** the card shows Done or Failed after approval and retains the answering-surface attribution

### Requirement: Restart requires a new request
A restored decision unknown to the current server SHALL become `needs_resubmit`. Ask again SHALL create a new request and new confirmation identifier without replaying an old token or automatically executing the old write. Decision history SHALL not be persisted to the Kubo repository.

#### Scenario: Restart clears confirmation
- **WHEN** a locally restored pending card refers to an unknown server confirmation
- **THEN** it shows Can't resume. Ask again to create a new request. and sends no old confirm token

### Requirement: Generated catalog parity
ApprovalCard and ConfirmationDialog catalog entries SHALL derive their eleven presentations from the shared model. Generated consumer types and fixtures SHALL agree with that contract rather than define independent state sets.

#### Scenario: Drift fails validation
- **WHEN** a presentation exists in the source contract but is missing from generated catalog entries or consumer fixtures
- **THEN** final validation fails and names the missing presentation

### Requirement: Disabled decision features preserve confirmation
Decision presentation and outcome features SHALL retain documented defaults and off switches. Disabling them SHALL leave existing confirmation requirements effective and SHALL not enable a third-party endpoint.

#### Scenario: Decision presentation disabled
- **WHEN** an operator disables the new presentation features
- **THEN** write and destructive tools still require their authorized confirmation and normal Kubo behavior remains available

### Requirement: Canonical additive decision contract
The implementation SHALL use `mcp/contract/confirmation/confirmation.schema.json` and its normative `README.md` as the shared source of decision shapes and semantics. The original pending/error required fields and token/request-state v1 acceptance SHALL remain compatible. New optional metadata SHALL NOT replace the existing immutable session, identity, tool, argument, expiry, resolver and single-use binding.

#### Scenario: Actual outcome is separate from legacy approval
- **WHEN** an approved call is consumed before invocation
- **THEN** legacy state may be `executed`, while additive `execution` records running, succeeded, failed or unknown truth; only matching `SuccessfulReceipt` evidence permits Done

#### Scenario: Resolver is distinct from attribution
- **WHEN** the server supplies `resolved_by` and `resolved_surface`
- **THEN** `resolved_by` names an existing authorized mechanism, the optional surface instance distinguishes panes only for display, and any legacy `resolver` agrees with the mechanism

#### Scenario: Safe source-distinguished observations
- **WHEN** an authenticated identity reads a node or UAR decision
- **THEN** the projection is `DecisionObservation`, filtered privately by full identity binding, with no raw binding key, subject, token, grant, sensitive argument payload or other identity's native request; `source/source_kind` distinguish node confirmation from UAR backend approval

#### Scenario: Continuation carries no new authority
- **WHEN** an owned open interruption supplies continuation metadata
- **THEN** it identifies only the existing authenticated `/agui` thread/interrupt resolution path; owner, open interrupt, digest, expiry and allowed resolver are rechecked, and all other observations offer only the known authorized surface

#### Scenario: Wrong receipt cannot claim success
- **WHEN** a structurally valid receipt has a different decision ID, tool, effect digest or inconsistent completion time
- **THEN** semantic validation refuses the Done presentation; UAR text or stream completion alone cannot provide node success evidence

#### Scenario: Consumer generation preserves constraints
- **WHEN** SU generates the confirmation types and shared model
- **THEN** every referenced definition and conditional source/native/execution/capability constraint is represented faithfully, without changing the JSON Schema to accommodate an incomplete converter
