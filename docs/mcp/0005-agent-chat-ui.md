# ADR-0005: The agent chat is a React 19 shadow-root island in the Web UI fork that talks only to `ipfs mcp`, and the Desktop fork is hardened

## Status

Accepted, 2026-10-09, per the operator's binding answers in `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`, "Answers to the G9-G12 addendum":

- Chat UI (Q6): "**Web UI island, Promethity.** React 19 + shadcn chat island in the GQAdonis/ipfs-webui fork (reaching Desktop through its bundled Web UI), Promethity identity, talking to `ipfs mcp`, never directly to UAR."
- Desktop (Q7): "**in scope this phase, owned by ui-ux-engineer.** CSP added, `webSecurity` re-enabled, `safeStorage` key bridge; security-reviewer reviews."
- Round 0 answer R4: the Desktop repository is `tribe-health/ipfs-desktop` at `/Users/gqadonis/Projects/references/ipfs-desktop`, branch `main` (HEAD `9bd9904`).

## Date

2026-10-09

## Context

- Goal G11: a right-side sliding shadcn/ui chat sidebar in IPFS Desktop and the IPFS Web UI, rendering A2UI components from the shared catalog in `mcp/ui`, with tool calls subject to the same confirmation gate.
- The Web UI fork (`GQAdonis/ipfs-webui`, read-only at HEAD `24ddaa89`; facts re-read 2026-10-09):
  - React 16.14 (`webui:package.json:83, 91`), Create React App 5 (`webui:package.json:100`) and Tachyons (`webui:package.json:104`);
  - still mounts with `ReactDOM.render` (`webui:src/index.js:35`);
  - at least eleven dependencies cap their React peer at 16, more at 17 and 18 (`assessment-addendum.md`, G11, [sub]); `overrides` also pins `react-identicons` to React 16 (`webui:package.json:312-315`).
- The Web UI is served on Kubo's RPC origin and holds admin RPC (`core/corehttp/webui.go:93-147`). Kubo sets no Content-Security-Policy for it.
- IPFS Desktop (read-only at HEAD `9bd9904`) bundles the Web UI (`desktop:package.json:24-25`) and loads it with `webSecurity: false` and no CSP (`desktop:src/webui/index.js:41-47`). Its preload bridge exposes only analytics consent and language (`desktop:src/webui/preload.js:55-90`).
- UAR's CORS is `allow_origin(Any)` (`uar:src/server.rs:2530`) with no configuration key, so a browser-to-UAR path cannot enforce an origin allowlist on the UAR side.
- `PRODUCT.md`: Promethity identity v1 applied in full; no network fetches from the UI; bundled fonts only; WCAG 2.2 AA in light and dark; "Deny is final."

### Scores

From `assessment-addendum.md`, G11 (weights: goal fit 3, upstream-rebase cost 2, security 3, reach 2, effort 1, brand control 1; maximum 60; options with goal fit at or below 2 excluded):

| Option | Goal fit | Rebase | Security | Reach | Effort | Brand | Weighted |
|---|---|---|---|---|---|---|---|
| U1 upgrade the Web UI fork to React 19 | 5 | 1 | 3 | 5 | 1 | 3 | 40 |
| U2 island, direct to UAR | 5 | 4 | 2 | 5 | 3 | 5 | 47 |
| **U2 island, through `ipfs mcp`** | **5** | **4** | **3** | **5** | **2** | **5** | **49** |
| U3 Desktop-only side view in its own origin | 3 | 5 | 5 | 2 | 3 | 5 | 46 |
| U4 iframe UAR's own UI | 2 | | | | | | excluded |

The margin of U2 through `ipfs mcp` over U3 is 3 points, which the method calls not decisive. With the security weight at 4, U2 scores 52 and U3 51; at 5, U3 wins.

## Decision

- **Package.** `mcp/chat` builds `<ipfs-agent-chat>`, a shadow-root custom element: React 19, Tailwind 4, shadcn, the A2UI renderer and chat primitives from `mcp/ui`, and an AG-UI client. Promethity tokens and the bundled woff2 fonts ship inside it. It loads no remote assets and needs no `unsafe-eval`. It is a right-side sliding sidebar.
- **Mount in the Web UI fork.** The fork's `src/agent-chat/**` launcher mounts the element next to the notification layer, behind an off-by-default Settings toggle, and loads it by dynamic import only when the toggle is on. The fork's visual changes stay limited to the launcher and the toggle.
- **Desktop.** Desktop gets the island through its bundled Web UI (delivery D1, decided with ADR-0002).
- **Backend: `ipfs mcp` only.**
  - The island talks only to `ipfs mcp --http` at `/agui` (ADR-0004). It never talks to UAR, an LLM provider, a gateway or the RPC API for chat.
  - `ipfs mcp` enforces an origin allowlist (config `agent.origin_allowlist`). It is empty by default, so no browser origin can call `/agui` until the operator lists one. To enable the chat the operator adds `webui://localhost`, `http://127.0.0.1:5001` or `http://localhost:5001`, plus any subdomain-gateway origins that serve the Web UI; those are documented values, not defaults (clarified in the Round 7 amendment).
  - The island authenticates as a configured principal of kind `ui` with its own bearer token, read-only unless the operator grants write groups (ADR-0003, ADR-0004).
- **Token handling.**
  - Desktop: the main process holds the token in `safeStorage` and attaches it through a new preload bridge method. It is never in page storage.
  - Browser `/webui`: the user enters the token once, and it is held per origin. This is the admin-origin risk below, accepted by the operator.
- **Confirmation in chat.** A write requested through the chat ends the AG-UI run with an interrupt. The island renders it with the shared confirmation dialog, and only a user gesture resolves it. Destructive tools also raise the native prompt (ADR-0004).
- **Desktop fork changes (tribe-health/ipfs-desktop), this phase.**
  - A CSP for `webui://localhost` whose `connect-src` lists only the RPC origin and the `ipfs mcp` origin.
  - `webSecurity` re-enabled (`desktop:src/webui/index.js:43`).
  - The `safeStorage` token bridge.
  - The paired confirmation window and its local authenticated channel to `ipfs mcp` (the human-gate resolver in ADR-0004).
  - Owner: ui-ux-engineer. Reviewer: security-reviewer.
- **Off switches.** The Web UI toggle is off by default. Desktop chat and pairing are off and unpaired by default. `ipfs mcp` mounts `/agui` only when an LLM backend is configured, and `noagent` compiles it out.

## Consequences

### Positive

- The Web UI fork stays close to upstream `ipfs/ipfs-webui`: no React upgrade, no shell rewrite.
- One component set (`mcp/ui`) serves the MCP App, the A2UI catalog and the chat.
- Promethity applies fully inside the shadow root without restyling Tachyons pages.
- No browser holds a credential to UAR or to an LLM provider. Kubo controls the only browser-facing chat boundary.
- Desktop gains a CSP and `webSecurity`, which it lacks today regardless of chat.

### Negative

- The chat runs in the admin-privileged Web UI origin. A cross-site scripting flaw in the island is admin RPC on the node.
- Two React copies are loaded when the chat is enabled.
- The Desktop fork diverges from upstream (CSP, `webSecurity`, preload bridge, confirmation window) and carries a rebase cost.
- In the browser, the `ui` token lives at the origin's trust level.

## Rejected alternatives

| Alternative | Rejected because (evidence) |
|---|---|
| U1: upgrade the Web UI fork to React 19 | Rewrite-sized: React 16.14 with CRA 5 and Tachyons (`webui:package.json:83, 91, 100, 104`), `ReactDOM.render` at `webui:src/index.js:35`, a class-component shell with the legacy `DropTarget` HOC (`webui:src/App.js:9, 127`), and at least eleven dependencies capped at React 16 (`@tableflip/react-dropdown`, `react-ace` 8, `simplebar-react` 1, `@reach/router` 1.3.4 among them). Scored 40/60, the lowest surviving option, with the worst rebase cost (1). |
| U3: Desktop-only side view in its own non-admin origin | Leaves browser `/webui` users without the chat, which G11 names (goal fit 3, reach 2; 46/60). It is the better option on security alone (5), and it becomes the decision if falsifier G11-1 fails or if the operator weights security above the scored value. |
| U4: iframe UAR's own UI | Not IPFS-specific, not Promethity, no shared A2UI catalog, and it would make the browser a direct UAR client (goal fit 2; excluded). |
| U2 direct to UAR from the browser | UAR's CORS is `allow_origin(Any)` with no configuration key (`uar:src/server.rs:2530`), so the origin allowlist cannot be enforced, and the UAR key would sit in the browser. Scored 47/60 against 49/60 through `ipfs mcp`. The operator chose "never directly to UAR". |

## The admin-origin risk and the U3 flip

The chat shares an origin with admin RPC. The bounds are:

- the island loads no remote resource and needs no `unsafe-eval` (falsifier G11-4);
- Desktop applies a CSP whose `connect-src` lists only the RPC and `ipfs mcp` origins;
- the `ui` principal is read-only unless granted, and every write needs a user gesture in the island or the native prompt;
- the token is never in Desktop page storage.

These bound the exposure; they do not remove it. The decision flips to **U3** (a Desktop-only view in its own origin) for this phase if G11-1 fails, if the security review finds G11-4 violated and it cannot be fixed, or if the operator decides that keeping the chat out of the admin origin outweighs browser reach.

## Where this is weak

- **The margin over U3 is not decisive** (3 points), and it reverses with a higher security weight. The choice rests on reach and on the operator's answer.
- **Shadow-root isolation of Tailwind 4 from Tachyons in Electron 44** is unmeasured, as is the size of two React copies.
- **Credentialed `fetch` from the `webui://` scheme to a loopback `ipfs mcp` origin** under the proposed CSP is unverified in Electron 44.
- **Re-enabling `webSecurity` may break Desktop features** that relied on it being off; G11-2 tests this.
- **How far Promethity applies inside the Web UI fork** beyond the island is still a `PRODUCT.md` open decision.

## Falsifiers

| ID | Assumption | Observable falsifier | Consequence |
|---|---|---|---|
| G11-1 | The shadow root isolates Tailwind 4 from Tachyons in browsers and Electron 44 | Visual regression: any Web UI page differs with the island loaded and closed, or the island renders unstyled in Electron 44 | U3 for this phase |
| G11-2 | Re-enabling `webSecurity` and adding the CSP does not break Desktop | The Desktop smoke test (files, explore, peers, settings, add, pin) fails | Fix the allowlists. Never ship with `webSecurity: false` restored silently; report the regression |
| G11-3 | Chat interrupts can only be resolved by a user gesture | Security review or a test shows page script resolving an interrupt without a user gesture | The native prompt for all chat writes |
| G11-4 | The admin-origin exposure is bounded | The island bundle loads any remote resource, or the `ipfs mcp` token is readable from Desktop page storage | Block |

## Amendment (2026-10-09, chat island contract gaps)

Gaps the `agent-chat-island` build found (`mcp/chat/EVIDENCE.md`, "Gaps found") are closed in the contract (`mcp/contract/fixtures/agui.schema.json` and `fixtures/agui/`); `mcp-agui-endpoint` implements them and replaces the fixtures with recordings.

- **Deny.** The island's shape is confirmed: `resume: [{interruptId, status: "resolved", payload: {approved: false}}]`. The server answers with a run that reports `confirmation_declined` as a `TOOL_CALL_RESULT` on the original `toolCallId` (`write-resume-denied.json`). `cancelled` is only for an expired interrupt answered in a batch with live ones.
- **Native-prompt return path.** A `requires_native_prompt` interrupt accepts `{approved: false}` or `{wait: true}`. `{wait: true}` opens a run that stays open until the system prompt decides or the confirmation expires, then reports the tool result (approved), `confirmation_declined` (denied) or `confirmation_expired` on the original `toolCallId`, and finishes (`destructive-native-prompt-wait-*.json`). `{approved: true}` is refused with HTTP 422. If the observation stream drops, the confirmation stays pending until it expires, and a new `{wait: true}` resume picks it up again. A chat deny sent while the observation is open resolves the confirmation as denied, and the open observation reports it. A call already approved in the system prompt still runs exactly once, whatever the chat sends afterwards.
- **History.** The server keeps no thread history. A `parentMessageId` names an assistant message that the run introduces (here with a `MESSAGES_SNAPSHOT` after `TOOL_CALL_END`) before the turn's first result. The client keeps that message with its `toolCalls` in later history.
- **Expiry is denial.** No resume is sent for an interrupt that expired on its own. A new user message is accepted once every open interrupt has expired. A resume for an expired interrupt is answered with `confirmation_expired` and never executes.
- **No default client endpoint.** Clients connect only to a configured `/agui` URL; `127.0.0.1:5050` is the server's default listen address and may be shown as a suggestion.
- **CORS.** Config `agent.origin_allowlist` (default empty) makes `ipfs mcp` echo a listed origin, allow `POST, OPTIONS` and the headers `authorization, content-type`, and send `Vary: Origin`. It never sends `Access-Control-Allow-Credentials`: the bearer travels in the `Authorization` header with the default `fetch` credentials mode, so the "credentialed fetch" concern above does not apply.

## Amendment (2026-10-09, Round 6: one interrupt per run)

The implemented loop (`mcp-llm-proxy-loop`) ends a run at the first call of a model reply that needs confirmation and keeps later calls of that reply queued. So a run carries exactly one interrupt and a resume holds exactly one entry; mixed resume batches (a write approval plus a native-prompt `{wait: true}`) cannot arise. A reply with a write and a destructive call becomes two interrupts in two consecutive runs (`fixtures/agui/two-calls-write-interrupt.json`, `two-calls-resume-then-native-interrupt.json`). This supersedes the batch clauses of the previous amendment: `cancelled` is a denial of the one open interrupt (reported as `confirmation_declined`, or `confirmation_expired` if it had already expired), and a resume naming any other interrupt gets HTTP 422, resolves nothing and starts no run. `agui.schema.json` now says `maxItems: 1` for `resume` and for `outcome.interrupts`. The island's mixed-batch path (`mcp/chat/EVIDENCE.md`, "A native-prompt call that shares a batch") is dead code against this server.

## Amendment (2026-10-09, Round 7: what the implemented /agui does)

Recorded from the `mcp-agui-endpoint` implementation and its tests; details in that change's design.md ("Recorded behavior").

- **Origin default is empty.** The Decision text above listed the Web UI origins next to the allowlist and read as if they were defaults; they never were (the config schema has `default: []`). The proposal of `mcp-agui-endpoint` said otherwise and is corrected.
- **Client-declared input is ignored.** `tools`, `context`, `state` and `forwardedProps` are accepted and ignored. Real AG-UI clients send `tools: []`, so `agui.schema.json` no longer pins `maxItems: 0`; the model is offered only the principal's contract tools whatever the client sends.
- **Only the `ui` principal on `/agui`.** Any other principal, including one sending a resume that approves, is refused with 403 before the body is read; it resolves nothing.
- **A resume is single-use.** The interrupt ID is the idempotency key. A replayed resume gets 422, starts no run and executes nothing; the write ran exactly once.
- **The chat is a resolver only for the chat's own calls.** A `ui` principal calling a write tool over `/mcp` gets a confirmation without the `agui-interrupt` resolver; the chat cannot approve it.
- **CORS exposes `Retry-After`.** For an allowlisted origin, `/agui` also sends `Access-Control-Expose-Headers: Retry-After`, so the browser chat can read when to retry a 503 `resource_exhausted` (the 64-session cap). Without it the browser hides the header from page script. Nothing else is exposed, and the CORS amendment above is otherwise unchanged (config `agent.origin_allowlist`, `cors.exposeHeaders`; `TestAGUI` origin tests in `mcp/server/agui_test.go`).
- **Failures after the stream begins are `RUN_ERROR`.** A loop limit is `RUN_ERROR` with the reason as `code`; a backend failure is `backend_error` with a message naming at most the HTTP status, never the backend body. Fixtures: `run-error-step-limit.json`, `run-error-backend-failure.json`. Error codes before the stream follow ADR-0004's Round 7 amendment.
- **The two-calls sequence is complete.** `two-calls-native-wait-approved.json` is the third run: `{wait: true}` on the queued `repo_gc` interrupt, approved in the system prompt and reported on its original `toolCallId`. The island's replay previously had no recorded third request for this sequence.

## Amendment (2026-10-11, operator decisions after R11: chat token storage, Desktop `openExternal`)

Operator decisions of 2026-10-11 on two R11 security findings (`.agent-team/kubo-ipfs-agentic/findings/security/r11-findings.md` M6 and L6; `docs/mcp/progress-report.md` section 5.1; `.prometheus/decisions.md`). No code changes.

### 1. The Web UI chat token stays in `localStorage` (M6, accepted)

- **Decision.** The operator re-confirmed the decision of 2026-10-10: in the browser `/webui`, the `ui` principal's bearer token stays in `localStorage` under `ipfs-webui:agent-chat:token`. M6's alternatives (persist only by opt-in, never persist on `/ipfs/` or `/ipns/` paths) are not adopted. M6 is closed as accepted risk.
- **Rationale.** The person enters the token once per browser profile instead of on every visit. The token authenticates only the `ui` principal, which is read-only unless the operator grants write groups, and every write it requests still needs a user gesture in the island or, for destructive calls, the native presence prompt (`requires_native_prompt`). The token cannot approve anything by itself.
- **The accepted risk.** Any script running in the Web UI origin can read the token, including content served from the same origin. M6's scenario: another CID served on the same gateway origin as `/webui` reads it and then calls `/agui` as the `ui` principal. Such a caller can do what the `ui` principal can do, within its grants, and can request writes that still wait for a human gesture or the native prompt. It also learns what the principal's read grants expose.
- **Bounds.** Desktop is unaffected: there the token stays in `safeStorage` and is never in page storage (falsifier G11-4 unchanged). `/agui` accepts only listed origins (`agent.origin_allowlist`, empty by default). Turning the chat off or "Forget token" removes the stored token.
- **How to avoid it.** Serve the Web UI from an origin that serves no other content (the RPC port, `http://127.0.0.1:5001/webui`, rather than a shared gateway origin), keep the `ui` principal read-only, keep gateway origins out of `agent.origin_allowlist`, or rotate the token file and use "Forget token" when done.

### 2. No `openExternal` scheme allowlist in the Desktop fork (L6, won't fix)

- **Decision.** The operator decided not to restrict `openExternal` in the Desktop fork. `src/webui/open-external.js` is upstream IPFS Desktop code, and the fork does not diverge from it for this. L6 is closed as won't-fix by operator decision. No upstream report is planned in this phase.
- **The accepted risk.** A crafted URL with an unusual scheme, opened through the Web UI's external-link path, reaches the operating system's handler for that scheme. This is upstream Desktop behavior and is not introduced or widened by the chat.

## References

- `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`: G9-G12 answers Q6 and Q7, Round 0 R4
- `.kbd-orchestrator/phases/ipfs-mcp-server/assessment-addendum.md`: Summary 6, G11 options and scores, DECISION (G11), ADR DRAFTS (ADR-0005)
- `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`: D5, section 4 (G11 falsifiers), section 4a
- `PRODUCT.md`
- ADR-0003 (confirmation and authentication), ADR-0004 (agentic endpoints, principals, human gate); ADR-0002 (WebMCP and Web UI delivery) is pending the WebMCP host spike
