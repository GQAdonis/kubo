# Proposal: webui-webmcp-integration

**CONDITIONAL:** this change is applied only if `webmcp-host-spike` reports GO for at least one host (all four GO criteria met). If both hosts are NO-GO, the change is deferred and ADR-0002 is recorded as Deferred.

Phase `ipfs-mcp-server`, change 16 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G8: agents in WebMCP-capable hosts (Codex desktop and Kimi desktop first) should drive the node through the Web UI. The Web UI already holds admin RPC access, so the feature must be off by default and confirm every write.

## What Changes

- In the GQAdonis/ipfs-webui fork (`/Users/gqadonis/Projects/references/ipfs-webui`):
- A `src/webmcp/` bundle that registers `mcp/webmcp` tools only when the Settings toggle is on and `document.modelContext` exists.
- A second toggle for write tools, plus an in-page confirmation modal for every write. No destructive tools. An empty `exposedTo`.
- Documentation of `Permissions-Policy: tools=()` as the operator off switch.
- The operator commits in the fork.

## Capabilities

### New Capabilities

- `webui-webmcp`: WebMCP tool registration in the IPFS Web UI fork, off by default with per-call confirmation for writes

### Modified Capabilities

None.

## Impact

External fork repository only. No Kubo code. Delivery to users is decided in `webui-cid-delivery`.

## Product and brand inputs

Binding inputs (added 2026-10-09 after `/impeccable init`): `PRODUCT.md` at the repository root (users, positioning, constraints, voice) and the Promethity identity v1 at `/Users/gqadonis/Projects/promethity/branding` (`design-system/README.md` rules, `tokens.json`/`tokens.css`, bundled woff2 fonts, components, logos). The visual world is established through impeccable new-work into `DESIGN.md` before UI code is written; fonts are bundled, never fetched.
