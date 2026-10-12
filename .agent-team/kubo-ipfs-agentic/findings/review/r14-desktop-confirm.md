# R14 review: IPFS Desktop pairing and confirmation window (`desktop-agent-chat` 1.4, 1.5)

Date: 2026-10-12. Reviewer: reviewer (independent context, read-only; nothing was run). Recorded by the lead from the reviewer's hand-back.

Verdict: request changes. Counts: CRITICAL 0, HIGH 2, MEDIUM 8, LOW 9. No kubo `/api/v0`, gateway, CID or wire change in scope. The protocol client matches the server.

## HIGH

- **H1.** New tests are not platform-guarded but the fork CI runs macOS, Ubuntu and Windows (`.github/workflows/ci.yml:83,133,142`): Unix-socket listens, `process.getuid()` and symlinks fail on Windows; e2e expects pairing and channel traffic while the module is inert on win32; on Linux CI `safeStorage` is `basic_text`, so seeding the pairing store throws and probes time out. 1.5's Verify is evidenced on macOS only. Fix: skip socket cases on win32 with an inert assertion; a keyring or test `safeStorage` on Linux; re-run on all three OSes.
- **H2.** Kubo docs promise Windows confirmation through Desktop (`MECHANISMS.md:20,140-143`, `0003:59,136,289`) while Desktop is inert on Windows. Fix: docs say Windows has no resolver this phase; follow-up OpenSpec change for a Node-reachable transport.

## MEDIUM

- **M1.** The observed stray click has no product mitigation (arming, no focus steal); kubo docs still describe promoting `desktop` to a presence resolver once the window ships. Record the incident; keep `desktop` write-only.
- **M2.** New UI strings are not localized (i18next and `assets/locales/en.json` per the Desktop README).
- **M3.** Results not announced and focus lost after deciding or pairing.
- **M4.** Pairing copy says `--confirm=desktop` (drops `native`), omits stop-first for `ipfs mcp pair`, and does not say why the window reopened after a 403.
- **M5.** Usable only by hand-editing the store: no UI for `agentChat.enabled`, no tray entries (pair again, unpair, re-enable after "Turn off"), confirmations coupled to the chat being on.
- **M6.** Spec scenario "Deny in Desktop: `pin_rm` denied, pin remains" not covered (only `pin_add`).
- **M7.** Re-pair race in e2e (`mcp.close()` then `ipfs mcp pair` without waiting); the scripted-renderer flake is unexplained and relies on fixed sleeps; reproduce with `--repeat-each=20` and a trace.
- **M8.** Closing the window can leave a request pending with no visible window (`windows.js:150-157`, `controller.js:123-124,231-233`).

## LOW

- **L1.** Comment and doc drift (`src/webui/index.js:237-238`, `consts.js:27-28`, a non-existent `confirm.css` in a test).
- **L2.** Warnings on every 1 s poll.
- **L3.** `FORBIDDEN_PORTS` lacks 50051 and the design's port list.
- **L4.** Approve label built from the tool id rather than verb plus object.
- **L5.** Input fill contrast about 1.3:1 with no border (design-system issue).
- **L6.** Turn-off uses `store.set` not `safeSet`; no confirmation or documented way back.
- **L7.** `native_prompt` handling unreachable against the current server (FYI).
- **L8.** `refused` never pruned; `enabled` stays true after "Turn off"; pairing window always-on-top can cover the terminal.
- **L9.** Snap/Flatpak may remap `XDG_RUNTIME_DIR`/`TMPDIR` so sockets are invisible to Desktop (unverified).

## OpenSpec

1.4 Verify met by `agent-confirm.e2e.test.js:126-149,235-278` (modulo M7). 1.5 Verify met on macOS only (H1). 1.6 operator run open.
