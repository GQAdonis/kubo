# Security review: agent-backend-settings Round 7, settings-security-review 1.1

Reviewer: `security-reviewer` (Claude Opus, independent context, read-only). Date: 2026-10-15. Static reading of kubo `master` (uncommitted tree touches only embeddings, contract and docs; no `settings*.go` modified) and ipfs-webui / ipfs-desktop `feat/ipfs-mcp-server`. No tests, Electron, browser or cluster access. Builds on Round 5, Round 6 and its confirmation, and lke-origin-isolation 1.6; accepted residuals not re-raised (R1, referenced-key-follows ruling, xberg loopback port). Saved by the lead from the reviewer's returned report.

**Verdict: PASS WITH FINDINGS.** No CRITICAL, no HIGH. Two MEDIUM residual-risk rulings (M1 console inline Basic credentials in localStorage; M2 chat `ui` token in localStorage on the console origin). Six LOW (L1-L6), five INFO (I1-I5). Round 6 N1 (failed apply left the overlay) and N2 (Test connection copy) resolved.

## 1. Settings API, `admin` principal, test-connection and models, egress extension, overlay and secret storage, `accept_secrets`

**L1 LOW: a settings write can move the backend to plain http on a remote host when no key goes with it, and an https→http downgrade on the same host needs no D4 confirmation.** The write path checks only length and whitespace (`mcp/server/settings.go:487-494`) then `validateBackendURL` (`agent_settings.go:188-201`) accepts http to any host; plain http to a remote host is refused only when a key is attached (`agent_settings.go:143-145`). The probe is stricter (`settings_probe.go:213-214`). D4 compares host and port only (`settings.go:557-562`, `HostFromURL` `egress.go:80-86`), so `https://h/v1` → `http://h/v1` asks no `egress_confirmation`; `backendMoved` counts the scheme so an API-stored key is cleared (`settings.go:533-546`) and a referenced key fails resolve — a keyless downgrade goes through silently. Chat text and allowed node content then travel unencrypted. Web UI refuses (`ipfs-webui/src/settings/agent/core/url.js:22`); Desktop accepts after Touch ID (`ipfs-desktop/src/agent/settings-service.js:24`, `:104`). Fix: apply `validateCandidateURL`'s scheme rule to writes (with an exact-match opt-in for in-cluster http like `embeddings.insecure_http_hosts`); treat a scheme change as a host change for D4.

**L2 LOW: settings writes leave no audit trail at the default log level.** Logged only at `Debug` without the principal (`settings.go:647`). Fix: `Info` (or `Warn` for a host move or egress extension) with principal ID, field names, old and new host, `key_not_carried`; never the key or prompt.

**L3 LOW: the settings route is not on the strict origin policy ADR-0009 §4 describes.** Mounted without `AllowedOrigins`/`CORS` (`settings.go:150`), so it gets the listener-wide policy (`http.go:290-294`, `:358-370`) and `setCORS` (`:395-403`); ADR-0009 `:56` says exactly the console origin. Safe today (admin bearer never in a cookie; `credentials: 'omit'`, `http-client.js:61`; `/agui` and `/index` strict). `setCORS` names neither `PUT` nor `If-Match` and exposes no `ETag`, so a cross-origin page cannot write (lead noted, `execution.md` item 1). Fix: strict `CORSPolicy` over `agent.origin_allowlist` (methods `GET, PUT, POST, OPTIONS`; headers `authorization, content-type, accept, if-match`; expose `ETag`), or amend ADR-0009 §4.

**L4 LOW: no rate or concurrency limit on the settings route and probes (MCP-03).** Each probe can hold 10 s of outbound I/O (`settings_probe.go:32`); no SSE subscriber cap (`settings_events.go:41-47`). Admin-only, 256-bit token. Fix: per-principal token bucket with 429 `Retry-After`, plus a subscriber cap.

Verified: admin-only kind-gated mount (`settings.go:150`, `http.go:408-416`), admin absent from default kind lists (`http.go:243-257`), admin has no tool groups (`principals.go:291-296`), route only with an admin principal and `agent.settings.enabled` not false (`cmd/ipfs/kubo/mcp_agent.go:209-216`, `principals.go:67`); exact `If-Match` (`settings.go:402-407`, `:433-436`); unknown fields refused (`:395`, `settings_probe.go:83`, `settings_overlay.go:91`); key write-only, responses `{set, source}` (`settings.go:186-205`, `:248-277`); `accept_secrets: false` refuses `api_key` (incl. `null`) first (`:464-468`); key file and overlay 0600 via temp and rename (`settings_overlay.go:229-259`); failed-apply rollback byte for byte (`settings.go:597-627`, `:651-695`), cleared key file removed only after success (`:697-710`), pinned by `TestSettingsFailedApplyKeepsStoredKey` (`settings_keymove_test.go:190-223`) — Round 6 N1 RESOLVED; ADR-0012: API-stored key cleared on scheme/host/port move (`settings.go:531-546`), referenced key follows and D4 says so (`:580-585`), probes key only to the current endpoint under one lock (`settings_probe.go:145-156`, `:224-228`), no redirects, direct transport (`:263-266`), candidate https or loopback http without userinfo/query/fragment (`:197-216`), echoed key scrubbed (`:300-306`); D4 `egress_confirmation` must equal the new host (`settings.go:563-591`), overlay adds exactly that host for that principal (`settings_overlay.go:207-214`), the extension feeds only the admin's own next settings check (`settings.go:426-428`); the settings API cannot widen tool-content egress (`agent.go:795-801`, `:825-827`; `principals.go:298`); overlay key references must be absolute files, `egress_ack` entries validated (`settings_overlay.go:95-109`).

## 2. Admin token storage in the Web UI settings page

No new finding; Round 5 M3, L2 hold. Memory by default; optional sessionStorage `{token, root}` under `...:v2`, v1 removed unread (`admin-token.js:11-33`, `:61-67`); handed out only for its root (`:51`), forgotten on endpoint change (`use-settings-client.js:26-28`, `ConnectionSection.js:51`), client built only when bound (`use-settings-client.js:30-35`); header only with `credentials: 'omit'`, `redirect: 'error'`, `referrerPolicy: 'no-referrer'`, `cache: 'no-store'` (`http-client.js:56-66`); SSE retry stops on 401/404/no token (`:183`). Round 6 N2 RESOLVED (`OpenAiBackendForm.js:37`, `:63`, `:69`).

## 3. Desktop bridge

No new code finding; Round 5 H1, H2, M1 and Round 6 L2, I1, I2 hold. Handlers answer only the Web UI main frame (`settings-bridge.js:37-41`); preload API only with the flag, no key getter (`src/webui/preload.js:119-138`); push events revision-only to the Web UI origin (`settings-bridge.js:67-75`, `preload.js:126-136`); closed validation (`settings-service.js:88-173`), whitelisted read shape (`:183-225`) and error `details` (`settings-api.js:53-72`), `redirect: 'error'`, body cap while reading, timeouts (`:81-105`, `:130-141`); Touch ID else native dialog with Cancel default, fails closed (`host-confirm.js:57-76`, `confirm/native-dialog.js:38-57`); one prompt at a time, declines per scope (`host-confirm.js:44-47`, `:85-88`); host from `new URL().host` (punycode) (`settings-service.js:75-77`); key deleted before the PUT and restored on refusal (`:468-486`) — with the server rollback the Round 6 N1 Desktop path is closed; `sidecarBaseUrl` only `http://127.0.0.1:<reported port>` in `running` (`sidecar-registry.js:27-29`, `webui/index.js:257`); CSP admits the sidecar only as plain-http loopback (`security.js:79-92`, `:103-126`), `frame-ancestors 'none'`, `script-src 'self' 'wasm-unsafe-eval'`, `webSecurity`, `contextIsolation`, `sandbox` (`:159-171`). Same-user process with Accessibility rights clicking the dialog: outside the boundary (ADR-0003 residual).

## 4. Cloud console design (ADR-0009) and the live interim state

**M1 MEDIUM (residual-risk ruling, operational): inline Basic credentials in the console's localStorage.** The interim API address `https://<user>:<password>@console.prometheusags.ai` (`execution.md:103-104`) is stored verbatim as `ipfsApi` in localStorage (`ipfs-webui/src/bundles/ipfs-provider.js:457`), turned into `Authorization: Basic` on every call (`:243-249`), and shown in a plain text field (`src/components/api-address-form/api-address-form.tsx:21`, `:70-74`). Previously in the browser's HTTP-auth cache (unreadable by scripts); now readable by any same-origin script, persistent, in the profile on disk. Grants full `/api/v0` (config replace, key export, pin removal, shutdown) from anywhere. Contained by the console `/ipfs` pin to `WebUIPath` (lke-origin-isolation MEDIUM-2 resolved), kubo's Origin check and the GET 405. Residual: XSS or supply-chain compromise of that bundle, or profile access, takes the node; the same script also gets the `ui` token (M2) and an admin bearer in that tab. Ruling: acceptable only time-boxed until session auth, with conditions: (1) a dedicated htpasswd user for the console browser; (2) rotate it when Kratos and `flint-gate` replace Basic auth and clear `ipfsApi` in every profile; (3) no screenshots or evidence of console Settings or Status while in place; (4) investigate whether the installed Envoy Gateway can send a fixed `realm` (would remove the workaround; unverified).

Design notes for the Kratos and `flint-gate` ADR (not findings): cookie sessions bring back CSRF — `SameSite=Strict`, `HttpOnly`, `Secure`, an Origin check at the gateway, no ambient credentials reaching bearer-designed `ipfs mcp` routes; identity headers accepted only from the proxy, stripped from client input, with a NetworkPolicy stopping direct pod access (lke-origin-isolation MEDIUM-4 still open, becomes load-bearing); the admin capability mapped to a distinct Kratos identity or role with step-up, not any logged-in session.

Verified: settings route outside Basic auth and bearer-only (ADR-0009 `:55`), bearer header with `credentials: 'omit'` so cached Basic credentials never attach; `accept_secrets: false` keeps keys out of the browser on LKE; ADR names its residuals (`0009-*.md:100-103`); the settings API was not live on LKE per `execution.md` (re-check before rollout).

## 5. W7 items

### 5a. `mcp-agent-hot-reload`: secret lifetime and interrupt resume across swaps

**L5 LOW: a retained snapshot lets an open interrupt keep using the previous backend and key after a move or key clear; chained interrupts extend it.** A run ending on an interrupt hands its snapshot to the session (`agent.go:488-494`); resume uses it (`:474-479`); `swapSnapshot` retires but drops the key only when no references remain (`:420-426`, `:228-232`); sessions last 30 minutes idle, reset by each resume (`agent_sessions.go:19`, `:146-151`). After the operator moves away from an untrusted host or removes the key, a conversation waiting on a write confirmation still sends the tool result and further steps to the old host with the old key. ADR-0009 `:27` allows failing with a stable error. Fix: when a swap changes the endpoint or clears/replaces the key, revoke retained snapshots so a resume fails with a stable code (e.g. `backend_changed`) and the key is dropped at once; model- or prompt-only swaps keep current behaviour.

I1 INFO: `dropKey` clears the field only (`llm_client.go:154`); key bytes remain until GC; do not call it zeroing.

Verified: one `atomic.Pointer` shared by AG-UI and A2A (`agent.go:238-241`); `currentSnapshot` skips retired (`:404-413`); sessions, buckets and the confirmation broker survive a swap (`:250-259`, `:415-419`); apply validation equals startup (`:298-305`, `:338-349`); change hook payload-free, event revision-only (`settings_events.go:21-26`).

### 5b. `desktop-mcp-sidecar`: token generation and environment passing

**L6 LOW: Desktop secrets reach grandchildren not run with a minimal environment.** The child gets `IPFS_MCP_DESKTOP_ADMIN_TOKEN`, `..._UI_TOKEN`, `..._LLM_API_KEY` (`ipfs-desktop/src/daemon/mcp-sidecar.js:499`); the surreal `version` check runs with no `cmd.Env` (`mcp/server/embeddings/supervisor.go:182`) and the long-lived `sh` watchdog keeps a copy (`embeddings/supervisor_unix.go:51`), while the store launch and xberg calls use `minimalEnv()` (`supervisor.go:302`, `extractor_supervisor.go:198`, `:279`). Fix: `cmd.Env = minimalEnv()` on both.

Verified: 32-byte `crypto.randomBytes` base64url (`mcp-sidecar.js:61-63`); admin token rotated per spawn and stored as safeStorage ciphertext before the spawn (`:119-123`, `:487-489`); `starting` until the current child's first anchored bound line (`:29`, `:453-482`, `:745-749`), clients gated on `running` (`sidecar-registry.js:27-29`, `settings-service.js:264`, `:331`, `webui/index.js:361`); allowlisted env, sidecar names never inherited, secrets last (`mcp-sidecar.js:174-201`, `:499`); no secret in config or argv (`:143-162`, `:363-378`); fails closed without safeStorage (`:385-388`); UI token not rotated (acceptable).

### 5c. Chat `ui` token in localStorage on the console origin

**M2 MEDIUM: W7 ruling, accepted only as an interim with conditions.** Stored under `ipfs-webui:agent-chat:token` with no expiry or endpoint binding (`ipfs-webui/src/agent-chat/settings-store.js:13`, `:25-41`, `:81-90`). On LKE it is a bearer usable from anywhere: `/agui` and `/mcp` accept `ui` (`http.go:249-252`). Its holder confirms its own writes (`agui_resume.go:54`; `/mcp` elicitation; `confirmation.modes: []` per ADR-0009 §10). A stolen token means unattended reads plus the console `ui` principal's write groups (e.g. pins); destructive calls stay refused. Small increment over M1 today. Ruling: accept the 2026-10-10 operator decision (ADR-0004 R11, ADR-0005 amendment) only while M1 is in force, with conditions: (1) when session auth replaces Basic auth, the `ui` token leaves localStorage (session cookie from `flint-gate`, or memory plus sessionStorage); (2) until then the console `ui` principal gets the smallest `write_groups` that works; (3) consider an explicit `/mcp` kind list on LKE excluding `ui`; (4) rotate it with the M1 password.

## Other INFO

- I2: overlay per-principal `egress_ack` grows unpruned (`settings_overlay.go:207-214`); moving back needs no new confirmation; a re-created principal name inherits hosts. Clear on explicit reset; document.
- I3: `BackendHosts` for `ui`/`a2a` preflight from the config file, not the overlay (`principals.go:305-307`); after a settings move the `/mcp` preflight checks stale hosts (`egress.go:163`); content check is the snapshot host (`agent.go:795-801`) so nothing leaks, refusals can be wrong; D4 copy overstates when the `ui` ack does not cover the new host (fail-closed, `docs/mcp/chat-setup.md:43`).
- I4: probes reach any https host including private addresses and loopback http on any port (`settings_probe.go:197-216`), returning `ok`, an error excerpt (`llm_client.go:206-218`) and model IDs; admin-only, bounded. Optionally refuse link-local and RFC1918 targets unless they are the current backend.
- I5 (NEEDS INVESTIGATION): `IsLocalHost` treats any `*.localhost` as loopback (`egress.go:58`), affecting the key-over-http rule (`agent_settings.go:143`), the probe's http allowance and `AckCovers("local")`. If Go's resolver sends such names to DNS instead of answering loopback per RFC 6761, a key could travel in cleartext. Verify the toolchain in `go.mod`, or accept only literal loopback IPs and `localhost`.

## Verified controls

| Control | Evidence |
|---|---|
| Route only with an admin principal and the switch on | `cmd/ipfs/kubo/mcp_agent.go:209-216`; `principals.go:67` |
| Admin kind only; no tool groups; absent from default kinds | `settings.go:150`; `http.go:243-257`, `:408-416`; `principals.go:291-296` |
| Strict `If-Match`; unknown fields refused; size bounds | `settings.go:395-407`, `:433-436`, `:487-511`; `settings_probe.go:83`, `:198` |
| No key material in any response or log | `settings.go:186-205`, `:248-277`, `:647`; `settings_probe.go:300-306` |
| `accept_secrets: false` refusal first | `settings.go:464-468` |
| Key file and overlay 0600, atomic, directory 0700 | `settings_overlay.go:229-259` |
| Failed-apply rollback of both files | `settings.go:597-627`, `:651-695`; `settings_keymove_test.go:190-223` |
| ADR-0012 key cleared on a move; referenced key disclosed in D4 | `settings.go:531-546`, `:580-585`, `:712-744` |
| Probe key only to the current endpoint, one lock, no redirects, direct transport | `settings_probe.go:145-156`, `:224-246`, `:263-266` |
| D4: exact host, one host, persisted per principal | `settings.go:553-591`; `settings_overlay.go:207-214` |
| Settings API cannot widen tool-content egress | `agent.go:795-801`, `:825-827`; `principals.go:298` |
| Hot reload: atomic swap, runs keep their snapshot, key dropped at the last reference | `agent.go:298-323`, `:404-426`, `:473-494`; `llm_client.go:151-154` |
| Desktop per-spawn admin token; `starting` gate | `mcp-sidecar.js:119-123`, `:453-489`, `:745-749`; `sidecar-registry.js:27-29` |
| Desktop env allowlist; secrets last; none in argv or config | `mcp-sidecar.js:143-201`, `:363-378`, `:499` |
| Desktop human gate | `host-confirm.js:57-76`; `native-dialog.js:38-57`; `settings-service.js:441-456` |
| Desktop key deleted before a move, restored on refusal | `settings-service.js:468-486` |
| Desktop IPC trust, closed validation, whitelisted shapes | `settings-bridge.js:37-41`; `settings-service.js:88-225`; `settings-api.js:53-72` |
| Desktop CSP | `security.js:79-126`, `:159-171` |
| Web UI admin bearer in memory, bound to its root, header only | `admin-token.js:51-67`, `:87-89`; `use-settings-client.js:26-35`; `http-client.js:56-66` |
| Settings change stream carries the revision only | `settings_events.go:21-26`, `:88-97`; `settings-api.js:243-251`; `preload.js:126-136` |

## AGENTS.md

No tracking; no hardcoded endpoints (operator- or admin-supplied URLs only); off switches: `agent.settings.enabled: false` or no admin principal, `accept_secrets`, Desktop `agentSettings.enabled` (`webui/index.js:238`) and `mcpSidecar.enabled` (`mcp-sidecar.js:52`, `:433`); `/api/v0`, gateway, CID recipe untouched; no silent CID injection (M2 stolen-token pins are credential abuse; I4 loopback-gateway probe is admin-initiated).

## MCP checklist (settings surface; network-exposed on LKE, loopback on Desktop)

| Control | Status | Justification |
|---|---|---|
| MCP-01 | PASS | Bearer plus kind gate (`http.go:408-416`); admin scope settings only (`principals.go:291-296`); probes never forward the inbound token (`settings_probe.go:231-246`) |
| MCP-02 | N/A (settings) / PASS (agent) | Agent session IDs CSPRNG (`token.go:114-120`) bound to the principal (`agent_sessions.go:155-157`) |
| MCP-03 | FAIL (LOW, L4) | No limiter on the settings route or probes; agent run limiter exists (`agent_sessions.go:93-117`) |
| MCP-04 | PASS | `DisallowUnknownFields` and bounds (`settings.go:395`, `:461-511`; `settings_probe.go:83-95`) |
| MCP-05 | PASS | `modelcontextprotocol/go-sdk` |

| RCE vector | Status | Justification |
|---|---|---|
| Command injection | SAFE | Fixed argv (`mcp-sidecar.js:363-378`; `supervisor.go:298`) |
| Dynamic eval | SAFE | None |
| Unsafe deserialization | SAFE | JSON with unknown fields refused |
| Path traversal | SAFE | Settings dir from flag or config; fixed names (`settings_overlay.go:22-27`); absolute key reference from a trusted file (`:95-99`) |
| SSTI | N/A | None |
| Dependency hijacking | NOT RE-AUDITED | No dependency change |
| SSRF | SAFE (bounded) | Admin-only; key only to the current endpoint; no redirects; I4, I5 |

| OWASP | Status | Note |
|---|---|---|
| MCP01 | PASS WITH FINDINGS | L5, L6; residuals M1, M2 |
| MCP02 | PASS | Admin cannot reach tools; D4 cannot widen content egress |
| MCP03, MCP05 | PASS | Static tools; no shell |
| MCP04 | NOT RE-AUDITED | — |
| MCP06 | PASS | Fixed safety prefix (`agent.go:195-201`) |
| MCP07 | PASS | Server-side kind gate enforced in `ipfs mcp` |
| MCP08 | FAIL (LOW, L2) | Settings writes at `Debug` only, no principal |
| MCP09 | NEEDS INVESTIGATION | LKE route inventory not visible |
| MCP10 | PASS | Minimal read shape; whitelisted on Desktop |

## Manual follow-ups

1. LKE before rollout: admin principal present or absent; `accept_secrets: false`; console `ui` principal `write_groups` and `egress_ack`; no SecurityPolicy on `/agent/settings`, `/agui`, `/mcp`; admin token strength and Secret storage.
2. Envoy Gateway: fixed Basic `realm` possible (M1 condition 4)?
3. Browser: the console stores `ipfsApi` with userinfo; nothing else captures it.
4. Go resolver behaviour for `*.localhost` (I5).
5. Race detector on `settings`, `settings_probe` and the agent swap tests at the gate.
6. Desktop: forced 500 on a confirmed move; restarted sidecar should use the old backend and old key.
7. Not re-read: uncommitted embeddings/`insecure_http_hosts`/contract changes; `agui_resume.go` beyond `:48-77`; Desktop tray and search; framing of the kubo-served Web UI (Round 5 L3 open).

## For 1.2 (batched correction)

No CRITICAL or HIGH open. Priority: L5, L6, L1, L2, L3, L4. M1 and M2 are operator rulings with conditions to record in ADR-0009 and ADR-0004.

## Disposition (1.2)

Date: 2026-10-16. Batched correction (1.2) although no CRITICAL or HIGH was open, so every finding is fixed, rebutted or ruled on. Test names are those the server engineers report in `.kbd-orchestrator/phases/agent-backend-settings/execution.md` ("Round 7 corrections"); source files are under `mcp/server/`. The review text above is unchanged.

| Finding | Disposition |
|---|---|
| M1 (console inline Basic credentials in localStorage) | operator ruling: "Accept with the conditions (Recommended)", accepted as an interim residual until Kratos plus `flint-gate` session auth lands. Five conditions recorded in ADR-0009 (Round 7 amendment) and `.prometheus/decisions.md` |
| M2 (chat `ui` token in localStorage on the console origin) | operator ruling, same answer; accepted only while M1 is in force; recorded in ADR-0004 and ADR-0009 amendments |
| L1 (plain-http backend; scheme downgrade without D4) | fixed (`settings.go`, `settings_probe.go` `validateCandidateURL`): writes and probes accept plain http only to loopback or a host in `embeddings.insecure_http_hosts`; a scheme change needs `egress_confirmation`. Docs: `embeddings.md`, `agentic.md`, schema description of `insecure_http_hosts` |
| L2 (no audit trail) | fixed (`settings.go` `auditWrite`): Info, Warn for a host move or egress extension; principal ID, field names, old and new host, `key_not_carried`; no key, prompt, model or path |
| L3 (settings route off the strict origin policy) | fixed (`settings_limits.go` `settingsCORS`): strict policy over `agent.origin_allowlist`, methods GET, PUT, POST, OPTIONS, headers authorization, content-type, accept, if-match, exposes ETag and Retry-After. ADR-0009 section 4 amended; operators relying on `http.allowed_origins` add the console origin to `agent.origin_allowlist` |
| L4 (no rate or concurrency limit) | fixed (`settings_limits.go`): route 60 a minute, burst 60; probes 10 a minute, 2 in flight; 16 SSE subscribers; 429 with `Retry-After`. Contract note in `settings-error.schema.json` |
| L5 (retained snapshot after a move or key clear) | fixed (`agent.go`, `egress.go` `ReasonBackendChanged`, `agui.go`, `a2a_wire.go`; test `agui_backend_changed_test.go`): open interrupts are revoked when the endpoint or key changes; resume fails `failed_precondition`, `details.reason = "backend_changed"` (AG-UI 409; A2A `BACKEND_CHANGED`). A resume in flight keeps its snapshot until it ends (ADR-0009). Documented in ADR-0004, ADR-0009, `agentic.md`, the AG-UI and A2A endpoint design tables |
| L6 (Desktop secrets reach grandchildren) | fixed (`embeddings/supervisor.go`, `supervisor_unix.go`): minimal environment on the surreal version check and the watchdog |
| I1 (`dropKey` clears the field only) | acknowledged: wording only; docs say the key is dropped, not zeroed |
| I2 (overlay `egress_ack` grows unpruned) | rebuttal: moving back to a confirmed host without a new prompt is intended and the list is bounded by what the admin confirmed; documented instead. ADR-0009 amendment: persists per principal ID, a re-created principal inherits it, deleting the overlay clears it |
| I3 (preflight reads config hosts, D4 copy overstates) | documented, not changed: nothing leaks (the content check uses the snapshot host), the inaccuracy is fail-closed. ADR-0009 amendment and `chat-setup.md` (egress_ack bullet) corrected |
| I4 (probes reach private addresses) | rebuttal: admin-only, bounded (10 s, response and excerpt caps, no redirects, key only to the current endpoint); a link-local and RFC1918 block would also block legitimate in-cluster model servers. L1 and L4 now narrow plain http and the rate |
| I5 (`*.localhost` as loopback) | fixed (`egress.go` `IsLocalHost`): only `localhost` and literal loopback IPs (IPv4-mapped unmapped). Go's resolver sends `*.localhost` to DNS, so it was not safe. A remaining `*.localhost` check in `embeddings/settings.go` is a lead follow-up. Documented in `embeddings.md` and `agentic.md` |
