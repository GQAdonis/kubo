# @gqadonis/ipfs-mcp-ui

Pure presentational React 19 components for IPFS tool results and agent chat, plus an A2UI v0.9 catalog of the same components. No host, transport or network imports: components take typed props and callbacks. Design authority: `/DESIGN.md` (Promethity identity v1). Format and API sources: `SOURCES.md`.

## What is here

| Path | What |
|---|---|
| `src/components/*` | Seven contract-bound views: ContentCard, PinList, MfsTree, DagViewer, IpnsRecordCard, ConfirmationDialog, NodeStatusPanel |
| `src/chat/*` | MessageStream, ToolCallCard, ApprovalCard, EgressBadge, and a pure AG-UI event reducer. Shadow-DOM safe |
| `src/contract/generated/*` | Zod 4 schemas generated from `mcp/contract` by `scripts/gen-contract.mjs`. One data schema per component (`<Name>DataSchema`) |
| `src/a2ui/*` | `ipfsCatalog` (`@a2ui/web_core` Catalog), component APIs, `validateCatalogManifest` |
| `catalog/ipfs-mcp.catalog.json` | The published A2UI v0.9 catalog document, generated, `catalogId` = `A2UI_CATALOG_ID` |
| `src/host/*` | Maps MCP Apps host theme and style variables onto the shared tokens (identity-first) |
| `src/fonts/*` | Brand woff2 faces registered from embedded bytes (`FontFace`), no URL |
| `gallery/`, `tests/visual/` | Harness and Playwright specs (visual regression, real-browser axe, keyboard, CSP, shadow root) |

## Use

```tsx
import { PinList, UiScope, registerBundledFonts } from '@gqadonis/ipfs-mcp-ui';
void registerBundledFonts();
<UiScope theme="dark" paint><PinList tool="pin_ls" output={structuredContent} onLoadMore={...} /></UiScope>
```

Validate before you render: `renderToolResult(tool, structuredContent, handlers)` checks the output against the contract schema and reports a mismatch instead of drawing it.

## Commands

`pnpm gen` regenerates contract bindings and the catalog. `pnpm check` runs generation drift check, typecheck, unit/a11y tests, build and the no-remote-assets scan. `pnpm test:visual` (Playwright; builds and serves the gallery under the MCP Apps default CSP) and `pnpm test:visual:update` for baselines.

Fonts are SIL OFL: Syne, Hanken Grotesk, JetBrains Mono (Newsreader is copied to `fonts/` but not bundled). They register on `document.fonts` as `IPFS UI Syne`, `IPFS UI Hanken Grotesk` and `IPFS UI JetBrains Mono`, so a host page that uses the plain family names is not restyled; the `--font-*` tokens name the prefixed families.
