# Proposal: mcp-ui-component-library

Phase `ipfs-mcp-server`, change 5 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G4 requires a React 19 component library that is also an A2UI v0.9 catalog, for IPFS-specific agent UI, accessible in light and dark. The MCP App views and possibly other A2UI renderers consume it.

## What Changes

- Verify the A2UI v0.9 catalog format and the MCP Apps extension against primary sources first. Both are open knowledge gaps from the assessment.
- `mcp/ui`: React 19 + TypeScript with light and dark tokens.
- Components bound to contract output schemas: content card, pin list, MFS tree, DAG viewer, IPNS record card, confirmation dialog, node status panel.
- A generated A2UI catalog manifest with validation.
- WCAG 2.2 AA checks.
- No remote fonts, CDNs or analytics.

## Capabilities

### New Capabilities

- `ipfs-ui-components`: React 19 IPFS component library exposed as an A2UI v0.9 catalog, accessible in light and dark themes

### Modified Capabilities

None.

## Impact

New `mcp/ui/**` with its own JS toolchain. JS dependency pins are recorded by the orchestrator in `versions.toml`. No Go changes.

## Product and brand inputs

Binding inputs (added 2026-10-09 after `/impeccable init`): `PRODUCT.md` at the repository root (users, positioning, constraints, voice) and the Promethity identity v1 at `/Users/gqadonis/Projects/promethity/branding` (`design-system/README.md` rules, `tokens.json`/`tokens.css`, bundled woff2 fonts, components, logos). The visual world is established through impeccable new-work into `DESIGN.md` before UI code is written; fonts are bundled, never fetched.

## Plan revision 2

Tasks 1.1-1.5 are registered and unchanged. Group 2 (tasks 2.1-2.3) adds the chat primitives the addendum assigns to this change (message stream, tool-call card, interrupt/approval card, egress badge), shadow-DOM safe, for the chat island (N4). This grows the change from L toward XL; the trade-off is recorded in the plan (T11).
