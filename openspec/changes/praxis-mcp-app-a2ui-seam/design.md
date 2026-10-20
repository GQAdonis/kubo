# Design: praxis-mcp-app-a2ui-seam

## Context

See `proposal.md`. Two adapters already share one component set: the MCP App views (`mcp/app`, HTML resources at `ui://ipfs-mcp/*`, MIME `text/html;profile=mcp-app`) and the A2UI catalog (`mcp/ui/catalog/ipfs-mcp.catalog.json`, served through `mcp-agui-a2ui-registry`). `DESIGN.md` "Theming across hosts" already requires one token set for both. What is missing is the rule for which adapter a given host gets, and a declared contract for the iframe case.

## Goals / Non-Goals

**Goals:**
- First-party hosts (Web UI, chat island) never load view HTML; they get A2UI data.
- External hosts keep working, with the iframe exception declared and checked.
- Anything the renderer cannot draw is inert and says so.

**Non-Goals:**
- New views or components.
- Changing which tools have an `appView` (contract-owned).

## Decisions

- **Adapter selection by advertised catalog.** A client that advertises the IPFS catalog id (`catalogId` in the catalog file) in its AG-UI run input gets A2UI surface updates for `appView` results. A client that does not, and that supports MCP Apps, gets the `ui://` resource link as today. Selection happens on the server; the client never chooses HTML by sending a flag.
- **Mapping derived, not listed twice.** `VIEW_COMPONENT: Record<ViewId, CatalogComponent>` lives next to `VIEW_IDS`; a test asserts every `appView` in the contract has a view and every view has a catalog component.
- **Declared iframe exception.** Each view resource carries metadata: `exception: "iframe"`, `reason`, and the correlation rule. At render, the view generates a random correlation id, sends it in its first message to the host, and drops any inbound message whose `origin` differs from the first host message's origin or whose correlation id differs. This follows the existing `mcp/app/src/host/port.ts` message port, adding the checks there.
- **Fallback copy.** "This result can't be shown here. Nothing in it runs." plus tool name and the text content of the result. The card has no buttons and no links except copy-text. The same fallback is used for: unknown component name, component outside the pinned catalog version, and output that fails the tool's output schema.

## Risks / Trade-offs

- [External hosts that ignore metadata] → the checks run inside the view, so they hold regardless of host support.
- [A2UI catalog version drift] → pinned at v0.9; a surface for any other version renders the fallback.
- [Two delivery paths to test] → one fixture set per view drives both the A2UI and iframe tests.
