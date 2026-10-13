# Evidence: agent-chat-island

Change `agent-chat-island`, phase `ipfs-mcp-server`, tasks 1.1 to 1.4. Produced 2026-10-09 by the ui-ux-engineer role. Updated the same day for the architect's amendment of the AG-UI contract (`docs/mcp/0005-agent-chat-ui.md`, "Amendment (2026-10-09, chat island contract gaps)"): deny resume, the native-prompt `{wait: true}` observation, the snapshot that introduces the parent message, expiry as denial, no default endpoint, and CORS by `agent.origin_allowlist`. Everything below was measured in this checkout with the pinned stack: react and react-dom 19.3.0, typescript 7.0.2, vite 8.3.4, zod 4.6.5, `@a2ui/*` 0.12.0 (`versions.toml`), tailwindcss and `@tailwindcss/postcss` 4.3.3, `@gqadonis/ipfs-mcp-ui` linked from `mcp/ui`.

## Status

| Task | State | Verify clause |
|---|---|---|
| 1.1 Build `<ipfs-agent-chat>` | Done | `pnpm build` passes. `pnpm test:scan` finds no external URL in `src` or `dist` (0 and 0). |
| 1.2 AG-UI client, A2UI rendering, gesture-gated interrupts, bearer by property | Done | 188 of 188 unit, component and accessibility tests pass, including replay of all nine recorded contract fixtures, a scripted gesture-free resolve that fails, and no token in storage. |
| 1.3 Isolation tests (Chromium, Electron 44) | **Written, not run** | The project tier guard blocks `playwright test` during implementation and I did not bypass it. They run at the phase-boundary gate: `pnpm test:visual`. |
| 1.4 Bundle size and fixture replay | Done | Size and replay results below; raw data in `evidence/bundle-size.json` and `evidence/replay.json`. |

## 1.4 Bundle size

`pnpm measure` (JavaScript and CSS in `dist/`, source maps excluded; kB is 1024 bytes):

| File | Raw | Gzip -9 | Brotli -11 | Loaded |
|---|---|---|---|---|
| `dist/ipfs-agent-chat.js` | 3.8 | 1.6 | 1.4 | with the host page, when the module is imported |
| `dist/chunks/endpoint-*.js` | 1.4 | 0.7 | 0.6 | with the entry (imported statically) |
| `dist/chunks/mount-*.js` | 912.6 | 218.9 | 177.9 | on first open |
| `dist/chunks/fonts-data-*.js` | 193.8 | 145.2 | 144.6 | on first open (the bundled woff2 faces, base64) |
| **Loaded but closed** | **5.3** | **2.3** | **2.0** | |
| **First open (lazy)** | **1106.5** | **364.1** | **322.5** | |
| Total | 1111.7 | 366.4 | 324.5 | |

What this says:

- A host page that imports the module and never opens the sidebar pays 2.3 kB gzip and runs no React.
- The first open pays 364 kB gzip once. `web/performance.md` budgets 300 kB gzip of JavaScript for an app page; the island is over that **only because of the brand fonts** (145 kB gzip). Without them the lazy JavaScript is 219 kB gzip.
- Where the JavaScript goes (source bytes before minification, from the source map): react-dom 620 kB, zod 506 kB (Zod 4 for the library and the island, Zod 3 for `@a2ui/web_core`), `@a2ui/web_core` 503 kB, this package plus `@gqadonis/ipfs-mcp-ui` 183 kB, `zod-to-json-schema` 52 kB, `@a2ui/react` 38 kB, lucide-react 17 kB. `tailwind-merge` (about 9 kB gzip) was removed: shadcn's `cn` is `clsx` alone here.
- Two React copies are loaded when the chat is enabled (the Web UI's React 16 and this one). That is ADR-0005's accepted cost; this is its size.
- Ways to shrink it if the budget binds: drop Zod 3 by moving `@a2ui/web_core` off it (upstream), subset the fonts to the glyphs the product uses (the library ships full Latin faces), or serve woff2 as files instead of base64 (rejected here: it would add a fetch, and the island loads no remote asset).

## 1.4 Fixture replay

`tests/unit/replay.test.ts` replays every recorded contract fixture (nine files in `mcp/contract/fixtures/agui/`, provenance `authored-from-spec`) through the island's client and session against `tests/helpers/fixture-server.ts` on a port the OS picks. The server answers a POST only when the request body **equals** the fixture's `input`, so a pass also proves the wire format and the history the island sends back. No live `ipfs mcp` ran; the live run is `webui-agent-chat` 1.3 after `mcp-agui-endpoint` lands `/agui`.

The three `destructive-native-prompt-wait-*` fixtures share one input, because the outcome belongs to the system prompt and not to the request. The replay therefore serves the approved one from the shared server and gives the denied and expired ones a server of their own.

| Fixture(s) | Requests (matched fixture) | Result |
|---|---|---|
| `read-pin-ls` | 1 (`read-pin-ls`) | Run finished; `pin_ls` done; answer shown. History is `msg-u1`, `msg-a1` (introduced by the MESSAGES_SNAPSHOT), `msg-a2`; no message is made up |
| `egress-refused` | 1 (`egress-refused`) | Run finished; `ipfs_cat` failed with `egress_not_acknowledged`; no content shown |
| `write-interrupt` then `write-resume-approved` | 2 | Run 1 ends interrupted and sends nothing more; one approval sends the exact recorded resume input, history included; `pin_add` done on the original `toolCallId`, not re-emitted |
| `write-interrupt` then `write-resume-denied` | 2 | A deny is `resolved` with `approved: false`; no 422 and no error; the call ends `error` with `confirmation_declined` on `tc-7`; the agent's reply is shown; run finished |
| `destructive-native-prompt` then `-wait-approved` | 2 | After the card shows, the session sends `{wait: true}` by itself (no click); the observation run reports `repo_gc` done on `tc-3`; the card ends Approved from the result; an approve from the chat is refused (`native_prompt`) and nothing is sent |
| `destructive-native-prompt` then `-wait-denied` | 2 | Card ends Denied; `tc-3` is `error` with `confirmation_declined`; nothing left waiting |
| `destructive-native-prompt` then `-wait-expired` | 2 | Card ends Expired (not Denied); `tc-3` is `error` with `confirmation_expired`; nothing left waiting |

`evidence/replay.json` holds the per-run detail and is rewritten by the test. `tests/unit/timeline.test.ts` also rebuilds `write-resume-approved.input` from the state of the interrupted run and checks equality. Behavior the fixtures cannot hold open is covered in `tests/unit/session.test.ts` and `interrupt-ui.test.tsx` with a run that stays open until aborted: a denial while the system prompt is watched aborts the watch and sends `{approved: false}`; a dropped watch offers "Check the system prompt again"; a server that repeats an interrupt cannot make the session loop; a write approval waiting alongside a native-prompt call carries the `{wait: true}` for it in the same resume.

## 1.1 and 1.2 verification

| Check | Result |
|---|---|
| `tsc --noEmit` (strict, `noUncheckedIndexedAccess`) | clean |
| `vitest run` | 15 files, 188 tests, all pass |
| `pnpm build` | passes; production shadow root is closed, harness build's is open |
| `pnpm test:scan` | `src`: 0 findings; `dist`: 0 findings |

Unit tests cover: `sse`, `endpoint`, `client`, `replay` (all nine fixtures), `session` (batches, expiry, the native-prompt watch), `gesture`, `timeline`, `interrupt-ui` (including the watch, deny during the watch, and watch again), `a2ui-render`, `states`, `element`, `css`, `csp-eval`, `scan`, and `a11y` (axe, WCAG 2.2 AA and best-practice rules, five panel states in dark and light, plus through the shadow root).

**G11-4, no remote resource and no `unsafe-eval`.**

- The scan allows only identifiers and message text, each listed with its reason in `scripts/lib/scan.mjs` (XML namespaces, react-dom error-message prefixes, JSON Schema dialect ids, the A2UI catalog and schema ids, library documentation text). Run `node scripts/scan-external-urls.mjs --verbose` to see them.
- `fetch` appears in exactly one module (`src/agui/client.ts`); `scan.test.ts` asserts it.
- The scan flags one eval-shaped string in `dist`: zod 4's `allowsEval` probe, `return Function(""), true`. It is never reached: `src/island/zod-config.ts` sets `jitless` first, and `csp-eval.test.tsx` counts `Function` constructions during full chat turns of three fixtures (zero from the island; jsdom's own selector engine is filtered out and named in the test).
- `@a2ui/react`'s `v0_9` entry appends a `<style>` to `document.head` when it is imported. That would change the host page and, under a strict CSP, raise a violation. `vite.config.ts` strips that block at build time and **fails the build** if a newer `@a2ui/react` changes its shape. 0.12.0 offers no supported way to leave it out (checked: no option, no entry point without it), so the strip stays and an upstream report is drafted below. `element.test.ts` asserts no `<style>` or `<link>` reaches `document.head`.

**G11-3, interrupts need a user gesture.**

- The gate reads `Event.isTrusted` on the click that activates the decision control, and only for the control that matches the answer (Approve for approve, Deny for deny). `element.click()`, `dispatchEvent(new MouseEvent(...))` and a direct call to the handler have no trusted activation behind them and are refused with a visible reason. Tested with the real card in `interrupt-ui.test.tsx` (jsdom marks every synthetic event untrusted, which is what a script's click is) and with real browser input in `tests/e2e/gesture.spec.ts` (written; gate).
- The positive path in jsdom provides a trust probe through React context (`TrustProbeContext`); the custom element never sets it, so page script cannot.
- Native-prompt calls: the card draws no Approve control, `session.answer(..., 'approve')` is refused with `native_prompt` in every state and whoever asks, and the chat can only deny or watch (`{wait: true}`, which answers nothing and needs no gesture). The card ends Approved, Denied or Expired only from the observation run's tool result, never from a click (`docs/mcp/0003`, Amendment item 4; `docs/mcp/0005`, amendment).
- `ApprovalCard` now hands the click to `onApprove` and `onDeny`. The island still gates on its capture-phase record rather than on that argument, because a script that obtained a handler could pass any object as the event.

**Bearer.** `token` is a write-only property over a private field; reads return `undefined`; a `token` attribute is removed and never read; the Authorization header is the only place it travels; `credentials: 'omit'`. `element.test.ts` checks the DOM, shadow root, JSON, URL, `localStorage`, `sessionStorage`, `cookie` and `Storage.prototype.setItem` after a full chat turn.

## 1.3 What the gate runs

`pnpm test:visual` builds `dist/` and `dist-harness/` (same code, shadow root open so Playwright can pierce it), then runs `tests/e2e/*.spec.ts` in two projects, `chromium` and `electron44`. Local servers start in `global-setup.ts` on OS-chosen ports. Electron 44 loads the page from `webui://localhost` with `webSecurity`, `contextIsolation` and the sandbox on, the way Desktop loads the Web UI.

| Spec | Proves |
|---|---|
| `isolation.spec.ts` | A host page (Tachyons plus hostile global rules) is pixel-identical (`maxDiffPixels: 0`) and structurally identical, with the island absent versus loaded and closed, at 320, 768 and 1024 px in light and dark; a closed island loads only its entry; opening and closing changes no host node, style, rule, layout or computed style; hostile rules do not reach the sidebar |
| `render.spec.ts` | Styled rendering at 320, 768, 1024, 1440 px in both themes (tokens, fonts, flush right edge, no sideways scroll); transform slide and reduced motion; a closed panel is hidden, inert and out of the tab order; 20 state screenshots (empty, result, pending approval, native prompt, refused egress) |
| `gesture.spec.ts` | Scripted clicks fail, real clicks and Enter succeed, deny is sent as `approved: false` (and is answered by `write-resume-denied`), native-prompt calls have no Approve |
| `csp.spec.ts` | Works under `default-src 'none'; script-src 'self'; style-src 'self'; connect-src <mcp origin>`; no `securitypolicyviolation`; requests only to the page origin and the configured `ipfs mcp` origin; bearer in no storage, URL, attribute or markup |
| `a11y.spec.ts` | axe with `color-contrast` and `target-size` in a real engine, ten states; measured control sizes |
| `keyboard.spec.ts` | Focus into the panel on open, Escape and Close return focus to the opener, a 2px accent ring on every control, tab order |
| `runtime.spec.ts` | The engine is the one the project names (`electron 44.x` versus Chrome); the page origin in Electron is `webui://localhost` |

The fixture server for these specs now matches resumes by payload and, with `holdWait`, answers a `{wait: true}` resume with `RUN_STARTED` and keep-alives until the client leaves, as a real unanswered system prompt does. The native-prompt states (`render.spec.ts`, `a11y.spec.ts`) therefore show a run in flight ("Waiting for system prompt" in the header and on the card, Stop run in the composer) and skip the idle check. I updated these specs by reading, not by running them. Screenshot baselines do not exist yet. The first gate run creates them (`pnpm test:visual:update`); review them there. I did not render the panel in a browser here, so its visual quality is unreviewed beyond the code and the token contrast tests (`css.test.ts` recomputes 11 pairings in both themes from the shipped library CSS: all at or above 4.5:1 for text and 3.7:1 for the focus ring).

Electron: `electron@44.3.0` (Desktop's pin) is a dev dependency; its binary is not downloaded (install scripts were skipped). The gate needs `ELECTRON_EXEC_PATH` set to an Electron 44 executable, or `node node_modules/electron/install.js` once. `harness.ts` throws a clear error rather than skipping.

## Design decisions made here

- **Tokens and look come from `mcp/ui`.** The island adds no palette. Tailwind's theme maps shadcn's semantic names (`sidebar`, `muted`, `foreground`) onto Promethity tokens (`@theme inline`), so one token set themes the library's components and the island's layout. Spacing steps are named `s1`..`s7` (4, 8, 14, 24, 40, 56, 88) so shadcn's numeric spacing meaning is never confused with the product scale.
- **No border, no shadow.** The panel is a `surface` fill; its leading edge is an 8 px `raised` strip (a fill, not a rule) where a pointer finds it; the footer is a `raised` well holding a `control` input; Send is the one accent fill.
- **Heading levels.** The panel title is `h3`; tool-call and approval titles from the library are `h4`; the empty-state title is `h4`. axe's `heading-order` caught an `h2` to `h4` skip.
- **Non-modal.** `complementary` landmark named "Agent chat", not a dialog; the host page stays usable; Escape closes; closed means `inert`.
- **Voice.** "Chat is not configured. The host page sets the ipfs mcp address. Until it does, this panel sends nothing." States the mechanism and the stakes, present tense, sentence case, no exclamation marks.

## Risks that remain

- **Same-origin script in the admin origin** (the ADR's accepted risk). The production shadow root is closed and the gate lives in the card, so ordinary page script cannot reach the island's DOM, React tree or session. Script that patches `attachShadow` before the island loads could. The session itself has no gate of its own (`session.answer` trusts its caller); only reaching it directly bypasses the gesture check. The server-side confirmation and the native prompt for destructive tools remain the backstop. A bypass of the gate can deny or approve a write, but cannot approve a native-prompt call: the session refuses that regardless of caller.
- **Fonts are registered on `document.fonts`** (the library's `registerBundledFonts`), now under prefixed family names: `IPFS UI Syne`, `IPFS UI Hanken Grotesk` and `IPFS UI JetBrains Mono`. A host page's own `Syne`, `Hanken Grotesk` or `JetBrains Mono` rules no longer match the registered faces, so the page cannot be restyled by the sidebar being opened. The faces stay visible to the page by name (any page script can list `document.fonts`); registration still happens on first open only. `mcp/app` was rebuilt for the rename.
- **Watching the system prompt holds a request open** for as long as the confirmation lives (`confirmation.ttl`). If the stream drops, the confirmation stays pending on the server until it expires; the island shows the error and "Check the system prompt again", and a new message stays blocked until the card ends or expires. The island has no way to cancel the confirmation other than Deny.
- **A native-prompt call that shares a batch with a write approval is watched only when the write is answered.** The resume then carries `{wait: true}` for it. Whether the server accepts a mixed batch is an assumption of this change (the fixtures hold only single-interrupt batches); `session.test.ts` pins the shape the island sends.
- **An approved native-prompt card reads "The call runs once."** The library's approved sentence is present tense and was written for a write approved in the chat. For a call the system prompt approved it is accurate but not specific. I did not change the shared copy.
- Credentialed-looking `fetch` from `webui://` to a loopback origin is unverified in Electron 44 until the gate runs (`runtime.spec.ts`, `csp.spec.ts`). The amendment settles the CORS side: the bearer travels in the `Authorization` header with the default `fetch` credentials mode and the server never sends `Access-Control-Allow-Credentials`; the island already uses `credentials: 'omit'`.

## Gaps found in the first pass, and where they stand

`mcp/ui`:

1. **Handlers got no event.** Closed: `ApprovalCard` and `ApprovalPanel` pass the click (`ApprovalActivation`, a React `MouseEvent`) to `onApprove` and `onDeny`. Handlers that ignore it keep working. The island keeps its capture-phase gate for the reason given above.
2. **No exported message item.** Closed: `ChatMessageItem` is exported; `MessageStream` draws it, and the island's transcript uses it instead of copying the `iu-msg` markup.
3. **Unprefixed font families.** Closed: faces register as `IPFS UI Syne`, `IPFS UI Hanken Grotesk`, `IPFS UI JetBrains Mono`, and the `--font-heading`, `--font-body` and `--font-mono` tokens name the same families, so `mcp/ui`, `mcp/chat` and `mcp/app` share one token set (`mcp/app/dist` rebuilt).
4. **`focusSafeAction` not offered by `ApprovalCard`.** Closed: the prop exists (off by default); the island sets it and no longer moves focus in an effect.
5. **`@a2ui/react` appends a `<style>` to `document.head`.** Open upstream; the build-time strip stays. Draft below.

`mcp/contract` and `mcp-agui-endpoint`, all closed by the amendment, with how the island follows:

1. Deny: `{interruptId, status: "resolved", payload: {approved: false}}`, answered by a run that reports `confirmation_declined` on the original call (`write-resume-denied`). Replayed.
2. Native-prompt return path: `{wait: true}`, sent by the session after the card shows; the run ends with the tool result, `confirmation_declined` or `confirmation_expired`. Replayed for all three outcomes.
3. The assistant message that carries a tool call is introduced by a MESSAGES_SNAPSHOT after `TOOL_CALL_END`. The island's placeholder message is gone; the history sent back is the snapshot's message with its calls (matched byte for byte by the exact-body server).
4. Expiry is denial: no resume for an interrupt that expired on its own, `cancelled` only inside a batch with live ones, and a new message is accepted once every open interrupt expired. Unchanged; tested.
5. No default client endpoint. Unchanged: the island has none and exports `SUGGESTED_AGUI_ENDPOINT` only as a suggestion.
6. CORS by `agent.origin_allowlist`, bearer in the header only. The fixture server echoes a listed origin, allows `POST, OPTIONS` and `authorization, content-type`, sends `Vary: Origin` and never sends `Access-Control-Allow-Credentials`.

Fork owners (`webui-agent-chat`, `desktop-agent-chat`): the element takes `token` or `tokenProvider` as properties only; Desktop's CSP needs `script-src 'self'` for the island's chunks (loaded by relative `import()` from the entry's URL), no inline-style permission, and `connect-src` for the `ipfs mcp` origin.

## Upstream report draft (not filed)

For `a2ui-project/a2ui`, `renderers/react`. Nothing has been sent.

> **`@a2ui/react/v0_9` appends a `<style>` to `document.head` when the module is imported, with no way to opt out**
>
> Version 0.12.0. `v0_9/index.js` runs, at module scope, `if (typeof document !== "undefined") { ... document.head.appendChild(style) }`, adding `<style id="a2ui-date-time-input-webkit-styles">` for the `DateTimeInput` WebKit pseudo-elements. It runs on import whether or not the catalog uses `DateTimeInput`.
>
> Why it matters: (1) a host page with a CSP that has no `style-src 'unsafe-inline'` reports a violation the moment the renderer loads; (2) a renderer mounted in a shadow root (an embedded widget) still changes the host document, which breaks the isolation the embedder chose; (3) an import should not change the DOM.
>
> Suggested fixes, any one of them: apply the rule through a stylesheet the component owns (a `<style>` inside the component's own root, or an adopted stylesheet on its root node); expose the rule as an exported string or a `registerDateTimeInputStyles(root)` function the host calls; or move the block into the `DateTimeInput` entry so only catalogs that use it pay.
>
> Workaround today: strip the block at build time and fail the build when its shape changes (`mcp/chat/vite.config.ts`).
