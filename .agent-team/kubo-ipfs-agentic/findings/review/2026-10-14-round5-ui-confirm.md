# UI review confirmation: Round 5, phase `agent-backend-settings`

Reviewer: `reviewer` (Claude Opus, separate context, read-only). Date: 2026-10-14. One confirmation pass after the batched correction cycle. Written to disk by the lead from the returned report. No skill, shell, browser, Jest, axe or contrast tooling; no new screenshots exist (`mcp/server/evidence/2026-10-14-round5-ui-fixes/README.md:5-7`). Layout claims are "code-verified, render unverified". Lead's runs cited: jest 25 suites / 406 tests, island vendor check.

**Verdict: PASS WITH FINDINGS.** H1 and H2 fixed in code (H2 not seen rendered); M1-M9 resolved in code; L6 partial; M10 open (no captures). New: C1, C2 MEDIUM; C3, C4, C5 LOW. None structural.

## Status of earlier findings

| ID | Status | Evidence |
|---|---|---|
| H1 | CODE-VERIFIED (render unverified); see C2 | `AgentSettings.js:169`, `form.js:138,85,77,84`, `BackendSection.js:119`, `agentSettings.json:235`; test `AgentSettingsPage.test.js:411-428` |
| H2 | CODE-VERIFIED, render unverified | `AgentChatLauncher.js:118-131`, `launcher.css:7-15` (below 1280px), `primitives.css:434`, `use-scroll-padding.js:19`; jsdom test proves set/remove only (`agent-chat.test.js:326-340`). Unchecked: clearance at 375/320, the 1280 breakpoint, 320x568 viewport use. |
| M1 | RESOLVED | `BackendSection.js:82-86`, `agentSettings.json:59-71,41`, `ApplyStatus.js:47-49`, `use-chat-on.js:11-17`. Nit: "nothing chosen" still said three times. |
| M2 | RESOLVED in code; no-token render owed | `AgentSettingsPage.js:69-76` (test `:493-498`), `AgentSettings.js:198-205`, `agentSettings.json:25,29` |
| M3 | RESOLVED | `agentSettings.json:111-114,202,209`, `ApplyStatus.js:38`, `form.js:108` |
| M4 | RESOLVED | `BackendSection.js:109,31,33-41`; test `:288-300` |
| M5 | RESOLVED in code; tab capture owed | 7 files use `promethity/icons.js`; `PROMETHITY-SWEEP.md:24`; 0 glyph hits |
| M6 | RESOLVED in code; capture owed | `PeersTable.js:11,21`, `icons.js:79,170-171` |
| M7 | RESOLVED | `BackendSection.js:95-98` |
| M8 | RESOLVED; see C4 | `ApiKeyField.js:22-27,87-97`; test `:323-334` |
| M9 | RESOLVED | `BackendEmpty.tsx:21-39` |
| M10 | OPEN (deferred to operator) | README lists 7 captures owed |
| L1 | RESOLVED | `UarBackendForm.js:22` (nit: `aria-labelledby` cleaner) |
| L2 | RESOLVED | `pa3`; back link `AgentSettingsPage.js:56`; FYI `AgentSettingsSummary.js:31` still `pa2` |
| L3 | RESOLVED | `PeersTable.js:128-148` |
| L4 | RESOLVED | `File.js:110,128,135`, `files.json:163-167` |
| L5 | CODE-VERIFIED, render unverified | `is-not-connected.tsx:27-30`, `Connected.js:17-21` |
| L6 | PARTIALLY RESOLVED | island fixed (`Conversation.tsx:143`, `island.css:112`); Diagnostics play button still accent during the nudge (`log-viewer.css:153-155`, `log-viewer.tsx:90,106`) |
| L7 | RESOLVED | `FeaturesSection.js:66-70,116` |

## `key_sent` states and the typed-host gate

Works: "Reachable, key not sent" is its own warn state (`OpenAiBackendForm.js:60-67`; tests `:256-281`); absent `key_sent` read as not sent (`http-client.js:127`; `core.test.js:309-313`); nothing about keys when none is stored.

- **C1 MEDIUM: "confirmed" means two things.** The gate note says the key goes only to a confirmed host (`agentSettings.json:135`); the person types the host and sees "Matches", but the test request carries no acknowledgement (`AgentSettings.js:108`, `http-client.js:119`) and the server sends the key only to the current backend or a principal-acknowledged host (`settings_probe.go:144-158`), so the result says the host "is not confirmed" (`agentSettings.json:139`). The gate also applies with no key stored (`form.js:213`) and on every first run; the host field sits below the disabled Model field (`BackendSection.js:102-116`); step 2 copy omits the host (`agentSettings.json:66`). Fix: gate only when a key is stored; place the host field above Test; reword ("A test of a new host never carries the stored key; the key goes to it after you apply").
- **C3 LOW:** with the key withheld and `/models` refused, the page says "The endpoint hides its model list" (`settings_probe.go:126-128`, `http-client.js:130`, `agentSettings.json:129`). Say the list needs the key and to type a model name.
- **C5 LOW:** the disabled Test button's reason is not linked (`OpenAiBackendForm.js:55,58`). Use `aria-describedby` or `aria-disabled`.

## Regressions and new findings

- **C2 MEDIUM:** after "Reload settings" following a conflict, the status keeps "your edits are kept … Retry" and a disabled "Retry" (`form.js:70-87` never clears `apply` when edits are discarded; `ApplyStatus.js:32-41`; `BackendSection.js:122`; test `AgentSettingsPage.test.js:430-437` checks only the model). Fix: in `loaded`, when `!keep`, `apply = settle(state.apply)`; assert "No changes" and "Apply".
- **C4 LOW:** Cancel in the remove-key confirm drops focus to the body (`ApiKeyField.js:85,93`). Return focus to "Remove key".
- **FYI:** the sweep excludes `src/settings/agent/**` and `src/agent-chat/**` (`PROMETHITY-SWEEP.md:8,33`).
- **FYI:** `src/status/PeerBandwidthTable.js` loads an image from `icon.now.sh` (security, hardcoded-endpoint rule).

Checked and clean: no `localStorage` admin token; key input never re-rendered (test `:304-321`); island link contrast intact (`island.css:136-146`); reduced-motion gates on the log toolbar; every status has a word.

## Still needs a browser check

1. H2 at 375 and 320, both themes, chat on and island closed; also 1024 and 1280-1440; 320x568.
2. H1 real 412 through Retry to Applied, and the Reload branch (C2).
3. M2 no-token first visit.
4. M10 `03` at 375 scrolled to the test result and combobox.
5. M5 Diagnostics DHT provide tab.
6. M6 Peers with a local-network peer.
7. Security H1: Test held back for a new host, then "Reachable, key not sent" (judge C1 in place).
8. L5 and L6: Welcome not connected; Diagnostics logs first view.
