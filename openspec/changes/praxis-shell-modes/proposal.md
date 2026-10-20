# Proposal: praxis-shell-modes

Phase `agent-backend-settings › praxis-agentic-ux`, change 3 of 8, child plan r1, Round 2. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

The assessment marks these KnowMe Praxis ed. 1.2 clauses as conflicts or partial:

- WS-01 one workspace (navigator, thread, canvas): **X**. Chat is a shadow-root island launched from a fixed bottom-right button; the IPFS routes are a separate app.
- WS-03 layout as a mode (conversation-forward or application-forward; canvas closed, docked, expanded, full width; keyboard resize, persisted): **X**. One arrangement only; the launcher overlays content and needs `--ac-launcher-clearance` padding.
- WS-07 mobile (drawer navigator, full-screen canvas, back restores thread and draft): **P**. The rail moves to the bottom below 540px; the island is not tuned for phones.
- HOME-01 one composer: **X**. Three inputs: the top-bar CID box, the chat composer and `#/search`.
- HOME-03 suggestions never execute on click: not met by any current input.

Decisions D1, D2 and D3 (decision-log PX-03..PX-05) resolve these while keeping every IPFS app one click away (goal 2).

## What Changes

- **Two layout modes in one shell (D1).** IPFS routes run application-forward: navigator, IPFS app, assistant on the right. Agent-led work runs conversation-forward: navigator, thread, canvas. The assistant or canvas pane has four states: closed, docked (480–560px), expanded (about 760px), full width. `[` and `]` step between states; the state is persisted per device in local storage. The floating launcher and `--ac-launcher-clearance` are removed.
- **Navigator (D2).** Home, then Files, Pins, Explore, Peers, Diagnostics, then Activity, then Settings. Home replaces Status: it holds the single composer, node and agent cards (HOME-02 states: loading, empty, offline, stale, denied, error) and the decision inbox slot that `praxis-decision-inbox` fills. The Status content (node identity, bandwidth, repo, peers count) moves into Home cards; `#/` keeps working and `#/status` redirects to Home. Praxis "Hands" and "Memory" are not applicable to an IPFS node; Praxis "Chat" is the conversation-forward mode reached from Home.
- **One composer (D3).** The top-bar CID box, semantic search and chat input merge into one composer with three intents: "Go to CID or path", "Search my content", "Ask the agent". The intent is inferred (a CID or `/ipfs/`, `/ipns/` path selects Go to) and always shown and changeable before submit. Suggestions are drafts placed in the composer; they never execute on click.
- **Mobile (WS-07).** Below 540px the navigator is a drawer, the canvas is full screen, and Back from the canvas restores the thread scroll position and the unsent draft.
- The chat island stays the renderer for the thread (`<ipfs-agent-chat>`); the shell hosts it in the pane instead of the launcher.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope:
  - `../ipfs-webui/src/App.js`, `src/navigation/**` (NavBar order and labels), new `src/shell/**` (layout modes, pane state, composer, mobile drawer)
  - `src/status/**` (content moves into Home cards), `src/search/**` (entry point moves into the composer; the results page stays), `src/agent-chat/AgentChatLauncher.js`, `src/agent-chat/launcher.css` (removed), `src/agent-chat/island-loader.js` (mount target becomes the pane)
  - `src/bundles/routes*` for `#/status` → Home redirect and the Activity route placeholder
  - `public/locales/en/*.json` for new strings
- Must survive: Status/Overview content (now Home cards), Files, Pins, Explore, Peers, Diagnostics, Settings, the CID go-to behaviour of the old top-bar box, and Kubo RPC and gateway compatibility.
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. The agent pane keeps its existing off switch: with agent features off, the composer offers Go to and Search only and the pane is absent.
- Size: L.

## Dependencies

- Change 2 `praxis-visual-roles` (the shell binds to roles, not raw tokens).
- Changes 4, 5 and 7 depend on this one.
