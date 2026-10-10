# Design: mcp-app-views

## Context

See proposal.md. go-sdk v1.8.0 has no MCP Apps helpers, so the resource MIME type and `_meta.ui` fields are set from the spec verified in `mcp-ui-component-library` task 1.1.

## Goals / Non-Goals

**Goals:**
- Self-contained bundle.
- Reproducible build.
- Graceful absence.

**Non-Goals:**
- Hosting the UI over HTTP outside MCP.

## Decisions

- **The bundle is committed under the server package.** This avoids a Node dependency in `make build` (T4). CI rebuilds the bundle and diffs it.
- **The CSP is `default-src 'none'`.** It is widened only to `ui://`-bundled assets.
- **Off switch `tools.app_views` (architect, 2026-10-09, ADR-0007 section 5).** Boolean, default `true`, flag `--no-app-views`. `false` registers no `ui://` resource and sets no `_meta.ui` on any tool; tools are otherwise unchanged. Tool selection alone is not the off switch, because it removes the tools too.
- **Linked-tool rule confirmed.** With views on, a view is registered only while at least one tool linked to it in the manifest is registered. No per-principal filtering of `resources/list`: the bundle is static and holds no node data.
- **`confirmation_deny` is out of the contract.** Expiry is denial; an app-only visibility is host-enforced, not server-enforced. Revisit only if task 1.4 shows a usability problem.

## Risks / Trade-offs

- [The committed artifact drifts from its source] → A rebuild-and-diff check.
- [Hosts that do not render MCP Apps] → Tools return text and `structuredContent` regardless.
