# @gqadonis/ipfs-agent-chat

`<ipfs-agent-chat>`: a React 19 chat island with a right-side sliding sidebar, in a shadow root. The IPFS Web UI fork (React 16, Tachyons) mounts it by dynamic import; IPFS Desktop gets it through its bundled Web UI. It talks to `ipfs mcp` `/agui` and to nothing else. Decision record: `docs/mcp/0005-agent-chat-ui.md`. Visual authority: `/DESIGN.md` (Promethity identity v1). Evidence: `EVIDENCE.md`.

## Use

```js
// Only when the person turns the chat on. A page that never does pays nothing.
await import('@gqadonis/ipfs-agent-chat');

const chat = document.createElement('ipfs-agent-chat');
chat.setAttribute('endpoint', 'http://127.0.0.1:5050/agui'); // the ipfs mcp /agui URL
chat.token = bearerForTheUiPrincipal;                       // PROPERTY ONLY, write-only
document.body.append(chat);
chat.open = true;                                           // or chat.toggle() from a launcher button
```

| Member | Kind | Notes |
|---|---|---|
| `endpoint` | attribute and property | The `ipfs mcp` `/agui` URL. No default: unset, the element sends nothing. https, or http on a loopback host; path ends in `/agui`; no credentials, query or fragment. |
| `open` | attribute and property | Shows or hides the sidebar. |
| `theme` | attribute and property | `dark`, `light`, or `auto` (default; dark unless the system asks for light). |
| `token` | property, write-only | The bearer for the `ui` principal. Reads return `undefined`. Never an attribute (a `token` attribute is removed, never read), never in a URL, never written to any storage. |
| `tokenProvider` | property, write-only | `() => string \| Promise<string>`, called per request. For a host that holds the token elsewhere (the Desktop `safeStorage` bridge). |
| `tokenSet` | read-only property | Whether a token or provider is set. |
| `openPanel()`, `close()`, `toggle()` | methods | There is deliberately no method to send a message or answer an approval. |
| `ipfs-agent-chat-openchange` | event | `detail.open`; bubbles, composed. Focus returns to the opener on close. |
| `--ipfs-agent-chat-z-index` | CSS custom property | Stacking of the island; default 1000. |

`SUGGESTED_AGUI_ENDPOINT` (`http://127.0.0.1:5050/agui`) is exported for hosts that want the documented address. The element never assumes it.

## What it is

- `src/element.ts`, `src/index.ts`: the entry a host loads (about 2 kB gzip with its shared chunk). It defines the custom element and owns the shadow root and the host-facing properties. The shadow root is **closed** in the production build; page script has no reference to the island's DOM or React tree. A `harness` build opens it so Playwright can pierce it.
- `src/island/*`: loaded by `import()` on first open: React 19, the sidebar, the session, the transcript, the composer, the A2UI surfaces. `mount.tsx` adopts the stylesheets into the shadow root (constructable stylesheets, so a page CSP without inline-style permission works), registers the bundled brand faces from embedded bytes, and sets `z.config({ jitless: true })` first so zod never probes for `new Function`.
- `src/agui/*`: the AG-UI client. `client.ts` is the only module that calls `fetch`. A request goes to the configured origin only (an origin guard throws before any other is sent), with `credentials: 'omit'`, `redirect: 'error'`, no referrer, no cache. `wire.ts` builds `RunAgentInput` exactly as `mcp/contract/fixtures/agui.schema.json` describes it. `gesture.ts` is the user-gesture gate.
- `src/ui/sidebar.tsx`: a right-side offcanvas sidebar in the shape of shadcn/ui's (`SidebarProvider`, `Sidebar`, `SidebarHeader`, `SidebarContent`, `SidebarFooter`), adapted for a shadow root: no cookie, no Radix portal, controlled by the host element, a non-modal `complementary` landmark.
- `src/styles/island.css`: Tailwind 4, compiled by `scripts/build-css.mjs` into a stylesheet for a shadow root: no preflight, no `@property` (replaced by initial declarations), `:root` replaced by `:host`, no `html`/`body` selector, no `url()`, no `@import`, and only the utilities a `className` uses.
- A node mounted with `/agui` but no model backend answers `503 backend_not_configured`. The client reads that code (only the code; the body is never shown) and the panel becomes a "Pick a model to start" state with a link to `#/settings/agent`, the Web UI route that holds the backend settings. The refused message is taken back out of the transcript and kept, so "Send message again" works once a backend exists. A 503 with any other code or no readable body stays the session limit and honours `Retry-After`. Tests: `tests/unit/run-errors.test.tsx`, `tests/unit/a11y.test.tsx`.
- Tool results are drawn by the shared A2UI catalog: each tool call is an A2UI v0.9 surface (`createSurface` with the catalog id, one `ToolCallCard`, `updateDataModel` as the call changes) rendered by `@a2ui/react` with `ipfsCatalog` from `@gqadonis/ipfs-mcp-ui/a2ui`. A result the contract does not describe is shown as data.

## Security properties, and what tests them

| Property | Test |
|---|---|
| Talks only to the configured `ipfs mcp` origin; loads no remote asset | `tests/unit/endpoint.test.ts`, `client.test.ts`, `scan.test.ts`; `tests/e2e/csp.spec.ts` |
| No `unsafe-eval` | `tests/unit/csp-eval.test.tsx`, `scan.test.ts`; `csp.spec.ts` (no `securitypolicyviolation`) |
| Bearer by property only; not in URL, storage, attributes or markup | `tests/unit/element.test.ts`; `csp.spec.ts` |
| An interrupt is answered only by a person's gesture (`Event.isTrusted`, on the matching control) | `tests/unit/gesture.test.ts`, `interrupt-ui.test.tsx`; `tests/e2e/gesture.spec.ts` |
| A call with `requires_native_prompt` has no Approve control, and the session refuses an approve for it | `interrupt-ui.test.tsx`, `session.test.ts`, `replay.test.ts` |
| The chat watches a native-prompt call with `{wait: true}` (no answer, no gesture), ends the card from the result (Approved, Denied or Expired), and a denial in the chat stops the watch first | `interrupt-ui.test.tsx`, `session.test.ts`, `replay.test.ts` |
| Expiry is denial | `session.test.ts`, `interrupt-ui.test.tsx` |
| Loaded but closed, the island changes nothing on the host page | `element.test.ts`; `tests/e2e/isolation.spec.ts` |

Residual risks are in `EVIDENCE.md`.

## Commands

| Command | What |
|---|---|
| `pnpm typecheck` | Builds the stylesheet, then `tsc --noEmit` |
| `pnpm test` | Unit, component and accessibility tests (Vitest, jsdom and node) |
| `pnpm build` | Stylesheet, `dist/` (entry plus lazy chunks) and declarations |
| `pnpm test:scan` | No external URL, no network API outside `client.ts`, no code generation, in `src` and `dist` |
| `pnpm measure` | Writes `evidence/bundle-size.json` |
| `pnpm test:visual` | Builds both variants, then the Playwright specs in Chromium and Electron 44. Phase-boundary gate only. `ELECTRON_EXEC_PATH` points at an Electron 44 executable, or run `node node_modules/electron/install.js` once |

Servers in tests listen on ports the operating system picks; nothing here hardcodes one.
