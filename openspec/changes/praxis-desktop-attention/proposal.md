# Proposal: praxis-desktop-attention

Phase `agent-backend-settings › praxis-agentic-ux`, change 8 of 8, child plan r1, Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

KnowMe Praxis ed. 1.2 HOME-07 asks for a single decision inbox, and WS-05 for an approval that shows object, destination, scope and named authority bound to an effect digest. The assessment marks HOME-07 **X** (approvals appear in four places, one of them the Desktop native confirmation window, with no shared list) and WS-05 **P**. On Desktop the tray is the surface a person sees first, but today it leads with node controls and buries agent state: status line, Restart/Start/Stop, Status/Files/Peers, screenshot, Preferences (mixing node settings, app preferences and experiments), Advanced, About, Quit. A pending decision is visible only if its native window happens to be in front.

Decision D12 (decision-log PX-14): Desktop stays Electron. The tray becomes an attention surface with pending decisions first, and the native confirmation window renders the same decision card v2 as every other surface. The Praxis desktop profile is Tauri (RT); that migration is out of scope and recorded as a follow-up.

HG-1 (a same-user process cannot approve the native prompt) is still unproven on hardware. This change keeps the existing Touch ID gate (`src/agent/confirm/touchid.js`) in front of every Desktop approval and does not claim more than it does.

## What Changes

- **Tray order.** Top to bottom:
  1. Status line (node state, unchanged item ids).
  2. Pending decisions: "N decisions waiting" plus the top 3 by expiry, each "<class in words>: <object>" opening the native confirmation window for that decision. "No decisions waiting." when empty.
  3. Open Promethity (brings the main window forward at its last route).
  4. Home (`#/`).
  5. Files (`#/files`).
  6. Activity (`#/activity`).
  7. Node controls: Restart, Start, Stop, Peers, Take screenshot, node settings, provide strategy, CID profile, the agent endpoint items (status, sidecar toggle, from `src/agent/tray-menu.js`) and the Desktop confirmation submenu (from `src/agent/confirm/tray-menu.js`).
  8. Preferences, app options only (launch at login, app language, experiments, app preferences file). Node settings move out to node controls.
  9. Advanced (logs, config file, repo dir, Kubo config, garbage collection, move repository, custom binary; unchanged).
  10. About.
  11. Quit.
- **Pending decisions source.** The tray reads the decisions list endpoint from `praxis-decision-inbox` through the sidecar, and refreshes on its change feed. The tray item count matches the Web UI inbox count.
- **Native confirmation window on decision card v2.** `src/agent/confirm/ui/confirm.html` and `confirm.js` render the ten states, badge words and card fields from `praxis-decision-model-v2`: object, destination, scope, class in words, named authority, egress badge, short effect digest. Approve stays behind Touch ID; on a computer without Touch ID the window states that this app cannot approve and offers Deny only (the existing rule).
- **Resolved elsewhere.** When a decision shown in the native window is answered in the Web UI or chat, the window shows "Answered on another screen", names that surface, and closes after 5 seconds or on Escape.
- No change to `productName`, `io.ipfs.desktop`, Keychain items or the confirmation pairing.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope:
  - `../ipfs-desktop/src/tray.js` (template order and groups), `src/agent/tray-menu.js`, `src/agent/confirm/tray-menu.js`, new `src/agent/decisions-tray.js`
  - `src/agent/confirm/ui/confirm.html`, `src/agent/confirm/ui/confirm.js`, `src/agent/confirm/ui/ui.css`, `src/agent/confirm/strings.js`, `src/agent/confirm/controller.js` (state mapping only; the Touch ID call path is unchanged)
  - `assets/locales/en.json` for new strings
  - `test/**` tray template and confirm window tests
- Must survive: start, stop, restart; open Status (now Home), Files, Peers; take screenshot; node settings; every Advanced item; About; Quit.
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. With agent features off, the pending decisions group and Activity item are absent and the rest of the order holds.
- Size: M.
- Follow-up recorded, not in scope: Tauri desktop profile (Praxis RT); hardware proof of HG-1.

## Dependencies

- Change 1 `praxis-decision-model-v2` (card states and fields).
- Change 4 `praxis-decision-inbox` (decisions list endpoint, change feed, Activity route).
