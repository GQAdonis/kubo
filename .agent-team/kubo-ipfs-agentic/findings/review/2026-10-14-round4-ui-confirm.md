# UI review confirmation: Promethity redesign (Web UI fork) and Desktop display branding

Phase `agent-backend-settings`, Round 4 confirmation pass after the one batched correction cycle. Reviewer: `reviewer` (Claude Opus, separate context, read-only). Date 2026-10-14. Written to disk by the lead from the returned report.

Method: `prometheus-ui-review` not loadable; reviewed against kubo `DESIGN.md` (`:20` fills not borders, `:41` no borders/rules/shadows, `:42` D7), `ipfs-webui/docs/PROMETHITY-FORK.md`, `src/promethity/**`, earlier `2026-10-14-round4-ui.md`. Opened 27 of 56 screenshots in `mcp/server/evidence/2026-10-14-webui-review-fixes/`; read the uncommitted code. Not run: builds, tests, Playwright, axe, contrast tooling.

**Verdict: PASS WITH FINDINGS.** C1 and H1-H5 resolved; the Round 4 UI FAIL is lifted. One new HIGH (N1) on the write-confirmation surface; N2-N6 MEDIUM.

## Status of earlier findings

| ID | Status | Evidence |
|---|---|---|
| C1 | RESOLVED (new defect at 375: N1) | `webmcp.css:16-30`; `webmcp-dialog-*` captures |
| H1 | RESOLVED | `theme-overrides.css:40-45`; `status-*-1440`, `files-dark-1440`, `explore-dark-1440` |
| H2 | RESOLVED (regression at 1440: N4) | `components.css:154-176`; `files-dark-320.png` |
| H3 | RESOLVED | `Checkbox.css:4-7`, `radio.css:4-7` |
| H4 | RESOLVED | `ThemeSettings.js`; `ThemeSettings.test.js:36,44`; e2e not added (CDP reload capture stands in) |
| H5 | RESOLVED | `Connected.js:20-33`; focus captures |
| M1 | RESOLVED | `button.tsx:16-21`; helper text visible |
| M2 | RESOLVED in code, partly verified | `PeersTable.js:195`, `components.css:180-186`; no populated rows |
| M3 | RESOLVED | `NavBar.css:89-100` |
| M4 | RESOLVED | sentence-case titles in captures |
| M5 | RESOLVED for Tachyons; incomplete (N2, N5) | `theme-overrides.css:299-310` |
| M6 | RESOLVED in code (not captured) | `theme-overrides.css:138` |
| M7 | RESOLVED | `index.css:23-27` |
| M8 | RESOLVED in code (not captured) | `LoadingAnimation.css:13-41` |
| M9 | RESOLVED | `App.js:85`; `theme-overrides.css:306-310,373-380` |
| M10 | RESOLVED | `PROMETHITY-FORK.md:30` |
| M11 | PARTIALLY RESOLVED | still upstream: `FilesPage.js:18` `GlyphSearch`, `File.js:10` `GlyphDots`, `modal.tsx:2,79-82` `GlyphSmallCancel`, modal header icons (`NewFolderModal.js:4` etc.), `ContextMenu.js:6-15`, pinning/IPNS managers |
| L1 | PARTIALLY RESOLVED | Title Case remains: `diagnostics.json:12` "Streaming Stopped", `:31-33,38` "Total Entries", "Estimated Size", "Memory Buffer"; `app.json:62` "Retrieval Check Service URL". `diagnostics.json:5` is sentence case but captures show "Retrieval Check" (stale locale cache; recapture) |
| L2 | ACCEPTED | protocol-name use |
| L3 | RESOLVED | `FileInput.js:15-16` |
| L4 | PARTIALLY RESOLVED | neutral "Out"; duplicate y ticks remain (upstream rounding, reads as a bug) |
| L5 | RESOLVED | ~44 px root chip; 28 px footer pitch |
| L6 | RESOLVED | `log-viewer.tsx:95-124` |
| L7 | RESOLVED (honest) | `display-branding.spec.js:31-57` |
| L8 | DEFERRED | no new Desktop captures |
| L9, L10 | ACCEPTED | |

## New defects

### HIGH

- **N1. WebMCP dialog at 375: sticky decision row hides the countdown unit and both safety disclosures.** In `webmcp-dialog-*-375` the body ends "denied after 30"; "seconds.", "Deny is final. Nothing is sent to your node." and "This check catches mistakes. An agent that controls this browser can answer it too." sit under the sticky footer (`webmcp.css:166-180`, ~250 px: three stacked full-width buttons plus a two-line right-aligned hint `WebMcpConfirmDialog.js:291`; body `:261-264`). Footer fill equals the sheet fill, so nothing signals more content. Approve arms after 1 s. Fix: at ≤30em Deny and Approve side by side, "Deny all and turn off agent writes" on its own row or as a link; left-align the hint on one line; move the expiry line into the footer above the buttons; give the footer a `--surface` fill step; recapture at 375 and 320 in both themes; assert the expiry text is visible at scrollTop 0.

### MEDIUM

- **N2. Shared modals: no visible scrim, shadow kept.** `overlay.tsx:19` `bg-black o-50` maps to `--ground` (no dimming in light, barely visible in dark); `modal.tsx:73` `shadow-4`. Fix: token scrim like `.webmcp-backdrop`; drop `shadow-4`; update `modal.test.tsx:17,26,247`.
- **N3. Stray unlabelled "0" on Peers.** `PeersTable.js:139` absolute count with no positioned ancestor (`:128`, `:193`). Fix: `relative` filter row and a labelled count. Also hard-coded `'Filter peers'` placeholder without a label (`:133`); empty table has no "No peers yet" sentence (`:145`).
- **N4. Files header wraps at 1440.** `components.css:154-157` `flex-wrap` at all widths. Fix: scope to `max-width: 30em` or `flex: none` at `-l`.
- **N5. Inline row rules in the Files list.** `File.js:76,97` `borderTop: 1px solid var(--control)`. Record a craft exception for tabular data or separate by fill/spacing.
- **N6. New folder modal overflows at 375.** `modal.tsx:73` `w-80`, buttons `minWidth` 140 (`button.tsx:28`). Fix: `flex: 1; min-width: 0` or stack at ≤30em.

### LOW

- **N7.** Diagnostics active tab has no `aria-current="page"`; inline transition not gated by reduced motion (`diagnostics-content.tsx:36-55`).
- **N8.** "Streaming Stopped" in error red; use `text-muted`, sentence case.
- **N9.** Modal close is a clickable `svg` (`modal.tsx:79-82`), keyboard-unreachable.
- **N10.** Settings IPNS and Pinning tables truncate headers at 375.
- **N11.** Evidence gaps: WebMCP Settings at 375 clipped by the 4000 px capture; no populated Peers; no reduced-motion, hover, forced-colors or Files empty-state captures.

Checked and clean: focus rings (2 px accent) on URL field, help, connection, nav; global border removal keeps D7 form edges and the segmented toggle edge; wrapped mobile top bar legible; new strings sentence case; no avoid-list words found.

## Finished product?

Close, not yet. At 1440 Status, Settings, Explore, Welcome and Diagnostics read as Promethity in both themes, and the WebMCP dialog is a credible on-brand trust surface. Not finished at phone width (N1, N6, N3), in upstream-owned shared pieces (N2, M11), and in the Files header at desktop (N4). None is structural.

## Re-capture required

`webmcp-dialog-*-{375,320}` after N1; `modal-new-folder-*` light 1440 and dark 375 after N2/N6; `files-*-1440` after N4; Peers with a connected peer at 375 and 1440; Diagnostics with a cleared locale cache; one reduced-motion capture of the Files skeleton.
