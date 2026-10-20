5.1: PASS

# 5.1 confirmation pass: mcp-oidc-hardening correction cycle

Reviewer: kubo-ipfs-agentic security-reviewer, in a separate context from the builders. I read code only: `correction.patch` plus the working-tree `agent_sessions.go` (1-260), `agent.go`:580, `uar_proxy.go` (216-350, 596), `policy.go`:96-108, `agui_resume.go`, and the gate evidence. I ran nothing.

All three items and LOW-3 are now closed. The correction adds no CRITICAL or HIGH, and no path is still keyed on the raw principal ID where ownership matters.

## Status

| Item | Status | Evidence |
|---|---|---|
| **S-1** (AG-UI binding) / **LOW-3** | **CLOSED** | See the detail below. |
| **S-3** (false stale Warn after idle) | **CLOSED** | `lookup` no longer logs (patch :353-357). `reportStale` runs only where the stale answer is final: the refetch window forbids a fetch (`jwks.go` patch :337) or the refetch failed (:345). It logs only when a key set is held and its age is at least `maxStale`, once per period, with the latch cleared on a successful fetch (`jwks.go`:196). The idle-then-successful-refetch path at :105 never calls it. `TestJWKSIdlePastCapWithIssuerUpLogsNothing` (patch :395-420) would fail against the old code. `TestJWKSMaxStaleCapsHeldKeys` (:34-94) still expects exactly one record on a real outage, and the new call sites still produce one. |
| **I-1** (unclear load error) | **CLOSED** | `oidc_config.go` (patch :439-443): with the key absent, the error says "absent (the default is 24h)", names the TTL and the minimum, and no longer prints `""`. Pinned by `TestJWKSMaxStaleAbsentWithLongTTLNamesTheDefault` (patch :422-429). The changelog, ADR D2, config reference, schema `offSwitch` and design Migration Plan all disclose that a TTL above 24h now needs an explicit `jwks_max_stale` (patch :10, :36, :52, :93, :608-610). |

### S-1 / LOW-3 detail

**Agent sessions**
- Each session now stores two keys: `principal`, which keys the quotas, and `owner`, which is `BindingKey()` (`agent_sessions.go`:65-66).
- `Run` passes both, `eff.ID` and `eff.BindingKey()` (`agent.go`:580). `withoutWrites` copies `Principal` whole, `sub` included (`policy.go`:98). So a session identity whose writes were withdrawn keeps its owner key; it does not fall back to the bare principal ID.
- `acquire` compares the owner (`agent_sessions.go`:166) and stores it (:185).
- `OpenInterrupt` compares `s.owner` with `id.BindingKey()` (:226).

**Resume paths.** All four resume variants (approve, deny, cancel, wait) go through `OpenInterrupt` first (`agui_resume.go`:22, before the switch at :31-38). So B never reaches `ResolveConfirmation` (:54, :77) or `awaitDecision` on A's thread.

**Eviction and quota**
- `makeRoom` counts the quota per principal (:204) but evicts only the caller's own sessions (`evictOldestIdle(owner)`, :248). If the caller has nothing to evict, the result is `ErrTooManySessions`.
- The per-principal quota bounds the table, so sharing it cannot be used to get past the cap.

**UAR backend**
- Threads are keyed on the owner (`uarThreadKey(owner, thread)`, patch :470/:485).
- Eviction is owner-only (`uar_proxy.go`:272), and the quota is counted per principal.
- Both call sites pass `id.BindingKey()` (`uar_proxy.go`:326, :348).
- The thread ID UAR sees hashes the owner (patch :498; `uar_proxy.go`:596 on abandon). UAR receives only a hash, never `sub`.

**Broker external cap.** It stays keyed per principal on purpose: it is a shared limit that fails closed, and a comment now says so (`broker.go`, patch :290-292).

**Tests, checked for whether they would pass without the fix**
- `TestAGUITwoSubjectsOnePrincipalCannotShareAThread` (patch :687-699): with the old principal key, B's post would get 200. The test also checks that B's 404 body is the same as the body for a thread that does not exist, so the thread's existence is not revealed.
- `...CannotAnswerEachOthersInterrupt` (:703-730): with the old code, B's approve would pass `OpenInterrupt` and change A's broker state. The test asserts the state is still `StatePending`, that no `pin/add` happened, and that A can still approve.
- `...NeverEvictEachOther` (:733-755): the run burst is 120 (`agent_sessions.go`:26) and the test makes 21 runs. So it really reaches the session quota of 16, not the rate limit. The old principal-keyed eviction would drop `thread-a` first.
- `TestUARThreadsAreOwnedAndEvictedPerSubject` (:759-787) calls the backend directly, because no UAR agent is available to drive it over HTTP. That is acceptable, since the HTTP handlers' call sites were read above.

**Docs.** The changelog (patch :10), ADR D4 (:39) and the user guide (:80) now describe the AG-UI binding accurately, and state the shared limits plainly.

## New observations from the correction

| ID | Severity | file:line | Note |
|---|---|---|---|
| C-1 | INFO | `agent_sessions.go`:204-206; `uar_proxy.go`:268-284 | Two people on one principal can now block each other. Before, either one's new session could evict the other's idle session, because the two were indistinguishable. Now subject B can hold all 16 sessions of the shared principal quota, or use up its shared run budget of 120 runs a minute. Subject A, with no idle session of its own to evict, then gets `ErrTooManySessions` or 429. This only affects availability, fails closed, and is documented (changelog patch :10, ADR D4 :39). Optionally, give each subject a sub-quota later. |
| C-2 | INFO | `agui.go` watches map (`agui_resume.go`:103-106) | The watch map is keyed on the thread ID alone. It is reached only after the owner check in `OpenInterrupt`, and thread IDs are unique across the whole session table (`agent_sessions.go`:164-168), so two people cannot collide on it. Noted for completeness only. |
| C-3 | INFO | `http.go` (patch :325); `auth_chain.go` (patch :277-281) | Moving `warnOpenRoutes` out of `adoptLogger` keeps it running once per `NewHTTPServer`, after the logger is set. Behaviour is unchanged. |

Unchanged and still accepted: R-1, the residual of LOW-1 (single-`aud` tokens shaped like access tokens, deferred), and I-2 to I-5 from the first pass.

## Not verified

- **That the new tests passed.** `gate-rerun.log` ran `go test ./mcp/server/...` without `-v`, so the log does not list the new test names. The claim that `TestAGUITwoSubjects*`, `TestUARThreadsAreOwnedAndEvictedPerSubject`, `TestJWKSIdlePastCapWithIssuerUpLogsNothing` and `TestJWKSMaxStaleAbsentWithLongTTLNamesTheDefault` passed rests on the package-level PASS reported in `README.md`:26. I did not see it per test.
- **Carried from the first pass:** the `azp` value in a live Hydra ID token (the task 1.8 check), the deployed config, and the Envoy and NetworkPolicy posture.

5.1: PASS
