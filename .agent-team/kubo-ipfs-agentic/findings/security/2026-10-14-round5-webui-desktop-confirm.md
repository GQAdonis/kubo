# Security confirmation: agent-backend-settings Round 5 (Web UI and Desktop)

Reviewer: `security-reviewer` (Claude Opus, independent context, read-only). Date: 2026-10-14. Static reading of the uncommitted correction set; no tests, Electron, Touch ID, native dialog, Playwright or screenshots run by the reviewer. Lead's runs cited (Desktop specs under the plain-node shim, Web UI jest 406/406, `go test ./mcp/server/...` ok). Written to disk by the lead.

**Verdict: PASS WITH FINDINGS.** H1, H2, M1-M3, L1, L2, L4, I1, I2 resolved; L3 open (LOW, investigate); new N1, N2, N3 LOW; N4, N5 INFO; R1 accepted residual. No CRITICAL or HIGH open.

## Status of earlier findings

| ID | Status | Evidence |
|---|---|---|
| H1 | RESOLVED | Server `settings_probe.go:144-158` (`probeKey`: exact host:port of the current backend, `egress.go:44-48`, or `AckCovers(effectiveAck(id), host)`), both endpoints (`:94-95`, `:124-125`), `ok:true, key_sent:false` on withheld 401/403 (`:165-178`), no redirects (`:260-262`); contract `test-connection.schema.json:34-37` additive; tests `settings_probe_test.go:245-276`, `:280-293`; Desktop `settings-service.js:398-404`, `:412`; Web UI gate `core/form.js:211-221`, `OpenAiBackendForm.js:33-34` (UX only). Residuals N3, R1. |
| H2 | RESOLVED | `mcp-sidecar.js:696-700` `starting` until `boundChild === child` (`:410-439`); `sidecar-registry.js:27-29`; per-request reads `settings-api.js:121-124,198-200`; pump `src/webui/index.js:309-312`; restart poll `settings-service.js:296-312`; per-spawn token `mcp-sidecar.js:445`, `:118-122`; rotation synchronous with `child = proc`, token read from the store each call (`secret-store.js:48-59`). |
| M1 | RESOLVED | `confirmHost` `settings-service.js:337-342` (Touch ID, else native dialog, else false); `native-dialog.js:15-38` (parented, Cancel default and cancel id, `noLink`, host named, exception = decline); wired `src/webui/index.js:290-294`; gates `set()` (`:371-375`) and test connection (`:398-404`). Residuals N1, N4. |
| M2 | RESOLVED | `ipfs-desktop/docs/agent/HARDENING.md:191-207`. |
| M3 | RESOLVED | `core/admin-token.js:51` exact root match, `:v2` `{token, root}`, v1 removed (`:11-13,23-33`); root from `core/url.js:49-61` after URL normalisation; no prefix matching; token forgotten on endpoint change (`use-settings-client.js:26-28`, `ConnectionSection.js:51`). |
| L1 | RESOLVED | `restart()` (`mcp-sidecar.js:666-671`) never writes the store; `start()` re-checks `isEnabled()` (`:603-606`). New race N2. |
| L2 | RESOLVED | `agentSettings.json:311`. |
| L3 | OPEN (investigate) | No `frame-ancestors`/`X-Frame-Options` in `ipfs-webui/src` or kubo Go; one-click key removal clickjackable if framed; no CSRF. |
| L4 | RESOLVED | one `before-quit` listener (`src/webui/index.js:235-238`); `replaceProp` (`:369`, `src/context.js:80-84`). |
| I1 | RESOLVED | `settings-api.js:18-19,62-70`. |
| I2 | RESOLVED | `settings-api.js:81-105`. |

## New or residual findings

- **N1 LOW: unlimited Touch ID / native-dialog prompts from the renderer.** `settings-service.js:367-375`, `:391-404`; `native-dialog.js:18-38`. A looping renderer can fatigue the operator; one Allow moves the backend and the server extends the admin's ack (`settings.go:535-548`). Fix: one pending confirmation at a time; cooldown or per-window refusal after a decline for the same host; a spec.
- **N2 LOW: `restart()` can respawn after a concurrent daemon stop or quit.** `mcp-sidecar.js:666-671` awaits `halt()` then `start(path)` even if `stop()` (`:636-639`) or `killNow()` (`:677-691`) ran meanwhile. Fix: after the halt return when `repoPath !== path` or the epoch moved; spec for the interleaving.
- **N3 LOW: "acknowledged host" now also means "may receive the model key".** `probeKey` uses `effectiveAck(id)` (`settings.go:409-411`), including hosts added by past confirmed writes, kept indefinitely. Not a new exfiltration path (a backend move already carries the key, `settings.go:426`), but: a once-acknowledged host keeps getting the key on probes; `egress_ack: ["local"]` sends the key to any loopback port (`settings_probe_test.go:283` asserts this); a content-egress ack for provider B lets provider A's key reach B. Fix: key only to the current backend host:port (or hosts acknowledged with this key in place); `local` must not cover any port for key sending; or document the widened meaning beside `egress_ack`.
- **N4 INFO:** Touch ID reason for a backend change does not mention the key (`assets/locales/en.json:377`; the dialog does, `:393`); the test-connection prompt (`:397`) guards an unauthenticated probe and only adds fatigue.
- **N5 INFO:** native dialog parented to a hidden window (`native-dialog.js:31-33`; close hides, `src/webui/index.js:185-190`) may not show on macOS; fails safe. Parent only when visible.
- **R1 accepted residual:** on the kubo-served Web UI the admin bearer can supply its own `egress_confirmation`; the typed host is renderer UX. Desktop now adds a main-process gate. ADR needed on whether a base-URL host change should withhold the stored key unless re-supplied.

## AGENTS.md

No new tracking or hardcoded endpoints; off switches documented and honoured; no CID injection; `/api/v0/` untouched; contract change additive.

## MCP summary

RCE vectors safe; SSRF bounded (https or loopback http, no redirects; key only to backend or acknowledged hosts, N3); MCP01 PASS WITH FINDINGS (N3); MCP07 PASS on Desktop (N1 open).

## Manual follow-ups

1. Mac without Touch ID: native dialog as a sheet, Enter and Escape decline, hidden-window behaviour (N5).
2. macOS arm64: a squatter on `0.0.0.0:5050` gets no settings traffic before or after the bound line (H2 live check).
3. `frame-ancestors`/`X-Frame-Options` on the kubo-served Web UI (L3).
4. ADR deciding N3 and R1 before the next phase.
