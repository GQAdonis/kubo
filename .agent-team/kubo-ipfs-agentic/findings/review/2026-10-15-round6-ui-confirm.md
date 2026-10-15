# UI review confirmation: Round 6, phase `agent-backend-settings`

Reviewer: `reviewer` (Claude Opus, separate context, read-only). Date: 2026-10-15. `prometheus-ui-review` loaded from its SKILL.md. Not run: shell, Jest, Playwright, axe, lint, real Electron; contrast from token hex values. Lead-reported: Web UI Jest 509 pass, Desktop specs pass. Written to disk by the lead.

**Verdict: FAIL.** One new HIGH (N1). H1, H2 and M1-M4, M6, M7 resolved; M5 open as a gate item. The Search page and Desktop Search window would pass with findings alone; not finished until N1 is fixed.

## Earlier findings

| ID | State | Evidence |
|---|---|---|
| H1 | RESOLVED | `FeaturesSection.js:6,82-89,200`; `SearchSwitch.test.js:78-103`; `test/e2e/search.test.js:112`; docs `defaults-table.json:58-69`, `chat-setup.md:90-91`, `config-reference.md:24-25`, `PROMETHITY-FORK.md:66`; screenshots `settings-search-switch-*`, `metrics.json` (checked true→false, navEntry 1→0). Residual L-a |
| H2 | RESOLVED | `search.css:34` inset `--text-muted` edge (~6.5:1 dark, ~6.9:1 light), checkbox `:35-55`, no dimming `:84-86`, off state in words (`search.html:22`); `search-off-*`, `search-on-light-640`, `search-typing-dark-640` |
| M1 | RESOLVED | `launcher.css:20-32`; `logs-screen.css:6,13,29-38`; `launcher-css.test.js`; `search-focus-last-chip-*` (`clearOfLauncher:true`), `diagnostics-logs-*` |
| M2 | RESOLVED | `IndexStatusPanel.js:105-108,128-130`; Jest `SearchPage.test.js:240,249`; `status-403-*` |
| M3 | RESOLVED (spec amended) | `webui-vector-search/tasks.md:10`; `search.json:82` |
| M4 | RESOLVED | 15 real-Electron captures; dev switch gated on unpackaged (`window.js:21-26,106-108`) |
| M5 | OPEN (gate) | Playwright `search.test.js` not run; e2e has no axe; Jest axe only for the error state (`SearchPage.test.js:419`) |
| M6 | RESOLVED | `search/ui/search.js:112-126`, `window.js:146`; `search-typing-dark-640.png` |
| M7 | RESOLVED | `SearchPage.js:125-141`, `IndexStatusPanel.js:111-112`, `search.json:140`; Jest `:211,222,233`. Nit: a non-`SearchError` still falls into "not set up" (`availability-store.js:99`) |
| L1 | RESOLVED | `docs-link.js:6-11`, `brand.js:16-17`; anchors `embeddings.md:100,432` |
| L2, L3, L5, L8, L9 | RESOLVED | `search.json:78,104`; `status-line.js:31,34`, `SearchPage.js:48,51`; `search.json:146-148`; `search.css:4-11`; `ResultItem.js:32-37` |
| L4 | RESOLVED | `en.json:406` (see L-c) |
| L6 | PARTIALLY | tray pending label (`tray-menu.js:16-25`, `en.json:480`); window shows it in the error tone (L-b) |
| L7 | RESOLVED | `search.js:29-39,127,149`, `search.css:62`; `search-error-dark-420.png`. Residual L-d |

## New findings

### HIGH

- **N1: referenced-key copy contradicts itself.** `ConfirmHost.js:50-64` follows the ruling (`agentSettings.json:190`, `:193`), but `OpenAiBackendForm.js:62`, `:68` render `test.gated` (`:136`), `test.noteNewHost` (`:142`) and `test.keyNotSent.text` (`:140`) unconditionally, saying "the key does not follow a move: supply a new key with the change" even for `keyServerManaged`. The false statement understates where a credential goes. Jest `AgentSettingsPage.test.js:357-369` renders the state but does not assert the contradicting sentence is absent. Fix: server-managed variants (keep "A test of a new host never carries the stored key"; replace the clause with "after you apply, the key the server supplies goes to the new host"); add `expect(queryByText(/does not follow a move/)).toBeNull()`; also `key.help` (`:86`) while replacing an env or file key (`ApiKeyField.js:63,68`).

### LOW

- **L-a:** `defaults-table.json:62` (and `chat-setup.md`, `config-reference.md`) still say test names are to be confirmed; name `src/settings/agent/SearchSwitch.test.js`, `SearchPage.test.js`, `test/e2e/search.test.js`.
- **L-b:** Desktop window shows the pending state in the `err` tone (`view.js:48-51`; `search/ui/search.js:67`); carry a `pending` state with `warn`/`info`.
- **L-c:** `en.json:406` "It is off until you turn it on." shown while Search is on; say "Off by default" or drop it.
- **L-d:** at 420 px the error notice sits far below the marked field (`search-error-dark-420.png`); move `#notice` above the notes.
- **L-e:** two accent fills when Search is checked (checkbox `search.css:48` and Save) against `ui.css:3`; use `text` fill or amend the rule.

### Nits and FYI

Duplicate 403 sentence in main column and panel; `SearchSwitch.test.js:97-100` cannot catch a request fired by the toggle itself; WebMCP sub-switch "Off" clips at 375 (`settings-search-switch-dark-375.png`); confirmation windows' inputs still edge-less (`ui.css:202-210`); "Remove key" offered for an env/file key (`ApiKeyField.js:94`) — check the server refuses and the page says so.

## Regression checks (clean)

Site-wide `main[data-route]` padding matches the earlier values with chat off; `scroll-padding-bottom` cooperates with `use-scroll-padding.js:17-27`; launcher `pointer-events` only on the padding band; Desktop D7 in both themes with the `aria-invalid` ring and focus outline; off-state wording at full contrast; tray labels; no horizontal overflow at 320 and 420.

## Acceptance

`webui-vector-search` 1.3 met (amended), 1.4 met, 1.5 met with axe pending, **1.6 met**, 1.7 not run; `desktop-search-sidecar` 1.5 UI **met** (L-b, L-c, L-d open); `webui-agent-settings-page` 1.8 **not met for referenced keys (N1)**; `agent-features-default-on` 1.1 met.
