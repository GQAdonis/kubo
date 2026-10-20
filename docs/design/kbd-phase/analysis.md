# Analysis — agent-backend-settings › praxis-agentic-ux

Stage: analyze · Mode: adopt-vs-adapt of an internal standard (no external library search; the "candidates" are Praxis clauses).

## Decision summary

| # | Decision | Adopts | Deviates |
|---|---|---|---|
| D1 | **Two layout modes in one shell.** IPFS routes run *application-forward* (navigator · IPFS app · assistant, assistant right). Agent-led work runs *conversation-forward* (navigator · thread · canvas). The assistant/canvas has four states: closed, docked (480–560px), expanded (~760px), full width. Resize by keyboard (`[` / `]`), persisted per device. | WS-01, WS-03, S41-003 widths | — |
| D2 | **Navigator = Home + IPFS apps + Activity + Settings.** Home replaces Status and is the single composer (HOME-01) plus cards (HOME-02) plus the decision inbox (HOME-07). Files, Pins, Explore, Peers, Diagnostics stay as first-class apps — they are the "application" in application-forward mode. Praxis "Chat" = the Thread mode reached from Home; "Hands" and "Memory" are NA for an IPFS node. | HOME-01/02/07, WS-01 | Five-destination nav (S28-002) is not adopted verbatim: IPFS apps MUST stay one click away. |
| D3 | **One composer.** The top-bar CID box, semantic search and chat input merge into one composer with three intents: Go to CID/path · Search my content · Ask the agent. Suggestions are drafts and never execute on click (HOME-03). | HOME-01, HOME-03 | — |
| D4 | **Activity is the durable unit.** Every agent request becomes an Activity row with Praxis states (running, waiting for approval, waiting for input, paused, completed, failed, cancelled, unknown/reconciling). An interrupt ends the run; approving starts a new run on the same Activity. | HOME-08, API-06 | Host cursor store is specified as a Kubo follow-up, not built here. |
| D5 | **Decision model v2** (see below) extends `mcp/ui` approval states. | WS-05, API-06, HOME-10 #15, CT-05 | Existing words kept where they already pass the brand voice. |
| D6 | **Visual system: Praxis roles, Promethity values.** Token roles from Flat 2.0 (canvas, chrome, surface, raised, hover, muted, text tiers, action, AI/streaming, ok/warn/err) map onto Promethity tokens. AI/streaming role → Promethity `info` (#7FB2D9 dark / #1F5E8C light). Fonts stay Syne / Hanken Grotesk / JetBrains Mono. No borders, no layout shadows (both systems agree). | UI-07 roles, no-border rule | Flat 2.0 hex values and Space Grotesk/Inter/Roboto are not adopted (brand assumption, revisitable). |
| D7 | **Visible context and inspector.** The assistant shows a context strip: container (current page/CID/path), files attached, model placement, memory scope (none), tools offered ("12 of 44 tools offered"), each removable with provenance. A context inspector (Overview, Context, Prompt plan, Events) opens from the strip. | WS-06, CX-01, inspector sections | The UI displays budget decisions only; Kubo's loop owns them (CX-01). |
| D8 | **Lifecycle strip, not spinners.** Each turn shows named phases: Planning → Calling `pin_ls` → Waiting for approval → Running → Done; turn anatomy follows S05 §6.1. | Turn anatomy, L12907 | — |
| D9 | **MCP App views through the A2UI seam.** Inside the webui the five `ui://ipfs-mcp/*` views render as A2UI surfaces from the same catalog. In external MCP hosts they stay sandboxed iframes declared with origin + correlation checks. | FR-12, UI-08, UI-10 | — |
| D10 | **Origin guard.** When the webui detects it is served from a shared path gateway (`/ipfs/<cid>/` on a public gateway host), agent chat, WebMCP writes and approvals are disabled with an explanation and a link to the subdomain/localhost origin. | WEB-06 | — |
| D11 | **CID honesty copy.** Unpin and publish flows say what they do: "Unpinning stops this node from keeping a copy. Other nodes that have it can still serve it." | SEC-06 | — |
| D12 | **Desktop stays Electron.** The tray becomes an attention surface (pending decisions first) and the native confirmation window renders the same decision card v2. | WS-05, HOME-07 | Praxis desktop profile is Tauri (RT); migration is out of scope and recorded as a follow-up. |
| D13 | **No Praxis container API in Kubo yet.** Container = the node (one container per node); threads and Activities are scoped to it. Multi-container is NA until Kubo exposes containers. | CT-01 vocabulary | API §05 routes not implemented. |

## Decision model v2

| State (machine) | Badge word | Controls | Notes |
|---|---|---|---|
| `pending` | Awaiting a person | Deny (focused), Approve after 1s arm | unchanged |
| `waiting_system_prompt` | Waiting for system prompt | Deny | unchanged |
| `approved` | Approved, not yet run | — | **changed copy**: accepted ≠ committed |
| `committed` | Done | View receipt | **new** — effect ran; links to receipt (CID, tool, time) |
| `denied` | Denied. Nothing was changed. | — | unchanged |
| `expired` | Expired. Nothing was changed. | Ask again | unchanged + resubmit |
| `invalidated` | Changed since you saw it | Review again | **new** — effect digest changed (WS-05) |
| `resolved_elsewhere` | Answered on another screen | — | **new** — HOME-10 #15; names the surface |
| `uncertain` | Outcome unknown, checking | Check now | **new** — CT-05 timeout is not failure |
| `needs_resubmit` | Can't resume. Ask again to create a new request. | Ask again | **new** — restored after restart |

Every card shows: the prepared object (tool + human description), destination (node, remote pinning service, IPNS name), scope (CIDs/paths), class in words (Write / Destructive), named authority (principal kind + name), egress badge, and a short effect digest (`sha256:7f3a…c1`).

## Rejected alternatives

- **Full KnowMe Flat 2.0 reskin** — rejected for now: would undo the shipped Promethity delivery (`promethity-delivery`) and the brand requires Promethity on these products. Revisit if the corporate standard mandates values, not roles.
- **Five Praxis destinations verbatim** — rejected: hides Files/Pins/Peers behind Home, failing goal 2.
- **Keep the floating chat island** — rejected: conflicts with WS-03 and HOME-01.
- **Tauri rewrite of Desktop** — rejected for this child: implementation-scale work with no UX gain the Electron shell cannot deliver.

## Open contests

None requiring elicitation; the brand choice is an operator assumption recorded in `decision-log.md` and can be flipped before execution of `praxis-visual-roles`.
