# Assessment (UI): agent-backend-settings, goal 1 (settings UI) and goal 5 (Promethity rebrand)

Stage ASSESS. Role: ui-ux-engineer. Read-only inspection; no repo was modified. Date 2026-10-13.

Skills applied: `prometheus-ui-ux` (project `.agents/UI_UX_PROTOCOL.md` read; v1), `impeccable` (launcher printed the repo PRODUCT.md; it is the redesign branch: the old look is evidence and anti-reference, product truth and function stay), `ui-ux-pro-max` (one focused `ux` query on settings forms: labels, inline errors tied by `aria-describedby`, visible submit feedback), `vercel-react-best-practices` and `vercel-composition-patterns` (applied to the settings component design in section 5). `create-mcp-app`, `add-app-to-server`, `refine-a2ui`, `web-design-guidelines` were not needed at this stage. No skill was unavailable.

Classification: **explicit redesign** of the fork Web UI (mode **Operate**, dials variance 3, motion 2, density 7, taken from the existing kubo `DESIGN.md`), plus a **new** settings surface. Authority: kubo `DESIGN.md` and `PRODUCT.md` (scope today: `mcp/ui`, `mcp/app`) already carry the Promethity v1 tokens; they need a scope extension to the fork Web UI and the Desktop windows (section 6, P-01). `PRODUCT.md` "Open decisions" line "How far the Promethity identity is applied inside the ipfs-webui fork" is answered by the operator's 2026-10-13 direction: the whole UI.

---

## 1. The Web UI fork today (GQAdonis/ipfs-webui, branch feat/ipfs-mcp-server)

### 1.1 Stack and structure

- Branch head `2be6b78c` (WebMCP re-vendor), `a204c4ce` (WebMCP), `5aa15de3` (agent chat) on top of upstream `24ddaa89`. The fork is 74 files, +34,989 lines against upstream, almost all additions (vendored island and WebMCP package, `src/agent-chat/**`, `src/webmcp/**`), so the existing app files diverge little from upstream.
- Stack: React **16.14**, react-dom 16.14, `react-scripts` 5.0.1 (CRA, via `react-app-rewired-esm`), `redux-bundler` routing, Tachyons 4 + `ipfs-css` for styling, i18next 23 + react-i18next 15, `react-hook-form` 6, Chart.js 2 / d3 5, `react-joyride` tour, Helmet for titles (`package.json:60-100`, `src/index.css:1-5`). `"type": "module"`, ESLint with warnings-as-errors, Jest + Playwright e2e (`test/e2e`).
- Hash routing (`createRouteBundle`, `routeInfoSelector: 'selectHash'`), so the app works from any gateway path such as `https://ipfs.prometheusags.ai/webui`.
- The chat island is not part of this React tree. It is a React **19** shadow-root custom element loaded by native dynamic import from a vendored static file (`public/agent-chat/ipfs-agent-chat.js`, `src/agent-chat/island-loader.js:8-13`) so the two Reacts never meet. This constraint stays: no React 19 upgrade of the Web UI shell in this phase.
- Delivery: Kubo serves the Web UI from the pinned CID in `core/corehttp/webui.go` (`WebUIPath`, D2, `bafybeiblix77…stki`). **Any rebrand build produces a new CID**, which needs a `WebUIPath` bump in Kubo (architect/mcp-server-engineer territory, `webui-cid-delivery`) and a re-pin on the LKE node (the 2026-10-13 progress-report entry shows this was a manual CAR import). Plan one delivery pass at the end, not one per round.

### 1.2 Routes and pages (every route)

`src/bundles/routes.js:13-28`:

| Route | Page | Notes |
|---|---|---|
| `/`, `''`, `/status*` | Status | bandwidth charts (Chart.js), node info, country chart |
| `/files*`, `/ipfs*`, `/ipns*` | Files | list/grid, breadcrumbs, modals (share, rename, add by path, CID info), drag-drop overlay (`App.js:70` aqua tint `rgba(99,202,210,0.2)`) |
| `/pins*` | Pins | |
| `/explore`, `/explore*` | IPLD Explorer | external package `ipld-explorer-components` (own markup and styles; cannot be restyled in source) |
| `/peers` | Peers | world map (d3, blues at `WorldMap.js:141-143`), peers table, add connection |
| `/settings*`, `/settings/analytics` | Settings, Analytics | see 1.3 |
| `/welcome` | Welcome / not-connected help | protocol explainer ("What is IPFS?"); stays |
| `/blank` | Blank page | Desktop unmounts the app here when hidden (preload) |
| `/diagnostics*` | Diagnostics | check, DHT provide, logs screens (TS) |

Shell: `src/App.js` renders a hard-coded washed-blue top bar (`App.js:74`, `#F0F6FA`), white `<main>` (`:83`), navy left rail `bg-navy` (`:90`), `NavBar` with the IPFS cube-text logo (`src/navigation/NavBar.js:5-6,66-67`), `Notify`, `ReactJoyride` tour, then the two agent mounts at `App.js:106-107` (`AgentChatLauncher` bottom-right fixed button, `WebMcpHost`). The settings page is reachable before the node connects (`App.js:84`: `ipfsReady || /welcome || /settings*`), which matters for first-run (section 5.5).

### 1.3 Settings page structure

`src/settings/SettingsPage.js` (427 lines). A single scroll column inside `mw9 center`; each topic is a `<Box>` (`#fbfbfb` inline background, `Box.js:16`) with a `Title` (`<h2 class="ttu tracked f6 fw4 teal">`, `Title.js:5`). Order: API address (hidden under Desktop), local gateway, share-link type, IPNS publishing keys, pinning services, retrieval diagnostic service, language, analytics (conditional), `Experiments` (`:138`), **`AgentChatSettings` (`:141`)**, **`WebMcpSettings` (`:145`)**, CLI tutor mode, then the JSON config editor (Ace, `ipfs_dark` theme, `src/settings/editor/theme/ipfs_dark.js`). There is no tabbing, no sub-navigation, no section anchors; the agent sections sit in the middle of a 14-section scroll.

### 1.4 Existing agent settings (to be replaced or absorbed)

Agent chat (`src/agent-chat/`):
- `AgentChatSettings.js:14-124`: one checkbox "Enable agent chat" (`id=agent-chat-toggle`, `:68-75`), an endpoint text field with `validateEndpoint` (https or loopback http, path ends `/agui`, no credentials/query/fragment; `endpoint.js:12-25`), a password field for the bearer token, status text, Save/Set/Forget buttons. Under Desktop (`getBridge()` present) it collapses to one `role=status` sentence (`:35-43`).
- `settings-store.js:13-14`: `ipfs-webui:agent-chat` (`{enabled, endpoint}`) and `ipfs-webui:agent-chat:token` in **localStorage**; **defaults are off and empty** (`:20`). This contradicts the new rule (commit `5f019f0df`: agent features ship enabled). Turning the chat off deletes the token (`:62`).
- `AgentChatLauncher.js`: renders nothing unless enabled and endpoint valid (`:60-72, :96`); lazy-loads the island on first press; button styled with Tachyons `bg-navy white` (`:119`). The Desktop bridge (`bridge.js:9-13`, `window.ipfsDesktop.agentChat.{getEndpoint,getToken}`) is read-only today.
- Locale: `public/locales/en/agentChat.json` (35 lines) exists only for `en` plus one other locale (2 of 33), and falls back to `en`.

WebMCP (`src/webmcp/`):
- `settings-store.js:17`: `{enabled, dnslink, writes}`, all off by default; `enabled`/`dnslink` in localStorage, **`writes` never persisted by design** (security R13 L3, `:5-11,40`). `WebMcpSettings.js` has three `Toggle`s with `aria-describedby` help, a live `role=status` registration line, and the `Permissions-Policy: tools=()` operator note. `WebMcpConfirmDialog.js` (298 lines) is the in-page approval; `webmcp.css` holds its styles.
- Per 5aa15de3's replacement rule, **WebMCP `enabled` must now default on**, `writes` stays per-load and off (writes still require confirmation; the persisted-`writes` hazard is unchanged and must not be relaxed by the "ships enabled" rule).

What exists is good in structure (labelled inputs, described-by help, `role=status`/`alert`, write-only token field) but visually Tachyons-default, and it models a single endpoint, not a backend choice.

### 1.5 i18n

`src/i18n.js:41-52`: namespaces `app, welcome, status, files, explore, peers, settings, diagnostics, notify, agentChat, webmcp`; `fallbackLng` default `en`; `load: 'currentOnly'`. 33 locale directories under `public/locales`. `docs/LOCALIZATION.md` says locale files are Transifex-managed ("please don't edit the locale files directly"). Consequence: edit **`en` only**, add new keys there, and let every other language fall back to `en` for the new strings. Do not hand-edit 33 files.

### 1.6 Theming

There is no theme system. Colour comes from `ipfs-css` Tachyons classes (navy `#0b3a53`, aqua `#69c4cd`, teal `#378085`, charcoal `#34373f`, snow, washed-blue; `node_modules/ipfs-css/theme.json`) plus **36 source files with hard-coded hex or rgba** (chart series `#69c4cd #f39021 #ea5037 #3e9096` in `CountryChart.js:38`, `NetworkTraffic.js:46-54`, `NodeBandwidthChart.js:121`, map blues in `WorldMap.js:141-143`, `Box.js:16`, `App.js:70,74`). There is **no dark mode** and no `prefers-color-scheme` handling in the app (only `Loader.css`, `JsonEditor.js`, `ipfs_dark.js`, `FileImportStatus.js` mention dark). Fonts are Inter and Montserrat from `ipfs-css` (bundled woff2/otf; nothing fetched).
Border use: 40 files carry `b--*` Tachyons border-colour classes plus `ba/bb/bt`-style borders; `NavBar.css:1-4,24-30` uses 5px active-link bars. Shadows: 11 files. These conflict directly with Promethity Flat 2.0 (no borders, no shadows).

### 1.7 Logos, favicon, manifest, title

- `public/index.html:6` theme-color `#0b3a53`; `:7` generator meta names `ipfs-shipyard/ipfs-webui`; `:9` description "A web interface for Kubo RPC…IPFS node stats…"; `:15` `favicon.ico` (34,494 bytes, IPFS cube); `:25` `<title>WebUI | IPFS</title>`.
- `public/manifest.json`: `short_name "IPFS WebUI"`, `name "IPFS Web UI"`, one 512 icon `ipfs-logo-512-ice.png`, `theme_color`/`background_color` `#0b3a53`. No 192, no maskable.
- `src/navigation/ipfs-logo-text-vert.svg`, `ipfs-logo-text-horiz.svg`, `ipfs-logo.svg` (NavBar and Welcome).
- Document titles: 9 files use Helmet (`SettingsPage.js:47` `{t('title')} | IPFS`, and the same suffix in 5 more places; Explore is `| IPLD`; Pins is a literal `Pins Status`). Locale titles hard-code the suffix in 15 of 33 locale files. `test/e2e/navigation.test.js:13-41` asserts exact titles `Status | IPFS`, `Peers | IPFS`, `Settings | IPFS`, `/ | Files | IPFS`, `Explore | IPLD`.

### 1.8 Where IPFS branding appears (count)

- Source: **139** files under `src` mention "ipfs" in any form (most are API names such as `ipfs-provider`, `ipfs-path`, which must not change). User-facing brand surfaces that matter: NavBar logos and code/bug links (`NavBar.js:62-63,83-89`, which also link to `github.com/ipfs/ipfs-webui` and show the git revision), `index.html`, manifest, favicon, 9 Helmet titles, Welcome page, Tour copy (`src/lib/tours.js`), `Connected` status, `Notify`, About-IPFS components, share-link text.
- `en` locale: **84** occurrences of "ipfs" across **9** files (`app, welcome, status, files, explore, settings, notify, agentChat, webmcp`), but most are the protocol ("What is IPFS?", "IPFS Companion", "native IPFS address", "Connected to IPFS"), not the product name. Only product-name uses (title suffix, "IPFS Web UI", "IPFS Desktop manages…", welcome page "IPFS cube logo") need rewriting. Rule: **the protocol keeps its name; the product wears Promethity.**
- Estimated rebrand edit set: ~45 to 60 source files (shell, NavBar, Box/Title/Button, settings, charts, logos, Helmet titles, index.html, manifest, css), 9 `en` locale files, 1 e2e file, plus assets. Not 139.

### 1.9 What the rebrand must keep

- `LICENSE` (MIT, Copyright 2018 Protocol Labs) stays at the repo root and in the build; add the Promethity fork notice beside it, never replace it. Same for ipfs-desktop (`LICENSE`, MIT) and Kubo (MIT/Apache-2.0 dual).
- A visible, truthful attribution in the app: an About/footer line such as "Built on the IPFS Web UI (MIT, Protocol Labs). IPFS is a protocol; Promethity is not affiliated with IPFS Foundation / Protocol Labs unless that is true" (open question 3). Keep the `NavBar` "source" and "report a bug" links but point them at the fork, and keep the upstream link in About.
- Dependency licences: the bundled fonts are OFL 1.1; OFL requires the licence text to travel with the fonts. `ipfs-css` already ships `Inter-LICENSE.txt` and `Montserrat-LICENSE.txt`; the Promethity `design-system/fonts/` folder has **no licence file** (gap, section 4).
- Protocol-named UI stays: "IPFS", "IPLD", "Kubo", "CID", "Companion", "Desktop" in feature copy. The product chrome says Promethity.
- Do not break Kubo behaviour: the Web UI is a client of `/api/v0` and the gateway; none of the rebrand touches wire or API contracts. The WebMCP `vendor/` tree and `public/agent-chat/` are hash-pinned (`webmcp:vendor:check`, `MANIFEST.json`); they are rebuilt from kubo `mcp/`, never hand-edited in the fork.

---

## 2. The agent-chat island (kubo `mcp/chat`, `mcp/ui`, `mcp/app`)

- `mcp/chat` (`@gqadonis/ipfs-agent-chat`, React 19.3, Tailwind 4, shadcn-style sidebar, `@a2ui/react` 0.12): `<ipfs-agent-chat>` with `attachShadow` (`src/element.ts:123`), a constructable stylesheet adopted into the shadow root (`element.ts:216-220`, `island/mount.tsx:22-26`), observed attributes `endpoint`, `open`, `theme` (`element.ts:48`). Tailwind entry `src/styles/island.css` maps shadcn semantic names to Promethity tokens with `@theme inline` and deliberately omits preflight (`island.css:1-30`). Built bundle `dist/` is 4.4 MB with chunks.
- `mcp/ui/src/styles/tokens.css` (103 lines) scopes the Promethity v1 tokens to `[data-ipfs-ui]`, not `:root`, so they work inside a shadow root and an iframe alike. `mcp/ui/fonts/` already holds **Hanken Grotesk 400/500/600/700**, Syne 600/700, JetBrains Mono 400/500/600, Newsreader; `mcp/ui/src/fonts/` embeds them as data (no remote fonts).
- Conclusion: **the island is already on Promethity tokens.** What the rebrand needs from it:
  1. `theme` attribute driven by the Web UI theme switch (today there is none; the island follows `prefers-color-scheme`/attribute). One line in the launcher: pass `theme` from the shell's resolved theme.
  2. Shell/island visual coherence: the launcher button is a navy Tachyons button (`AgentChatLauncher.js:119`); it must become the Promethity primary (accent) or secondary (control) button, and sit with the same fixed-position and focus rules.
  3. Backend-aware empty state inside the island: no backend chosen yields "Pick a model to start" with a link into Settings (section 5.5). The island needs a small `backend-status` input (attribute or event) rather than reading settings itself, because it must keep talking only to `/agui`.
  4. The MCP App views (`mcp/app`) are iframe-hosted and unaffected by the Web UI rebrand; they already carry the tokens.
- Cost note: the island's React 19 + Tailwind bundle means the Settings UI should **not** live in the island (it would require the island to be loaded, which contradicts "loaded only when used"). The Settings panel lives in the Web UI shell (React 16), small and dependency-free.

---

## 3. IPFS Desktop fork (tribe-health/ipfs-desktop, head `0ea60ce`)

Where Desktop shows settings today:
- **Tray menu** (`src/tray.js:240-262`): checkboxes for app preferences, experiments, CID profile, provide strategy, and the agent confirm submenu (`src/agent/confirm/tray-menu.js`). The agent chat has **no tray entry**: it is configured only by hand in the Desktop store (`agentChat.enabled`, `agentChat.endpoint`, `src/agent/consts.js:9-11`; `src/agent/settings.js:34-39`).
- **Web UI window**: `webui://localhost`, sandboxed preload (`src/webui/preload.js`), CSP and `webSecurity: true` (`src/webui/security.js`). The preload exposes `window.ipfsDesktop.agentChat.{getEndpoint,getToken}` only when the chat flag is on (`preload.js:19-21`; `bridge.js:25-26`). The Web UI already has the full Settings page, including Desktop-only sections (`isIpfsDesktop` branches in `SettingsPage.js`).
- **Native windows**: confirmation and pairing windows (`src/agent/confirm/ui/*`, 360-line `windows.js`) are already Promethity (`ui.css:1-36`: tokens verbatim, bundled Hanken 400/600, Syne 600, JetBrains 400, "surfaces separate by fill, never by border"). Their font set lacks Hanken 500/700 and Syne 700, which is fine for those two windows.
- `package.json` productName is still "IPFS Desktop"; icons under `assets/icons` (status, tray) and `assets/build` are IPFS-branded. Goal 5 says "Web UI"; Desktop shell branding (app name, dock/tray icon, installer) is **not** in scope unless the operator says so (open question 4).

**Recommendation: one panel in the bundled Web UI, for both Desktop and cloud. No new native settings window.**

Reasons:
1. One implementation, one test surface, one accessibility audit; Desktop loads the same Web UI build (`scripts/copy-local-webui.js`), so a cloud fix and a Desktop fix cannot diverge.
2. The Settings route is reachable offline (`App.js:84`), so first-run works before the node is up.
3. A native window would duplicate model-picker, test-connection, and status UI in a second renderer with its own preload and IPC surface, enlarging the hardened attack surface Desktop just closed (`docs/agent/HARDENING.md`).
4. Native windows stay for what only native can do: the **confirmation window** (Touch ID, signed channel) and **pairing**. They are trust surfaces, not settings.

What Desktop must add so the Web UI panel works there (owner: ui-ux-engineer, `../ipfs-desktop/src/agent/**`, `src/webui/**`):
- Extend the read-only bridge to a small, validated, write bridge: `ipfsDesktop.agentSettings.{get, set, testConnection, setApiKey, clearApiKey, status, onChange}` with channels registered only for the trusted main frame (reuse `isTrusted` in `bridge.js:19-26`). The API key is write-only into `safeStorage` (`secret-store.js`) and never returned to the renderer; `get` returns `{keySet: boolean}`.
- Desktop starts the local endpoint by default now (goal 4), so `agentChat.enabled` defaults true in the store, and the tray gets one read-only status line plus "Open agent settings" (navigates the Web UI window to `#/settings/agent`) and an off switch. No new native form.
- Under Desktop the panel hides the manual endpoint field (Desktop owns it) and shows "Local endpoint, managed by Desktop" with the port and running state.

---

## 4. Promethity brand (read from `/Users/gqadonis/Projects/promethity/branding`)

Sources read: `README.md`, `promethity-brand-system.md` (complete), `design-system/README.md` (identical rules), `design-system/components/*/README.md` (Button, Input), `logo/p-icon/README.md`, the file trees of `logo/`, `design-system/`, `docs/markdown/` (7 business documents, not UI guidance) and `publish/` (PDFs, OG PNGs, `cids.json`; business collateral, not a UI source).

### 4.1 Tokens

- **Surfaces** (dark / light): `ground` `#0C0B0A`/`#F5F2EE`, `surface` `#171614`/`#FBF9F6`, `raised` `#222120`/`#ECE7E0`, `control` `#2D2C2A`/`#E1DBD2`. **Text**: `text` `#F5F2EE`/`#141210`, `text-muted` `#A09890`/`#5E5750`. **Accent**: `accent` `#E96A12`/`#CE3012`, `on-accent` `#0C0B0A`/`#FFFFFF`, `accent-text` `#E96A12`/`#B02A10`, `accent-tint` `#342214`/`#F6E5DF`, `glow` `#F49E12` (illustration only). **State** (each with `-tint`): `ok` `#5FBF7A`/`#1A6B35`, `warn` `#F2B33D`/`#8A5A00`, `err` `#F0705A`/`#B3261E`, `info` `#7FB2D9`/`#1F5E8C`; always paired with a word.
- **Type**: Syne 700/600 headings (sentence case, never below 17px, never caps, never paragraphs), Hanken Grotesk body/UI, Newsreader quote (one per page, never UI), JetBrains Mono eyebrow/code. Scale: display 56, h1 44, h2 28, h3 22, h4 17, lead 19, body 16/1.65, small 14, label 14.5/600, code 13/1.7. DESIGN.md narrows body to 15/1.55 for Operate.
- **Space**: 4, 8, 14, 24, 40, 56, 88. **Radius**: sm 6 (buttons, inputs), md 10 (cards, tables), lg 16, pill 999 (badges only). **Depth**: fills only; no shadows, no gradients, no glow, no purple.
- **Motion**: brand file defines none; the kubo DESIGN.md sets Operate 150-250ms, colour/opacity/transform only, reduced motion removes all.
- **Focus**: solid 2px `accent` outline, offset 2px. **Hover**: one surface level lighter, +8% brightness on accent. **Active tab/link**: `accent-text` + 600, no pill, no underline. **Disabled**: 45% opacity.
- **Tailwind 4/shadcn map** exists (`promethity-brand-system.md` "Tailwind 4 and shadcn"): `--color-border: transparent`, no outline button variant; already used by `mcp/chat`.

### 4.2 Logos and assets

- `logo/wordmark/`: `promethity-wordmark-{dark,light}.svg`, `-on-{dark,light}-transparent.svg`, `promethity-{ai,com}-{dark,light}.svg` lockups. Rules: min width 120px, clear space one flame height, never recolour/stretch/outline/caps, never on an accent fill, **never wordmark and icon side by side**.
- `logo/p-icon/`: `svg/` masters (dark/light, square, transparent marks), `web/` (`favicon.svg`, `favicon.ico` 16/32/48, `favicon-16/32/48.png`, `apple-touch-icon.png` 180), `ios/` (1024 light/dark), `android/` (adaptive layers + 512), `pwa/` (`icon-192`, `icon-512`, `maskable-512`). Min icon size 16px. README gives the exact `<link>` and manifest snippets.
- Concepts 01 and 02 are kept for reference only; concept 03 (Torch) is the identity.

### 4.3 Voice and tone

Statement, mechanism, stakes. Short declarative sentences, present tense for system state ("Denied. Policy no-export-phi blocked this action."). Sentence case everywhere except mono eyebrows. "You" and "we". No exclamation marks, no emoji. Real numbers, exact names. Banned: revolutionary, game-changing, cutting-edge, seamless, world-class, empower, disruptive, unlock, leverage, harness, delve, utilize, synergy, roadmap, journey, military-grade, trust us; and the virtue words (safe, private, ethical, trustworthy, responsible, secure) unless the mechanism is in the same or next sentence. Existing `en` strings that trip this: `explore.json:48` ("Your journey with IPLD Explorer…"), `settings.json:57` ("the safer choice…" has no mechanism), `explore.json:34` "privateGateway" (a key, not copy). Small list, easy to fix. The product tagline "Fire you can hold." is marketing; do not put it in operator chrome.

### 4.4 Gaps and conflicts found in the brand

1. **Flat 2.0 "no borders" fails WCAG 2.2 1.4.11 for form fields.** Computed from the token hexes: dark `control` on `surface` is **1.30:1**, on `ground` 1.41, on `raised` 1.15; light `control` on `surface` 1.31, on `ground` 1.23. A text input, select, checkbox or radio identified only by a fill that close to its container fails the 3:1 non-text contrast rule, and the brand's own Input component is a borderless fill (`components/Input/README.md`, `bundle.css:26`). The task's bar is WCAG 2.2 AA, which outranks "no borders". Proposed exception, to be recorded in `DESIGN.md`: form controls (inputs, selects, checkbox/radio/switch glyphs, the segmented backend picker) draw a 1px inset edge in `text-muted` (4.9:1 on `control` dark, 5.2:1 light; 6.4+ on surfaces). All other surfaces stay borderless. This is a deliberate narrowing, the same kind DESIGN.md already makes ("Craft decisions that narrow the brand"). Needs operator sign-off (open question 2).
2. **Light-theme accent on `control`**: accent `#CE3012` on `control` is 3.76:1 (above the 3:1 UI floor, below 4.5:1 for text); `accent-text` `#B02A10` on control is 4.79:1. Brand rule already forbids dark `accent-text` on `control` (4.34:1). Keep that rule and apply it in the Web UI (no accent-coloured text inside inputs).
3. **Dark-first brand, light-only Web UI.** The Web UI has no dark theme. The rebrand must ship both (task requirement), with a theme control (System, Dark, Light), persisted in localStorage, defaulting to System and dark when the system has no preference (matches the brand "dark-first" and DESIGN.md "follows the host").
4. **Font coverage.** `branding/design-system/fonts/` holds only Latin subsets and only Hanken 400/500/600 (no 700, no italic), Syne 600/700, JetBrains 400/500/600, Newsreader 400/500/italic (12 files). The Web UI ships **33 locales** including Cyrillic (ru, uk), Arabic/Urdu, Hindi, CJK, Greek-adjacent. Latin subsets fall back to system fonts for those scripts: acceptable (declare a system stack with `unicode-range`), but a decision to document. `mcp/ui/fonts/` already has Hanken 700; reuse that copy.
5. **Font licensing.** Brand README states all four families are SIL OFL 1.1 via Google Fonts; the shipped folder contains no `OFL.txt` per family. OFL requires the licence to accompany redistribution (and forbids selling the fonts alone). Add `public/fonts/LICENSES/` with the four OFL texts and a notice in About. Ship woff2 files only, `font-display: swap`, subsetted; drop `ipfs-css` Inter/Montserrat to remove ~100 font files from the bundle (they are fetched only when used, but they sit in the pinned CID and inflate the CAR the operator imports).
6. **Favicon sizes.** The p-icon set has 16/32/48 PNG+ICO, SVG, 180 touch icon, 192/512/maskable PWA icons: complete for the web. Missing: a monochrome/pinned-tab mask (optional), a 256 for Windows tiles (Desktop only), and a **light-theme `theme-color`** value (two `<meta name=theme-color media=…>` tags, `#0C0B0A` and `#F5F2EE`).
7. **Icons.** Brand: Lucide 1.5px stroke, 16/20/24. Web UI: 153 files in `src/icons` (ipfs-css Stroke*/Glyph* set, svgr-generated, plus a small `src/icons/lucide/` folder of six glyphs). No lucide-react dependency in the Web UI (`mcp/chat` has it). Replace gradually: nav rail and settings first, the rest recoloured with `currentColor`. Several glyphs (`GlyphCloud`, `GlyphLock`, `GlyphPinCloud*`) violate the "no clouds, padlocks, shields" imagery rule if used as illustration; as small utility icons with text they are acceptable, but the Welcome page illustrations must go.
8. **No motion tokens** in the brand; use DESIGN.md's.
9. **No documented component for a switch/toggle, select, segmented control, stepper or inline-status line.** The brand has Button, Badge, Card, Callout, Input, Table, Quote, PolicyDecision, Cover only. Settings needs a Switch, Select/combobox (model picker), Segmented control (backend), Status line, Field. Build them as Promethity-conformant primitives in the fork and propose them back to `branding/design-system/` (do not edit the branding repo from here).
10. **`PolicyDecision` is the brand signature** (one action and what was decided). Use it for the Settings "Writes need confirmation" row and for test-connection results instead of inventing a status pattern.

---

## 5. Settings UI requirements (goal 1) and the visual direction (goal 5)

### 5.1 Information architecture

Add a dedicated route **`#/settings/agent`** ("Agent") rather than a 15th box in the long scroll. Keep `/settings` as the home with a one-line "Agent" summary row (status badge + link) near the top, since the agent now ships enabled and its health matters. `route`: add `'/settings/agent': AgentSettingsPage` before `'/settings*'` (`routes.js:21`). Old `AgentChatSettings` and `WebMcpSettings` boxes at `SettingsPage.js:141,145` are removed and their content moves to the new page. Keep `data-id` hooks for the existing tests.

Page layout (Operate, density 7, one column up to 760px reading width, two-column field/help at >= 860px; breakpoints 860 and 540 from the brand):

1. **Header strip**: h1 "Agent" (Syne), one `PolicyDecision`-style status line stating the current truth ("Agent chat is on. Backend: not chosen. Pick a model to start." / "Backend: OpenAI-compatible, liter-llm, model qwen3:8b. Applied 14:02."). The only accent fill on the page is the **Apply** primary button.
2. **Backend** (segmented control, radio group semantics): `Universal Agent Runtime` | `OpenAI-compatible endpoint`. No default selected on first run (operator rule: no third-party backend chosen for the operator). Selecting one reveals its field set; the other set unmounts.
3. **Backend fields** (section 5.2).
4. **Features** (three switches, section 5.4).
5. **Connection** (Desktop: read-only "Local endpoint, managed by Desktop :PORT"; cloud: the agent endpoint field and token field, retained from today).
6. **Apply bar** (sticky bottom within the page on small screens): Apply, Revert, last-applied time.

### 5.2 Fields per backend

Common contract (what the UI needs from the settings API, which the architect designs; the UI assumes JSON over the same authenticated origin as `/agui`):
- `GET settings` returns `{ backend: null|"uar"|"openai", features: {chat, webmcp, confirm}, uar: {url, agentId?, agentStatus}, openai: {baseUrl, model, keySet: boolean}, applied: {at, ok, error?}, restartRequired: false }`. **Never returns a secret.**
- `PUT settings` accepts the same shape plus write-only `openai.apiKey`/`uar.apiKey` fields; returns the applied state synchronously or a job id.
- `POST settings/test` with a candidate `{backend, url, apiKey?}` returns `{ok, latencyMs, models?: string[], error?: {code, message}}` without persisting anything.
- `GET settings/models` returns the model list for the saved/tested backend.
- `POST settings/uar/agent` (create/ensure the IPFS agent) and `GET settings/uar/agent` for its status; steps reported as an ordered list so the UI can show progress.
- A change event channel (SSE or poll) for `applied` so a second tab and Desktop reflect changes.

**UAR instance**
| Field | Control | Behaviour |
|---|---|---|
| Instance URL | text, `inputmode=url`, https or loopback http (reuse `validateEndpoint` logic minus the `/agui` rule) | validated on blur; inline error tied by `aria-describedby` |
| Access credential | password, write-only | shows "A key is stored" + Replace/Remove; never echoed |
| Test connection | secondary button | `aria-busy`, result as an inline status line, `role=status` |
| IPFS agent | status block + "Create agent" primary-in-section (secondary style; Apply is the only accent) | states: Not created / Creating (step list) / Ready / Failed (reason + Retry). Shows what it creates in words: system prompt, the kubo MCP tools, the IPFS agent skills |
| Agent name / model | read-only after create; model select if UAR exposes models | |

**OpenAI-compatible (liter-llm and others)**
| Field | Control | Behaviour |
|---|---|---|
| Base URL | text | `https` or loopback `http`; hint "Example: http://127.0.0.1:4000/v1" (a documentation example, not a default) |
| API key | password, write-only | optional (local liter-llm may need none); shows stored/not stored |
| Test connection | secondary button | calls `settings/test`; on success populates the model list |
| Model | combobox (`role=combobox` listbox, typeahead, free-text allowed) | disabled until a test passes or the list loads; shows count ("12 models") and the chosen id in mono; free text allowed for gateways that hide `/models` |
| Advanced (disclosure) | temperature/max tokens only if the proxy supports them | collapsed by default; omit if the architect does not expose them (do not invent) |

Rules for both: label above, help below, placeholder shows format never label (ui-ux-pro-max Forms + brand Input rule). Errors are specific and on the field ("The server answered 401. Check the key."), plus a page-level summary region only when Apply fails. Never colour alone: every state has a word and an icon.

### 5.3 Apply without restart: feedback model

Four visible states in one component (a single `ApplyStatus` status region, `role=status`, `aria-live=polite`; errors `role=alert`):
1. **Unsaved changes** (dot + "Unsaved changes", Apply enabled, `beforeunload` guard is not used; instead a labelled in-page note).
2. **Applying** (`aria-busy` on the form, Apply shows spinner text "Applying", fields remain readable, not disabled to avoid focus loss; concurrent edits queue to the next Apply).
3. **Applied** ("Applied 14:02. New chats use qwen3:8b. A chat in progress finishes on the previous backend." (the exact semantics must be confirmed by the architect: does an in-flight run continue?)).
4. **Failed** (reason, what stayed unchanged: "Nothing was changed. The previous backend is still in use." Deny-is-final tone) with Retry.
Never "restart required": if the architect's API cannot hot-apply something, the UI names it as a limitation per field, with its own state, not a generic reload prompt. A "last applied by this browser/Desktop at …" line is the only history shown (no logging beyond the operator's own machine, per PRODUCT.md constraints).

### 5.4 Off switches and first-run

Three switches, each with its own help text and a live status chip (word + icon): **Agent chat** (default on), **WebMCP tools** (default on; sub-options DNSLink off, "Allow writes this session" off every reload, retained verbatim from today), **Desktop confirmation window** (Desktop only; default on; hidden in the cloud UI). The "writes still require confirmation" rule is displayed as a fixed non-switch `PolicyDecision` row ("Writes and deletes ask you first. This cannot be turned off here.") so no one searches for a switch that does not exist. Every switch has a disabled-state test (task requirement) and a documented off path in `docs/config.md` terms owned by the architect.

**First run** (no backend chosen): the chat launcher is still present (agent features ship on) but the island shows the empty state "Pick a model to start. The agent needs a backend before it can answer. Choose one in Settings." with a single link button "Choose a backend" (route `#/settings/agent`, focus lands on the segmented control). The Settings page itself leads with a plain two-step list (1 Choose a backend, 2 Pick a model or create the agent), no wizard modal. No sample or hosted default is pre-selected. The Welcome page and Status page get one `Callout` ("The agent has no model yet") until a backend applies, dismissed only by configuring or switching chat off.

### 5.5 Cloud (basic auth) versus Desktop

- **Cloud, `https://ipfs.prometheusags.ai/webui`**: the Web UI loads over HTTPS behind HTTP basic auth, so the browser already holds credentials for the origin. The settings API must live on a path the browser can reach with those same credentials, or the browser will prompt per request (open dependency on the architect: same-origin route, reverse-proxy rule, and `agent.origin_allowlist` containing the origin). The UI must therefore (a) treat 401 from the settings API as "Sign in again", not a generic failure, (b) never put secrets in URLs, (c) keep the key write-only so a shared basic-auth operator cannot read another's key. Token storage stays in localStorage for the bearer token only where today's decision (2026-10-10) stands; the new provider API keys never touch the browser after submit.
- **Desktop**: the Web UI talks to the bridge (3 above), the endpoint and secrets are managed in the main process, the manual endpoint/token fields are hidden, and the page shows "Managed by IPFS Desktop" with the running state and port. Both paths share every component; only a thin `SettingsClient` adapter differs (HTTP vs bridge), injected through context (composition rule: provider is the one place that knows the transport).

### 5.6 Component and code design (React 16 shell, vercel rules applied)

- Compound components with a context provider: `AgentSettings` (state + actions + meta via a `{state, actions, meta}` context), children `AgentSettings.Backend`, `.UarFields`, `.OpenAiFields`, `.Features`, `.ApplyBar`, `.Status`. No boolean-prop modes; explicit variants `UarBackendForm` and `OpenAiBackendForm` (composition: architecture-compound-components, patterns-explicit-variants).
- Transport via an injected `SettingsClient` interface (`http` and `desktop-bridge` implementations); the provider is the only place that knows (state-decouple-implementation).
- Effects: no data fetching in `useEffect` chains (React 16 has no `use`/Suspense data); one request per user action; derive validity during render (rerender-derived-state-no-effect), use functional `setState`, hoist static option arrays, `startTransition` is not available in React 16: debounce the test-connection button instead (disabled while pending). Lazy-load the settings route (`@loadable/component`, like the other pages) so the Status page does not pay for it (bundle-dynamic-imports); do not import the page from `routes.js` eagerly.
- Persistence: `localStorage` only for non-secret UI preferences (theme, last tab); schema-version the key (client-localstorage-schema).
- Limits of React 16/CRA: no `useId`, no `useTransition`, no `forwardRef` removal, no React 19 `ref`-as-prop; generate ids with a stable counter hook. Tailwind 4 and shadcn **cannot** be used in the shell (CRA 5, PostCSS 8 toolchain, `react-scripts` pinned): use plain CSS with custom properties (`src/promethity/*.css`) and keep Tachyons class names in markup (minimum diff). The island keeps Tailwind inside its shadow root.

### 5.7 Accessibility (WCAG 2.2 AA, light and dark)

- Contrast: every token pairing in DESIGN.md meets text >= 4.5:1 and focus ring >= 3.7:1; add the form-edge exception (4.4 item 1) to satisfy 1.4.11; a unit test recomputes pairings from the shipped CSS, as `mcp/ui` already does.
- Keyboard: segmented control is a radio group (arrow keys move and select, Tab leaves); combobox follows the APG pattern (Up/Down, Enter, Esc, `aria-activedescendant`); switches are `role=switch` with Space; all inputs reachable in DOM order; no keyboard trap; the sticky Apply bar must not hide a focused control (2.4.11 Focus Not Obscured: scroll-padding equal to bar height).
- Target size 2.5.8: 24x24 minimum; the brand sets 44px buttons, adopt 44 for Apply/Test, 24 minimum for switch glyph hit areas.
- Forms: visible label, programmatically associated help and errors (`aria-describedby`, `aria-invalid`), error summary only on Apply failure with `role=alert` and focus moved to it; autocomplete off on key fields; password fields not exposed to password managers as login (`autocomplete="off"`, plus `data-1p-ignore`).
- Status never by colour alone (word + icon); live regions polite for progress, assertive only for failure; no timeouts on messages that need reading (3.3.8 Accessible Authentication does not apply, but avoid cognitive-test patterns for the key).
- Motion: 150-250ms colour/opacity/transform only; `prefers-reduced-motion: reduce` removes transitions and any spinner animation (spinner replaced by static "Applying" text).
- Reflow 320px and 400% zoom without horizontal scroll; tables and long CIDs scroll in their own frame.
- Verification evidence for the Plan stage: axe in both themes for every state, keyboard traversal script, reduced-motion check, 320/768/1024/1440 screenshots in both themes (web-testing rule), plus an independent `reviewer`.

### 5.8 Rebrand design direction (goal 5)

- **World** (from DESIGN.md, extended): "a dark, well-built room with one warm light". The Web UI becomes a tool in that room: `ground` page, `surface` rail and panels, `raised` nested wells, `control` fields; ember used once per view. Light theme is the same room with the lights on (Ash/Paper/Sand).
- **Shell redesign** (not a recolour): replace the navy left rail and washed-blue top bar with: a `surface` left rail (wordmark at top, p icon when collapsed under 860px, Lucide icons at 1.5px, active item `accent-text` + 600 and a fill step, no bars), a `ground` top bar that holds the explore form and connection status (status is a word + icon badge, not a green dot), and `main` on `ground` with `surface` panels (no `Box` white sheets). Settings panels use fills, not `b--` borders. The `AgentChatLauncher` becomes the one persistent action area bottom-right (secondary style; primary only when the chat has a pending confirmation).
- **Typography**: Syne for page and panel titles only (h1 44 is too large for an Operate tool: use h3 22 / h4 17 per DESIGN.md); Hanken 15/1.55 for UI; JetBrains Mono for CIDs, multiaddrs, peer ids, sizes, with `font-variant-numeric: tabular-nums`; replaces Inter/Montserrat and the `ttu tracked` teal section titles (`Title.js:5`).
- **Data visualisation** (rule: data viz is part of the design system): charts get a Promethity series palette from the state/neutral tokens (`info`, `text-muted`, `ok`, `warn`, with `glow` allowed in charts per brand), never raw hex in components; map dots and gradients (`NodeBandwidthChart.js:121`) become flat fills. A `tokens.js` module exports resolved colours for Chart.js/d3, read from CSS variables at render and on theme change.
- **Implementation strategy that keeps the fork mergeable**:
  1. New `src/promethity/` folder (tokens.css, fonts.css, base.css, theme-overrides.css, `brand.js` constants, logo components) imported last in `src/index.js`. `theme-overrides.css` remaps the ipfs-css colour classes (`.bg-navy`, `.aqua`, `.teal`, `.bg-snow`, `.charcoal`, `.b--*`, etc.) onto tokens, so the ~163 files using Tachyons classes recolour with **zero edits**.
  2. Targeted edits only where structure changes: `App.js` shell wrapper (inline styles lines 70/74, `main` class), `NavBar.js` + `NavBar.css`, `Box.js`, `Title.js`, `button.tsx`/`button.css`, `SettingsPage.js` (remove two boxes, add summary row), charts (3 files), `index.html`, `manifest.json`.
  3. Brand name in one constant (`src/promethity/brand.js`: product name, title suffix, source URL) and one Helmet helper; stop editing 15 locale files for the `| IPFS` suffix (they are Transifex-managed): compose the title as `${t('title')} | ${brand.name}` in the nine places and keep the locale `title` values.
  4. Keep a `docs/PROMETHITY-FORK.md` delta list (file, why, upstream-merge note) so future `git merge upstream/main` conflicts are mapped.
  5. Expect conflicts only in the ~12 structurally edited files; the override-CSS approach does not conflict at all.
- **Do not**: migrate Web UI to React 19/Vite/Tailwind (large, off-goal, breaks the island isolation rationale and the CID-pinned CRA build); rewrite Tachyons markup across 163 files; touch `ipld-explorer-components` internals (restyle through the override layer and a wrapper; the package's own markup stays, which is a visible compromise on the Explore route, flagged in risks).

---

## 6. Gap list, risks, open questions, proposed changes

### 6.1 Gap list

| # | Gap | Evidence |
|---|---|---|
| G1 | No backend concept in settings; only a chat toggle + `/agui` URL + token | `AgentChatSettings.js:68-122` |
| G2 | Agent chat and WebMCP default **off**, contradicting `5f019f0df` | `settings-store.js:20` (chat), `webmcp/settings-store.js:17` |
| G3 | No settings API, no test-connection, no model list, no apply/feedback state | none in either repo |
| G4 | Desktop bridge is read-only; no write path, no tray entry for the chat | `bridge.js:25-26`, `consts.js:9-11`, `tray.js:240-262` |
| G5 | No dark theme, no theme control, no token system in the Web UI | section 1.6 |
| G6 | 36 files with raw hex, 40 with border classes, 11 with shadows (vs Flat 2.0) | section 1.6 |
| G7 | Branding: logos (3 svg), favicon.ico, manifest (1 icon, no maskable, wrong colours), titles in 9 places + 6 e2e assertions + 15 locale files | section 1.7 |
| G8 | Brand fonts missing Hanken 700, non-Latin coverage, OFL texts; ipfs-css Inter/Montserrat to remove | section 4.4 items 4-5 |
| G9 | Brand borderless inputs fail 1.4.11 (1.30:1 / 1.31:1) | section 4.4 item 1 |
| G10 | No brand components for switch, select/combobox, segmented control, status line | section 4.4 item 9 |
| G11 | Island has no "no backend" empty state and does not follow a shell theme switch | section 2 |
| G12 | Rebrand needs a new `WebUIPath` CID and an LKE re-pin | section 1.1 |
| G13 | `DESIGN.md`/`PRODUCT.md` scope excludes the Web UI fork and Desktop windows | repo `DESIGN.md:3` |
| G14 | Voice: three `en` strings break brand rules | section 4.3 |
| G15 | `agentChat.json` and `webmcp.json` exist in few locales (fallback to en is fine but untested for new keys) | section 1.4 |

### 6.2 Risks

1. **Size of the rebrand versus mergeability** (highest). 163 files use Tachyons. Mitigation in 5.8: override-layer-first, structural edits in ~12 files, delta doc. Residual: upstream restyles that touch those 12 files will conflict.
2. **CRA / React 16 ceiling.** No Tailwind 4, no React 19 features, no `useId`/transitions; settings must be plain CSS and hand-rolled accessible primitives. This costs time (combobox, switch, segmented control) but avoids a stack migration.
3. **ipld-explorer-components** keeps its own look on `/explore`. Acceptable visual seam, or a wrapper-level re-skin; call out to the operator rather than patching the package.
4. **Accessibility versus brand.** The borderless rule must be narrowed for form fields (4.4 item 1); if the operator refuses the exception, 1.4.11 fails. This is the one place the brand and the task's AA bar collide.
5. **Cloud basic auth plus secrets.** If the settings API shares one basic-auth identity among operators, "write-only key" protects against reads through the API but not against another authorised operator replacing the key; say so in the panel ("Anyone who can open this page can change the backend").
6. **UAR dependency.** Agent creation and model listing depend on UAR conformance (`Prometheus-AGS/universal-agent-runtime#379`, all nine items missing or partial at `b4244aea`). The UAR half of the form can ship only against what UAR exposes at execute time; the UI must show "UAR does not offer X yet" honestly, not mock it.
7. **Delivery chain.** New Web UI build, new CID, `WebUIPath` bump, pin on LKE; a failed pin makes `/webui` unavailable on forks (the 2026-10-13 incident). Stage a verification gate: retrieve CID before the bump.
8. **Kubo rules**: the rebrand touches no `/api/v0`, gateway, CID recipe, or wire behaviour. Settings add no telemetry. The theme preference and settings are local only. The new persisted "backend chosen" state needs its off switch and disabled-state test per feature (task rule): settings-page-off, chat-off, WebMCP-off, confirm-off, each with a test.
9. **Translations**: dozens of `app.json` keys reference the old brand; leaving 32 other locales on old strings is acceptable only because of `en` fallback for **new** keys; changed keys in other locales keep showing "IPFS" product text until Transifex catches up. Mitigate by moving the product name out of locale strings into `brand.js`.
10. **Brand logo placement rule** ("wordmark and icon never side by side"): the collapsed rail and the page header must not show both; checked in review.

### 6.3 Open questions for the operator (real decisions only)

1. **Product name in chrome and titles.** Is the product named "Promethity" (title `Settings | Promethity`, wordmark in the rail) or something like "Promethity node console"? One word decides titles, manifest `name`, e2e assertions and the About text.
2. **Form-field edge exception.** Approve a 1px `text-muted` inset edge on form controls only (needed for WCAG 1.4.11; the brand's borderless inputs measure 1.30:1 against their surface)? If not, the AA bar and the brand cannot both hold.
3. **Attribution wording.** Is "Built on the IPFS Web UI (MIT, Protocol Labs)" enough, and do you want any non-affiliation statement? (The MIT notice itself is required regardless.)
4. **Desktop shell branding.** Goal 5 says the Web UI. Should the Desktop app name, tray icons, dock icon, installer and the `productName` "IPFS Desktop" also become Promethity in this phase, or later? (Separate change, large asset work; the native confirm windows are already branded.)
5. **Default theme.** Follow system with dark when unspecified (recommended), or force dark first and offer a toggle only?
6. **Same-origin settings and agent endpoints on the cloud node.** The UI needs the settings API and `/agui` reachable at the same basic-auth origin (`ipfs.prometheusags.ai`). Which path and who configures the proxy and `agent.origin_allowlist`? (Architect/devops; flagged here because the UI cannot be made to work around it.)

### 6.4 Proposed changes for the Plan stage (ids, owner, order)

Owner is `ui-ux-engineer` (UXE) unless noted. Order is dependency order; items with the same number can run in parallel after their predecessors. Verification, as the phase rules require, happens once per round at the phase boundary with an independent `reviewer`, not per edit.

| Order | Id | Change | Owner | Depends on |
|---|---|---|---|---|
| 0 | UI-00 | Extend `PRODUCT.md`/`DESIGN.md` scope to the fork Web UI and Desktop windows; record the form-edge exception, theme default, product name, attribution (decisions 6.3 q1-q5) | UXE (architect signs the doc ownership, since `DESIGN.md` sits at repo root) | operator answers |
| 0 | UI-01 | Settings API contract for the UI (shapes in 5.2; error codes; change events; same-origin route) | ipfs-agent-architect + mcp-server-engineer | goal 2/3 assessments |
| 1 | UI-10 | `src/promethity/` foundation in the fork: tokens.css (light/dark), fonts (bundled woff2 + OFL notices), base, theme-overrides remapping ipfs-css classes, `brand.js`, theme provider and toggle | UXE | UI-00 |
| 1 | UI-11 | Assets: favicon set, manifest (192/512/maskable, colours), `index.html` (two theme-colors, title, description), wordmark/p-icon logo components, remove IPFS logos from nav/Welcome | UXE | UI-00 |
| 2 | UI-20 | Shell redesign: `App.js` wrapper, `NavBar`, `Box`, `Title`, `button`, `Notify`, `Connected`, `Tour` skin; Lucide for nav and settings icons | UXE | UI-10, UI-11 |
| 2 | UI-21 | Data-viz re-skin: Chart.js/d3/map series from tokens, drop gradients | UXE | UI-10 |
| 3 | UI-30 | Primitives: Field, Switch, SegmentedControl, Combobox, StatusLine, ApplyBar, PolicyDecision row (plain CSS, accessible, unit + axe tests, both themes) | UXE | UI-10 |
| 3 | UI-31 | Titles and copy: `brand.js` title helper in 9 places, `en` locale rewrites (product name, banned words), e2e title assertions updated | UXE | UI-00, UI-10 |
| 4 | UI-40 | `#/settings/agent` page: provider, `SettingsClient` interface, HTTP client, backend forms (UAR, OpenAI-compatible), features switches, apply/feedback, first-run; remove the two old boxes; default-on migration of `agent-chat` and `webmcp` stores with off switches; disabled-state tests per feature | UXE | UI-30, UI-01 |
| 4 | UI-41 | Island: `theme` pass-through, "pick a model" empty state input, launcher restyle (rebuild `mcp/chat`, re-vendor into `public/agent-chat/`) | UXE | UI-10, UI-40 shape |
| 4 | UI-42 | Desktop: write bridge (`agentSettings.*`), safeStorage key path, default-on endpoint start, tray entry "Agent settings" + status line, hardening tests (`test/**`), `package.json` scripts | UXE (+ mcp-server-engineer for the local endpoint start) | UI-01 |
| 5 | UI-50 | Remaining routes polish (Files, Pins, Explore wrapper, Peers, Diagnostics, Welcome illustrations) against the override layer; fix stragglers found by a grep for raw hex, `b--`, shadows | UXE | UI-20 |
| 6 | UI-60 | Single verification round: unit, Jest, Playwright e2e, axe in light and dark for every route and settings state, keyboard script, reduced motion, 320/768/1024/1440 screenshots both themes, bundle size and no-remote-asset scan; one batched correction and one confirmation | UXE then independent `reviewer` and `security-reviewer` (separate contexts) | all above |
| 7 | UI-70 | Delivery: production build, new CID, `webui-cid-delivery` bump of `WebUIPath`, retrieval check, LKE re-pin, Desktop `copy-local-webui` rebuild; `docs/PROMETHITY-FORK.md` and progress-report update | UXE + mcp-server-engineer (Go bump) | UI-60 |

Parallelism: UI-10/UI-11 together; UI-20/UI-21/UI-30/UI-31 together; UI-40/UI-41/UI-42 together once the API contract (UI-01) is fixed.
