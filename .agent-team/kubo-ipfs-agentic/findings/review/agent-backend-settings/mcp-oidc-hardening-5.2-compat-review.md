# 5.2 Quality and compatibility review: mcp-oidc-hardening

Reviewer: `reviewer` (kubo-ipfs-agentic), working in a separate context from the builders, read-only (Read, Glob, Grep). Inputs read:

- `code.patch` and `docs-contract.patch` under `/private/tmp/claude-501/-Volumes-T7-Projects-references-kubo/43b10494-d504-43f2-9ba1-af8ce53bff62/scratchpad/review-hardening/`;
- the new untracked files the patch omits: `mcp/server/jwks_stale_test.go`, `jwt_azp_test.go`, `jwt_binding_test.go`, `test/cli/dag_jose_test.go`, `mcp/contract/config/invalid/oidc-max-stale-below-ttl.json`;
- the working-tree sources cited below, and the change's proposal, design, tasks and spec delta;
- the revised `lke-agent-settings` proposal and tasks;
- the 4.1 gate evidence (`README.md` and `gate.log`).

I could not run any command. "Passes" below comes from the gate log or from reading code and tests, never from a run I saw.

**Summary.** Nothing changes `/api/v0`, the HTTP Gateway, the default CID recipe or the wire protocols. All ten 8.2 findings are closed. One new MAJOR is open: the spec's per-subject binding requirement is not met on `/agui`, which is the route `lke-agent-settings` is about to open to session tokens.

## A. Status of the 8.2 findings

| ID | Status | Evidence |
|---|---|---|
| 8.2-MAJOR-1 (p2p-forge replace not disclosed) | CLOSED | `docs/changelogs/v0.44.md:18` (TOC) and `:121-123` (Highlights). The note names the fork, all three modules, go-jose v4.1.6, that `go install ...@` breaks (and why: Go refuses a module with a `replace`), the `make build` alternative, p2p-forge#96, and the "only once that release requires go-jose v4.1.6 or later" condition. Only the TOC and Highlights changed; the Changelog and Contributors sections did not. |
| 8.2-MINOR-1 (proxy statement false) | CLOSED | `docs/environment-variables.md:272-277` names the JWKS fetch as honoring the proxy variables. This matches the code: production builds the verifier with `Client` nil (`cmd/ipfs/kubo/mcp_http.go:260-264`), so `newJWKSCache` uses an `http.Client` with a nil Transport, i.e. `DefaultTransport` (`mcp/server/jwks.go:60-67`). A wording issue remains (NIT-B). |
| 8.2-MINOR-2 (subjects off-switch wording) | CLOSED | The schema `offSwitch` (`ipfs-mcp-config.schema.json` hunk at line 287) and `docs/mcp/config-reference.md:93` both use the agreed wording. The gate's `config_reference.py --check` passed (`gate.log:76-79`). |
| 8.2-MINOR-3 (real-binary off test dropped the session principal) | CLOSED in code; the task box is still unchecked (NIT-A) | `test/cli/mcp_session_token_test.go:153-170` keeps both `chat` and `console-ui` and asserts 401 on `/agui`, 404 on `/auth/session` and zero issuer hits. The new "routes absent" subtest is at `:172-182`. |
| 8.2-MINOR-4 (fetch cancelled with the request) | CLOSED | `jwks.go:159` uses `context.WithTimeout(context.WithoutCancel(ctx), jwksFetchTimeout)`. `TestJWKSFetchSurvivesCancelledRequest` (`jwks_stale_test.go:158-190`) tells the two behaviors apart: before the fix, the second request would land in the 30 s window with no key. `TestJWKSFetchStaysBoundedByTheTimeout` (`:200-211`, synctest) passes an already-cancelled context and asserts the fetch ends at exactly `jwksFetchTimeout` with `DeadlineExceeded`, which proves both properties. |
| 8.2-MINOR-5 (untested JWKS safeguards) | CLOSED | Redirect: `jwks_stale_test.go:214-232`, where the second server records zero hits. 1 MiB bound: `:235-250`, which asserts "not a key set"; the body uses `"keys":[]`, so a body that was not truncated would fail with a different message. Cancel: as above. Both cases go through the default client (`newTestJWTVerifier` sets no `Client`, `jwt_test.go:24-36`). |
| 8.2-MINOR-6 (empty credential list in the startup error) | CLOSED | `cmd/ipfs/kubo/mcp_http.go:280-303` names the session-only principals and `auth.oidc.enabled`. The test asserts both and asserts the empty-list text is gone (`mcp_http_oidc_test.go:94-96`). It is only reachable with OIDC off (`mcp_http.go:219-221`). |
| 8.2-MINOR-7 (dag-jose untested under go-jose v4.1.6) | CLOSED | `test/cli/dag_jose_test.go`: dag-json in, dag-jose stored, the codec asserted as 0x85, the block re-put with `--input-codec dag-jose` gives the same CID and bytes, and `dag get` agrees. I checked go-dag-jose v0.1.1: the general-JWS encode path calls `unflattenJWS`, then `ipldNodeToGoPrimitive` and `goPrimitiveToIpldNode` (`jose_utils.go:217,222`), which use `github.com/go-jose/go-jose/v4/json`. go-jose is really exercised. The test comment "parses JWS with go-jose" overstates it slightly: only go-jose's `json` package is used. |
| 8.2-NIT-1 (discarded encode error) | CLOSED | `auth_session.go:68-71` logs the error at Debug. The logger is wired at `http.go:214`. |
| 8.2-NIT-2 ("Kubo gains no code") | CLOSED | The ADR-0013 Stability bullet is reworded (docs patch line 64), and the phrase is gone. |

## B. New findings

| ID | Severity | Location | Issue and scenario | Recommendation |
|---|---|---|---|---|
| 5.2-MAJOR-1 | MAJOR | `mcp/server/agent.go:580` (`a.acquire(in.SessionID, eff.ID, ...)`), `mcp/server/agent_sessions.go:159,177,196,218` (`s.principal != id.ID`), `mcp/server/agui_resume.go:22,54` | **The spec says "every confirmation binding" is keyed per subject; `/agui` does not do this.** The requirement (spec.md:104-113) says two subjects on one principal "SHALL NOT be able to continue each other's MCP session or answer each other's pending confirmations". D4 was implemented only in the go-sdk `/mcp` binding, the broker dedupe key and the gate's confirm and elicitation tokens. AG-UI agent sessions and the island interrupt still bind on the principal ID: `OpenInterrupt` admits any identity with `id.ID == s.principal`, and `approve` then calls `ResolveConfirmation(open.ConfirmationID, ResolverAGUIInterrupt, true)` with no subject check. Scenario: A and B are both listed in `console-ui.subjects`, and routes are `["session","agui"]`, which is exactly the `lke-agent-settings` 1.1 config. B posts to `/agui` with B's own valid token, A's `threadId` and A's interrupt ID. B then approves A's pending write. The same check lets B continue A's thread, which feeds A's conversation snapshot into B's run. B also shares A's session quota, so B's new chat can evict A's idle thread through `evictOldestIdle(principal)` (`agent_sessions.go:193-205`). Exploiting this needs A's thread ID (chosen by the client) and A's interrupt ID, but the requirement is about binding, not about how secret the IDs are. This route is the reason `lke-agent-settings` waits on this change, and that change's proposal (patch line 415) treats per-subject binding as delivered. Tests cover only `/mcp` and in-memory gate calls (`jwt_binding_test.go`), so nothing catches the gap. | Key AG-UI sessions and `OpenInterrupt` on `Identity.BindingKey()`: pass `eff.BindingKey()` at `agent.go:580`, and compare `s.principal != id.BindingKey()` at `agent_sessions.go:218`. Decide whether `takeRun` and `makeRoom` quotas stay per principal (a shared cap is defensible; cross-person eviction is not). Add an HTTP-level test: two subjects on one `ui` principal with routes `[agui]`; B resuming A's thread or interrupt gets the not-found answer, and A's confirmation stays pending. Alternatively, narrow the spec requirement to `/mcp`, and add a config-load rule allowing one subject per `ui` principal while `agui` is routed. Either way the architect amends the spec, and `lke-agent-settings` 1.1 asserts the result. |
| 5.2-MINOR-1 | MINOR | `mcp/server/jwks.go:79` with `:121-126` | **False stale-key warning after an idle period.** `key()` calls `lookup(kid, true)` before any fetch. Once `fetchedAt` is older than `jwks_max_stale`, that first lookup fires `onStale`, which logs Warn "session tokens are refused until it is fetched again". The fetch that follows then succeeds and the token is accepted. Scenario: a home or LKE node gets no session-token traffic for 25 h (a weekend). The first sign-in on Monday logs a Warn that never happened, which teaches operators to ignore the one log line that reports a real outage. | Fire `onStale` only on the paths where the stale lookup is the final answer: after a failed fetch (`:100`) and inside the refetch window (`:92`). Do not fire it in the fresh probe. Add a case to `TestJWKSMaxStaleCapsHeldKeys`: idle past the cap, issuer up, token accepted, zero stale records. |
| 5.2-MINOR-2 | MINOR | `mcp/server/oidc_config.go:119-124`; design.md "Migration Plan" | **Existing configs with a long `jwks_cache_ttl` stop loading, and this is not disclosed.** A config valid under console-session-auth with `jwks_cache_ttl: "48h"` and no `jwks_max_stale` now fails at load. The error reads `auth.oidc.jwks_max_stale "" is out of range ...`, which names a key the operator never set, and the value is empty. The design's Migration Plan says old configs load unchanged except for an absent `routes`, so it is wrong on this point. The changelog says only "It cannot be shorter than `jwks_cache_ttl`". | Say in the changelog and the config-reference row that a TTL above 24 h now needs an explicit `jwks_max_stale`. When the value is empty, make the error print the effective default (`24h (default)`). Correct the Migration Plan. |
| 5.2-MINOR-3 | MINOR | `docs/mcp/config-reference.md:110` and `docs/mcp/config-reference.tests.json` (`auth.oidc.jwks_max_stale`); task 2.2 acceptance | **The doc row names the wrong tests.** The row cites `TestParseConfigOIDCRejections`, which has no `jwks_max_stale` case (`oidc_config_test.go:38-57`). It omits the tests that actually cover the key: `TestJWKSMaxStaleConfig`, `TestJWKSMaxStaleCapsHeldKeys` and the disabled-state `TestJWKSMaxStaleDisabledFetchesNothing`. The generator's check only confirms that the named functions exist, so it passed. Task 2.2's literal acceptance ("`TestParseConfigOIDCRejections` rejects a cap below the TTL") is not what was built; the behavior is covered by `TestJWKSMaxStaleConfig`. | Replace `TestParseConfigOIDCRejections` with the three `TestJWKSMaxStale*` tests in `config-reference.tests.json`, then regenerate. Alternatively, add the below-TTL case to `TestParseConfigOIDCRejections`. |
| 5.2-MINOR-4 | MINOR | `openspec/changes/lke-agent-settings/proposal.md` (patch line 424) vs its own OQ-1 text (patch line 443), `tasks.md` 1.1 (patch line 473) and `mcp/server/settings.go:178-183` | **The revised `lke-agent-settings` contradicts itself.** The proposal's Stability list says removing the `admin` principal or its Secret "unmounts the settings route (404)". Elsewhere the proposal and its tasks say the answer in that state (404 or 401) "is not pinned by any test" and must be observed. The code mounts the settings route whenever the settings store exists (`settings.go:178-183`), not when an admin principal exists, so 404 is unlikely. Otherwise the revision agrees with this change: routes `["session","agui"]`, the allowlist is set (so the D5 Warn does not fire), and the admin principal is static with no `subjects` (static binding under D4). | Reword the off-switch bullet to the observed-answer wording that 1.1 already uses, or name the actual off switch (`agent.settings` off). |
| 5.2-NIT-A | NIT | `openspec/changes/mcp-oidc-hardening/tasks.md:64` (3.2), `:73` (4.1) | 3.2 is implemented and appears in the gate's CLI run, but its box is unchecked. 4.1 has its evidence README, but its box is unchecked. | Tick them, or say why they stay open. |
| 5.2-NIT-B | NIT | `docs/environment-variables.md:275` | "list the issuer host in `NO_PROXY`" is not quite right: the fetch goes to the host in `auth.oidc.jwks_url`, which can differ from the issuer host. | Say "the `jwks_url` host". |
| 5.2-NIT-C | NIT | `mcp/server/auth.go:52-54` | The comment still says the identity's "ID in TokenInfo.UserID". It is now `BindingKey()`. | Update the comment. |
| 5.2-NIT-D | NIT | `mcp/server/auth_chain.go:77` | `adoptLogger` now also emits the D5 startup Warn, a side effect its name does not suggest. The Warn depends on `NewHTTPServer` calling `adoptLogger` exactly once. | Call `warnOpenRoutes` explicitly from the listener setup, or rename. |
| 5.2-NIT-E | NIT | `docs/mcp/0013-console-session-auth.md` amendment D4 (docs patch line 93) | The ADR says the binding key is "the principal ID plus the `sub`". The code uses a SHA-256 digest of `sub` (`principal.go:45-51`), and only `design.md`'s implementation note says so. | Add one sentence to the ADR amendment. |
| 5.2-INFO-1 | INFO | Changelog `v0.44.md:117`; `version.go:31` (`0.44.0-dev`) | **Routes default narrowed from 4 routes to `[session]`.** The changelog discloses it with the exact list that restores the old behavior, and `[]` is documented. A `!` marker is not needed: the old default never shipped in a tagged release (v0.44 is `-dev`), and the one deployed config sets `routes` explicitly (design Context). If this fork ever tagged a v0.44 release with the old default, the merge commit would need `!`. | None, provided the merge lands before any v0.44 tag. |
| 5.2-INFO-2 | INFO | Schema `jwks_max_stale` `offSwitch`; changelog `:119`; user guide | **The `jwks_max_stale` off-switch wording is accurate.** It says no value removes the cap, and that `auth.oidc.enabled: false` turns off the whole verifier. This matches the code: `0` and negative values fail at load (`oidc_config.go:122`, `parseOIDCDuration` at `:153-161`); a hand-built zero becomes the default (`jwt.go:125-129`); and the contract check `maxStaleError` agrees. The disabled state is tested: `TestJWKSMaxStaleDisabledFetchesNothing` and the "validated while disabled" case in `TestJWKSMaxStaleConfig`. With `jwks_max_stale == jwks_cache_ttl` (allowed as the minimum), stale fallback is effectively off. That is consistent but not stated. | Optionally state that the minimum disables stale fallback. |
| 5.2-INFO-3 | INFO | `docs/mcp/user-guide.md` | **The user-guide edit is not scope creep.** `docs/mcp/**` is architect-owned, and the guide stated the old routes default, which would have become false. The proposal's Impact list does not name the file. | Add it to the Impact list for traceability. |
| 5.2-INFO-4 | INFO | `mcp/server/broker.go:245-276` | The cap of 2 open out-of-band confirmations (`openExternalLocked`) still counts per principal ID, so two people on one principal share it. This is a fail-closed shared cap, not a binding leak. | Record it next to the 5.2-MAJOR-1 fix. |
| 5.2-INFO-5 | INFO | `gate.log:66-67`, `:83-92` | **Gate-evidence caveats.** CLI tests ran without `-v`, so the log does not show that `TestDagJose` and the new `TestMCPSessionToken` subtests ran rather than being filtered out. `gmake -O test_go_lint` exited 2 on one ST1005 in `fuse/node/mount_darwin.go:29`, outside the diff. The README's reading of that ("PASS for this change", identical on master) is plausible, but I did not verify it. | Re-run the CLI selector with `-v` in the correction cycle. |

## C. Compatibility checks

- **`/api/v0`.** Code diffs are limited to `cmd/ipfs/kubo/mcp_http*.go`, `mcp/server/**` and `test/cli/**`. Nothing under `core/commands`, `core/coreiface`, `core/coreapi`, `client/rpc`, `config/` or `core/corehttp` (patch file list; `gate.log:80-81`). `ipfs mcp` stays a local CLI command.
- **Gateway.** No `core/corehttp` change and no boxo bump.
- **CID recipe.** No Import or add path touched. The gate ran `TestDefaultMatchesExpectedProfile` (per its log).
- **Wire.** No libp2p, bitswap or DHT change.
- **go.mod.** Unchanged (`gate.log:1-3`). No new `replace`.
- **Keys.**
  - `auth.oidc.jwks_max_stale` has a default (`24h`, the operator's ruling, quoted verbatim in `.prometheus/decisions.md:156` and in the ADR amendment), an off switch (accurate, INFO-2), a doc row (test list wrong, MINOR-3), a changelog line, and a disabled-state test.
  - `auth.oidc.routes` has a changed default, an off switch (`[]`), and tests (`TestJWTRouteScopeDefaultIsSessionOnly`, the `[]` case now including `/auth/session`, `TestConfigDefaults`, `TestParseConfigOIDCDefaultsAndOffByDefault`, and the real-binary "routes absent" subtest).
  - The Warn line is tested with captured slog (`TestJWTRoutesWarnWhenOriginAllowlistIsEmpty`).
- **Changelog.** Only the TOC and Highlights changed.

## D. Test quality and Go style

- **The tests assert behavior, not shape.** The `azp` table covers all four spec scenarios plus two accept cases, and asserts that refusals never log the token (`jwt_azp_test.go`). The stale-cap test covers inside the cap, past the cap, exactly one log record, a static bearer past the cap, recovery, and a second stale period (`jwks_stale_test.go:34-94`).
- **The `/mcp` binding test is end to end over HTTP.** It checks the 403 on a session mismatch, that no `sub` appears in the body or headers, and that A's session survives B's attempt. The confirmation-binding test is in-memory at the gate level, which is acceptable, but nothing covers `/agui` (MAJOR-1).
- **synctest and contexts.** synctest is used where timing matters and no real network is involved. The non-concurrent clock tests use an injected atomic clock instead, which is acceptable. `t.Context()` is used throughout.
- **Go style.** Errors are wrapped with `%w`, the code uses early returns and `slices.Contains`/`ContainsFunc`, no error is discarded, and gofmt is clean per the gate.
- **One test helper changed.** `http_mount_test.go:24-28` was edited because `UserID` changed meaning. Task 2.6 asked that broker and gate tests pass unmodified; they did, and only this echo helper moved.

## E. OpenSpec verification scorecard

| Dimension | Status |
|---|---|
| Completeness | 13 of 15 boxes ticked. 3.2 is done but unticked, 4.1 is done but unticked, and 5.x is pending by design. |
| Correctness | Token validation (azp), route scope default, bounded staleness, staleness configuration and fetch independence are implemented and tested. Per-subject binding is only partly implemented: `/mcp` and the gate are done, the AG-UI sessions and interrupt are not (MAJOR-1). |
| Coherence | D1, D2, D3, D5 and D6 are followed. D4 is followed for the broker and gate, but the spec is broader than D4's file list. The Migration Plan misses MINOR-2. |

## F. Not verified here

- Every test and lint result: I relied on `gate.log`.
- That `TestDagJose` and the new subtests ran: the log has no `-v` output.
- The `azp` rule against a live Hydra ID token: deferred to `lke-agent-settings` 1.8.
- The GQAdonis/p2p-forge fork contents, which the changelog describes as "v0.10.1 plus go-jose v4.1.6".
- Gateway conformance CI.
- What the settings route answers with no admin principal (MINOR-4).

## Relevant paths

- `/Volumes/T7/Projects/references/kubo/mcp/server/agent.go`
- `/Volumes/T7/Projects/references/kubo/mcp/server/agent_sessions.go`
- `/Volumes/T7/Projects/references/kubo/mcp/server/agui_resume.go`
- `/Volumes/T7/Projects/references/kubo/mcp/server/jwks.go`
- `/Volumes/T7/Projects/references/kubo/mcp/server/oidc_config.go`
- `/Volumes/T7/Projects/references/kubo/mcp/server/principal.go`
- `/Volumes/T7/Projects/references/kubo/mcp/server/jwt_binding_test.go`
- `/Volumes/T7/Projects/references/kubo/mcp/server/jwks_stale_test.go`
- `/Volumes/T7/Projects/references/kubo/docs/mcp/config-reference.tests.json`
- `/Volumes/T7/Projects/references/kubo/docs/environment-variables.md`
- `/Volumes/T7/Projects/references/kubo/openspec/changes/mcp-oidc-hardening/tasks.md`
- `/Volumes/T7/Projects/references/kubo/openspec/changes/lke-agent-settings/proposal.md`
- `/Volumes/T7/Projects/references/kubo/mcp/server/evidence/2026-10-20-mcp-oidc-hardening-4.1/gate.log`

5.2: FAIL
