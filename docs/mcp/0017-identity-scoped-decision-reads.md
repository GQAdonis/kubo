# ADR-0017: Identity-scoped decision reads and idle subscription

## Status

Accepted for implementation, 2026-10-20, under UI-05/06/07/11/12/13 and I/1.1 of adopted `agentic-ui-implementation` revision `3`, digest `648adf5c5bfe4b3bb17d31517a3b22737182d0f85a20236acaa6a4bb08cf846a`. This supplies contract shapes and bounds; no running endpoint or acceptance receipt. ADR-0003/0010/0013/0015 remain authoritative.

## Date

2026-10-20

## Context

Run-bound AG-UI cannot update an idle inbox. The existing broker has pending confirmations, identity bindings and one-use claims, while settings SSE demonstrates protected bounded subscription. Neither grants a generic browser resolver. Two OIDC subjects can share a configured principal name, so principal-only list/feed filtering would disclose another person's decisions.

The uncomfortable limit is history: the operator chose bounded process-local observation, not a durable audit log. A server-wide 200-record ring cannot guarantee 200 entries per identity, and another identity's activity can cause eviction. Missing records or a disconnected feed cannot prove denial, success or zero effects. The UI must disclose that limitation instead of presenting an incomplete stream as authoritative history.

## Decision

Create a dedicated [decisions route descriptor](../../mcp/contract/decisions/routes.json), strict [read schemas](../../mcp/contract/decisions/decision-read.schema.json) and [normative semantics](../../mcp/contract/decisions/README.md). Do not extend vector-search `routes/index.json` or inherit its tool-group/egress assumptions. Add the decisions directory to the owned contract embedding directive; server/UI integration remains later work.

Expose only GET /decisions, GET /decisions/{id} and authenticated fetch-streaming GET /decisions/events on the existing opt-in MCP HTTP listener. Use shared Host/Origin/fetch-metadata/body/AuthVerifier protections and explicit ui/human-mcp/a2a/uar kinds; refuse admin and anonymous even on loopback. Cookies and URL credentials are never read. JWTs require explicit `auth.oidc.routes: [..., "decisions"]`; default session-only routes stay unchanged. Read authorization never implies AG-UI/native approval authority.

Bind private ownership to the full Principal.BindingKey including subject, across that identity's sessions. Filter before pagination and publication. Reuse ADR-0015's safe DecisionObservation, source_kind distinction and exact owned-interrupt continuation hints. Exclude raw keys/subjects/tokens/grants/sensitive args, other identities' native requests and paired local-app queue contents. Unknown, foreign and evicted item IDs all return404.

Keep a server-wide ring of at most200 resolved observations for seven days, filtered by caller. Disclose retention.scope=server, history_started_at and random restart server_epoch. Pending/approved/running source authorities retain their quotas; bound new UAR live projections to64 globally, one per existing owned open thread, without changing node confirmation authority. Do not persist observation to the repo or use retention eviction to rerun/approve/deny operations.

List pages default50/max100, sorted newest created_at/ID first, capped at1MiB with safely projected items capped256KiB. Bind 60-second HMAC cursors to private opaque owner tag, epoch, own snapshot revision, last key and limit. Existing unsigned generic cursors are insufficient. Never encode raw binding keys in a base64 cursor. Own stale epoch/revision/expiry returns409 cursor_refresh_required; malformed/foreign/tampered handles return generic400. Do not silently skip/duplicate pages after mutations or expose global counts.

The idle stream emits an initial ipfs.decisions.snapshot_required after atomically subscribing before reading own revision. Changed payloads contain only id/state/revision; snapshot markers contain epoch/revision/reason. State is an invalidation hint, never a success receipt or permission. Reconnect and overflow always reconcile authenticated snapshots; no durable replay or SSE id promise. An old Last-Event-ID cannot recover history or approve anything.

Bound streams to4 per identity,16 per configured principal and64 global; queue16 hints, event1024bytes, keepalive25seconds, write deadline5seconds and maximum lifetime15minutes. Coalesce overflow to a latest-own-revision snapshot marker; disconnect a blocked writer and release resources. Close by authentication verdict expiration or known revocation/disable/cancellation; reauthenticate on reconnect. Do not extend streaming through expiry via HTTP clock-skew grace, or block broker mutation on subscriber I/O.

`agent.decisions.enabled` defaults true behind HTTP; false unmounts reads, closes subscribers and releases observer history. HTTP-off/noagent/nomcp also remove the read surface. Existing gates/native prompts/Kubo/MCP operations remain protected. The feature requires no active run or LLM, opens no listener/service of its own and reports nothing out. D/4.2 completes generated config/default/changelog documentation and disabled-state mappings at its assigned boundary.

## Alternatives considered

- Poll or attach only to a run stream: fails the idle-screen requirement or adds recurring work without the chosen subscription semantics.
- Reuse vector-search route descriptor: wrong authorization, grants, egress and response contracts.
- Filter by principal ID: leaks across subjects sharing that configuration.
- Keep200 records per identity without a global cap: expands process memory with identity count and promises capacity not approved by the operator.
- Durable replay/history: explicitly outside UI-06 and unnecessary for read-side reconciliation.
- Add /decisions/approve: widens authority; existing owned interruption/native/host paths already supply authorized resolution.

## Consequences and handoff

### I/1.4 activity correlation amendment

The shared optional ActivityId remains an opaque bounded string (1–128 characters), assigned by the server for each accepted new user activity and retained across its authorized resumes/queued interruptions. Expose it at existing `CUSTOM ipfs.run.facts.value.activity_id`, optional `Interrupt.metadata["ipfs-mcp"].activity_id`, and `DecisionObservation.activity_id`. Standard RUN_STARTED stays unchanged. The interrupt field remains available when run facts are disabled, without emitting synthetic facts or depending on model configuration. This supplies display association, not an authority handle or durable-history promise.

A new accepted message gets a fresh ID; rejected inputs do not create accepted activities. Keep client run/thread IDs, raw backend IDs and identity bindings distinct. Full existing owner/open-interrupt checks still govern continuation, and missing correlation is unreported rather than guessed from targets or tool prose. The minimal idle SSE stays id/state/revision; fetch the authenticated observation for the association. Server-owned UAR activity correlation never promotes backend custom messages to trusted metadata. See the decisions README amendment and authored activity fixtures; server/UI integration remains assigned to their own tasks.

BE implements source lifecycle projection, scheduled expiry, bounded history, auth-aware route registration, signed snapshots/cursors, subscription cleanup and actual outcomes under these shapes. UI subscribes then reconciles authenticated snapshots, handles409/epoch changes, coalesces invalidation and fetches detail before terminal display. Native-required and foreign records gain no resolver rights; UAR synthetic approval stays distinct from this node's broker.

Fixtures author identity/retention/cursor/auth/overflow/reconnect/off-state expectations. No tests, verification builds, generated UI or independent production reviews ran in I/1.1. Final UI-107 and related shared outcome/authority scenarios establish implementation behavior after all production tasks. Source-only fixtures do not certify a passing stream or hardware presence.

## References

- [Canonical inbox OpenSpec change](../../openspec/changes/praxis-decision-inbox/design.md), [operator decisions](../../.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md), [backend research](../../.kbd-orchestrator/phases/agentic-ui-implementation/research-backend.md).
- Source patterns inspected: `mcp/server/settings_events.go`, `http.go`, `auth.go`, `auth_chain.go`, `principal.go`, `broker.go`, `pagination.go`. Reuse their supported boundaries; no library/API constructor assumptions are introduced here.
