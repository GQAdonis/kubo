# Spec Delta: ipfs-decision-inbox

## Purpose

An identity-scoped decision inbox and Activity history show pending work across the current operator's sessions, with bounded local history and no expansion of approval authority.

## ADDED Requirements

### Requirement: Identity-scoped read-only decision lookup
The MCP listener SHALL provide authenticated read-only list and item/outcome lookup for the current identity across its sessions. Identity binding SHALL include the authenticated subject, not only a shared principal name. Responses SHALL exclude confirmation secrets and SHALL not disclose other identities' decisions.

#### Scenario: Same identity across sessions
- **WHEN** an identity opens two authenticated sessions
- **THEN** both can read its pending decisions and recent outcomes with shared activity identifiers, revision and model fields

#### Scenario: Distinct OIDC subjects
- **WHEN** two subjects share a principal configuration name
- **THEN** neither can list, read or subscribe to the other's decisions

#### Scenario: Read-only endpoint
- **WHEN** a client attempts a mutation through list or item lookup
- **THEN** the request is refused and no decision changes state

#### Scenario: Safe model fields
- **WHEN** a decision is returned
- **THEN** it includes available prepared object, destination, scope, class, authority, digest, approval state, outcome, resolver mechanism, surface attribution, activity id and lifecycle times without replayable confirm tokens

### Requirement: Bounded in-memory history
Resolved history SHALL retain at most the most recent 200 entries server-wide younger than seven days, filtered for each identity. It SHALL disclose retention.scope=server and promise only up to200 retained results, not200 per identity. Other identities may cause eviction but SHALL never gain content visibility. History and pending confirmation state SHALL be process-local, cleared on server restart and never written into the Kubo repository. The response SHALL identify the current server epoch so clients can discard stale assumptions.

#### Scenario: History cap and expiry
- **WHEN** the server resolves 201 decisions across identities or a retained entry reaches seven days
- **THEN** only the newest 200 unexpired resolved entries remain server-wide, each visible only to its owner

#### Scenario: Restart
- **WHEN** the agent server restarts
- **THEN** its epoch changes, previous history and pending items disappear, and restored client cards require reconciliation rather than replay

### Requirement: Protected idle subscription
A dedicated authenticated decision subscription SHALL stay available without an active chat run. It SHALL scope events by identity, enforce expiry and disconnect handling, and bound subscribers and buffers. Changes SHALL contain only decision id, state and revision; reconnect SHALL reconcile an authenticated snapshot without promising durable replay.

#### Scenario: Idle inbox receives a decision
- **WHEN** a pending decision changes after the chat run has ended on an interrupt
- **THEN** its owner's idle subscription receives the change and the client reads current details through the protected lookup

#### Scenario: Minimal change event
- **WHEN** the feed emits `ipfs.decision.changed`
- **THEN** the payload contains id, state and revision without tool arguments, targets, credentials or confirmation tokens

#### Scenario: Expired authentication
- **WHEN** the subscription's authentication expires or its identity becomes invalid
- **THEN** the subscription ends and sends no further identity-scoped data

#### Scenario: Slow client and reconnect
- **WHEN** a subscriber overruns its bounded buffer or reconnects after missed events
- **THEN** it reconciles a current snapshot and epoch rather than claiming that every historical event was delivered

### Requirement: Resolution uses existing authority
The inbox SHALL offer continuation only for an owned chat interruption through the existing AG-UI route and its ownership, digest, expiry and resolver checks. Other items SHALL name their authorized answering host or native surface. Read access, item selection and continuation metadata SHALL NOT authorize approval or synthesize native presence.

#### Scenario: Owned chat interruption
- **WHEN** the current identity selects its resumable chat decision
- **THEN** any answer continues through the existing AG-UI confirmation path and preserves once-only execution

#### Scenario: Host-owned or native-required decision
- **WHEN** a decision requires another host or native presence
- **THEN** the inbox directs the operator to that surface and offers no generic web approval bypass

#### Scenario: Stale or foreign continuation
- **WHEN** continuation metadata references a stale digest, another identity or an unauthorized resolver
- **THEN** the server refuses continuation and performs no write

### Requirement: Activity identity across runs
Every agent request SHALL have an activity identifier at its first run. Interruptions and their continuation runs SHALL preserve that identifier. An open approval SHALL be represented as waiting for approval rather than failure; successful, failed and uncertain outcomes SHALL follow the decision model's evidence rules.

#### Scenario: Interrupt then continue
- **WHEN** a run interrupts for approval and later resumes
- **THEN** both runs and the decision refer to the same Activity

#### Scenario: Waiting is not failure
- **WHEN** an Activity has an open decision
- **THEN** it reports waiting for approval without inventing a failed run

### Requirement: Shared inbox and Activity rendering
DecisionInbox SHALL group waiting and recent decisions using the shared eleven presentations. ActivityRow SHALL show Activity state, last run time and linked decisions. Both SHALL be generated catalog components and display bounded-history scope honestly.

#### Scenario: Eleven-state inbox
- **WHEN** all eleven presentations are rendered
- **THEN** pending, waiting_system_prompt, invalidated, uncertain and needs_resubmit appear under Waiting, the others under Recent, and all use shared labels with only authorized controls

#### Scenario: Empty inbox
- **WHEN** the inbox contains no decisions
- **THEN** it says No decisions waiting. and History starts when the node's agent server starts.

### Requirement: Disabled inbox preserves normal use
The inbox and subscription SHALL honor documented feature off switches. Disabling them SHALL not disable normal Kubo functions or bypass confirmation on other enabled transports.

#### Scenario: Inbox disabled
- **WHEN** the operator disables the inbox feature
- **THEN** no inbox subscription remains active and existing authorized confirmation flows remain protected
