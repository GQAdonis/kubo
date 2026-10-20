# Design: praxis-decision-model-v2

## Context

See `proposal.md`. One state machine already drives the approval card and the confirmation dialog (`DESIGN.md`, "Confirmation states"). This change grows that machine to cover the Praxis ed. 1.2 decision states (WS-05, API-06, CT-05, HOME-10 #15) without adding a second model. Every surface that shows a decision (MCP App views, A2UI catalog, chat island, Web UI inbox, Desktop native window) renders from this one model.

## Goals / Non-Goals

**Goals:**
- Ten states, each with one badge word, one tone and a fixed set of controls.
- Accepted is never shown as committed.
- A card is bound to the effect digest the person saw; a changed effect cannot be run on an old approval.
- Recovery states for timeouts and restarts that never claim failure or success they cannot prove.

**Non-Goals:**
- The decision inbox and Activity list (`praxis-decision-inbox`).
- The Desktop native window layout (`praxis-desktop-attention`).
- A host cursor store or the Praxis container API (D4, D13: follow-ups).

## Decisions

### State table (verbatim from analysis D5)

| State (machine) | Badge word | Controls | Tone |
|---|---|---|---|
| `pending` | Awaiting a person | Deny (focused), Approve after 1s arm | warn |
| `waiting_system_prompt` | Waiting for system prompt | Deny | warn |
| `approved` | Approved, not yet run | — | ok |
| `committed` | Done | View receipt | ok |
| `denied` | Denied. Nothing was changed. | — | err |
| `expired` | Expired. Nothing was changed. | Ask again | err |
| `invalidated` | Changed since you saw it | Review again | warn |
| `resolved_elsewhere` | Answered on another screen | — | muted |
| `uncertain` | Outcome unknown, checking | Check now | warn |
| `needs_resubmit` | Can't resume. Ask again to create a new request. | Ask again | warn |

- Approve and Deny appear only on `pending`, and Approve only when this surface can answer (the existing truthfulness rule). "Approve after 1s arm" means the Approve control is inert for one second after the card first renders or after its content changes, so a click aimed at something else cannot approve.
- "Ask again" never re-sends the old confirm token. It asks the agent to create a new request with a new confirmation id, digest and expiry.
- "Review again" re-renders the card with the new digest in `pending`; the old approval is discarded.
- "Check now" queries the confirmation's current state; it never re-runs the call.

### Card fields

Every card shows: the prepared object (tool and human description from the catalog `confirmationSummary` template), destination (this node, a named remote pinning service, or an IPNS name), scope (CIDs or paths, from `targets`), class in words ("Write" or "Destructive"), named authority (principal kind and name, for example "Chat session, Travis"), the egress badge, and a short effect digest shown as `sha256:` plus the first four and last two hex characters (`sha256:7f3a…c1`). The full digest is available as the accessible description and on copy.

### Contract mapping

| Contract (`PendingConfirmation.state`) | Model |
|---|---|
| `pending` + `requires_native_prompt` | `waiting_system_prompt` |
| `pending` | `pending` |
| `approved` | `approved` |
| `executed` | `committed` |
| `denied` | `denied` |
| `expired` | `expired` |

The remaining four states are derived by the client from contract facts:

- `invalidated`: the server rejects the confirm with a digest-mismatch error, or a newer payload for the same confirmation id carries a different `effect_digest`.
- `resolved_elsewhere`: a state change arrives with `resolved_by` naming a resolver other than this surface.
- `uncertain`: an `approved` confirmation has not reached `executed`, `denied` or `expired` within the outcome timeout (default 30s, configurable).
- `needs_resubmit`: on load, a persisted `pending` or `approved` record is found whose confirmation id the server no longer knows (restart, token store cleared).

New optional contract fields: `effect_digest` (string, `sha256:` + 64 hex), `destination` (object: `kind` = `node` | `pinning_service` | `ipns_name`, `name`), `resolved_by` (Resolver enum). The digest is computed by the server over the canonical JSON of the tool name and arguments that will run.

### Copy

`approved` sentence changes from "The call runs once." to "Approved, not yet run. The call runs once; this card says Done when it has." Present tense, no exclamation marks, sentence case. Banned-word list from `DESIGN.md` applies.

## Risks / Trade-offs

- [Contract field addition touches Go] → fields are optional and additive; the UI renders without them (digest row reads "Digest not supplied by this server"), so the UI change can ship before the server change.
- [The 30s outcome timeout is a guess] → configurable; `uncertain` is honest at any value because it claims nothing.
- [Ten states increase test surface] → one fixture per state, driven by one table, so adding a state means adding one row.
