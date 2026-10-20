# Promethity node and agent: one agentic experience, Praxis-aligned

Design proposal for the Kubo fork, the ipfs-webui fork and the ipfs-desktop fork.
KBD phase: `agent-backend-settings › praxis-agentic-ux` (Kubo). Date: 2026-10-20.
Standard: KnowMe Praxis agentic workspace container specification, edition 1.2 ("Proposed integration").

Prototype: `index.html` in this project (with `app-core.js`, `app.js`, `assets/`). No build step.

---

## 1. What changes, in one paragraph

The shipped Promethity skin stays. The structure changes. The agent stops being a floating island bolted onto an IPFS dashboard and becomes part of one workspace: a navigator on the left, the IPFS app in the middle, and the assistant on the right. The assistant can be closed, docked, expanded or full width. Every request to change the node, from any agent on any screen, becomes one decision card with one state machine. It shows up in one inbox (Activity) and is recorded in one decision record. Home replaces Status: it has one composer, a "Needs your decision" card, and node and assistant health side by side. Desktop's tray becomes an attention surface that lists pending decisions first.

## 2. Praxis adoption at a glance

| Praxis | Adopted as | Where in the prototype |
|---|---|---|
| WS-01 one workspace (navigator · thread · canvas) | Shell: rail · IPFS app · assistant pane | every page |
| WS-03 layout as a mode; canvas closed/docked/expanded/full; keyboard resize, persisted | Pane modes + `[` / `]` keys, saved per device | pane header, Settings › Appearance |
| WS-03 conversation-forward mode | Thread view: thread left, live A2UI surface right | Home › Continue working › any activity |
| WS-05 approval shows object, destination, scope, authority; bound to digest | Decision card fields: Asked by · Where · What · Network · Digest | Activity, pane, Desktop window |
| WS-06 visible context with provenance, removable | Context strip + Context inspector | pane |
| HOME-01 one composer | Home composer with three intents; ⌘K opens the same composer anywhere | Home, top bar |
| HOME-02 cards with distinct states | Home cards; six states drawn | Home, Design board |
| HOME-03 suggestions are drafts | Suggestions fill the composer, never send | Home, ⌘K |
| HOME-07 single decision inbox | Activity › Waiting for you | Activity |
| HOME-08 Activity outlives runs | Activities list with runs count and Praxis states | Activity, Home |
| API-06 interrupt ends a run; answering starts a new run on the same Activity | Notice under the approval in chat; run count goes up | pane |
| CT-05 timeout is uncertain, not failed | `uncertain` state with "Check now" | Activity (IPNS publish) |
| UI-01/02 one catalog; unknown component inert | Surface strip names catalog and revision; PeerMap shown as inert text | Thread canvas |
| UI-08/10 iframes only as declared, origin-checked | MCP App view drawn inside a host strip | Design board |
| SEC-04 retrieved content is data | "Content from the network" frame | Explore, Search |
| SEC-06 CID is not access control or deletion | Unpin confirmation copy | Pins |
| WEB-06 no privileged agent on shared path-gateway origin | Origin guard state | Design board, Settings › Node help |
| G15 accessibility | Keyboard paths, focus rings, words with every colour, reduced motion | everywhere |

## 3. Navigation

Navigator (left rail, collapses to icons below 860px, bottom bar below 540px):

**Home · Files · Pins · Explore · Peers · Activity**. The footer holds **Diagnostics · Settings**.

- Praxis's five destinations (Home, Chat, Hands, Memory, Settings) are **not adopted verbatim**: an IPFS node UI must keep Files, Pins and Peers one click away. Chat is a mode, not a destination. Hands and Memory don't apply.
- Pins gets an entry; it had none.
- Search no longer appears and disappears in the rail. It is an intent of the composer (*Search my content*).
- Activity carries the pending count. The same count goes in the document title and the Desktop tray.

## 4. The assistant pane

- **States:** closed, docked (480px), expanded (760px), full width. The narrow widths follow Praxis S41-003 at the low end; full width hides the IPFS app but keeps the rail.
- **Header:** title, size modes, context inspector, close.
- **Context strip:** what the assistant can see right now. The page, any file you added with *Ask about this*, the model placement (AI role colour), the tool shortlist ("12 of 44 tools offered") and memory scope. Page and file chips can be removed. Hovering a chip shows its provenance.
- **Turns follow Praxis turn anatomy:**
  1. A lifecycle strip of named phases instead of a spinner, for example *Planned › pin_ls › Waiting for your approval*.
  2. Tool cards, collapsed, with results.
  3. The answer.
  4. Citations as CID chips next to the answer.
  5. Inline decision cards.
  6. A run notice.
- **Context inspector:** four tabs.
  - *Overview:* container, thread, activity, backend, limits.
  - *Context:* every item with provenance.
  - *Prompt plan:* tools offered and not offered, tokens used, truncation. It only displays the node loop's choices; the screen does no budget arithmetic of its own (CX-01).
  - *Events:* a replay from stored AG-UI events.

Replaces: the `<ipfs-agent-chat>` floating launcher, with its `--ac-launcher-clearance` padding hack.

## 5. Decision model v2 (shared by every surface)

| State | Badge word | Controls |
|---|---|---|
| `pending` | Awaiting a person | Deny (initial focus) · approve verb, unlocked after 1 s |
| `waiting_system_prompt` | Waiting for system prompt | Deny only |
| `approved` | Approved, not yet run | — |
| `committed` (new) | Done | receipt line |
| `denied` | Denied. Nothing was changed. | — |
| `expired` | Expired. Nothing was changed. | Ask again |
| `invalidated` (new) | Changed since you saw it | Review again |
| `resolved_elsewhere` (new) | Answered on another screen | names the screen |
| `uncertain` (new) | Outcome unknown, checking | Check now |
| `needs_resubmit` (new) | Can’t resume | Ask again |

Card fields: eyebrow with class in words and tool name · title (human description) · *Asked by* (named principal and kind) · *Where* (this node, a remote pinning service, an IPNS name, the public DHT) · *What* (CIDs or paths) · *Network* ("Stays on this node" or "Leaves this node: …") · *Digest* (short effect digest).

UAR approvals carry "UAR approval, not a node confirmation". A2A peers can never approve. The WebMCP card opens as a modal alertdialog with the page inert (ADR-0002 R13), caps the queue at three, and always offers *Deny all and turn off agent writes*.

The same card renders in four places: the assistant pane, Activity, the WebMCP modal and the Desktop native window. All ten states are drawn on the Design board.

## 6. Home

- **H1 by state:** "Your node is running." The sub-line counts the decisions waiting and says that nothing changes until you answer.
- **Composer:** three intents (Ask the assistant, Search my content, Go to CID or path) with suggestion chips that only fill the composer.
- **Cards:**
  - Needs your decision: the top three, plus the single accent button "Review N decisions"
  - Node: peers, pins, repository meter
  - Continue working: unfinished activities
  - Bandwidth
  - Assistant: backend, model, index, Desktop pairing
  - Recently added
- **Card states:** each card has six distinct states (loading, empty, offline, stale, not allowed, error). A failed request never reads as zero.

## 7. Desktop (Electron stays)

- **Tray order:**
  1. Status line: running and peers, version and repository
  2. **Decisions waiting** (count plus the top two)
  3. Open Promethity, Home, Files, Activity
  4. Restart / Stop node
  5. Preferences (app options only)
  6. Advanced (logs, config, repo, GC, custom binary, the legacy pairing key)
  7. About, Quit

  Agent endpoint, confirmations and search move out of Preferences into the Web UI's Settings and Activity.
- **Native confirmation window (520 × 640):**
  - Renders decision card v2.
  - Opens without taking focus. Approve unlocks after 1.5 s in front and asks for Touch ID; Deny is the default.
  - HG-1 is still unproven on hardware, so the Touch ID gate stays exactly as shipped.
- **Pairing:** the Web UI shows a 6-character code in Setup step 5, and Desktop's *Pair this node…* window takes it. No terminal is needed. The 43-character `ipfs mcp pair` key moves to Advanced.
- Praxis's desktop runtime profile is Tauri. A migration is recorded as a deviation and possible follow-up, not part of this design.

## 8. External MCP hosts

The five `ui://ipfs-mcp/*` views keep rendering in Claude, Codex and Kimi as sandboxed iframes with origin and correlation checks. The host draws the strip, and the view drops its own header and follows host tokens. Inside the Web UI the same views render as A2UI surfaces from the one catalog, so there is no iframe in our own shell (the Thread canvas shows a PinList surface). Tools still return text where MCP Apps is not supported.

## 9. Visual system (operator assumption PX-02)

Praxis structure and Flat 2.0 token **roles**, with Promethity **values**:

| Flat 2.0 role | Promethity token (dark / light) |
|---|---|
| canvas | `--ground` #0C0B0A / #F5F2EE |
| chrome, surface | `--surface` #171614 / #FBF9F6 |
| raised | `--raised` #222120 / #ECE7E0 |
| hover, muted | `--control` #2D2C2A / #E1DBD2 |
| text primary / secondary | `--text` / `--text-muted` |
| brand / action (Ember) | `--accent` #E96A12 / #CE3012, once per view |
| reasoning / AI / streaming (Cyan) | `--info` #7FB2D9 / #1F5E8C |
| ok / warn / err | `--ok` / `--warn` / `--err` on their tints |

Fonts stay Syne (headings), Hanken Grotesk (interface) and JetBrains Mono (CIDs, eyebrows). Both systems agree on no borders and no layout shadows. The one exception is D7, the 1px edge on form controls. The light theme's warm Ash ground breaks the general "no warm grounds" default, because the Promethity brand requires it.

If the corporate standard requires Flat 2.0 *values*, only the token file changes (OpenSpec change `praxis-visual-roles`).

## 10. Per-repo implementation map

| OpenSpec change (Kubo `openspec/changes/`) | Repo · files | Round |
|---|---|---|
| `praxis-decision-model-v2` | kubo `mcp/ui/src/approval/model.ts`, `ApprovalCard`, `ConfirmationDialog`, catalog; `mcp/app` OutcomeView | 1 |
| `praxis-visual-roles` | ipfs-webui `src/promethity/tokens.css`; kubo `mcp/ui` tokens | 1 |
| `praxis-shell-modes` | ipfs-webui `src/App.js`, `src/navigation/`, new `src/shell/` (pane host, composer, Home) | 2 |
| `praxis-mcp-app-a2ui-seam` | kubo `mcp/app`, `mcp/ui` renderer; webui mount | 2 |
| `praxis-decision-inbox` | ipfs-webui `src/activity/`; kubo decisions list over `/agui` state | 3 |
| `praxis-context-inspector` | kubo `mcp/chat` island; webui context bridge | 3 |
| `praxis-origin-and-cid-honesty` | ipfs-webui `src/agent-chat/`, `src/files/`, `src/pins/` copy | 3 |
| `praxis-desktop-attention` | ipfs-desktop `src/tray.js`, `src/agent/confirm/ui/` | 4 |

Fork changes use `skip_specs: true`, following the existing `webui-*` and `desktop-*` convention. Kubo-owned changes carry spec deltas.

## 11. Deviations from Praxis (recorded in the phase decision log)

- Five-destination navigation: not adopted (IPFS apps must stay one click away).
- Flat 2.0 colour values and fonts: not adopted (PX-02, revisitable).
- Tauri desktop runtime: not adopted (Electron 44 stays).
- Container API (`/api/v1/containers…`): not implemented; container = the node.
- Dots, calls, Hands, Memory, plugin capability packages: not applicable to an IPFS node app.
- The host-owned AG-UI cursor store is specified as follow-up work, not designed in detail here.

## 12. Assumptions and sample data

- **Real:** CIDs for the Praxis spec directory, the Wikipedia mirror, the xkcd archive and the five published Promethity PDFs.
- **Samples:**
  - peers, latencies, bandwidth, repository size, pin counts and file sizes
  - the backend host `llm.lan:8080` and model `qwen3-32b`
  - the conversation
  
  These are labelled as samples in the UI where they appear.
- The prototype is static HTML. It has not been rendered, opened or tested.

---

## 13. Full views: Files, Pins, Explore, Peers, Activity (prototype revision 2)

Implemented in `app-views.js`; routed from `app.js`.

**Files.** Covers everything today's webui file browser does:
- breadcrumb, Import (files, folder, from an IPFS path, new folder), drag and drop with progress
- list and grid views, sort, multi-select with a bulk bar
- a details drawer

The context menu opens from the row "⋯", from right-click, or with Shift+F10, and arrow keys move through it. It keeps every existing action: share link, copy CID, inspect, set pinning, download, download as CAR, rename (inline), publish to IPNS, check retrieval, remove. "Show the command for each action" replaces CLI tutor mode. A separate **Assistant** group, in the AI role colour, adds:
- Ask about this
- Summarise
- Find similar files
- Who else provides this
- Describe for search (asks first)
- Tidy this folder (asks first)

Read-only actions run in the assistant pane. Anything that writes becomes a decision card. Grid thumbnails are the real Promethity link-preview images, stored locally in `assets/img/` (800 × 420).

**Pins.** Built around one question: is my content kept somewhere else?
- **Summary strip:** pins, storage, remote copies, pins only on this node.
- **Filter chips:** All, At risk, Pinned by the assistant, Remote copies, Recursive, Direct.
- **Each row:** a redundancy meter in words; who pinned it, linked to the decision when an agent did; when it was last verified; the remote-copy state (queued, copying, copied, failed).
- **Actions:** Verify, Add a remote copy, Update to a new CID, Unpin. Unpin uses the honest confirmation copy and warns when no other node has the content.
- **"Protect N at-risk pins"** is the one accent action. It creates a decision to copy those pins to the remote service, with the egress shown.

**Explore.** An agent workspace with three columns:
1. **Agent profiles:** IPLD inspector, Content search, Provider scout, Archivist. Each shows its tools, its model, and whether it can ask to write.
2. **Context and prompt:**
   - Context builder: CIDs, paths, peers and pin sets as removable chips; an "Ask the network" switch; "content from the network is data" always on; "N of 44 tools offered".
   - Prompt box with draft-only suggestions.
   - Results with named run phases and CID citations, framed as data. The IPLD inspector draws a DAG; the Provider scout lists providers with their build; the Archivist produces a decision card.
3. **Plain CID inspector**, kept from today's Explore.

"Manage" opens the profile editor: name, instructions, model, read groups and write groups. Letting a profile ask for changes requires typing its name. Profiles are a proposed node setting; Kubo has none today, so this needs an OpenSpec change.

**Peers.** The existing layout plus a Build column. A peer's libp2p agent version announces its build. Promethity Kubo builds from the GQAdonis fork end in `/GQAdonis`: `version.ImplicitAgentSuffix` derives that suffix from the fork's module path. On that basis each peer is labelled *Promethity Kubo*, *Kubo* or *Other* (Helia, Iroh, go-libp2p apps). The view adds:
- filter chips and a summary
- a latitude/longitude plot that marks builds by shape as well as colour
- a peer drawer: raw agent version, protocols, a note when the build includes agent features, Ping, Copy peer ID

*Mark as trusted* (the `trusted_peers` setting) requires typing the last six characters of the peer ID. The copy always says "reports", because agent versions are self-reported. A dedicated libp2p protocol for stronger detection is proposed, not built.

**Activity.** Three tabs:
- **Waiting for you:** decision cards.
- **Activities:** each with a run timeline drawer showing runs, interrupts, decisions, receipts and timeouts, plus Pause (pause requested, then paused).
- **Decision record:** search, filters by outcome and by who asked, export to JSON or CSV, and a proposed 90-day retention.

"Deny all and turn off agent writes" stays.

**Samples.** Peer IDs, locations, latencies, agent versions, provider counts, sizes, the remote service (Filebase) and the agent answers are sample data. CIDs and images are real.

## 14. Phone form factor (≤ 540px, checked against 375px)

- **Navigation:** a bottom bar with five items: Home, Files, Pins, Activity, More. More opens a sheet with Explore, Peers, Diagnostics, Settings and the Design board. The Activity count stays on its icon. The bar respects the home-indicator safe area.
- **Top bar:** shorter. The composer button keeps its label, and the status badge keeps its dot.
- **Assistant:** opens full screen. The size modes are hidden because they don't apply on a phone. The composer sits above the safe area.
- **Tables** in every view (Files, Pins, Peers, decision record, recently added, Settings) become stacked cards. Each value carries its column name as a label. The row checkbox sits at the left, and a lone row menu "⋯" sits at the top right. The page never scrolls sideways.
- **Bottom sheets:** detail drawers, context menus, the composer palette, the WebMCP approval and the agent-profile editor all slide up from the bottom over a scrim.
- **Layout simplifications:**
  - Home cards stack in one column, and the Files grid shows two tiles per row.
  - Pins stats show two per row, and Explore agent profiles become a horizontal swipe row.
  - The setup flow shows only the current step.
  - Decision cards and key-value lists stack their labels above their values, and approve/deny buttons share the width.
- **Touch targets:** 44px wherever the pointer is coarse (row menus, CID copy, chips, segmented controls, tabs).
