# Round 6 evidence: agent settings residuals and default-on (ui-ux-engineer, builder B)

Run date 2026-10-15. Changes: `webui-agent-settings-page` 1.8 and 1.9, `agent-features-default-on` 1.1 (Web UI half). Web UI branch `feat/ipfs-mcp-server` working tree (uncommitted), `ipfs mcp` from the `make build` binary `cmd/ipfs/ipfs` (copied to a temp dir so another builder's rebuild could not replace it).

## Setup (non-default ports, `mktemp -d` dirs, fake keys)

| Process | Address |
|---|---|
| temporary Kubo node A (`test` profile, bootstrap empty, `Routing.Type none`, MDNS off) | swarm 4299, API 5299, gateway 8299 |
| temporary Kubo node B (same profile), connected to A with `swarm connect` for the local-network peer | swarm 4298, API 5298, gateway 8298 |
| `ipfs mcp --http=127.0.0.1:5287 --agui --mcp-config` (`admin` and `ui` principals from env, settings dir in the temp dir) | 127.0.0.1:5287 |
| fake OpenAI-compatible model server (`scripts/fake-llm.mjs`) | 127.0.0.1:5286 |
| Web UI dev server | 3299 |
| one-origin proxy (`scripts/proxy.mjs`) | localhost:3297 |
| headless Chrome 155 driven by `scripts/cdp.mjs` (raw CDP) on debug port 9335 | 9335 |

Chrome debug port: the round 5 scripts used 9333, which another builder's capture run held this round, so these scripts use 9335. The admin token is stored under the session key `ipfs-webui:agent-settings:admin-token:v2` as `{token, root}` (`scripts/common.mjs`). No blank prime page exists in this tree: `prime()` loads the app, writes storage, and reloads.

Every shot name ends `-<width>-<theme>`. Shots were looked at before being cited here.

## Owed items from the round 5 confirmation ("Still needs a browser check")

| # | Item | Shots | What they show |
|---|---|---|---|
| 1 | H2 launcher clearance, 375 and 320, both themes, chat on, island closed; 1024; 1280 to 1440; 320x568 | `b-h2-375-dark/light`, `b-h2-320-dark/light` (320x568), `b-h2-1024-dark`, `b-h2-1279-dark`, `b-h2-1280-dark`, `b-h2-1366-dark/light`, `b-h2-1405-dark`, `b-h2-1440-dark/light`, `b-h2-section-end-320-dark` | The sticky Apply bar stops above the launcher at every width. **A defect was found and fixed**: with the old 1280px breakpoint the bar and launcher overlapped at 1280 (bar right edge 1194 against launcher left edge 1131, same rows); the overlap persists up to about 1405px because the 960px column is centred. `launcher.css` now lifts the bar at every width. The geometry is printed in the run log: overlap false at all 12 views after the fix. |
| 2 | H1 real 412 through Retry to Applied, and the Reload branch (C2) | `d1-conflict-1440-dark`, `d1-conflict-375-light` (412: "Not applied ... Retry ... Reload settings"), `d2-retry-applied-1440-dark`, `d2-retry-applied-375-light` (Retry carries the new revision: "Applied"), `d3-after-reload-1440-dark`, `d3-after-reload-375-light` (after Reload settings: "No changes", Apply present and off, no Retry, no conflict text) | The conflict is real: a second client changes the model with `PUT` between load and Apply. Run log: Reload branch printed `No changes true`, conflict text gone, Retry gone, Apply `present, disabled=true`. These were taken before the 1.9 copy edit, so the key help line is the older sentence. |
| 3 | M2 no-token first visit | `c-m2-no-token-1440-dark`, `c-m2-no-token-375-light` | "Needs a connection" and the Connection section; no admin token set. |
| 4 | M10 `03` at 375 scrolled to the test result and combobox | `e5-key-not-sent-375-dark/light` (test result), `e6-tested-combobox-375-dark/light` (combobox open with four models) | At 375 the result and the list are not hidden by the Apply bar or the launcher. |
| 5 | M5 Diagnostics DHT provide tab | `f2-diagnostics-dht-provide-1440-dark/light`, `-375-dark/light` | Tab icons and tabs render; the node has no sweep provider, so the screen says so. |
| 6 | M6 Peers with a local-network peer | `f1-peers-local-1440-dark` (the row), `f1-peers-local-375-light` (peer count only) | One peer on `127.0.0.1` shows "Local network" with the network icon at 1440. At 375 the map and the count fill the first screen and the row is below the fold, so the row itself is seen only at 1440. |
| 7 | Security H1: Test held back for a new host, then "Reachable, key not sent" (judge C1 in place) | `e4-test-held-back-1440-dark/light`, `-375-dark/light`; `e5-key-not-sent-1440-dark/light`, `-375-dark/light` | A key is stored and the Base URL moves to `localhost:5286`. The host field sits above Test; Test is off and the reason is linked (`aria-describedby`) and says the stored key never goes on a test and does not follow a move. After the host is typed, Test runs and the result is its own warn state "Reachable, key not sent". Run log: `gated: Test present, disabled=true`, `order: host field above Test? true`, `typed: Test ... disabled=false`. |
| 8 | L5 and L6: Welcome not connected; Diagnostics logs first view | `f4-welcome-not-connected-1440-dark/light`, `-375-dark/light`; `f3-diagnostics-logs-first-view-1440-dark/light`, `-375-dark/light` | L5: the not-connected panel. L6: at 1440 the play button is in the text colour (computed `rgb(20, 18, 16)` equals `--text`, not `--accent-text`) and the accent stays with Apply. At 375 the log toolbar is below the fold behind the launcher, so the button is not visible in `f3-...-375-*`. |

## Other residuals

| Item | Shots | What they show |
|---|---|---|
| C1 no key stored: Test not gated; Apply still asks for the host | `e7-no-key-test-open-1440-dark/light`, `e8-no-key-test-result-1440-dark` | After "Removed", a new host leaves Test on; the note is the plain "The test asks the node to reach the URL."; "Reachable ... answered in 7 ms" with no key wording; Apply is off with "Type the host above to confirm the change." |
| C4 focus after cancelling Remove key | `e2-remove-confirm-1440-dark`, `e3-cancel-focus-1440-dark/light` | Focus rests on Cancel, then returns to "Remove key" (focus ring visible). Run log: `after Cancel focus on: Remove key`. |
| `agent-features-default-on` 1.1 | `a1-default-on-features-1440-dark/light`, `-375-dark` | A profile that stored only an endpoint boots with chat on and WebMCP read tools on, writes and DNSLink off; the launcher shows. Run log: stored value after boot `{"endpoint":...,"enabled":true}`. |

## Not captured

- The ADR-0012 `key_not_carried` state ("The key for the old host is not sent to the new host"): covered by Jest only (`AgentSettingsPage.test.js`, `core.test.js`); this node build was not driven through a host move that returns the field.
- C3 ("The model list needs the stored key"): needs an endpoint that refuses `/models` without a key; the fake server lists models freely. Jest only.
- Diagnostics logs at 375: the log-level form fills the first screen and the toolbar icons sit under the launcher at scroll 0 (`f3-diagnostics-logs-first-view-375-light`). Seen, not fixed.
- Diagnostics logs footer: at 1440x900 the launcher covers the right end of the "Streaming stopped" status text (`f3-diagnostics-logs-first-view-1440-dark`). Seen, not fixed (outside this round's files); route to whoever owns `src/diagnostics/**`.

## Teardown

Every process was stopped by PID: Kubo A (timeout 5667), Kubo B (9075), `ipfs mcp` (14100), fake model (10318), proxy (30173), dev server (npm 17906, node 18127 and 18147). Afterwards ports 3299, 3297, 5286, 5287, 5299, 4299, 8299, 5298, 4298, 8298 and 9335 had no listener. 4001, 5001, 8080 and 5050 were never touched.
