# Spec Delta: ipfs-decision-inbox

## Purpose

A single, principal-scoped list of decisions and an Activity identity across runs, aligned with KnowMe Praxis ed. 1.2 HOME-07, HOME-08 and API-06.

## ADDED Requirements

### Requirement: Decisions list endpoint
The MCP HTTP listener SHALL expose a read-only endpoint that returns the calling principal's pending decisions and a bounded history of resolved decisions. Each item SHALL carry the decision model v2 fields: state, prepared object, destination, scope, class, named authority, effect digest when known, `resolved_by` when resolved, `activity_id`, and created, expiry and resolved times.

#### Scenario: Pending and recent decisions
- **WHEN** a principal with one pending `pin_rm` confirmation and one denied `mfs_rm` confirmation reads the endpoint
- **THEN** the response lists both, the first in `pending` and the second in `denied`, each with its fields and `activity_id`

#### Scenario: Principal isolation
- **WHEN** two principals each have a pending confirmation and each reads the endpoint
- **THEN** each response contains only that principal's confirmation

#### Scenario: Read-only
- **WHEN** a client sends a write method to the endpoint
- **THEN** the server refuses it, and no decision changes state

### Requirement: Bounded resolved history
Resolved decisions SHALL be kept in memory up to 200 entries or 7 days, whichever is smaller, and SHALL NOT be written to the repository.

#### Scenario: History cap
- **WHEN** 201 decisions have been resolved for one principal
- **THEN** the endpoint returns the 200 most recent resolved decisions

#### Scenario: Restart clears history
- **WHEN** the MCP server restarts
- **THEN** the endpoint returns no resolved history and no pending decisions from before the restart

### Requirement: Decision change feed
The server SHALL emit an AG-UI `CUSTOM` event named `ipfs.decision.changed` carrying only the decision id and its new state whenever a decision is created or changes state, to the stream of the principal that owns it.

#### Scenario: Answered in another surface
- **WHEN** a pending decision is approved through the Desktop resolver while the principal's AG-UI stream is open
- **THEN** the stream receives `ipfs.decision.changed` with the id and state, and the event carries no tool arguments or targets

### Requirement: Activity identity across runs
Every agent request SHALL receive an `activity_id` at its first run. A run that ends on an interrupt SHALL record the `activity_id`, and the continuation run started after approval SHALL carry the same `activity_id`.

#### Scenario: Interrupt then continue
- **WHEN** a run ends on a confirmation interrupt and the person approves
- **THEN** the new run reports the same `activity_id` as the interrupted run, and the decision item carries that `activity_id`

#### Scenario: Waiting is not failure
- **WHEN** an Activity has an open decision
- **THEN** its state is "waiting for approval", not failed

### Requirement: Inbox and Activity catalog components
The component library SHALL provide `DecisionInbox` (decision cards grouped Waiting and Recent) and `ActivityRow` (Activity state, last run time, linked decisions), and both SHALL be entries in the generated A2UI catalog.

#### Scenario: Inbox renders states
- **WHEN** `DecisionInbox` renders a fixture with one decision in each of the ten decision states
- **THEN** waiting states (`pending`, `waiting_system_prompt`, `invalidated`, `uncertain`, `needs_resubmit`) appear under Waiting and the rest under Recent, each with its badge word

#### Scenario: Empty inbox
- **WHEN** `DecisionInbox` renders with no decisions
- **THEN** it shows "No decisions waiting." and the note "History starts when the node's agent server starts."
