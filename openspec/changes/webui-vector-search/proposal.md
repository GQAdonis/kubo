# Proposal: webui-vector-search

Phase `agent-backend-settings`, change 33 of 36 in plan revision 4 (proposed, pending OG-16), Round 6. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011 section V5.

## Why

The operator wants to search the node's files from a search box in the Web UI, not only through agents. The Web UI calls the `ipfs mcp` search route with the same endpoint and token source as the agent chat, shows hits with their provenance, and shows indexing status read-only. Hit text is untrusted and is rendered as text.

## What Changes

- 1.1: Route `#/search` and a nav entry, shown only when the index answers as configured.
- 1.2: Search box, kind filters and results (name or path, CID, kind, provenance label, score, snippet), opening a hit in Files or Explore.
- 1.3: Read-only index status panel, including bulk run progress and how to start a run through the agent chat.
- 1.4: Error and egress states.
- 1.5: Promethity styling, accessibility, `en` locale strings.
- 1.6: Off switch in the Web UI and its disabled-state test.
- 1.7: Playwright e2e spec against a stub backend.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer (skills: `prometheus-ui-ux` with `.agents/UI_UX_PROTOCOL.md` if present, `impeccable`, `ui-ux-pro-max`, `vercel-react-best-practices`, `vercel-composition-patterns`)
- Scope: `../ipfs-webui/src/search/**`, `src/bundles/routes.js`, `src/navigation/**` (one entry), `public/locales/en/*.json`, `test/e2e/search.test.js`
- Depends on: vector-search-contract, mcp-whole-node-indexing, webui-shell-redesign, webui-settings-primitives, agent-chat-island-integration
- Size: M; goals: 6, 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change; the page calls only the `ipfs mcp` routes. Every new feature keeps an off switch (AGENTS.md).
