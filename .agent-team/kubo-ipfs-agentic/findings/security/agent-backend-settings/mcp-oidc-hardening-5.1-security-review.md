5.1: FAIL

# 5.1 Security review: mcp-oidc-hardening (closes 8.1 LOW-1 to LOW-5)

Reviewer: kubo-ipfs-agentic security-reviewer. I reviewed this in a separate context from the builders, by reading only (Read, Glob, Grep). I ran no tests, no build and nothing against the cluster.

**What I read:**
- Both patches.
- These files in full: `mcp/server/jwt.go`, `jwks.go`, `oidc_config.go`, `principal.go`, `auth.go`, `agui_resume.go`, and the new tests `jwks_stale_test.go`, `jwt_azp_test.go`, `jwt_binding_test.go`.
- These files in part: `broker.go` (200-320), `agent.go` (560-925), `agent_sessions.go` (143-253), `uar_proxy.go` (215-354, 939-975), `uar_backend.go`:160, `policy.go`:143-165, `principals.go`:345-364, and `cmd/ipfs/kubo/mcp_http.go` (205-305).
- go-sdk v1.8.0 `mcp/streamable.go`:566-572 and :710-716.
- The change spec, design and spec delta; the 8.1 findings; and the schema pattern for principal names.

**Verdict.** No CRITICAL or HIGH is open. The verdict is FAIL because LOW-3 is only partly closed, with no justification for the gap. The spec delta says "every confirmation binding SHALL be keyed on the principal together with the token's sub" (spec.md:105). The MCP session, the gate, elicitation and the confirm token now meet that. The AG-UI paths do not: threads, interrupts and interrupt approval are still keyed on the raw principal ID. AG-UI is the route `lke-agent-settings` opens next (see S-1). The changelog and ADR amendment state the property without qualification, so they are currently inaccurate.

## Status of 8.1 LOW-1 to LOW-5

| ID | Status | Evidence |
|---|---|---|
| LOW-1 (azp and several `aud`) | **CLOSED** (required part); the optional part is deferred with a recorded reason | `jwt.go`:65-66 reads `azp`. `jwt.go`:244-246 runs `azpOK` after go-jose's issuer, `aud` and time checks. `jwt.go`:262-267 does two things: with no `azp`, only one `aud` is accepted; with an `azp`, it must be one of `audiences`. It fails closed with the same generic 401 (`jwt.go`:269-272) and logs only the reason at Debug. A non-string `azp` fails JSON decoding in `parsed.Claims` and is rejected as `signature` (`jwt.go`:226-228), so that also fails closed. Tests: `jwt_azp_test.go`:31-55, all six cases from the spec. Residual: R-1 below. Rejecting by `scp`, `client_id` or `typ` is a recorded non-goal (design.md:23, :78; ADR amendment, "Deferred"). |
| LOW-2 (staleness cap) | **CLOSED** | `jwks.go`:121-127: past `maxStale` from the last good fetch, `lookup` returns no key, fresh or stale, on every path in `key()` (:79, :85, :92, :100, :105). `fetchedAt` is set only on success (:194), and the log latch is cleared only on success (:196). `NewJWTVerifier` replaces 0 with 24h, so the cap has no off value (`jwt.go`:127-130). The loader rejects values below `jwks_cache_ttl`, as well as 0, negative and unparsable values (`oidc_config.go`:119-124, :153-162). That check also runs while the verifier is disabled (`oidc_config.go`:97-100; `jwks_stale_test.go`:137-140). The clock is `time.Now`, a monotonic `Sub`, so wall-clock jumps do not move the cap. Tests: `jwks_stale_test.go`:34-94. Notes: S-3 and I-1 to I-3. |
| LOW-3 (per-subject binding) | **PARTIAL** | Closed for these: go-sdk session binding (`auth.go`:75; go-sdk `streamable.go`:569), broker dedupe (`broker.go`:296), the gate's repeat lookup (`gate.go`:245), confirm-token minting and acceptance (`gate.go`:344, :444), and elicitation state (`elicit.go`:71). The subject is stamped only on session identities (`jwt_session.go`:194). Tests: `jwt_binding_test.go`:22-63 and :92-152. **Open for AG-UI**, both the in-process agent and the UAR backend (S-1). |
| LOW-4 (default routes) | **CLOSED** | `oidc_config.go`:67-69 and :128-131: an absent `routes` now means `["session"]`, and `[]` means none. The startup Warn names both keys (`jwt.go`:159-168), runs once per `NewHTTPServer` (`auth_chain.go`:80; `http.go`:188), is skipped when `require_origin` is false (design.md:82), and startup continues. Tests: `jwt_routes_test.go` (patch :474-521); CLI test at `test/cli/mcp_session_token_test.go` (patch :743-753). |
| LOW-5 (missing tests) | **CLOSED** | Redirect refused, and the target receives nothing: `jwks_stale_test.go`:214-232. The test would fail if the redirect were followed, because the target serves valid keys. Body over 1 MiB gives "not a key set": `jwks_stale_test.go`:235-250. An uncapped read would instead give "no usable signing key", so the test tells the two apart. Several-`aud` cases: `jwt_azp_test.go`:37-39. |

## Findings

| ID | Severity | file:line | Issue | Failure scenario | Recommendation |
|---|---|---|---|---|---|
| S-1 | **MEDIUM** (keeps LOW-3 open) | `agent.go`:580 (`a.acquire(in.SessionID, eff.ID, ...)`); `agent_sessions.go`:159, :218; `agui_resume.go`:22, :54; `uar_proxy.go`:321, :343 | AG-UI agent sessions and UAR threads are owned by `id.ID`, the principal name, not by `BindingKey()`. That covers thread ownership, `OpenInterrupt`'s owner check, and the approval path in `approve`, which calls `ResolveConfirmation(open.ConfirmationID, ResolverAGUIInterrupt, true)` by ID. That path never reaches `gate.acceptToken` or `elicitAnswer`, where the new binding applies. | `lke-agent-settings` lists `agui` in routes. ADR-0013 decision 2 expects more than one console identity under RBAC and `ui`. Two Kratos subjects A and B are mapped to `console-ui`, and B learns A's AG-UI `threadId` (a log, a screenshot or a shared device). Three things then become possible. (1) B posts a message on A's thread. `acquire` sees the same principal and continues A's session, so the model answers B with A's conversation history, including any IPFS content A retrieved. (2) A's run is waiting on an interrupt that does not need the system prompt. B resumes it with `approved: true`. `OpenInterrupt` passes and A's broker record moves to approved (`agui_resume.go`:54). The record keeps A's dedupe key, so A's next identical call on that thread runs at `gate.go`:245 without A approving it. B can also deny or cancel A's pending writes. (3) The per-principal run, session and external-prompt quotas are shared (`agent_sessions.go`:167-171, `broker.go`:251, :276), so one subject can starve the other. This breaks spec.md:105 and :111-113 on the AG-UI route. `jwt_binding_test.go` covers only `/mcp` and direct gate calls. Not reachable with today's deployed config: `routes: [session]` and one subject. | Key AG-UI ownership on `BindingKey()`. That means `agent.go`:580 (`eff.BindingKey()`), `agent_sessions.go`:218 (`id.BindingKey()`) and `uar_proxy.go`:321 and :343. `uarThreadID` already hashes its inputs (`uar_backend.go`:161), so no subject reaches UAR. Add a test with two subjects on one `ui` principal: B posting on A's thread gets `ErrSessionNotFound`, and B resuming A's interrupt is refused while A's confirmation stays pending. Quotas can stay per principal if that is documented. Alternatively, use the D4 fallback "one subject per write-granted principal" at load, recorded by the architect. Until one of these lands, qualify the claim in `docs/changelogs/v0.44.md` (patch :19) and the ADR amendment D4 (patch :93). |
| R-1 | LOW (residual of LOW-1, accepted in the design) | `jwt.go`:262-267 | A token with exactly one `aud` and no `azp` passes. Hydra JWT access tokens carry `client_id` and `scp`, not `azp`. | An operator switches Hydra to `strategies.access_token: jwt`. A Hydra client allowed the audience `ipfs-console` gets a user's access token with `aud: ["ipfs-console"]`. It passes D1 and authenticates as that user's `sub` on any listed route the gateway does not cover. Preconditions: two changes on the Hydra side, and a path that bypasses Envoy. | Already deferred. Revisit once a live Hydra ID token's claims have been read, and before any route beyond `session` is reachable without the gateway in front. |
| S-3 | LOW | `jwks.go`:121-125, :79 | The "expired" Warn fires on the first lookup after idle, even when the refetch that follows succeeds. `fetchedAt` advances only on a fetch, and fetches happen only on demand. So a key set nobody has needed for 24h is "stale" on the next request, and `onStale` logs "session tokens are refused until it is fetched again". Then `key()` refetches and the request authenticates. | The console sees no session traffic over a weekend. Monday's first sign-in logs a Warn reporting a refusal that never happened. Operators learn to ignore the one log line that should report a real outage (spec.md:72, :76). | Log from `key()` only when the stale lookup is followed by a failed or rate-limited fetch. Alternatively, have `lookup` report staleness and let the caller decide. Add a case for an idle period longer than `max_stale` with the issuer reachable: no Warn. |
| I-1 | INFO | `oidc_config.go`:119-124 | A config that sets `jwks_cache_ttl` above 24h and leaves `jwks_max_stale` absent loaded before this change and fails now. The message quotes the absent value as `""`. | An upgraded node fails at startup. It fails closed and loudly, so availability is the only impact, and no deployed config is affected (design.md:74). | Say "absent (default 24h)" in the error. Add a changelog line saying `jwks_max_stale` must be raised when `jwks_cache_ttl` is above 24h. |
| I-2 | INFO | `oidc_config.go`:122 | `jwks_max_stale == jwks_cache_ttl` is allowed. It removes the outage grace entirely: a single failed refetch at TTL expiry gives 401 for every JWT until a fetch succeeds, retried every 30 s. The comparison is also `>=` (`jwks.go`:122), so at exactly the bound the key is refused, where the spec says "more than". The difference is nanoseconds. | Operator-chosen and documented ("a larger value keeps held keys usable longer"). | None required. Optionally note it in `docs/mcp/config-reference.md`. |
| I-3 | INFO | `jwks.go`:121, :128 | Age uses the monotonic clock. On Linux and macOS that clock does not count time spent suspended. | A Desktop host that sleeps for days wakes with keys it treats as fresh, so a revoked key's 24h bound stretches by the sleep time. The LKE pod is unaffected. TTL already behaved this way before this change. | None for LKE. If OIDC is ever enabled on Desktop, compare wall-clock time (`Round(0)`) for the cap. |
| I-4 | INFO | `jwks.go`:82, :159 | `context.WithoutCancel` is correct and bounded. The fetch runs synchronously in the triggering request's goroutine, under `fetchMu`, with a 10 s timeout, and no goroutine is spawned. `TestJWKSFetchStaysBoundedByTheTimeout` (`jwks_stale_test.go`:200-211) pins the timeout. Waiters on `fetchMu` ignore their own cancellation for up to 10 s. That predates this change. A request whose client left now holds its goroutine for up to 10 s instead of returning early. Single-flight (`fetchMu` plus the re-lookup at :85) and the 30 s attempt limit (:91) are unchanged. | Bounded: at most one fetch per 30 s, each at most 10 s. Server shutdown waits up to 10 s for an in-flight fetch. | None. |
| I-5 | INFO | `principal.go`:45-51; `jwt_azp_test.go`:41 | (a) `BindingKey` is the principal ID, `#`, and the first 128 bits of SHA-256 of `sub`. A collision between two subjects is infeasible (birthday bound around 2^64, and Kratos assigns the subjects). The key is unsalted, but the confirm token that carries it goes only to the same subject, and the 404/403 bodies hold no `#` (`jwt_binding_test.go`:52-58). A static principal cannot be named `x#...` because the schema forbids `#` (schema:137, `^[a-z0-9][a-z0-9_-]{0,63}$`). I did not confirm that the Go loader enforces that pattern; it is operator-controlled either way. (b) An `azp` that is a configured audience but not in the token's `aud` is accepted. That is acceptable only because every configured audience is an operator-trusted client. | No exploit without operator misconfiguration. | None. |

## Other things I traced that are sound

- **Fail-open paths.** I found none.
  - Route scope is checked before validation (`jwt.go`:283-286).
  - Every `validate` failure returns `errTokenInvalid`.
  - Origin is checked after validation (`jwt.go`:291).
  - A key-set outage with no key held gives 401 (`jwt.go`:217-219).
  - Past the cap, the result is 401 whether the issuer is reachable but rate-limited (`jwks.go`:91-95) or unreachable (:97-103).
  - Static bearers do not depend on the JWKS (`jwks_stale_test.go`:80-81).
- **Disabled state.** With `enabled: false`, `OIDCSettings` is nil, so no verifier is built and nothing is fetched (`jwks_stale_test.go`:144-154; `mcp_http.go`:238-256). The CLI test keeps the session principal configured while the verifier is off (patch :734-740).
- **Startup.** An all-session-only config with OIDC off now fails and names `auth.oidc.enabled` and the principal, never a value (`mcp_http.go`:219-224; patch :17-41).
- **Log redaction.**
  - The stale Warn logs age, the cap and the URL with userinfo removed (`jwt.go`:140-143).
  - An `azp` refusal logs only `reason=azp`, never the token (`jwt_azp_test.go`:51-55).
  - The `/auth/session` encode error is logged at Debug with the error text only (`auth_session.go`, patch :118-121).
  - `Principal.sub` is unexported, so `encoding/json` never writes it into `Pending` or any other response. I found no slog call that formats a `Principal` struct.
- **Error messages.** Refusals stay generic. "The confirm token belongs to another principal" names no ID (`gate.go`:444). The go-sdk mismatch body is the fixed string "session user mismatch" (`streamable.go`:570).
- **Proxy documentation (D6).** It matches the code: the default client has no `Transport`, so `http.DefaultTransport` honours the proxy variables (`jwks.go`:60-66).

## MCP control summary (scoped to this change)

| Control | Status | Justification |
|---|---|---|
| MCP-01 | PASS on the deployed routes | Per-request verification, plus the `azp` rule (`jwt.go`:244-246, :278-299). On AG-UI, two people mapped to one principal are not isolated from each other (S-1). |
| MCP-02 | PASS for `/mcp` | go-sdk `UserID` is now `BindingKey()` (`auth.go`:75; `streamable.go`:569; `jwt_binding_test.go`:22-63). AG-UI's own threads: FAIL for multi-subject principals (S-1). |
| MCP-03 | FAIL (carried, a non-goal) | No in-process rate limit on bearer verification (8.1 INFO-1). |
| MCP-04 | N/A | No tool schema changes. |
| MCP-05 | PASS | `modelcontextprotocol/go-sdk` v1.8.0. |

**RCE vectors.**
- SSRF: SAFE. The JWKS URL comes only from config, must be https or loopback, and follows no redirect (`oidc_config.go`:185-199; `jwks.go`:65; test :214-232).
- Deserialization: SAFE. `encoding/json` with a 1 MiB cap (`jwks.go`:180) and a 16 KiB token cap (`jwt.go`:24).
- Command injection, eval, SSTI, path traversal: N/A. This diff adds no such sink.
- Dependency hijacking: unchanged (8.1 INFO-4, the p2p-forge fork `replace`, now disclosed in the changelog).

## AGENTS.md rules

- **No tracking.** Nothing new reports out. The only outbound request is the operator-configured JWKS fetch.
- **Off switches.**
  - `jwks_max_stale` has no off value by design. The off switch of the whole feature, `auth.oidc.enabled: false`, is documented.
  - The routes default narrows; nothing widens.
- **No silent CID injection.** N/A. No blockstore, pinset or provide path is touched.
- **`/api/v0`, gateway, CID recipe and wire.** Untouched by this diff.

## What closes 5.1

1. Fix S-1: key AG-UI and UAR threads, interrupts and `OpenInterrupt` on `BindingKey()`, and add the two-subject AG-UI tests. Alternatively, have the architect record the D4 fallback (one subject per write-granted principal, enforced at load) and qualify the changelog and ADR wording.
2. Optional in the same correction cycle: S-3 (no false stale Warn after idle) and the I-1 message.

## Not verified

- That any test passes. I ran nothing. The 4.1 evidence README was not re-observed.
- That the Go loader enforces the schema's principal-name pattern.
- Hydra's real ID-token claims: whether `azp` is present and what value it holds (the D1 risk named in ADR "The uncomfortable thing"). Also that `strategies.access_token` is still `opaque` (R-1).
- How the Web UI generates `threadId`. `mcp/chat/src/island/session.ts`:121 calls `deps.ids.thread()`; I did not read whether it is a CSPRNG. Unguessable thread IDs lower S-1's likelihood but do not remove it.
- The deployed sidecar config and subjects, and the Envoy and NetworkPolicy posture (carried from 8.1).
