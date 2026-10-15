# Round 7: Desktop Search window residuals (captures)

Date: 2026-10-16. Owner: `ui-ux-engineer`. Task `desktop-search-sidecar` 1.7 (review L-b, L-c, L-d, L-e).

Method as in `../2026-10-15-round6-fixes-desktop/README.md`: real Electron, the real `createSearchWindow`,
`search.html`, `search.css`, the shared `ui.css` and bundled fonts; a fake store and sidecar state feed the
real view builder; `IPFS_DESKTOP_DEV_ALLOW_CAPTURE=1` (unpackaged only). No Desktop start, daemon, sidecar or
network; ports 4001, 5001, 8080 and 5050 not touched. The error state is driven by a real click on Save with a
relative path in the `surreal` field.

| File | State | What it shows |
|---|---|---|
| `search-pending-dark-640.png`, `search-pending-light-640.png` | on, sidecar has not applied it | L-b: badge "Search is on and will start with the agent endpoint." in the `warn` tone (`badgeTone: warn`), not `err`. L-c: intro ends at "...on this computer." L-e: checked box in the `text` fill with a `ground` check mark; the only accent fill is Save (`accentFills: ["save"]`). |
| `search-on-dark-640.png`, `search-on-light-640.png` | on and applied | Badge `ok`; same checked box and single accent fill |
| `search-error-dark-420.png`, `search-error-light-420.png` | refused save at 420 px | L-d: the notice sits directly under the marked, focused `surreal` field (notice top 476 px, field bottom 462 px). The long "program is not private" reason stays in the `err` badge, as it is an endpoint reason. |
| `search-error-dark-640.png` | refused save at 640 px | same placement at the wide size |

Facts printed by the harness: `scrollWidth == innerWidth` in every run (no sideways scroll at 420 or 640);
text inputs report `outline: 1px solid` in `text-muted` (the D7 edge, now from the shared sheet).

Not captured: the pairing window (`pair.html`) whose password field gets the same D7 outline from `ui.css`
(checked by the stylesheet rule in `agent-confirm.spec.js`, evaluated by hand, see the report).
