# Round 6: Web UI Search page captures (webui-vector-search)

Captured 2026-10-15 from the `ipfs-webui` dev server on port 3199 against the stub `ipfs mcp` (`test/e2e/stub-ipfs-mcp.js --port 3198 --rpc-shim`), driven through the Chrome DevTools Protocol (system Chrome, headless; no Playwright). No Kubo node, no ports 4001, 5001, 8080 or 5050. The `--rpc-shim` flag only answers `id`, `version` and a few empty RPC calls so the shell treats the node as connected.

File names are `<state>-<theme>-<width>.png`, themes `dark` and `light`, widths 1440, 375 and 320 (tall captures are cut at 3200 px). Every capture reported no horizontal overflow.

States: `empty`, `results` (all five provenance labels), `filters` (Images and Text in an image pressed), `no-results`, `error-egress`, `error-unavailable`, `error-401`, `error-403`, `error-429`, `status-running`, `status-paused`, `status-failed`, `status-completed`, `status-store-down`, `status-not-configured`, `status-denied`, `gate-no-token`, `disabled` (search.enabled off: the page redirects to Status and the nav entry is absent).

## Looked at (cited)

- `results-dark-1440.png`, `results-light-1440.png`: five hits, kind and provenance badges ("From the file", "AI description by llava-7b" on the info tint, "Text in image", "Transcript"), the `<img onerror>` passage shown literally, CID with copy, Open in Files and Explore, the status panel beside the results.
- `filters-light-375.png`: pressed chips carry a check mark and the accent tint; one result after the Images + OCR filter; status panel under the results.
- `error-egress-dark-1440.png`: hosts named as text, the fix sentence and the setup link.
- `error-401-dark-320.png`: "Sign in again" with the link to Agent settings.
- `status-failed-light-375.png`: failed run, last error as text, skip reasons, "ask the agent" line, no run control.
- `status-not-configured-light-1440.png`: nav entry absent, one explanation with the docs link, short panel line.
- `gate-no-token-dark-375.png`: not connected, nothing requested.
- `disabled-dark-1440.png`: redirected to Status, no Search entry.

The other files were produced by the same run and were not individually reviewed.

## Observed and not changed

- At 320 and 375 the fixed "Agent chat" launcher (not part of this change) overlaps the filter help text in the viewport-anchored captures.
- The shell shows a page only when the Kubo RPC is connected, so Search needs the same connection as every other route.
