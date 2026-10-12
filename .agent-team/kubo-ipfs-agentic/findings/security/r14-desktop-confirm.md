# R14 security review: IPFS Desktop pairing and confirmation window (`desktop-agent-chat` 1.4, 1.5)

Date: 2026-10-12. Reviewer: security-reviewer (independent context, static review only). Recorded by the lead from the reviewer's hand-back.

Counts: CRITICAL 0, HIGH 1, MEDIUM 4, LOW 4, INFO 4.

## HIGH

- **H-1. Pairing Desktop lowers write approval from a presence factor to one click any same-user input source can make; the observed stray click during e2e is this path.** With default modes `native,desktop`, a write-class confirmation goes to every fitting resolver and the first approval wins (`kubo/mcp/server/gate.go:106-116,349-363`, `broker.go:357-390`), so a write that needed Touch ID/polkit can be approved by a click (`confirm/ui/confirm.js:143-144`; Tab then Space also reaches Approve). `bringForward` sets always-on-top, shows, focuses and steals app focus on every show (`confirm/windows.js:137-142,172`), turning the window into a click and keystroke trap for agents clicking stale coordinates. The README cites an acceptance that covers only verifier forgery (`docs/mcp/0003...:296-305`); the R11 amendment expected Desktop to add its own presence factor. Destructive calls from `ui`/`a2a` remain safe (`broker.go:46-51,378-380`). Fixes in order of strength: macOS Touch ID (`systemPreferences.promptTouchID`) before sending approve; stop stealing focus (`showInactive`, no always-on-top); arm Approve after >=1.5 s focused and visible, re-arm on blur or new request; deliberate act (press-and-hold or typed CID suffix); `setContentProtection(true)`; log approve timing and focus state. Not fixable in-window: injected OS input is `isTrusted`; overlay windows. Record the incident in a new ADR-0003 amendment and ask the operator whether `desktop` stays a default mode.

## MEDIUM

- **M-1.** Desktop sends the static pairing key as a bearer to any `^\d+-\d+\.sock$` socket in the user's 0700 directory every second (`channel-client.js:50-77,100-102`, `controller.js:165-168`); a rogue same-user listener harvests it and can approve writes on every server until re-pairing, contradicting `pairing.go:21-22`. Fix: additive, versioned HMAC challenge-response with server proof; until then record it as residual risk.
- **M-2.** Confirmation content can mislead: the wire form drops structured `Targets` (`channel.go:51-59`), Desktop regex-extracts CIDs from agent-shaped summary text (`cids.js:6`), missing base36/base58 CIDs; clipping can hide the destination; the sanitizer misses invisible and bidi characters. Fix: add `targets` to `PendingView`, render with kind labels, truncation marker, wider strip set.
- **M-3.** The request can be swapped under the cursor when the queue head changes (`controller.js:113-138`, `confirm.js:67-114`). Fix: disable Approve for >=1 s with a notice on every `ref` change.
- **M-4.** No reachable off switch once paired: "Turn off Desktop confirmations" only in the pairing window; `openPairing`/`unpair` not wired to the tray (`src/webui/index.js:239-245`); only strict `false` counts as off (`index.js:40`). Fix: tray entries and a control in the confirmation window; accept falsy string forms; document the key.

## LOW

- **L-1.** Idle (not total) request timeouts and `Promise.all` let one trickling or crowding socket stall polling; 16 lexically-first rogue sockets crowd out the real one.
- **L-2.** Warnings logged on every 1 s poll; log on state change only.
- **L-3.** Focus theft into the pairing password field (fixed by H-1 item 2).
- **L-4.** Kubo `ensurePrivateDir` follows symlinks (`channel.go:116-127`); use `Lstat` and refuse symlinks.

## INFO

- **I-1.** `platform_other.go:12` tells Windows users to pair Desktop, but Desktop is inert on win32.
- **I-2.** `src/webui/index.js:57` enables Node integration when `NODE_ENV=test`; check upstream origin.
- **I-3.** `shownRef` test getter ships in production (main process only).
- **I-4.** The e2e off-screen mitigation is test-only, so e2e never exercises production placement and focus.

## Verified safe

Renderer isolation (partition, sandbox, contextIsolation, no node, no devtools, navigation and permissions denied, file-URL filter); `default-src 'none'` CSP; IPC sender, frame, URL, ref and type checks; minimal frozen preloads; Web UI renderer cannot reach the channels (e2e-covered); `textContent` only; native-prompt requests unapprovable at four layers; Deny focus, Escape and close deny; ciphertext-only key storage; no secrets in logs; socket trust checks with caps; off switches inert (e2e-covered); Windows fails closed; decided requests do not return.

## Follow-ups

Before operator task 1.6: verify no focus steal and Touch ID behavior; verify `setContentProtection` against local capture APIs; `npm audit` and lockfile check; ADR-0003 amendment, README citation fix, stale comments in `broker.go:41-45` and `MECHANISMS.md:142,156`.
