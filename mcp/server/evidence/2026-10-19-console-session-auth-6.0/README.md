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
