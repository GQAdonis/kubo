# Confirmation: know-me-cluster #44 and #45 after sync (console-session-auth 6.0, 6.0a)

Relayed verbatim by the lead (the security-reviewer context has no Write or shell). This is a confirmation read, not a fresh review.

- Prior read: `pre-og26-pr44-pr45-security-read.md`.
- Evidence: `mcp/server/evidence/2026-10-19-console-session-auth-6.0/README.md`.
- Raw reports: `lke-evidence/argocd/verify-20261019T110714Z.txt`, `lke-evidence/flint-core/verify-20261019T110711Z.txt`.
- Merged tree: `scratchpad/kmc-main2`.
- Not verified here:
  - the merge SHAs (`14a5141`, `26b03a2`, `c8e5ca5`);
  - any live state;
  - listener `allowedRoutes` and attached-route counts, the live Hydra ConfigMap and the kratos-ui pod spec. These are in the README table only, not in the raw verify output. I accept them as the lead's observations.

## Decisions

| ID | Decision | Reason |
|---|---|---|
| LOW-2 | **RESOLVED** | Every 6.0 acceptance item has been observed (README lines 15-20). `hba_file` is `/etc/postgresql/hba/pg_hba.conf` with 0 errors in 9 rules. The rule order matches `kmc-main2/namespaces/flint-core/postgres-hba.yaml:41-43`. `hydra` to `hydra` succeeds over TCP from a non-loopback pod IP. The required negative check, `hydra` to `flint`, fails with `pg_hba.conf rejects connection`. That message is a pg_hba rejection, not a password failure, so it proves the rule fires. The connection to `kratos` and `postgres` is rejected the same way. Probing from postgres-0 to its own pod IP matches the same `host ... all` rules a Hydra pod would hit, so the probe is representative. The password was never printed. |
| INFO-B | **CLOSED** | Merged main has `flint-auth-https` set to `from: Selector` flint-core (`argocd-config/manifests.yaml:254-264`) and `flint-auth-http` set to `In [argocd, flint-core]` (`:396-410`). Live (README :31-32): 1 attached route on HTTPS and 0 on HTTP, both Programmed. The argocd raw report shows both listeners Programmed (lines 8, 18). It also shows `auth.know-me.tools/ui/login` returns 303 through `argocd-gateway` (lines 56-57). |
| LOW-8 | **CLOSED** | The live kratos-ui pod shows all four securityContext fields and is Ready (README :30). It is `deployment: kratos-ui ready 1/1` in the flint-core report (line 21). |
| LOW-4 | **CLOSED on config; 6.7 is a compatibility check, not proof of enforcement** | `pkce.enforced: true` is in merged main (`flint-core/hydra.yaml:86`) and in the live ConfigMap. Hydra rolled after the sync (README :28-29). I accept closing it now, because the setting only fails closed: if Envoy does not send S256, logins fail. They are never accepted without PKCE. One correction to README line 37: a successful login at 6.7 proves that Envoy sends S256 and that Hydra accepts it. It does not prove Hydra *rejects* a code request without a challenge. Proving that needs a negative test: complete login and consent on a hand-built `/oauth2/auth` URL with no `code_challenge`, and expect an `invalid_request` error at the redirect. That test is recommended, not a gate. Upstream fosite enforcement is standard behavior, and the client is not in use before 6.1. |

## flint-core `dns-address ... gateway=none in this namespace`

**Accepted as a tool-scope quirk.** The flint-core report (lines 27, 29, 31) fails only because that namespace has no Gateway to compare the DNS answer against. The same three hosts resolve to `23.239.29.33`, which is the `argocd-gateway` load balancer address (argocd report line 29). The DNS checks PASS there (lines 52, 54, 56). Every fetch PASSes in both runs.

**New INFO-G (not blocking):** `verify.sh` should report SKIP or INFO, not FAIL, when the namespace owns no Gateway. Otherwise a run that reads "FAIL (3 failing checks)" becomes normal, and a real DNS regression will be read the same way.

## Pre-OG-26 gates remaining

1. **MEDIUM-1 live:** probe from an unlabeled flint-core pod. Port 4445 must be refused, and kratos-ui must still get 200. The label-holder half is already done (lead's note in the prior read).
2. **MEDIUM-2:** still open. The `flint-sso`, `flint-gate`, `flint-api`, `flint-rt`, `knowme-site`, `knowme-www`, `knowme-runtime` and `design` HTTPS listeners are still `from: All` in merged main (`argocd-config/manifests.yaml:273-376`). Restrict them, or accept the gap in writing as an interim residual. The RBAC finding that only admin-equivalent principals can create HTTPRoutes supports accepting it, but it must be in writing.
3. **`ipfs mcp` JWKS client** (from #42): it caches keys and fails closed when a fetch fails.
4. **Out-of-band record:** the `ipfs-console` client's registered `post_logout_redirect_uris` change to `/webui/`, with `client_id` unchanged. This is separate from the server default `urls.post_logout_redirect`, which is already live (README :29).

Recommended, not blocking:

- INFO-A, LOW-9, LOW-10, INFO-7, LOW-7;
- the INFO-C offline pg_hba order check;
- INFO-G;
- the LOW-4 negative PKCE test.

Operational: watch the `wildcard-know-me-tools-tls` renewal Order before 2026-11-13.
