# Proposal: praxis-decision-inbox

Phase `agent-backend-settings › praxis-agentic-ux`, change 4 of 8, child plan r1, Round 3. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

The assessment marks these KnowMe Praxis ed. 1.2 clauses:

- HOME-07 single decision inbox: **X**. Approvals appear in four places (WebMCP modal, chat `InterruptItem`, Desktop native window, system prompt); there is no inbox and no history.
- HOME-08 Activity is the durable unit; waiting is not failure: **X**. Runs end on interrupt and nothing tracks the work across runs.
- API-06 interrupt ends a run; continuation is a new run on the same Activity; expired decisions are rejected: **P**. Interrupt and resume exist (`agui_resume.go`), but there is no Activity identity spanning the runs and no host-side list of decisions.
- HOME-10 required screen states: **P**. "Approval resolved elsewhere" and "expired" exist only in the model.

Decision D4 (decision-log PX-06): every agent request becomes an Activity with Praxis states; approving starts a new run on the same Activity. The Kubo server already tracks open confirmations (`Server.PendingConfirmations()` in `mcp/server/server.go`, backed by the broker) but exposes no list a UI can read.

## What Changes

- **Kubo `mcp/server`:** a read-only decisions list endpoint on the MCP HTTP listener returning pending confirmations and a bounded history of resolved ones (last 200 or 7 days, whichever is smaller), scoped to the calling principal, each with the decision model v2 fields (state, object, destination, scope, class, authority, effect digest, `resolved_by`, times) and an `activity_id`. A change feed over the existing AG-UI stream so open inboxes update without polling.
- **Kubo `mcp/server`:** an `activity_id` assigned at the first run of an agent request and carried by every continuation run (interrupt → approve → new run). Activity states: running, waiting for approval, waiting for input, paused, completed, failed, cancelled, unknown (reconciling).
- **Kubo `mcp/ui`:** a `DecisionInbox` component (list of decision cards from `praxis-decision-model-v2`, grouped Waiting / Recent) and an `ActivityRow` component, both added to the A2UI catalog.
- **Web UI:** `src/activity/` with the Home inbox slot (count plus the waiting decisions) and the `#/activity` route (Activity list with state, last run, linked decisions). The WebMCP modal, chat `InterruptItem` and inbox all render the same card; answering one moves the others to `resolved_elsewhere`.
- Waiting for approval is shown as waiting, never as failure.

## Capabilities

### New Capabilities

- `ipfs-decision-inbox`: the decisions list endpoint and change feed, Activity identity across runs, and the `DecisionInbox` and `ActivityRow` catalog components.

### Modified Capabilities

None. Web UI behaviour in `src/activity/` is outside this OpenSpec root; its acceptance is in tasks.md (PX-16).

## Impact

- Owners: mcp-server-engineer (endpoint, change feed, `activity_id`); ui-ux-engineer (`DecisionInbox`, `ActivityRow`, Web UI `src/activity/`).
- Scope:
  - Kubo: `mcp/server/server.go`, `mcp/server/broker.go` (resolved-history ring), `mcp/server/http.go` (route registration through the existing `protect` wrapper), `mcp/server/agui_resume.go` and `mcp/server/agui_stream.go` (`activity_id`, change events), `mcp/contract/routes/index.json` and a new route schema, `mcp/contract/schemas/**` (decision list item, additive)
  - Kubo: `mcp/ui/src/components/decision-inbox/**`, `mcp/ui/src/components/activity-row/**`, `mcp/ui/catalog/ipfs-mcp.catalog.json` (regenerated)
  - Web UI: `../ipfs-webui/src/activity/**` (new), `src/shell/**` Home slot from `praxis-shell-modes`, `src/webmcp/**` and `src/agent-chat/**` to render the shared card, `public/locales/en/*.json`
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. The endpoint is new, read-only and lives on the MCP listener, not the Kubo RPC port. It is off when the MCP server is off. History is in memory; nothing new is written to the repo.
- Size: L.

## Dependencies

- Change 1 `praxis-decision-model-v2` (cards and fields).
- Change 3 `praxis-shell-modes` (Home slot and Activity route).
- Change 8 `praxis-desktop-attention` depends on this one.
