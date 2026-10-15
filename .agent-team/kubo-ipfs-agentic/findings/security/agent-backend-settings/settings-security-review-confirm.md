# Confirmation: settings-security-review 1.3 (agent-backend-settings)

Reviewer: `security-reviewer` (Claude Opus), working read-only in a context independent of the builders. Date: 2026-10-16. I read the current kubo tree statically and did not run anything. The lead had already confirmed `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/`, `-tags noagent`, gofmt and `config_reference.py --check`, so I did not repeat them. The lead wrote this file to disk. Paths are under `mcp/server/` unless stated.

**Verdict: PASS WITH FINDINGS.** There is no open CRITICAL and no open HIGH, so the phase gate has no HIGH to name. All 13 earlier findings hold as disposed. The fixes introduced two new LOW findings (N1, N2) and one INFO (N3).

| ID | State | Evidence |
|---|---|---|
| M1 | ACCEPTED-RULING | ADR-0009 `:109-124`. The verbatim ruling and the time-box until Kratos plus `flint-gate` are recorded. All four original conditions are there: dedicated htpasswd user (`:118`), rotate and clear `ipfsApi` (`:119`), no screenshots (`:120`), Envoy realm check (`:122`). The residual is described accurately (`:113`). |
| M2 | ACCEPTED-RULING | ADR-0004 `:196-202` and ADR-0009 `:114`, `:121`, `:124`. It is accepted "only while M1 is in force". The smallest `write_groups`, rotation with M1, and the token leaving `localStorage` when session auth lands are all recorded. The `/mcp` kind list is correctly marked "suggested, not decided". |
| L1 | RESOLVED, with residual N1 | Writes go through `validateCandidateURL` (`settings.go:554-561`; `settings_probe.go:200-218`). Plain http is accepted only for loopback or an exact `insecure_http_hosts` match (`:227-237`). Same host:port with a changed scheme now needs D4 (`settings.go:633-650`). Pinned by `settings_hardening_test.go:37-106`. |
| L2 | RESOLVED | `settings.go:730`, `:734-747`: Info, or Warn on a move. Logs principal, fields, old and new host, `key_not_carried`. Pinned by `settings_hardening_test.go:120-158`, which also asserts that no key, prompt, model or path is logged. |
| L3 | RESOLVED | `settings_limits.go:112-120`; `settings.go:171-173`. When a route sets `cors`, only the route's own list is used (`http.go:292-294`, `:374-377`); preflight is answered before auth (`:317-326`); `setStrictCORS` sends no credentials header (`:385-394`). Pinned by `settings_hardening_test.go:167-202`, which also covers rejecting the listener's own origin and the no-allowlist case. |
| L4 | RESOLVED, with N3 | Limiter in `settings_limits.go:16-102`, applied at `settings.go:340-417`; method checks run before a token is charged. Subscriber cap at `settings_events.go:36`, `:89-93`. Pinned by `settings_hardening_test.go:223-304`. |
| L5 | RESOLVED, with N2 | `sameBackend` (`agent.go:376-380`). Revocation in `swapSnapshot`/`revokeRetained` (`:451-474`), stale in-flight runs in `retainOrRelease` (`:505-521`), resume in `takeRetained` (`:478-483`, `:568-578`). Wire mapping: AG-UI 409 with `reason` (`agui.go:214-218`, `agui_wire.go:206-207`), A2A `BACKEND_CHANGED` (`a2a_wire.go:247-248`). Pinned by `agent_swap_test.go:79-185` and `agui_backend_changed_test.go:19`, `:44`. |
| L6 | RESOLVED | `embeddings/supervisor.go:184-185`; `supervisor_unix.go:51-52`, `:76-77`. `minimalEnv` passes only PATH, HOME and temp-dir variables (`supervisor.go:327-335`). Pinned by `supervisor_unix_test.go:60-82`. |
| I1 | RESOLVED (wording) | `llm_client.go:151-154`. The docs say "dropped" (`docs/mcp/agentic.md:104`); no doc claims zeroing. |
| I2 | REBUTTAL HOLDS | Every host entry needed a typed confirmation from an admin. The behaviour is documented, including that a re-created principal inherits the hosts and that deleting the overlay clears them (ADR-0009 `:143`). |
| I3 | RESOLVED (documented) | ADR-0009 `:144`. It fails closed, and the content check uses the snapshot host (`agent.go:430`). |
| I4 | REBUTTAL HOLDS | Probes are admin-only and are now limited to 10/min and 2 in flight. The key goes only to the current endpoint (`settings_probe.go:146-157`). Plain http is narrowed by L1. https to RFC1918 hosts remains, which is needed for in-cluster models. |
| I5 | RESOLVED | `egress.go:53-71`, `listen.go:107-117`, and the lead's follow-up in `embeddings/settings.go:257-275` now accept only `localhost` and literal loopback IPs. Pinned by `egress_test.go:24`, `:40-41`, `embeddings/settings_test.go:87` and `settings_hardening_test.go:42`, `:76`. |

## New findings

**N1 LOW: an https to http downgrade on the same host skips D4 if the write also adds or changes the port.** `hostChanged` compares host:port literally (`settings.go:632`, `sameHostPort` `egress.go:47-51`). So `https://h/v1` to `http://h:80/v1` (or `http://h:8080/v1`) counts as a host change. A bare `egress_ack` entry `h` covers every port (`egress.go:109-116`), so the `AckCovers` branch skips confirmation (`settings.go:650`), and the scheme-only branch requires `!hostChanged`. This is reachable only when the operator lists bare `h` in `embeddings.insecure_http_hosts` (`settings_probe.go:216`), so keys are not exposed: an API-stored key is cleared on a move (`settings.go:603-616`), and a referenced key over remote http fails resolve (`agent_settings.go:143`). Chat content to that host loses TLS without a typed confirmation, which contradicts ADR-0009 `:137`. The test only covers the no-port case (`settings_hardening_test.go:84-106`). Fix: require confirmation whenever `schemeChanged`, or compare hostnames without the port for the scheme rule. Add a test case for `http://h:80`.

**N2 LOW (availability, fails closed): a revoked resume of an already-approved confirmation leaves the session stuck until the confirmation TTL expires.** `denyRevoked` calls `Resolve(..., false)` (`agent.go:489-499`), but `Resolve` refuses anything that is not pending (`broker.go:364-371`). The record therefore stays `approved` until `ExpiresAt` (default 120 s, `config.go:57`). Meanwhile `acceptMessage` returns `ErrInterruptOpen` for an approved record (`agent.go:645-648`). The error text ("send a new message to continue", `agent.go:73`) and `agentic.md:104` are then wrong for up to the TTL. The approved call cannot run elsewhere, because the dedupe key is bound to the agent session id (`gate.go:225-228`, `:245`). `TestAgentSwapRevokesOpenInterrupt` approves and then resumes (`agent_swap_test.go:114-125`) but never sends a new message afterwards. Fix: when `s.backendChanged` is set, `acceptMessage` treats the interrupt as ended, or the broker gets a cancel for approved-but-unclaimed records.

**N3 INFO: a rate refusal can round to zero.** `take` refuses with `retry = Duration((1-tokens)/perSecond*1e9)` (`settings_limits.go:89-91`). If `tokens` lands within about 1e-10 of 1, this truncates to 0 ns. `allowProbe` then returns `(nil, true)` and `defer done()` calls a nil func (`settings.go:365-369`, `:377-381`). net/http recovers the panic and logs it. `allowRoute` lets that request through without charging a token. The callers are admin-only and the case is rare. Fix: return an explicit bool from `take`, or clamp `retry` to at least 1 s.

## The four requested checks

- (a) Revocation racing a resume: no race found. `takeRetained`, `revokeRetained`, `retainOrRelease` and `acceptMessage` all take `a.mu`, and per-session work is serialised by `acquire`. A resume that has already taken its snapshot finishes on the old backend, as ADR-0009 `:27` and ADR-0004 `:203` document. A chained interrupt from that run is revoked at `retainOrRelease` (`agent.go:512-516`). The refcount and retired ordering stays sound (`:435-458`).
- (b) Plain-http scheme change: closed for the same host:port; open via a port change (N1).
- (c) CORS on the settings routes: strict and correct; see L3.
- (d) `*.localhost`: closed in every checker found (`IsLocalHost`, `isLoopbackHost`, `httpsOrLoopback`).

## AGENTS.md

No tracking was added. The audit line carries no content (`settings.go:739-747`). There are no new hardcoded endpoints. The off switches are unchanged. `/api/v0`, the gateway and the CID recipe are untouched. No CID injection path was found.

## Manual follow-ups

1. Race detector on `agent_swap_test.go` and `settings_hardening_test.go` at the phase gate (from the 1.1 follow-up list).
2. LKE: add the console origin to `agent.origin_allowlist` before rollout, or the settings page will get 403 (ADR-0009 `:56`).
3. The 1.1 manual follow-ups 1-3 and 6 are unchanged.

## Lead disposition

1.3 accepted (no open CRITICAL or HIGH). The Round 7 correction cycle is spent, so N1, N2 and N3 are carried as follow-up task 1.4 of `settings-security-review` for Round 8.
