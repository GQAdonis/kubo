# console-session-auth 6.0 and 6.0a: after-sync verification

Date: 2026-10-19. Context `know-me`, run by the lead, read-only except where noted.

Operator: "PR #45 is merged" (merge `14a5141`, 10:54:03Z), then "all the open PR's are merged" (#44 merge `26b03a2`, 10:55:01Z; #43 merge `c8e5ca5`, a flint-gate rollback to the serving image `346479e8`, not part of this change). Argo synced `flint-core` and `argocd-config` to `c8e5ca5` by 10:58:51Z; both Synced and Healthy. `flint-core` health is Healthy for the first time since the 2026-10-13 flint-gate rollout (the waiver carried since 5.2 no longer applies).

The security read is `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/pre-og26-pr44-pr45-security-read.md` (both PASS). The verify reports are posted on #44 and #45 (comments 6265777083 and 6265777439).

## 6.0 (#45, LOW-2)

postgres-0 restarted by the sync at 10:55:03Z and was Ready at 10:55:09Z.

| Check | Observed | Result |
|---|---|---|
| `show hba_file` | `/etc/postgresql/hba/pg_hba.conf` | PASS |
| `pg_hba_file_rules` errors | 0 errors, 9 rules | PASS |
| Rule order | lines 2-7 local and loopback (unchanged), 8 `host hydra hydra all scram-sha-256`, 9 `host all hydra all reject`, 10 `host all all all scram-sha-256` | PASS |
| `hydra` to `hydra` over TCP (pod IP 10.2.0.37) | `hydra@hydra` | PASS |
| `hydra` to `flint` (required) | `FATAL: pg_hba.conf rejects connection for host "10.2.0.37", user "hydra", database "flint"` | PASS |
| `hydra` to `kratos`, `postgres` | rejected by pg_hba, same message | PASS |

The connections ran from inside postgres-0 to its own pod IP, so they are TCP from a non-loopback address and match the `host` rules, not the loopback `trust` rules. The `hydra` password was read from the operator's login Keychain (service `know-me/flint-hydra`, account `dsn`) and piped over stdin into `psql`'s environment inside the pod; it was never on a command line or printed. INFO-D (extension trust) was checked before the merge: none of `pg_net`, `flint_llm`, `pg_cron`, `dblink`, `postgres_fdw` is trusted.

## 6.0a (#44: INFO-B, LOW-8, LOW-4, post-logout redirect)

| Check | Observed | Result |
|---|---|---|
| Rollouts | hydra 1/1, kratos-ui 1/1, flint-gate 2/2, all updated | PASS |
| Hydra config | `oauth2.pkce.enforced: true`, `urls.post_logout_redirect: https://console.prometheusags.ai/webui/` | PASS |
| kratos-ui pod | `runAsNonRoot: true`, seccomp `RuntimeDefault`; container `allowPrivilegeEscalation: false`, drop `ALL`; Ready | PASS |
| `flint-auth-https` | `from: Selector` flint-core; 1 attached route; Programmed | PASS |
| `flint-auth-http` | `from: Selector` In [argocd, flint-core]; 0 routes; Programmed | PASS |
| Discovery | 200 | PASS |
| `verify.sh argocd` with `oauth.know-me.tools=/.well-known/openid-configuration:200`, `oauth.know-me.tools=/admin/clients:404`, `auth.know-me.tools=/ui/login:303` | 0 failing | PASS |
| `verify.sh flint-core`, same host checks | fetches PASS; 3 FAIL, all `dns-address ... gateway=none in this namespace` | PASS with tool-scope note: the gateway is in `argocd`, where the same DNS checks PASS |

PKCE enforcement: an authorization request without `code_challenge` still redirects to the login page. Hydra checks PKCE when it issues the code after login and consent (fosite's PKCE handler at the authorize response), so the enforcement is proven by a full login, not by the first redirect. The first Envoy login at 6.7 proves it end to end (Envoy always sends S256).

## Result

6.0 PASS and 6.0a PASS, pending the security-reviewer's confirmation that LOW-2, INFO-B, LOW-4 and LOW-8 are closed. Still open before OG-26: MEDIUM-1 live probe (unlabeled flint-core pod), MEDIUM-2 (restrict or accept), the `ipfs mcp` JWKS fail-closed item, and recording the `ipfs-console` post-logout URI change.

Correction (security confirmation `pre-og26-pr44-pr45-confirmation.md`): the PKCE paragraph above overstates what 6.7 proves. A successful Envoy login at 6.7 proves that Envoy sends S256 and that Hydra accepts it; it does not prove that Hydra rejects a request without a challenge. That needs a negative test (complete login and consent on a hand-built `/oauth2/auth` URL with no `code_challenge`, expect `invalid_request` at the redirect), recommended and not a gate. The reviewer closed LOW-4 on configuration because the setting fails closed. LOW-2, INFO-B and LOW-8 are confirmed RESOLVED or CLOSED. 6.0 and 6.0a PASS.

## MEDIUM-1 live probe (2026-10-19T11:49:49Z, deleted 11:50:00Z)

Operator direction: "delete that lock and keep moving". One `busybox:1.36` pod in `flint-core` with only `purpose=csa-probe-medium1` (no `app` label), deleted afterwards; no pod is left with the label.

```
BLOCKED hydra-admin.flint-core.svc:4445
BLOCKED hydra-public.flint-core.svc:4444
REACHED postgres.flint-core.svc:5432
```

`postgres:5432` is the positive control (same namespace, no policy on postgres), so the Hydra failures are `hydra-ingress`, not DNS or routing. Allowed path, same minute: `kratos-ui -> 4445 200 {"status":"ok"}`. Label holders: only `flint-core/kratos-647859df8f-zz4gv` (`app=kratos`) and `flint-core/kratos-ui-5878768f85-6z6sn` (`app=kratos-ui`). With the cross-namespace probe in 4.5, MEDIUM-1's live part is PASS.

## `ipfs mcp` JWKS gate (from #42 confirmation item 6), code read 2026-10-19

`mcp/server/jwks.go`: keys are cached for `auth.oidc.jwks_cache_ttl` (default 15m); fetches are serialized and at most one per 30s; a fetch has a 10s timeout, a 1 MiB cap, no redirects, and keeps only public `sig` keys with a `kid`. On a fetch failure the cache keeps serving keys it already holds (so an issuer outage does not sign everyone out before their tokens expire) and returns an error for any key it does not hold. `mcp/server/jwt.go:184-191`: `errNoSuchKey` rejects with `jwtRejectKey`, any other key error rejects with `jwtRejectKeysDown` (401). So unknown keys fail closed. `go test ./mcp/server/ -run 'JWKS|Kid|KeyID|Unknown|Down' -count=1`: ok (0.425s), covering `TestJWTVerifierKeysUnavailableIs401NotAFault`, `TestJWKSCacheRefetchIsRateLimitedAndTTLBounded`, `TestJWKSCacheFetchesOnceForManyTokens`, `TestChainJWKSDownDoesNotStopStartup`. These are unit tests; the running sidecar image containing this code is task 6.3. For the security-reviewer: serving held keys during an outage is a deliberate availability choice, bounded by token lifetime (15m ID token) and the 30s refetch window.

## `ipfs-console` post-logout URI (operator-run, 2026-10-19T11:52:04Z)

Run by the operator: `bash ~/.local/share/know-me-ops/hydra-client-postlogout.sh` (JSON Patch `replace /post_logout_redirect_uris` through a port-forward to `hydra-admin`; no secret printed). The lead's attempt from inside the kratos-ui pod was refused by the permission classifier.

- before: `post_logout_redirect_uris ["https://console.prometheusags.ai/"]`, `updated_at 2026-10-19T08:42:08Z`
- after: `post_logout_redirect_uris ["https://console.prometheusags.ai/webui/"]`, `updated_at 2026-10-19T11:52:04Z`
- unchanged: `client_id ipfs-console`, `redirect_uris ["https://console.prometheusags.ai/oauth2/callback"]`, `token_endpoint_auth_method client_secret_basic`. The secret is untouched (a JSON Patch on one field; Secret `ipfs/ipfs-console-oidc` still matches).
