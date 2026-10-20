# Design: praxis-decision-inbox

## Context

Show the current authenticated identity its decisions across sessions using bounded memory and a protected idle event subscription; observation confers no approval authority. See proposal.md and the phase assessment/research for observed implementation gaps.

## Goals / Non-Goals

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Decisions

### I/1.1 canonical read contract

[ADR-0017](../../../docs/mcp/0017-identity-scoped-decision-reads.md) and `mcp/contract/decisions/{routes.json,decision-read.schema.json,README.md}` define the dedicated current-identity list/item/idle-stream contract. Full BindingKey filtering precedes pagination/publication; no token/subject/raw binding key or generic approval endpoint is exposed. Reuse D/1.1 DecisionObservation and existing owned-interrupt continuation only. Explicit OIDC route opt-in, bounded authenticated fetch SSE, snapshot/epoch reconciliation and signed cursors are required.

The resolved ring is **server-wide up to200 records/seven days**, filtered by identity, as confirmed under UI-06; there is no per-identity200 guarantee. Source-safe live projections keep their authoritative quotas. `agent.decisions.enabled` defaults true behind HTTP and has a documented false off state; D/4.2 remains the generated config/default/changelog assignment. Fixtures are authored expectations, not passing QA. No server/UI generation or independent review runs here.

Existing IDs: **1.1–1.5, 2.1–2.3, 3.1–3.5**.

- Architect owns **1.1** and prerequisite ADR: dedicated list/item/feed descriptors, schemas, auth/off switches; do not reuse vector-search-specific `routes/index.json` semantics. Server engineer consumes the contract.
- **1.2–1.3**: at most 200 resolved items or seven days (earliest eviction), in memory; query/filter by current identity BindingKey across its sessions. Test same configured principal with different OIDC subjects. No credentials, argument payloads, native requests from other identities or raw binding keys in list/feed.
- **1.4**: server-assigned Activity identity survives interrupt/continuation and backend-specific resume; no durable-history claim. UAR mapping preserves supported facts and labels missing ones.
- **1.5**: event id/state/revision payload only. Add **1.6**, server engineer: protected idle subscription on existing MCP listener with identity binding, expiry/disconnect handling, bounded subscribers/buffers, reconnect snapshot reconciliation and no durable replay.
- Add **1.7**, server engineer: read-only item/outcome lookup and capability-scoped continuation metadata. No new generic approval endpoint. Owned chat interruptions resume through existing `/agui` ownership/digest/resolver checks. Other items name their authorized host/native surface.
- **2.1–2.3** use all eleven shared presentations. **3.1–3.2** use authenticated fetch streaming, snapshot-first reconciliation and frame-coalesced invalidation; no bearer in URL and no invented EventSource header support.
- **3.3** reuses the existing trusted activation/resume route only where allowed. Re-reading or receiving an event cannot approve. Read-only observer, A2A/UAR and native-required cards get no new approval authority.
- **3.4–3.5** truthful Activity states and feature-off absence; no profile/history extras.

## Reuse evidence

### cand-001: React 19.3.1 — adopt

Explicit operator choice and source-level fit; reuse existing behavior and preserve authority boundaries.

- Source: https://react.dev/blog/2024/04/25/react-19-upgrade-guide. Evidence: Removed DOM/context/defaults APIs require source migration; existing shared packages already use React19.

Maintenance: React19.3.1 registry release verified; active upstream. Installed compatibility still untested.

Risk: Installed-runtime compatibility is unverified until final integration; see scoped research notes.

### cand-007: Existing Kubo broker and protected SSE patterns — adapt

Explicit operator choice and source-level fit; reuse existing behavior and preserve authority boundaries.

- Source: mcp/server/broker.go; mcp/server/settings_events.go. Evidence: Reuse one-time claim and authenticated subscriptions; build distinct outcomes, scoped snapshots, bounded history and epoch recovery.

Maintenance: In-repository code maintained by server team;17 existing broker/session baseline tests passed. New outcomes/feed require project-owned lifecycle/security tests.

Risk: Installed-runtime compatibility is unverified until final integration; see scoped research notes.

## Alternatives and trade-offs

- Preserve existing architecture and shared components instead of introducing a new agent runtime, new auth gate or protocol.
- Maintain source-controlled compatibility fixes instead of silent installed-dependency patching.
- Keep history in memory and label unknown facts instead of expanding into durable storage or pretending unavailable state is known.

## Risks / Trade-offs

- [Dependency declarations do not prove runtime compatibility] → inspect resolved sources and preserve all existing operations in final scenarios.
- [Identity or consent confusion] → subject-bound ownership, existing resolver authority and separate actual outcomes.
- [Large shared-tree work] → explicit native writer slices and one generator owner; no stash/reset or gate bypass.

## Migration and verification

### I/1.4 activity wire amendment

Server-assigned ActivityId is a bounded opaque 1–128-character string, fresh per accepted new user message and stable across its authorized resumed runs and queued interruptions. The optional exact fields are existing CUSTOM ipfs.run.facts.value.activity_id, Interrupt.metadata["ipfs-mcp"].activity_id, and DecisionObservation.activity_id, all referencing the confirmation schema's ActivityId. Standard RUN_STARTED fields and ordinary SDK behavior remain unchanged. Interrupt activity metadata is independent of agent.run_facts, so facts-off still supports decision association without a synthetic facts event. It adds no client authority, generic activity endpoint, credential or durable-history guarantee. Do not expose raw ownership keys/backend IDs or trust client/UAR custom correlation. SSE remains id/state/revision only; current identity filtering and retained-history/epoch limits remain in force. See ADR0017 and decisions README for the exact initial/resumed/new-message and missing-field semantics.

Local artifact assembly only; no deployed rollout or CID change. Tests/docs are authored with production, then the final packet evaluates real browser/Electron/backend behavior, gateway/telemetry/provider compatibility, negative controls and off switches. Unknown collaborators/hardware stay unverified. Independent reviewers run after complete implementation. No canonical task is complete merely because this document exists.

## I/1.3 bootstrap wiring ownership decision

The lead adopts the architect role for this narrow coordination decision while all three implementation slots are active. The native server-engineer role already owns `cmd/ipfs/kubo/mcp*.go`; I/1.3 explicitly grants the few initialization lines in `cmd/ipfs/kubo/mcp_agent.go` needed to pass the existing configured `Agent.Decisions` switch and `OriginAllowlist` into `Server.ConfigureDecisionReads` before optional LLM/AG-UI backend setup. Read-only decision observation must not depend on configuring an LLM. This adds no endpoint destination, approval authority, RPC/gateway change or new default outside the approved decision-read contract. Other command paths, help and bootstrap behavior are outside this grant. Author disabled/no-LLM initialization fixtures, run gofmt, and retain the required final make-build/CLI gates.

## I/1.3 cursor restart clarification

The lead records this architect clarification while the implementation slots are active: process-only HMAC keys are discarded on restart. Signature/owner verification precedes stale-epoch reporting; an old-key cursor is generic 400, while an authenticated own stale cursor is 409. Clients discard either rejected pagination cursor and reconcile once from a fresh first page, then surface persistent failure. This preserves the signed-cursor/privacy requirement without persistence or key reuse.
