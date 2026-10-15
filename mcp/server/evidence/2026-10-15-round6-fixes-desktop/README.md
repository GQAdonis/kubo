# Round 6 fixes: Desktop Search window (captures)

Date: 2026-10-15. Owner: `ui-ux-engineer`. Findings: UI review H2, M4, M6, L4, L6, L7, L8; Desktop review L3; security L2, L4, I1.

## How these were captured

Real Electron (44.3.0, the `ipfs-desktop` dev dependency), not a browser stand-in. A small harness in the
scratchpad (`search-shot/main.cjs`, not committed) starts Electron with a throwaway `userData` folder
and opens the real `createSearchWindow` from `src/agent/search/window.js`: the real preload, the
window CSP, the in-memory partition, `search.html`, `search.css`, the shared `ui.css` and the bundled
fonts. The service is the real `createSearchService` and `buildSearchView` fed a fake store and a fake
sidecar state, so the text comes from `assets/locales/en.json` through the real code path. No Desktop
app start, no daemon, no sidecar, no network: ports 4001, 5001, 8080 and 5050 were never touched.

The window sets content protection, which blocks capture. These captures use the new development-only
switch: `IPFS_DESKTOP_DEV_ALLOW_CAPTURE=1`, honoured only when `app.isPackaged` is false
(`captureAllowed` in `src/agent/search/window.js`). The window logged
`[agent-search] screen capture of the Search window is allowed (development build)` on every run.
The harness widened the window past the screen (`enableLargerThanScreen`) so the whole page is in one
image; it changes nothing the page itself does. The theme was switched with `nativeTheme.themeSource`.

Two flows were driven through the real input pipeline, not by setting values: typing into the address
field (`webContents.insertText`, trusted events) and a real mouse click on Save.

## Files

| File | State |
|---|---|
| `search-off-dark-640.png`, `search-off-light-640.png`, `search-off-dark-420.png`, `search-off-light-420.png` | Off on a fresh profile (H2: no dimming; D7 edges; the off hint in words) |
| `search-on-dark-640.png`, `search-on-light-640.png`, `search-on-dark-420.png`, `search-on-light-420.png` | On, filled, status "Search is on." |
| `search-remote-dark-640.png`, `search-remote-light-420.png` | Remote describer saved: "File bytes are sent to vision.example.com." and the video note |
| `search-typing-dark-640.png` | M6: the same warning after typing the address, before any save |
| `search-error-dark-640.png`, `search-error-light-420.png`, `search-error-dark-420.png` | Error notice for a refused save, the field marked and focused (L7), and the long "program is not private" status in the badge (L4) |

The tray entries (L6) are text in the OS menu bar and were not captured; they are covered by
`test/unit/agent-search.spec.js` ("L6").

## What each capture shows (all opened and read)

- Every text input has a 1px inset edge in `text-muted` (D7) in both themes, and the checkbox is drawn
  with the same edge; the checked state is the one accent fill with an `on-accent` check mark. No
  field or label is dimmed in the off state.
- Off state: "Search is off. These settings are saved but not used until you turn it on." sits under
  the toggle, in words, at full contrast.
- Legends sit inside their panels (the first capture of this run showed them straddling the panel
  edge; fixed in `search.css`, the captures here are of the fixed window).
- Warnings carry the `!` marker and their words, warn text on warn tint.
- Error: `aria-invalid` ring (2px `err`) on the field, focus on it (`active: surreal` in the page
  facts), the notice in words below. At 420 px the page does not scroll sideways
  (`scrollWidth == innerWidth` in every run).

Page facts printed by the harness for each run (`off-hint` visibility, `aria-invalid` ids, focused id,
scroll width, notice and badge text, visible warnings) are in the report that accompanied this
directory.

## Not captured

- Real Touch ID and the native confirmation dialog (macOS owns that surface; the dialog strings,
  parent window and purpose are asserted in `test/unit/agent-native-dialog.spec.js` and
  `test/unit/agent-search.spec.js`).
- The Search window inside a running IPFS Desktop (that needs the daemon on its default ports).
