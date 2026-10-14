# Web UI redesign polish, 2026-10-14

Phase `agent-backend-settings`, round 4. Fork: `GQAdonis/ipfs-webui`, branch `feat/ipfs-mcp-server` (uncommitted working tree).

Captured with headless Chromium against the CRA dev server (port 3000) and a temporary Kubo node (`IPFS_PATH` from `mktemp -d`, swarm 4199, API 5199, gateway 8199, empty bootstrap list, two sample files added). Tours disabled, language `en`.

- `after/`: Status, Files, Explore, Peers, Settings, Welcome and Diagnostics, dark and light, 1440x900 and 375x812 (28 images, `<page>-<theme>-<width>.png`).
- `before/`: the same pages for Status, Peers and Settings before the fixes (12 images), for comparison.

What the captures show:

- Accent only on the active nav item, the primary action (Import on Files, Add connection on Peers), the active diagnostics tab and the focus ring. Headings, figures, chart titles and table headers are text tokens; in-text links are primary text with an underline.
- Top bar: Browse and Inspect are 44px control-fill buttons that never shrink; below 600px the labels hide and the icon buttons stay.
- Peers table, filter input and Settings inputs sit on surface, control and `text-muted` edge tokens in both themes.
- Explore: the package's inline white panel and codec badges are flattened to tokens by wrapper-level selectors (D9 seam; the package is untouched).

Not captured: modal states, hover states, a populated Files list or Peers table (the temporary node has no peers).
