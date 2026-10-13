# Design: webui-webmcp-integration

## Context

See proposal.md and ADR-0002. The Web UI fork is React 16 with redux-bundler and `kubo-rpc-client` (`webui:package.json:76,83`). It is served from the RPC origin, so it holds admin RPC access.

## Goals / Non-Goals

**Goals:**
- Off by default.
- Confirmation in the page.

**Non-Goals:**
- Destructive tools.
- Cross-origin exposure.
- Upstream PR (fork-only, answer 1).

## Decisions

- **The toggles live in localStorage per origin, default off.** Tools are registered only after both conditions hold.
- **The confirmation modal names the tool, the CIDs/paths and the effect.** Declining resolves the hook false.

## Risks / Trade-offs

- [Any agent in the host can call the tools (assessment Risk 7)] → Off by default, writes behind a second toggle with per-call confirmation, `untrustedContentHint` on content tools.
