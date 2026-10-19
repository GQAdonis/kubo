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

## 6.0b (#46, MEDIUM-2 and LOW-9) after sync (2026-10-19)

Operator: "PR #46 is merged" (merge `c5bc0ef`). Argo `argocd-config` synced to `c5bc0ef` at 12:51:07Z, Synced and Healthy.

| Check | Observed | Result |
|---|---|---|
| All 25 argocd-gateway listeners Programmed | 25/25 `True` | PASS |
| Attached-route counts equal the pre-merge simulation (`m2live/live-sim.txt`) | every listener equal (e.g. flint-rt-https 1, knowme-runtime-https 0, onyx-https 2) | PASS |
| Negative probe (required by the read): namespace `csa-probe-medium2`, HTTPRoute for `rt.know-me.tools`, `sectionName: flint-rt-https`, rule = 302 redirect on a unique path | `Accepted=False`, reason `NotAllowedByListeners` ("No listeners included by this parent ref allowed this attachment"); the probe path returned 200 from the existing frf backend, not the probe's 302; flint-rt-https stayed at 1 route; namespace deleted 12:51:31Z to 12:51:40Z | PASS |
| `know-me-listener-check.sh` and `--self-test` on main `c5bc0ef` | PASS, PASS | PASS |
| `verify.sh argocd` with an external fetch of know-me.tools (200), www (301), design (401), sso (404), gate (404), api (404), rt (200), auth `/ui/login` (303), oauth discovery (200), realtor (200) | 0 failing | PASS |

The sso, gate and api roots answer 404 with an empty body; their routes are attached (counts unchanged), so the 404 is the backend's answer at `/`, not a missing route. Whether those roots answered 404 before #46 was not recorded.

Live DNS (LOW-12, raised to a pre-OG-26 gate pending the operator): `chat`, `livekit`, `matrix` `.know-me.tools` -> 172.169.221.204, and a `*.know-me.tools` wildcard -> 35.238.217.92 (Google Cloud); neither is a cluster gateway and both time out. See `pr46-security-read.md`, lead's checks.

## `ipfs-console` post-logout URIs for logout option (a) (operator-run, 2026-10-19T13:12:06Z)

Operator ran `bash ~/.local/share/know-me-ops/hydra-client-postlogout-both.sh` (JSON Patch on `post_logout_redirect_uris` only, via port-forward; no secret printed), per the 2026-10-19 decision "Disable token encryption".

- before: `["https://console.prometheusags.ai/webui/"]`, `updated_at 2026-10-19T11:52:04Z`
- after: `["https://console.prometheusags.ai/", "https://console.prometheusags.ai/webui/"]`, `updated_at 2026-10-19T13:12:06Z`
- unchanged: `client_id ipfs-console`, `redirect_uris ["https://console.prometheusags.ai/oauth2/callback"]`.

`/` is what Envoy always sends as `post_logout_redirect_uri`; after 6.1 the console route redirects `/` to `/webui/`.

## LOW-12: dangling know-me.tools records deleted (2026-10-19T13:16Z)

Operator direction: "Use the cloudflare API with the cloudflare token env variable in the `~/.bash_profile` file to handle these records". Token `CF_TOKEN` (value never printed) verified `active`. Zone `know-me.tools` (`a6d0ce41bbf9226033289de4ecaa96ed`, NS abdullah/yolanda.ns.cloudflare.com). Full pre-change export (47 records): `/Users/gqadonis/.local/share/know-me-ops/cloudflare-know-me.tools-20261019T131540Z.json`.

Reachability before deletion (TCP 443/80/22/5432 from the operator's machine): 172.169.221.204 and 35.238.217.92 refused every port.

| Record | Type | Content | Delete result |
|---|---|---|---|
| `*.know-me.tools` | A | 35.238.217.92 (Google Cloud) | success |
| `chat.know-me.tools` | A | 172.169.221.204 | success |
| `livekit.know-me.tools` | A | 172.169.221.204 | success |
| `matrix.know-me.tools` | A | 172.169.221.204 | success |

Both Cloudflare nameservers then answered NXDOMAIN for `chat`, `matrix` and a random name (the first query, seconds after the API call, still returned the old answers; propagation lag). Restore, if ever needed, from the export with the same type, name and content.

Not deleted (operator decision pending), other names pointing off-cluster: `feed`, `pds`, `pg`, `relay`, `surreal-api`, `surrealist`, `uar` -> 172.169.221.204 (all ports refused); `appview` -> 34.16.104.200 and `social` -> 34.136.169.92 (Google Cloud, all ports refused); `admin` -> 35.194.48.152 (Google Cloud, 80/443 open, HTTPS to the name failed); `flint`, `the-boss` -> 185.158.133.1 (Lovable, `flint` answers 200); `agent-runtime`, `agentic-social-networking` CNAME `cname.manus.space`. Every one of these names receives the `know-me.tools` Kratos cookie.

Follow-up, operator: "yes, delete the nine dead ones" (2026-10-19). Each record's id was re-read and matched to its name before the delete; all nine returned `success`: `feed`, `pds`, `pg`, `relay`, `surreal-api`, `surrealist`, `uar` (172.169.221.204), `appview` (34.16.104.200), `social` (34.136.169.92). Zone now 34 records. Off-cluster names kept by the operator: `admin` (35.194.48.152), `flint` and `the-boss` (185.158.133.1, Lovable), `agent-runtime` and `agentic-social-networking` (CNAME `cname.manus.space`). The pre-change export above restores any of them.
