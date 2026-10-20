# Proposal: praxis-context-inspector

Phase `agent-backend-settings › praxis-agentic-ux`, change 5 of 8, child plan r1, Round 3. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

The assessment marks these KnowMe Praxis ed. 1.2 clauses:

- WS-06 visible context with provenance, removable: **X**. The chat sends no visible context list, and page context is not passed.
- CX-01 the person can see what the agent was given: not met. The chat shows no tools-offered count, model placement or memory scope. CX-02..05 budget arithmetic stays NA for this UI (owned by the agent backend).
- Turn anatomy (S05 §6.1): turns show a spinner, not named phases, so a person cannot tell planning from calling a tool from waiting for approval.

Decisions D7 and D8 (decision-log PX-09, PX-10): a context strip with removable items and provenance, a context inspector with four sections, and a lifecycle strip with named phases.

## What Changes

- **Context strip (Kubo `mcp/chat` island).** Above the composer: container (current page, CID or MFS path, from the Web UI), files attached, model placement (local or the acknowledged remote host by name), memory scope ("None"), tools offered ("12 of 44 tools offered"). Each item shows where it came from (page, person, settings, server) and can be removed before send, except tools offered and model placement, which link to Settings instead.
- **Context inspector.** Opens from the strip. Four sections: Overview (container, model placement, memory scope, egress host), Context (the items sent with the last turn, with provenance), Prompt plan (the tool list offered and the system prompt digest the server reports), Events (the AG-UI events of the current run, read-only). The inspector displays decisions the server made; it computes no budget.
- **Lifecycle strip.** Each turn shows named phases from AG-UI events: Planning → Calling `<tool>` → Waiting for approval → Running → Done (or Failed, Cancelled). No spinner without a word.
- **Web UI context bridge.** The Web UI passes the current route's container (route name and CID or path) to the island through the existing bridge, only when the person has not removed it.
- **Kubo server.** The run-start response reports the offered tool names and the system prompt digest, so the strip and the inspector show server facts, not client guesses.

## Capabilities

### New Capabilities

- `ipfs-agent-context`: the context strip, context inspector and lifecycle strip in the chat island, and the server facts they display.

### Modified Capabilities

None. The Web UI bridge change is outside this OpenSpec root; its acceptance is in tasks.md (PX-16).

## Impact

- Owners: ui-ux-engineer (island, Web UI bridge); mcp-server-engineer (run-start facts).
- Scope:
  - Kubo: `mcp/chat/src/island/Composer.tsx`, `mcp/chat/src/island/Conversation.tsx`, `mcp/chat/src/island/Transcript.tsx`, `mcp/chat/src/island/timeline.ts`, `mcp/chat/src/island/session.ts`, new `mcp/chat/src/island/ContextStrip.tsx`, `ContextInspector.tsx`, `LifecycleStrip.tsx`, `mcp/chat/src/agui/**`
  - Kubo: `mcp/server/agui_stream.go` and `mcp/server/agent_frame.go` (run-start facts: offered tool names, system prompt digest), `mcp/contract/schemas/**` (AG-UI run-start payload, additive), regenerated `mcp/ui/src/contract/generated/agui.ts`
  - Web UI: `../ipfs-webui/src/agent-chat/bridge.js`, `src/agent-chat/island-loader.js`, `src/shell/**` (current container), `public/locales/en/*.json`
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. The run-start payload change is additive. The container item is sent only when present in the strip; removing it removes it from the request.
- Size: M-L.

## Dependencies

- Change 3 `praxis-shell-modes` (pane and current-container source).
