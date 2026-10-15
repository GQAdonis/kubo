# Round 5 correction: UI fixes (evidence)

Date 2026-10-14. Builder: ui-ux-engineer.

## Status of the screenshots: NOT CAPTURED

The re-capture could not run. The shell permission system denied two commands that start the capture rig: first the combined setup (create temporary repos for two Kubo nodes, start both daemons), then the narrower one (`ipfs init` into a temporary directory). No denial reason was given. Per the standing rule, the denial was not worked around and nothing else was started: no Kubo node, no `ipfs mcp`, no dev server, no proxy, no Chrome. Ports 3299, 3297, 5286, 5287, 5299, 4299, 8299, 5298, 4298, 8298 and 9333 were never bound by this run (nothing to stop). `make build` did run (exit 0, `cmd/ipfs/ipfs`, 15:31) and the server in it already reports `key_sent`.

Every UI state is covered by Jest in the meantime (listed in the report). The captures below are still required and the scripts to take them are staged in `scripts/` (`cdp.mjs`, `fake-llm.mjs`, `proxy.mjs`, copied from `../2026-10-14-round5-agent-settings/scripts/`). Note for the next run: the admin token session key is now `ipfs-webui:agent-settings:admin-token:v2` and holds `{"token": ..., "root": "http://localhost:3297"}`; the v1 bare-token key is deleted on load and never read.

| Finding | Capture still owed |
|---|---|
| H2 | `/settings/agent`, chat on, island closed, 375 and 320, both themes: the sticky Apply bar sits above the launcher (`--ac-bar-lift`) |
| M2 | no-token first visit: Connection first under the header |
| H1 | 412 recovery to a successful apply (Reload settings, Retry) |
| M10 | `03` at 375 scrolled to show the test result and the combobox |
| M5 | Diagnostics DHT provide tab (Lucide icons in the cards) |
| M6 | Peers with a local-network peer (second node on 4298/5298/8298) |
| security H1 | Test connection held back for another host; "Reachable, key not sent" |

## Sweep (M5), run

`sweep-output.txt` is the output of the commands in `ipfs-webui/docs/PROMETHITY-SWEEP.md` after the glyph pattern was widened to the relative barrel and the emoji scan added. Counts: hex 7, `b--` 16, shadows 19, row rules 0, glyph imports 0, emoji 1 (a code comment, allowlisted). All remaining hits are on the allowlist.
