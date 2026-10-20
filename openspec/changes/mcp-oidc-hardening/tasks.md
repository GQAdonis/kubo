# Tasks: mcp-oidc-hardening

Source findings: `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/8.1-verifier-security-review.md` (8.1) and `.agent-team/kubo-ipfs-agentic/findings/review/agent-backend-settings/8.2-console-session-auth-compat-review.md` (8.2). Design: `design.md` D1 to D6. Change-level prerequisite: `console-session-auth` complete. `lke-agent-settings` depends on this change being complete.

Stability rules that apply to every task (AGENTS.md):

- No change under `core/commands/`, `core/coreiface/`, `core/coreapi/`, `client/rpc/` or the gateway.
- No `go.mod` change and no new `replace`. The existing p2p-forge `replace` is operator-approved (ADR-0013:187) and is only disclosed here (task 1.5).
- Every new or changed key has a default, an off switch, a `docs/mcp/config-reference.md` row, a changelog line and a disabled-state test.
- No token, key or secret value appears in a test log, fixture name or doc.

## 1. Contract, ADR and docs (ipfs-agent-architect)

- [x] 1.1 Operator ruling on the `auth.oidc.jwks_max_stale` default (design D2; proposed `24h`, minimum `jwks_cache_ttl`, no value that removes the cap). Acceptance: the operator's words are quoted verbatim in `.prometheus/decisions.md` and in the 1.4 ADR amendment. If the ruling names another default, tasks 1.2 and 2.2 use that value and the spec delta is amended before 2.2 starts.
- [x] 1.2 Config schema `mcp/contract/config/ipfs-mcp-config.schema.json`:
  - add `auth.oidc.jwks_max_stale` with `default` from 1.1, `offSwitch` ("no value removes the cap; turn the verifier off with `auth.oidc.enabled: false`") and `ownerChange: mcp-oidc-hardening`;
  - change the `auth.oidc.routes` default to `["session"]`, with an `offSwitch` that says `[]` accepts a JWT on no route;
  - reword the `principals[].subjects` `offSwitch` per 8.2 MINOR-2: "With auth.oidc.enabled false, subjects authenticate nothing; the subject rules are still validated at load, and a config whose only principals are session-only fails startup.";
  - set `routes` explicitly in `examples/oidc-session.json`, and add `invalid/oidc-max-stale-below-ttl.json`.
  
  Acceptance: `go test ./mcp/contract/...` passes, and the invalid example is rejected with `auth.oidc.jwks_max_stale` named.
- [x] 1.3 Regenerate `docs/mcp/config-reference.md` with `docs/mcp/config_reference.py`. Acceptance: the generator's consistency check passes; the page shows `jwks_max_stale` with its default and off switch, `routes` defaulting to `["session"]`, and the reworded `subjects` off switch.
- [x] 1.4 ADR-0013 (`docs/mcp/0013-console-session-auth.md`): append a dated amendment that records design D1 to D6, the 1.1 ruling and the deferred `scp`/`client_id` question. Also fix the Stability wording per 8.2 NIT-2: "no `/api/v0` or gateway code changes; kubo's only edits are the WebUIPath bump and the dependency pins". Acceptance: the amendment names this change and each decision. `grep -n "Kubo gains no code" docs/mcp/0013-console-session-auth.md` returns nothing.
- [x] 1.5 `docs/changelogs/v0.44.md`, Highlights and TOC only:
  - 8.2 MAJOR-1: p2p-forge is replaced in all three modules by the fork `github.com/GQAdonis/p2p-forge` to get go-jose v4.1.6; `go install github.com/ipfs/kubo/cmd/ipfs@...` no longer works (build with `make build`); the replace is dropped when ipshipyard/p2p-forge#96 is released, and only once that release requires go-jose v4.1.6 or later;
  - the narrowed `auth.oidc.routes` default, with the explicit list (`["agui","settings","index","session"]`) that restores the old behavior;
  - `auth.oidc.jwks_max_stale` and its default.
  
  Acceptance: `grep -n "p2p-forge\|go install\|p2p-forge/pull/96\|jwks_max_stale\|\"routes\"" docs/changelogs/v0.44.md` matches each item in Highlights; the Changelog and Contributors sections are unchanged in `git diff`.
- [x] 1.6 `docs/environment-variables.md`: correct the proxy paragraph per 8.2 MINOR-1 and design D6. The JWKS fetch of `ipfs mcp` honors `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`; it carries no content, prompts or keys. This path is owned by no role; this change routes it to the architect. Acceptance: the paragraph names the JWKS fetch, and `mcp/server/jwks.go` still uses the default transport (no code change).

## 2. Verifier hardening (mcp-server-engineer, `mcp/server/**`)

- [x] 2.1 `azp` and multiple `aud` (8.1 LOW-1, design D1) in `jwt.go`. Acceptance: table cases added to `jwt_test.go` against the `httptest` issuer: several `aud` with no `azp` gives 401; several `aud` with a foreign `azp` gives 401; several `aud` with `azp` in `audiences` authenticates; one configured `aud` with a foreign `azp` gives 401. The refusals log no token, and `jwt_redaction_test.go` still passes.
- [x] 2.2 Staleness cap (8.1 LOW-2, design D2): parse `jwks_max_stale` in `oidc_config.go` (default from 1.1, rejected below `jwks_cache_ttl`, validated while disabled), and enforce it in `jwks.go` `lookup`, with one log line per stale period. Acceptance: tests with an injected clock show the following.
  - A held key works inside the cap while the JWKS server is stopped.
  - The same token gets 401 past the cap.
  - A static bearer still authenticates past the cap.
  - Exactly one log record reports the expiry.
  - A later successful fetch restores access.
  - `TestJWKSMaxStaleConfig` rejects a cap below the TTL (and validates it while disabled); `TestJWKSMaxStaleCapsHeldKeys` covers the cap and `TestJWKSMaxStaleDisabledFetchesNothing` the disabled state. (Corrected 2026-10-20 per 5.2 MINOR-3: the earlier text named `TestParseConfigOIDCRejections`, which has no `jwks_max_stale` case.)
  - With `auth.oidc.enabled` false and the key set, zero JWKS requests are made.
- [x] 2.3 Fetch on `context.WithoutCancel` (8.2 MINOR-4, design D3) in `jwks.go`. Acceptance: a test cancels the first request's context while a slow `httptest` JWKS is answering. A second request with a valid token, inside the 30 s window, authenticates. The fetch is still bounded by `jwksFetchTimeout`, shown by a hung server returning an error within the timeout.
- [x] 2.4 Tests for existing JWKS safeguards (8.1 LOW-5, 8.2 MINOR-5). Acceptance: a JWKS server answering 302 to a second valid key set gives 401 and the second server records zero hits; a key-set body over 1 MiB fails as "not a key set" and the token gets 401; both cases go through the default client, built with `JWTOptions.Client` nil.
- [x] 2.5 Default routes `[session]` and the startup Warn line (8.1 LOW-4, design D5) in `oidc_config.go` and the listener setup. Acceptance:
  - `TestJWTRouteScope` shows that with `routes` absent, a mapped token gets 401 on `/agui`, settings, `/index/*` and `/mcp`, and is accepted on `/auth/session`;
  - `routes: []` refuses `/auth/session` too;
  - a captured-slog test shows one Warn line naming `auth.oidc.routes` and `agent.origin_allowlist` when `routes` lists `agui` and the allowlist is empty, and none when the allowlist is set;
  - `TestParseConfigOIDCDefaultsAndOffByDefault` asserts the new default.
- [x] 2.6 Per-subject binding for session identities (8.1 LOW-3, design D4) in `auth.go`, `jwt_session.go`, `broker.go` and `gate.go`. Acceptance: tests with two subjects on one `ui` principal and `mcp` in `routes` show:
  - B presenting A's `Mcp-Session-Id` is refused;
  - B answering A's pending confirmation is refused and the call stays pending;
  - a static bearer session binds and continues unchanged (existing broker and gate tests pass unmodified);
  - no response body or header contains either `sub`.
  
  If D4 cannot be done without changing the confirmation contract in `mcp/contract/`, stop and report to the architect before implementing the design's fallback.
- [x] 2.7 8.2 NIT-1: in `auth_session.go`, log the `json.Encoder` error at Debug on the listener's logger instead of discarding it. Acceptance: `grep -n "_ = json.NewEncoder" mcp/server/auth_session.go` returns nothing, and the existing `/auth/session` tests pass.

## 3. Startup message and real-binary tests (mcp-server-engineer)

This change routes `cmd/ipfs/kubo/mcp_http.go`, `test/cli/mcp_session_token_test.go` and a new `test/cli/dag_jose_test.go` to mcp-server-engineer. These paths are owned by no role (AGENTS.md); console-session-auth set the precedent.

- [x] 3.1 8.2 MINOR-6: when every principal is session-only and `auth.oidc.enabled` is false, startup fails with a message that names `auth.oidc.enabled` and the session-only principals, not an empty credential list. Acceptance: a case in `cmd/ipfs/kubo/mcp_http_oidc_test.go` asserts the message contains `auth.oidc.enabled`, and `TestHTTPStartupSessionOnlyPrincipals` still passes.
- [x] 3.2 8.2 MINOR-3, plus the new defaults through the real binary. In `TestMCPSessionToken`:
  - the "off" subtest keeps both the static `chat` principal and the session principal, and asserts 401 on `/agui`, 404 on `/auth/session` and zero issuer hits;
  - a new subtest with `routes` absent asserts that a mapped token gets 200 on `/auth/session` and 401 on `/agui`.
  
  Acceptance: `make build && go test ./test/cli/... -run TestMCPSessionToken -v` passes.
- [x] 3.3 8.2 MINOR-7: `test/cli/dag_jose_test.go` round-trips a dag-jose object through the real binary (`ipfs dag put --input-codec dag-jose`, then `ipfs dag get` and `ipfs block get`), under go-jose v4.1.6. Acceptance: `make build && go test ./test/cli/... -run TestDagJose -v` passes, and the test asserts the returned CID's codec is `dag-jose` and the bytes round-trip unchanged.

## 4. Phase-boundary integration (ipfs-agent-architect)

- [x] 4.1 Integration gate, run once after groups 1 to 3 are complete, against a `make build` binary, on non-default ports with an isolated `IPFS_PATH`. Run in order:
  - `git diff --stat master... -- go.mod go.sum docs/examples/kubo-as-a-library/go.mod test/dependencies/go.mod` shows no change from this change's commits;
  - `go fmt ./...`, then `make build`, then `make -O test_go_lint`;
  - `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/`;
  - `go test ./test/cli/... -run 'TestMCPSessionToken|TestMCPHTTP|TestMCPPrincipal|TestDagJose|TestDefaultMatchesExpectedProfile|TestTelemetry' -v`.
  
  Acceptance: the commands and their observed output are recorded in a dated evidence README under `mcp/server/evidence/`. A `git diff --name-only` of this change lists no file under `core/commands/`, `core/coreiface/`, `core/coreapi/`, `client/rpc/`, `config/` or `core/corehttp/`. Any check that could not run is named as unverified.

## 5. Independent review (separate contexts from the builders)

- [x] 5.1 security-reviewer: review groups 1 and 2 and task 3.1 against the 8.1 findings. Confirm LOW-1 to LOW-5 closed, or record why one is not. Acceptance: a findings file under `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/` with no open CRITICAL or HIGH, and each 8.1 LOW marked closed or open with a reason.
- [x] 5.2 reviewer: compatibility and quality review of the whole change against the 8.2 findings. Confirm no `/api/v0`, gateway, CID or wire change; every new or changed key has a default, an off switch, a doc row and a disabled-state test; and MAJOR-1, MINOR-1 to MINOR-7, NIT-1 and NIT-2 are closed. Acceptance: a findings file under `.agent-team/kubo-ipfs-agentic/findings/review/agent-backend-settings/` with no blocking finding. One batched correction and confirmation cycle is allowed.
