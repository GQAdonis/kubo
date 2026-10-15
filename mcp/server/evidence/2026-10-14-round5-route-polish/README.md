# Web UI route polish and Round 4 confirm residuals, 2026-10-14

Phase `agent-backend-settings`, Round 5, builder A (`ui-ux-engineer`). Fork `GQAdonis/ipfs-webui`, uncommitted working tree. Changes: `webui-shell-redesign` 1.4 and `webui-route-polish` 1.1-1.4.

Captured with headless Google Chrome driven over the DevTools protocol (no Playwright; `scripts/capture.mjs` and `scripts/cdp.mjs`), against the CRA dev server (port 3199, `ESLINT_NO_DEV_ERRORS=true` so builder B's in-progress lint errors did not cover the page) and a temporary Kubo node (`IPFS_PATH` from `mktemp -d`; swarm 4199, API 5199, gateway 8199; empty bootstrap list, MDNS off). The node holds 9 MFS entries (two folders, seven files of different types: text, csv, binary, mp4, mp3, png, md) and one pin. A second temporary node (4198, 5198, 8198) was connected for the populated Peers captures, then shut down through its API for the empty-state captures. Tours off, language `en`, fresh browser context per shot (so no locale cache). The WebMCP dialog was produced the way the Round 4 captures were: headless Chrome has no WebMCP, so the page gets a stub `document.modelContext`, the real app registers its tools, the real "Also offer tools that change your node" switch (now on `#/settings/agent`) is turned on, and the real `ipfs_add` tool is called. The dialog closed on Escape in every run. Dev server, both nodes and every Chrome were stopped by PID; ports 3199, 4199, 5199, 8199, 4198, 5198, 8198 and 9199 confirmed free. Nothing was bound on 4001, 5001, 8080 or 5050.

## Mapping

| Finding or task | Screenshots and measurements |
|---|---|
| N1 WebMCP dialog at 375 and 320 | `webmcp-dialog-{dark,light}-{375,320}.png` (armed), `webmcp-dialog-unarmed-*.png`, `webmcp-dialog-{dark,light}-1440.png`. `capture.log` `webmcp-metrics` lines: at scrollTop 0 the three notes (deadline, "Deny is final", "not a lock") end at y=363 (375) and y=395 (320) while the sticky decision row starts at y=595 and y=431, so all three are in view at first paint. Deny and Approve share one row, "Deny all" has its own row, the hint is left-aligned, the footer is on `surface`. |
| N2 modal scrim, no shadow | `modal-new-folder-{dark,light}-{1440,375,320}.png`: the page behind is veiled by the token scrim in both themes and the sheet is a `raised` fill with no shadow (`prom-scrim`, `prom-modal`; `modal.test.tsx` updated). |
| N6 modal buttons at 375 and 320 | `modal-new-folder-*-375.png`, `-320.png`; `capture.log` `modal-metrics`: buttons span x=40..351 (375) and 40..296 (320) inside a sheet at 24..351 and 24..296, `scrollWidth` equals the viewport. They stack, Create above Cancel. |
| N9 modal close is a button | Close glyph in every modal capture (44px button; `modal.test.tsx` asserts the button, its `type` and its `aria-label`). |
| N3 Peers count, label, placeholder, empty sentence | Populated: `peers-{dark,light}-{1440,375,320}.png` ("1 peer" beside a labelled, translated field). Empty: `peers-empty-{dark,light}-{1440,375}.png` ("No peers yet. Peers you connect to will appear here."). |
| N4 Files header at 1440 | `files-{dark,light}-1440.png`: totals and Import on one row. `files-*-375.png`, `files-*-320.png`: wrap at 30em and below, breadcrumb row and sort controls on two rows at 320. |
| N5 Files row rules | `files-*-1440.png` (rows separated by spacing), `files-selected-{dark,light}-1440.png` (selected row is a `raised` chip with a gap beneath). No rule remains, so no craft exception was needed. |
| M11 remaining upstream glyphs | Lucide glyphs in `files-context-menu-{dark,light}-1440.png` (menu icons), `files-selected-*` (selection bar), file-type icons in `files-*`, the sort chevron and search icon in the Files header, modal icons. |
| L1 Title Case strings, locale cache | `diagnostics-{dark,light}-{1440,375,320}.png` from a fresh context: "Retrieval check", "Streaming stopped", "Total entries" and the rest are sentence case. |
| N7, N8 Diagnostics tab and "Streaming stopped" | `diagnostics-*`: the active tab is `aria-current="page"` (code); "Streaming stopped" is muted text in the footer line. |
| webui-route-polish 1.1 Files, Pins, Peers, Diagnostics | `files-*`, `pins-*`, `peers-*`, `diagnostics-*` in both themes at 1440, 375 and 320. Pins has nothing to list on this node (no pinning service), so it shows its empty sentence only. |
| webui-route-polish 1.2 Welcome | `welcome-{dark,light}-{1440,375,320}.png` (connected: state icon on its tint beside the heading), `welcome-notconnected-{dark,light}-{1440,375}.png` (unreachable API). |
| webui-route-polish 1.3 Explore | `explore-{dark,light}-{1440,375,320}.png` (Start exploring page: panels and codec badges through the `--explore-*` variables). `explore-cid-*` shows the explorer's own loading state: its Helia node did not finish loading a CID within 20 s in this offline environment, so the loaded object view was not captured. |
| webui-route-polish 1.4 sweep | `docs/PROMETHITY-SWEEP.md` in the fork (commands, results, allowlist with a reason per entry). |

## Looked at in this run

Opened and checked: `peers-dark-1440`, `peers-light-1440`, `peers-light-375`, `peers-empty-dark-375`, `files-dark-1440` (an earlier pass of the same page), `files-light-1440`, `files-light-375`, `files-dark-320`, `files-light-320`, `files-context-menu-dark-1440`, `files-selected-light-1440`, `modal-new-folder-dark-375`, `modal-new-folder-light-375`, `modal-new-folder-light-1440` (earlier pass), `webmcp-dialog-dark-320`, `webmcp-dialog-light-375`, `webmcp-dialog-light-1440`, `welcome-dark-1440`, `welcome-notconnected-light-375`, `explore-light-1440` (earlier pass), `explore-cid-light-1440`, `pins-dark-375` (earlier pass), `diagnostics-light-375`, `diagnostics-dark-1440`. The remaining captures come from the same script and were not individually opened. The late fixes (Files header rows at 320, the Peers empty sentence, `overflow: hidden` on the modal sheet) are covered by the final `files-*`, `peers-empty-*` and `modal-*` captures.

## Not captured

Hover states; reduced-motion (code path only: `diagnostics-tab`, `peersTableItem`, the Files skeleton); Windows forced-colors; the loaded Explore object view (see above); Pins with a populated list; Settings IPNS and Pinning tables at 375 (N10, not attempted).
