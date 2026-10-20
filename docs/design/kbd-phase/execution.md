# Execution — agent-backend-settings › praxis-agentic-ux

Stage: execute · Backend: tool (direct authoring) + one general-purpose subagent for OpenSpec changes.

## Dispatch

| Work | Executor | Output |
|---|---|---|
| Design prototype | main agent (opus) | Open Design project `b1a6131b-e247-45b0-937f-4a09ed47a99c`: `index.html`, `app-core.js`, `app.js`, `assets/fonts/*.woff2` (7), `assets/logo/*.svg` (5) |
| Design proposal | main agent (opus) | same project: `DESIGN-PROPOSAL.md` |
| OpenSpec changes | subagent (general-purpose, opus) | `openspec/changes/praxis-*` (8 changes) |

Absolute project path: `/Volumes/T7/Projects/references/open-design/.tmp/tools-pack/runtime/mac/namespaces/knowdesign/data/projects/b1a6131b-e247-45b0-937f-4a09ed47a99c/`

## Prototype coverage against plan acceptance

- Layout modes: application-forward (every IPFS page + pane), conversation-forward (`#/thread`), pane closed/docked/expanded/full, `[`/`]` keys, persisted in localStorage.
- Home: single composer with three intents, draft-only suggestions, six cards; card states on the design board.
- Decision model v2: all ten states on `#/surfaces`; live transitions pending → approved → committed, deny, check (uncertain → committed), resubmit (expired → pending), review (invalidated path).
- Inbox + activities + decision record: `#/activity`; pending count in rail, title, tray mock.
- Context strip + inspector (Overview, Context, Prompt plan, Events).
- IPFS functions kept: Files, Pins, Explore, Peers, Diagnostics, Settings (Node, Assistant, Search and index, Security, Appearance), Setup (5 steps).
- Desktop: tray, native confirmation window, pairing window. External MCP host view. Origin guard. SEC-06 unpin copy.

## Verification performed

`node --check` on `app-core.js` and `app.js` (syntax only). The prototype was not rendered or opened. No Go, webui or desktop source was changed, so Kubo's blocking constraints are not exercised by this child.

## Side effect to note

The OpenSpec refresh (required before phase creation) force-updated generated tool integrations for Claude Code, Codex, Kimi Code, MiniMax Code and OpenCode to OpenSpec 1.14.3 (`authoredPathsChanged: []`). Backups are under `~/.prometheus/openspec/backups/`.
