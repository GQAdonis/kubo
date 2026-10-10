# Proposal: mcp-app-views

Phase `ipfs-mcp-server`, change 14 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G4 requires an MCP App: interactive views served as `ui://` resources so hosts that render MCP Apps can show IPFS results visually. Tools must stay usable without the UI.

## What Changes

- `mcp/app` views (content, pins, MFS, DAG, node status) built on `mcp/ui`, compiled into a committed bundle.
- `mcp/server` embeds the bundle with `go:embed`, registers `ui://` resources and links tools via `_meta.ui`.
- A CSP that allows no remote origins.
- A rebuild-and-diff check, so the committed bundle matches its source.

## Capabilities

### New Capabilities

- `mcp-app-ui`: MCP App views served as ui:// resources from `ipfs mcp`, linked to tools and usable without network origins

### Modified Capabilities

None.

## Impact

`mcp/app/**` (ui-ux-engineer) and `mcp/server/**` (embed and registration, mcp-server-engineer). Kubo's `make build` stays free of a JS toolchain because the bundle is committed.

## Product and brand inputs

Binding inputs (added 2026-10-09 after `/impeccable init`): `PRODUCT.md` at the repository root (users, positioning, constraints, voice) and the Promethity identity v1 at `/Users/gqadonis/Projects/promethity/branding` (`design-system/README.md` rules, `tokens.json`/`tokens.css`, bundled woff2 fonts, components, logos). The visual world is established through impeccable new-work into `DESIGN.md` before UI code is written; fonts are bundled, never fetched.
