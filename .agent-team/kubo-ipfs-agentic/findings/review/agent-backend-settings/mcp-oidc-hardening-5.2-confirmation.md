# 5.2 confirmation pass: mcp-oidc-hardening (after the correction cycle)

**Verdict: PASS.** The open MAJOR is fixed, and nothing in the correction reopens a compatibility concern.

Reviewer: `reviewer` (kubo-ipfs-agentic), in a separate context, read-only. I read `correction.patch` (788 lines, including the new `mcp/server/agui_subject_test.go`), the working-tree `uar_proxy.go`, `uar_backend.go`, `agent_sessions.go` and the `a2a_store.go` owner sites, the rerun README and `gate-rerun.log`. I could not run anything; every result below comes from the gate logs.

## A. Status of the 5.2 findings

| ID | Status | Evidence |
|---|---|---|
| 5.2-MAJOR-1 (`/agui` bound on the principal ID) | CLOSED | **Native agent.** `agentSession` now carries `owner = Identity.BindingKey()` alongside `principal` (`agent_sessions.go` struct hunk). `acquire` refuses `s.owner != owner` (`agent_sessions.go:166`), and `OpenInterrupt` checks `s.owner != id.BindingKey()` (`:226`). `Run` passes `eff.BindingKey()` (`agent.go:580`). Eviction is limited to the owner's own sessions (`:248`), while the quota stays per principal (`:204`). |
| | | **UAR proxy.** Threads are keyed on `uarThreadKey(owner, thread)` (`uar_proxy.go:238`), and `message` and `resume` pass `id.BindingKey()`. `makeRoomLocked` counts the quota per principal and evicts only the owner's idle threads, failing closed (`uar_proxy.go:265-284`). `uarThreadID` hashes the owner (`uar_backend.go:160-163`), so the digest key is never sent raw to UAR. |
| | | **Tests, all HTTP-level through the real JWT chain** (two subjects on one `ui` principal). B posting to A's thread gets 404 / `CodeNotFound`, and A continues. B's approve, deny, cancel and wait on A's interrupt each get 404; A's confirmation stays `StatePending` with zero `pin/add`, and A's own approval then executes it. B filling the shared quota never evicts A's thread. The UAR rule is tested at the unit level. |
| | | **A2A.** `a2a_store` still owns contexts by `id.ID`. That is correct: `KindA2A` takes no `subjects`. |
| 5.2-MINOR-1 (stale warning after an idle period) | CLOSED | `lookup` no longer logs (`jwks.go:122-127`). `reportStale()` runs only on the two final-stale paths: inside the refetch window (`:95`) and after a failed fetch (`:104`). It also guards against a zero `fetchedAt`. New `TestJWKSIdlePastCapWithIssuerUpLogsNothing`: idle 10 h past a 2 h cap with the issuer up gives accept, 2 hits and zero stale records. The existing once-per-period test is unchanged and passes. |
| 5.2-MINOR-2 (long TTL configs fail with a `""` error) | CLOSED | When the key is absent, the error reads `absent (the default is 24h)` and states the minimum (`oidc_config.go:122-127`). `TestJWKSMaxStaleAbsentWithLongTTLNamesTheDefault` asserts "absent", "24h" and "48h", and that no `""` appears. The behavior change is now in the changelog (`v0.44.md:119`), the config-reference row, the schema `offSwitch`, the user guide, ADR D2 and the design Migration Plan. |
| 5.2-MINOR-3 (doc row cited the wrong test) | CLOSED | The `config-reference.md` row and `config-reference.tests.json` now name `TestJWKSMaxStaleConfig`, `TestJWKSMaxStaleCapsHeldKeys` and `TestJWKSMaxStaleDisabledFetchesNothing`. Task 2.2's acceptance text is corrected and annotated (`tasks.md:41`). |
| 5.2-MINOR-4 (`lke-agent-settings` contradicted itself on 404) | CLOSED | `lke-agent-settings/proposal.md:60` now names `agent.settings.enabled: false` as the off switch that gives 404, and records the no-admin answer as observed in 1.1. This matches `tasks.md` 1.1 and `settings.go:178-183`. |
| 5.2-NIT-A (task boxes) | CLOSED | Boxes 3.2 and 4.1 are ticked (coordinator statement; the 4.1 evidence README exists with a rerun section). |
| 5.2-NIT-B (`NO_PROXY` host) | CLOSED | `docs/environment-variables.md:275` now says the "`jwks_url` host ... (it can differ from the issuer host)". |
| 5.2-NIT-C (stale comment on `UserID`) | CLOSED | `auth.go:52-56` describes the binding key in `TokenInfo.UserID`. |
| 5.2-NIT-D (Warn hidden in `adoptLogger`) | CLOSED | `ChainVerifier.warnOpenRoutes()` is now separate (`auth_chain.go:80-87`) and called once, explicitly, in `NewHTTPServer` after `adoptLogger` (`http.go:189`). |
| 5.2-NIT-E (ADR D4 omitted the digest) | CLOSED | ADR-0013 amendment D4 names the key as principal ID, `#`, and the first 128 bits of SHA-256(`sub`) in hex. It lists every binding site, including AG-UI, and states that limits are shared. |
| 5.2-INFO-3 (user guide missing from the Impact list) | CLOSED | `mcp-oidc-hardening/proposal.md` Impact now lists `docs/mcp/user-guide.md` and says why. |
| 5.2-INFO-4 (out-of-band cap shared per principal) | CLOSED, as documented | A comment at `broker.go:243-245` says the cap is shared on purpose. The changelog, user guide and ADR D4 all say the agent quota and the two-confirmation cap are shared across people on one principal. |
| 5.2-INFO-5 (gate evidence) | CLOSED, with one caveat (INFO-C2) | `gate-rerun.log:29-85` ran with `-v`. `=== RUN` lines show `TestDagJose` and all three `TestMCPSessionToken` subtests, including "off" and "routes absent". 14 top-level `--- PASS` lines; `ok test/cli`. Lint still exits 2, but only on the pre-existing `fuse/node/mount_darwin.go:29` ST1005 (`:17-21`), outside the diff. |

## B. Do the docs match the code?

- **Changelog** (`v0.44.md:119`). Covers: the long-TTL load failure and the no-stale-fallback minimum; per-person binding on `/mcp`, confirmations, elicitations and the `/agui` threads and interrupts; shared limits; that one person's runs never end the other's session; and that static bearers are unchanged. All of it matches the code above. Only Highlights changed.
- **ADR-0013 D2 and D4.** Both match the code: `oidc_config.go:122-127`; `principal.go` `BindingKey`; the `agent_sessions.go` and `uar_proxy.go` owner keys.
- **User guide.** The staleness and identity bullets match the code.
- **Config-reference row and schema `offSwitch`.** The wording is identical in both. The generator check passes per the rerun log.
- **Design Migration Plan.** Now lists both exceptions: the absent `routes`, and a TTL above 24 h with no `jwks_max_stale`.
- **`lke-agent-settings` proposal.** Consistent with its tasks and with `settings.go`.

## C. Anything new from the correction

| ID | Severity | Location | Note |
|---|---|---|---|
| INFO-C1 | INFO | `agent_sessions.go:163-168` | **Existence oracle on thread IDs.** Native agent sessions are keyed by session ID across all owners. B posting a new message to a thread ID A owns gets 404, while a fresh ID gets 200, so B can tell the thread exists. The test compares against a resume, not a create. This oracle already existed between principals and does not let B into the thread. The UAR proxy instead keys on owner plus thread, so B silently gets a thread of its own. The two backends behave differently, which is harmless. No action needed for 5.2. |
| INFO-C2 | INFO | `gate-rerun.log:29` | The `grep -E "^(=== RUN\|--- ...)"` filter drops the indented subtest `--- PASS` lines. Subtest success is inferred from the parent's `--- PASS` (a failing subtest fails its parent), so the evidence holds. A future rerun could keep the indented lines. |
| INFO-C3 | INFO | `uar_proxy_hardening_test.go`, `uar_round8_test.go` | Existing tests changed only to pass the new `owner` argument (`"chat","chat"`). For static principals, owner equals principal, so their semantics are unchanged. |

Nothing in the correction touches `/api/v0`, the gateway, the CID recipe, the wire protocols or any `go.mod`: the rerun shows `go.mod` unchanged and no change under `core/` or `client/`. No new config key, no new dependency.

## D. Not verified here

- All test and lint results: I relied on `gate-rerun.log`.
- `-race` and the full suite.
- Gateway conformance CI.
- The `azp` rule against a live Hydra ID token: deferred to `lke-agent-settings` 1.8.

5.2: PASS
