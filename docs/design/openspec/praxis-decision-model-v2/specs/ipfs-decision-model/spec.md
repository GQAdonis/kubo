# Spec Delta: ipfs-decision-model

## Purpose

One decision model for every surface that shows a confirmation for a write or destructive tool call, aligned with KnowMe Praxis ed. 1.2 WS-05, API-06, CT-05 and HOME-10 #15.

State table (verbatim from analysis D5):

| State | Badge word | Controls |
|---|---|---|
| `pending` | Awaiting a person | Deny (focused), Approve after 1s arm |
| `waiting_system_prompt` | Waiting for system prompt | Deny |
| `approved` | Approved, not yet run | none |
| `committed` | Done | View receipt |
| `denied` | Denied. Nothing was changed. | none |
| `expired` | Expired. Nothing was changed. | Ask again |
| `invalidated` | Changed since you saw it | Review again |
| `resolved_elsewhere` | Answered on another screen | none |
| `uncertain` | Outcome unknown, checking | Check now |
| `needs_resubmit` | Can't resume. Ask again to create a new request. | Ask again |

## ADDED Requirements

### Requirement: Ten decision states
The decision model SHALL define exactly ten states. Each state SHALL render with exactly one badge word and only the controls listed for it in the state table in the Purpose section; the scenarios below restate each row.

#### Scenario: Pending
- **WHEN** a card renders in `pending`
- **THEN** the badge word is "Awaiting a person" and the controls are Deny (focused) and Approve after 1s arm

#### Scenario: Waiting for the system prompt
- **WHEN** a card renders in `waiting_system_prompt`
- **THEN** the badge word is "Waiting for system prompt" and the only control is Deny

#### Scenario: Approved
- **WHEN** a card renders in `approved`
- **THEN** the badge word is "Approved, not yet run" and no control is rendered

#### Scenario: Committed
- **WHEN** a card renders in `committed`
- **THEN** the badge word is "Done" and the only control is View receipt

#### Scenario: Denied
- **WHEN** a card renders in `denied`
- **THEN** the badge word is "Denied. Nothing was changed." and no control is rendered

#### Scenario: Expired
- **WHEN** a card renders in `expired`
- **THEN** the badge word is "Expired. Nothing was changed." and the only control is Ask again

#### Scenario: Invalidated
- **WHEN** a card renders in `invalidated`
- **THEN** the badge word is "Changed since you saw it" and the only control is Review again

#### Scenario: Resolved elsewhere
- **WHEN** a card renders in `resolved_elsewhere`
- **THEN** the badge word is "Answered on another screen" and no control is rendered

#### Scenario: Uncertain
- **WHEN** a card renders in `uncertain`
- **THEN** the badge word is "Outcome unknown, checking" and the only control is Check now

#### Scenario: Needs resubmit
- **WHEN** a card renders in `needs_resubmit`
- **THEN** the badge word is "Can't resume. Ask again to create a new request." and the only control is Ask again

#### Scenario: Every state renders its word and controls
- **WHEN** the approval card and the confirmation dialog render a fixture for each of the ten states
- **THEN** each shows exactly the badge word in the table, and the set of rendered controls equals the table's controls for that state

#### Scenario: Pending arms Approve after one second
- **WHEN** a `pending` card first renders on a surface that can answer
- **THEN** initial focus is on Deny, and the Approve control does not act until one second has passed since the card rendered or its content last changed

#### Scenario: Waiting for the system prompt offers no approval
- **WHEN** a card renders in `waiting_system_prompt`
- **THEN** only Deny is rendered, and the text names the system prompt as the place where approval happens

### Requirement: Card fields
Every decision card SHALL show the prepared object (tool and human description), the destination (node, remote pinning service or IPNS name), the scope (CIDs or paths), the class in words ("Write" or "Destructive"), the named authority (principal kind and name), the egress badge, and a short effect digest in the form `sha256:` followed by the first four and last two hex characters of the digest.

#### Scenario: Destructive pin removal card
- **WHEN** a `pending` card renders a `pin_rm` confirmation with targets, a principal, a destination of this node and an effect digest
- **THEN** the card shows the tool description, "This node" as destination, each target CID, the word "Destructive", the principal kind and name, the egress badge, and the short digest such as `sha256:7f3a…c1`

#### Scenario: Server supplies no digest
- **WHEN** a confirmation payload has no `effect_digest`
- **THEN** the digest row states that the server supplied no digest, and the card does not invent one

### Requirement: Accepted is not committed
The `approved` state SHALL NOT state or imply that the effect ran. Only `committed` SHALL say the effect ran, and it SHALL link to a receipt naming the tool, the resulting CID or path, and the time.

#### Scenario: Approval before execution
- **WHEN** a person approves and the server reports `approved` but not `executed`
- **THEN** the card shows "Approved, not yet run" and no receipt control

#### Scenario: Execution reported
- **WHEN** the server reports the contract state `executed` for the confirmation
- **THEN** the card shows "Done" with a "View receipt" control that opens the receipt with tool, CID or path, and time

### Requirement: Invalidation on digest change
A decision SHALL be bound to the effect digest shown to the person. When the effect digest for the confirmation changes, or the server rejects a confirm because the digest no longer matches, the card SHALL move to `invalidated` and the earlier approval SHALL NOT be used to run the call.

#### Scenario: Arguments change after review
- **WHEN** a newer payload for the same confirmation id carries a different `effect_digest` than the one rendered
- **THEN** the card shows "Changed since you saw it" with a "Review again" control, and no Approve control acts until the person reviews the new digest

#### Scenario: Server rejects a stale confirm
- **WHEN** the server answers a confirm with a digest-mismatch rejection
- **THEN** the card moves to `invalidated`, and the tool call does not run

### Requirement: Resolved elsewhere names the surface
When a decision is answered on a different surface, the card SHALL move to `resolved_elsewhere` and SHALL name that surface in words.

#### Scenario: Answered in the Desktop window
- **WHEN** the chat shows a pending card and a state change arrives with `resolved_by` set to `desktop`
- **THEN** the chat card shows "Answered on another screen" and names "the IPFS Desktop confirmation window", with no controls

### Requirement: Uncertain on timeout
When an approved decision reaches no final contract state within the outcome timeout, the card SHALL move to `uncertain`. The card SHALL NOT present this as a failure.

#### Scenario: Outcome does not arrive
- **WHEN** a confirmation stays `approved` past the outcome timeout
- **THEN** the card shows "Outcome unknown, checking" with a "Check now" control, uses no error tone, and "Check now" reads the current state without re-running the call

### Requirement: Resubmit after restart
A decision restored from persistence whose confirmation the server no longer knows SHALL move to `needs_resubmit`. "Ask again" SHALL create a new request with a new confirmation id and SHALL NOT replay the old confirm token.

#### Scenario: Server restarted while pending
- **WHEN** the host reloads a persisted `pending` decision and the server reports the confirmation id as unknown
- **THEN** the card shows "Can't resume. Ask again to create a new request." with an "Ask again" control

#### Scenario: Ask again sends no stale token
- **WHEN** the person activates "Ask again" on an `expired` or `needs_resubmit` card
- **THEN** a new request is created with a new confirmation id, and the old confirm token is not sent

### Requirement: Catalog parity
The A2UI catalog entries for `ApprovalCard` and `ConfirmationDialog` SHALL list all ten states and SHALL be generated from the model, not hand-edited.

#### Scenario: Catalog drift
- **WHEN** catalog validation runs after a state is added to the model but the catalog is not regenerated
- **THEN** validation fails and names the missing state
