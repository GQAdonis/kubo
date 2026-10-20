# Proposal: praxis-mcp-app-a2ui-seam

Phase `agent-backend-settings › praxis-agentic-ux`, change 6 of 8, child plan r1, Round 2. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

KnowMe Praxis ed. 1.2 prefers A2UI data from one approved catalog (UI-01) and allows iframe embeds only as a declared exception with origin and correlation checks (UI-08, UI-10). The assessment found:

- UI-01 one approved catalog, A2UI 0.9.x pinned: **C** (`mcp/ui/catalog/ipfs-mcp.catalog.json`, `protocolVersion` v0.9).
- UI-02 unknown component renders an inert, labelled fallback: **P**. `ToolCallCard` falls back client-side, but the fallback is not labelled as inert.
- UI-08 / UI-10 iframe only as a declared exception: **P**. The five MCP App views (`ui://ipfs-mcp/content`, `pins`, `mfs`, `dag`, `node-status` in `mcp/app/src/view-specs.ts`) are HTML iframed by external MCP hosts, with no declared origin and correlation contract; inside the Web UI they are not shown at all.
- Assessment conflict 4: MCP App HTML views versus "A2UI data only".

Decision D9 (decision-log PX-11): inside the Web UI and chat island, the five views render as A2UI surfaces from the same catalog. In external MCP hosts they stay sandboxed iframes, declared with origin and correlation checks. This also serves FR-12 (one component set for both adapters).

## What Changes

- **View-to-surface mapping.** Each `ViewId` maps to one catalog component: `content` → `ContentCard`, `pins` → `PinList`, `mfs` → `MfsTree`, `dag` → `DagViewer`, `node-status` → `NodeStatusPanel`. The mapping is derived from the contract `ui.appView` and fails a test when a view has no catalog component.
- **A2UI path for first-party hosts.** When the client is the Web UI or the chat island (it advertises the IPFS A2UI catalog id at `/agui`), a tool result with an `appView` is delivered as an A2UI surface update bound to the tool's structured output, through the existing `mcp-agui-a2ui-registry` path. No HTML is fetched.
- **Iframe path declared for external hosts.** For MCP hosts that render `text/html;profile=mcp-app`, each view's resource metadata declares its exception: expected host origin policy, a per-render correlation id echoed in every `postMessage`, and refusal of messages whose origin or correlation id does not match. The views already run without network access; that stays.
- **Inert labelled fallback (UI-02).** An unknown component, a component not in the pinned catalog, or output that fails the tool's schema renders a fallback card labelled "This result can't be shown here. Nothing in it runs." with the tool name and a text summary; it carries no actions.
- **Web UI mount.** The Web UI registers the `mcp/ui` A2UI renderer in the pane and canvas so these surfaces appear in conversation-forward mode.

## Capabilities

### New Capabilities

- `ipfs-mcp-app-a2ui-seam`: one catalog for MCP App views and A2UI surfaces, the declared iframe exception for external hosts, and the inert labelled fallback.

### Modified Capabilities

None archived. The unarchived capabilities `mcp-app-ui` (`mcp-app-views`) and `mcp-agui-a2ui` (`mcp-agui-a2ui-registry`) keep their requirements; this change adds the seam between them.

## Impact

- Owner: mcp-server-engineer (A2UI delivery for `appView` results, resource metadata); ui-ux-engineer (fallback, Web UI mount, iframe correlation in `mcp/app`).
- Scope:
  - Kubo: `mcp/app/src/view-specs.ts`, `mcp/app/src/views.ts`, `mcp/app/src/host/port.ts`, `mcp/app/src/host/mcp-port.ts` (origin and correlation checks), `mcp/app/src/core/**`
  - Kubo: `mcp/ui/src/a2ui/implementations.tsx`, `mcp/ui/src/a2ui/validate.ts`, `mcp/ui/src/chat/ToolCallCard.tsx` (fallback), `mcp/ui/catalog/ipfs-mcp.catalog.json` (regenerated)
  - Kubo: `mcp/server/agui_a2ui.go`, `mcp/server/agui_catalog.go`, `mcp/contract/catalog.json` `ui.appView` (read, not changed)
  - Web UI: `../ipfs-webui/src/shell/**` and `src/agent-chat/**` renderer registration
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. External MCP hosts receive the same `ui://ipfs-mcp/*` resources and MIME type as today, plus metadata.
- Size: M.

## Dependencies

- Change 1 `praxis-decision-model-v2` (confirmation surfaces inside views use the v2 card).
