# Design: praxis-decision-inbox

## Context

See `proposal.md`. The broker in `mcp/server/broker.go` already holds open confirmations per principal and session, caps out-of-band ones (`ErrTooManyOpen`), and `Server.PendingConfirmations()` lists them. `Server.AwaitConfirmation` lets a transport watch a confirmation answered somewhere else. What is missing is a principal-scoped read path for a UI, a short resolved history, and an identity that ties the runs of one request together.

## Goals / Non-Goals

**Goals:**
- One list a UI can read to show every decision for this person, wherever it was raised.
- Activity identity that survives interrupt and continuation (API-06).
- Live updates without polling.

**Non-Goals:**
- Answering decisions through the list endpoint. Decisions are still answered through the existing resolvers (AG-UI interrupt, elicitation, Desktop, system prompt, CLI); the endpoint is read-only.
- A durable host cursor store or event replay (WS-02, D4 follow-up).
- The Praxis container API (D13).

## Decisions

- **Read-only endpoint on the MCP listener.** Route registered through `h.protect` in `mcp/server/http.go` with the same principal kinds as `/agui`. It returns only the caller's decisions: a principal never sees another principal's decisions. A2A and UAR principals get their own decisions but, as today, no confirm token.
- **Bounded in-memory history.** A ring of resolved decisions (cap 200 entries or 7 days). On restart the history is empty and open decisions are gone; clients holding a persisted card show `needs_resubmit` (from `praxis-decision-model-v2`). This is stated in the inbox empty-history copy: "History starts when the node's agent server starts."
- **Change feed over AG-UI.** A `CUSTOM` event `ipfs.decision.changed` carrying the decision id and new state. Clients re-read the item from the list endpoint; the event carries no arguments or targets, so a stream observer learns no more than the list already allows.
- **`activity_id`.** Assigned at the first run of an agent request (ULID). The interrupt that ends a run records it; the continuation run created on approval carries the same id. Activity state is derived: any open decision → waiting for approval; an input interrupt → waiting for input; run active → running; terminal run outcome → completed, failed or cancelled; a run whose outcome is unknown after reconnect → unknown (reconciling).
- **One card everywhere.** The inbox, the chat `InterruptItem` and the WebMCP modal render the `ApprovalCard` from `mcp/ui`. When the feed reports a decision resolved by another resolver, the others show `resolved_elsewhere` naming that surface.

## Risks / Trade-offs

- [In-memory history disappears on restart] → accepted for this change; the copy says so. Durable history belongs with the host cursor store follow-up.
- [Feed volume on busy sessions] → events carry only id and state; clients coalesce re-reads per animation frame.
- [Principal scoping bugs would leak decisions across users] → a Go test with two principals asserts each sees only its own list; security review in `phase-integration-gate`.
