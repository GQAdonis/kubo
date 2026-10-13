# Design: webmcp-tool-package

## Context

See proposal.md and ADR-0002. WebMCP (CG draft, 8 Oct 2026) exposes `document.modelContext.registerTool({name, title, description, inputSchema, execute, annotations})`. Its annotations are `readOnlyHint`, `untrustedContentHint`, `consequentialHint` and `debugging`.

## Goals / Non-Goals

**Goals:**
- Generated definitions.
- Confirmation enforced in the executor layer.

**Non-Goals:**
- UI (that is in `webui-webmcp-integration`).
- Destructive tools over WebMCP. They are excluded this phase per ADR-0002.

## Decisions

- **The generator reads `mcp/contract` and emits a module.** The contract tests fail on drift.
- **Executors refuse write calls unless the injected confirmation hook resolves true.**
- **Only `document.modelContext` is targeted.** `navigator.modelContext` is not part of the current draft.

## Risks / Trade-offs

- [Two RPC client stacks (T6)] → Contract conformance tests run against a real daemon for both.
