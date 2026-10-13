# Design: mcp-ui-component-library

## Context

See proposal.md. The A2UI v0.9 catalog format has not been verified yet (assessment knowledge gap). Task 1.1 settles it before any component work.

## Goals / Non-Goals

**Goals:**
- One component set, usable both from React and as an A2UI catalog.
- Bindings to contract output schemas.

**Non-Goals:**
- The MCP App shell (that is `mcp-app-views`).
- Web UI integration (the Web UI is React 16 and is not a consumer).

## Decisions

- **The catalog manifest is generated from component metadata.** It is not hand-written, which keeps one source.
- **Self-hosted fonts or system fonts only.** No network fetches, to satisfy the no-tracking and configurable-endpoints gates.

## Risks / Trade-offs

- [A2UI v0.9 differs from expectations] → Task 1.1 runs first. If the format forces a structural change, the change is resized before 1.3.
