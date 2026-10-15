# Round 6 correction cycle: Web UI captures (ui-ux-engineer)

Captured 2026-10-15 from the `ipfs-webui` dev server on 127.0.0.1:3199 against `test/e2e/stub-ipfs-mcp.js --port 3198 --rpc-shim`, driven by `capture.mjs` (playwright-core already in `node_modules`, system Chrome, headless, reduced motion). The shell's RPC address is set to the stub through localStorage before the app starts, so no Kubo node and none of ports 4001, 5001, 8080, 5050 is touched. For the Diagnostics logs screen the script answers `id`, `version`, `log/level`, `log/ls` and `log/tail` itself (the stub reports no Kubo version, so the screen would show "Unsupported Kubo version"); the tail ends after three lines, which is the "Streaming stopped" state. Both processes were stopped by PID afterwards. `metrics.json` holds the measurements quoted below.

Re-run (from the ipfs-webui checkout, with both servers up): `node <this dir>/capture.mjs <out dir>` (`ONLY=search` or `ONLY=logs` for half).

## Looked at (cited)

- `search-focus-last-chip-light-320.png`, `search-focus-last-chip-dark-375.png`, `search-focus-last-chip-dark-1440.png`: keyboard focus (Tab from the search box) on the last chip, "AI description". The focused chip sits above the launcher at every width; measured with the browser's own focus scrolling, chip bottom at or above launcher top in all six combinations (`metrics.json`, `clearOfLauncher: true`). The other widths and the other theme were measured the same way and written, not each opened.
- `settings-search-switch-light-1440.png`, `settings-search-switch-dark-375.png`: the "Search your files" switch in Features, directly under "Enable agent chat", on by default. At 375 the Agent chat button visible over the right of the warning row is the fixed launcher in an element capture, not page content.
- `status-403-light-1440.png`: a 403 on the status probe. The panel says "You are signed in, but this identity does not have search access", with no "enter the token again" and no Agent settings link; the main column says the same, so the two no longer disagree.
- `diagnostics-logs-light-1440.png`, `diagnostics-logs-dark-1440.png`: the viewer toolbar and the "Streaming stopped" footer end above the launcher (footer bottom 813 px, launcher top 842 px).
- `diagnostics-logs-light-375.png`: first view at 375 (top of the page).
- `diagnostics-logs-end-dark-375.png`: scrolled to the end at 375. Footer bottom 612 px, launcher top 642 px: the footer is reachable above the launcher (`footerClearOfLauncher: true`). Before the fix the footer ended at 698 px of a 700 px viewport, under the launcher.

Not opened: the other 320/375 Search captures, the light-theme 375 settings capture, `settings-search-switch-off-*`, `status-403-*-375`, the other Diagnostics captures. `metrics.json` records the switch going off in the page: `checked` true to false and the Search nav entry count 1 to 0.

## Observed and not changed

- At 320 and 375 the idle "Ask in your own words" card is partly under the launcher in a viewport-anchored capture. The launcher is a fixed overlay; the content is no longer trapped under it (the main column ends with the clearance, `html` scroll padding keeps a focused control clear), but a static viewport still shows it overlapping text that scrolls away.
- The stub reports `AgentVersion` `kubo/stub`; the Diagnostics logs screen is shown only through the intercepted calls above, not a real Kubo.
