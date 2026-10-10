# Proposal: agent-chat-island

Phase `ipfs-mcp-server`, plan revision 2 (new change). N4: `<ipfs-agent-chat>`, a React 19 shadow-root chat island with a right-side sliding shadcn sidebar. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G11 asks for a right-side sliding shadcn/ui sidebar for AI chat in the Web UI and Desktop. The Web UI is React 16 / CRA 5 / Tachyons and cannot import React 19 components, so the chat ships as a self-contained custom element that the fork mounts (operator answer Q6). It talks only to `ipfs mcp`, never directly to UAR.

## What Changes

- `mcp/chat`: a `<ipfs-agent-chat>` custom element with a shadow root hosting a React 19 + shadcn sliding sidebar, built from `mcp/ui`, Promethity tokens from bundled files, loaded by dynamic import, no remote assets.
- An AG-UI client (SSE over `fetch`) to the `ipfs mcp` endpoint the host configures; there is no default endpoint and the island connects to nothing until one is set (`http://127.0.0.1:5050/agui`, the server's default listen address, may be shown only as a suggestion; amended 2026-10-09 to match `design.md` and the AG-UI fixture schema), with the bearer supplied by the host page or the Desktop bridge; A2UI rendering of tool results through the shared catalog; interrupts rendered with the approval card.
- Isolation tests for falsifier A11.1: Web UI pages are unchanged with the island loaded but closed, and the island renders styled under Electron 44.

## Capabilities

### New Capabilities

- `agent-chat-island`: N4: `<ipfs-agent-chat>`, a React 19 shadow-root chat island with a right-side sliding shadcn sidebar

### Modified Capabilities

None.

## Impact

New `mcp/chat/**` (ui-ux-engineer). No Go change. Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.
