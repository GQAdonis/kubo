# ADR-0016: Exact A2UI catalog negotiation and shared App mapping

## Status

Accepted for implementation, 2026-10-20, under UI-01–UI-16 and A/1.4 of the adopted `agentic-ui-implementation` contract, revision `3`, digest `648adf5c5bfe4b3bb17d31517a3b22737182d0f85a20236acaa6a4bb08cf846a`. This contract tranche supplies no generated artifacts or passing runtime evidence. ADR-0010/0015 and Kubo Stability/User Agency remain authoritative.

## Date

2026-10-20

## Context

The existing derivations name the A2UI ID and CUSTOM events, but AG-UI does not decode a compatible catalog offer. The five HTML views are separately listed in TypeScript/resources. The catalog already assigns mfs_read to ContentCard within the mfs view; a simplistic one-view/one-component table would break that behavior.

External MCP Apps use the standard pinned ext-apps SDK. A first-party negotiation extension or a private message correlation field cannot become mandatory for an ordinary host. The uncomfortable limit is that declarative metadata does not authenticate postMessage, and current documentation alone does not prove the pinned SDK's exact checks. The actual adapter must be inspected and exercised at its assigned implementation/final boundaries.

## Decision

Use `derivations/a2ui-components.json` as the single catalog identity/version/view mapping authority, retaining `catalog.json` as per-tool component/output authority. Add catalogVersion `0.1.0` and protocolVersion `v0.9` to the existing catalog ID, and allowlisted `viewBindings` for all five existing public resources. Preserve mfs_read's ContentCard selection; the mfs view permits ContentCard and MfsTree. Generated consumers must check exact per-tool mapping rather than overwrite it with the default view component.

At `forwardedProps.ipfs.a2ui.catalogs`, accept at most four bounded exact offers, with a 4096-byte capability namespace. Match ID, catalog version and protocol version exactly. Malformed/missing/oversize/unmatched offers disable A2UI delivery only. With an enabled exact match, the existing one-per-run a2ui.catalog advertisement adds the version tuple and the server emits validated server-built v0.9 surfaces. Existing protected catalog discovery and ordinary tool/text/structured results remain. Never fetch a client-selected catalog URI or trust UAR surface messages.

Keep ordinary MCP App negotiation separate: established ui:// resources, MIME, nested/legacy resource links, single-file assets, SDK lifecycle and standard JSON-RPC IDs remain. Declare the existing iframe exception through informational `ai.ipfs/iframe-exception` resource metadata; it grants no permission and requires no private handshake field. Source/origin/correlation checks must use supported pinned SDK capabilities and allow valid standard opaque-origin host bootstrap without accepting foreign sources.

Before rendering, verify tool/view/component and the actual output schema. Unknown/invalid/ambiguous data yields labelled inert text with Copy only. No HTML/Markdown/autolinks/actions/navigation, partial component controls or remote fetch. Decision presentations reuse ADR-0015's real receipt semantics and existing confirmation authority; display negotiation never approves a write.

The normative [negotiation semantics](../../mcp/contract/agentic/ui-negotiation.md) and [schema](../../mcp/contract/agentic/ui-negotiation.schema.json) specify the bounds, mapping/generation invariants and compatibility behavior. No new service, listener, default endpoint, tool or off switch is introduced; existing agent.a2ui/app-view controls remain independent.

## Alternatives considered

- Select by catalog ID alone: accepts incompatible semantics under the same identifier.
- Require catalog support or a private nonce from every MCP App host: breaks ordinary host interoperability.
- Give every mfs tool MfsTree: contradicts the existing content-read mapping.
- Maintain independent Go/TypeScript maps: permits drift from catalog output schemas and resource IDs.
- Render a partly valid component or Markdown fallback: retains executable/navigation behavior from invalid data.

## Consequences and handoff

A/1.1 derives Go/TypeScript mapping and manifest/resource projections from the same descriptor. A/1.2 decodes the bounded offer and selects delivery. A/2.2 inspects resolved ext-apps 2.0.3 for source/origin/correlation handling without invented APIs. C's later typed forwardedProps extension must preserve independent negotiation validation and generic-property non-forwarding. Tests/fixtures are authored now, evaluated only in final UI-106 after production completes. No build, generation or independent review runs in A/1.4.

## References

- [Canonical OpenSpec change](../../openspec/changes/praxis-mcp-app-a2ui-seam/design.md), [backend/UI research](../../.kbd-orchestrator/phases/agentic-ui-implementation/research-backend.md).
- [MCP Apps specification](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx), fetched through Context7 `/modelcontextprotocol/ext-apps` on 2026-10-20: ui:// resources, MIME, standard _meta.ui configuration and JSON-RPC over postMessage. This is protocol guidance, not proof of pinned implementation behavior.
