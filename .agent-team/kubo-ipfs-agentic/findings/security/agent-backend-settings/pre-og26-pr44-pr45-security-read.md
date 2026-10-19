# Security read: know-me-cluster PRs #44 and #45 (pre-OG-26 gates, console-session-auth 6.0a and 6.0)

Relayed verbatim by the lead (the security-reviewer context has no Write or shell).

- Artifacts:
  - #44, tree `scratchpad/kmc-hard`, diff `pr44.diff`, body `pr44-body.md`.
  - #45, tree `scratchpad/kmc-hba`, diff `pr45.diff`, body `pr45-body.md`.
- Prior reads: `hydra-pr40-security-read.md`, `hydra-pr42-confirmation.md` ("Before OG-26"); evidence `mcp/server/evidence/2026-10-19-console-session-auth-4.5/README.md`; tasks 6.0, 6.0a, 6.1.
- Not verified here:
  - commit SHAs of the two trees;
  - any check script, kubeconform, helm render or dry-run;
  - Envoy v1.39.1 `oauth2/filter.cc` and Envoy Gateway v1.9.1 `oidc_types.go`, which I could not read;
  - Hydra v26.2.0 `spec/config.json`;
  - any live cluster state, including RBAC on who can create HTTPRoutes in `argocd`.

## Verdict

- **#44: PASS.** No CRITICAL or HIGH. In the repo, INFO-B is closed for `auth.know-me.tools`, with the `argocd` deviation accepted. LOW-4 and LOW-8 are closed in the repo; each needs post-sync live confirmation, listed below. The post-logout redirect introduces no open redirect. The MEDIUM-1 repo claim is true. New: MEDIUM-2 (pre-existing, newly relevant), LOW-9 and LOW-10.
- **#45: PASS.** The pg_hba file is correct and first-match ordered. LOW-2 moves to RESOLVED once the 6.0 live acceptance passes; the hydra-to-flint negative check is required for that, not optional. New: INFO-C and INFO-D.

## Finding status

| ID | Status | Evidence and notes |
|---|---|---|
| INFO-B | RESOLVED in repo (auth host) | `kmc-hard/namespaces/argocd-config/manifests.yaml:254-272`: `flint-auth-https` has kinds HTTPRoute and `from: Selector` `kubernetes.io/metadata.name=flint-core`, the same stanza as `flint-oauth-https` (`:377-395`). `:396-412`: `flint-auth-http` admits `In [argocd, flint-core]`. The only route for the host is `flint-core/kratos-public` with `sectionName: flint-auth-https` (`flint-core/manifests.yaml:2393-2408`). See the `argocd` assessment below. |
| LOW-4 | RESOLVED in repo; live confirmation at 6.7 | `kmc-hard/namespaces/flint-core/hydra.yaml:84-86` `oauth2.pkce.enforced: true`; the source value is in `helm-values/hydra.yaml` (diff `:22-24`). The Envoy claim (always S256, no off switch) is not verified here. If it is wrong, the failure is closed: Hydra rejects the authorization request, and the first 6.1 login shows it. PKCE does not apply to the refresh_token grant, so refreshes are unaffected. Hydra has one client, so nothing else breaks. A successful 6.7 login proves Envoy sends S256. |
| LOW-8 | RESOLVED in repo; confirm live pod spec | `flint-core/manifests.yaml:526-530`: container `allowPrivilegeEscalation: false`, drop ALL. `:542-545`: pod `runAsNonRoot: true`, seccomp RuntimeDefault. No `runAsUser`, which is correct for a numeric image USER. The unexplained server-side-apply `restricted:latest` warning listing all four fields means the live pod spec must be checked after sync (follow-up 3). With maxSurge 25% at 1 replica, surge rounds to 1 and unavailable to 0, so the rollout has no gap. |
| MEDIUM-1 | Repo part CLOSED (claim true); live part OPEN, gates OG-26 | `kmc-hard/namespaces/ipfs/lke-checks/networkpolicy-check.sh:241-245` asserts `metadata.namespace` is `flint-core` for `hydra-ingress` and `kratos-ingress`. `:341-353` asserts that exactly one workload carries each allowed label (`app=kratos`, `app=kratos-ui`), which is the label-squatting guard. I could not confirm the `750788e` / #39 provenance (no git), and it does not matter. The 4.5 probe ran from namespace `default` (evidence README lines 27-36), so the cross-namespace half is proven. The unlabeled flint-core pod probe is still owed. |
| LOW-2 | RESOLVES on 6.0 live acceptance | `kmc-hba/namespaces/flint-core/postgres-hba.yaml:73-83`. See the pg_hba assessment below. |
| INFO-A | OPEN (recommended) | `kmc-hard/.../oauth-listener-check.sh:85` still treats only an explicit `sectionName` as naming the listener. Wildcard hostnames are still not matched. |
| INFO-7 | OPEN | Kratos `allowed_return_urls` still includes `https://*.prometheusags.ai` (`flint-core/manifests.yaml:1297-1298`). |
| LOW-7 | OPEN | Unchanged (`manifests.yaml:501-505`). |

## Assessment: does admitting `argocd` on `flint-auth-http` reopen INFO-B?

No, not in any way that matters.

1. **Who it admits.** Only principals that can create HTTPRoutes in `argocd`:
   - Argo CD itself. It already controls every namespace and could replace `flint-core/kratos-public` outright.
   - cert-manager. Its solver routes come from the ClusterIssuer `letsencrypt-http01` second solver (`kmc-hard/cluster/manifests.yaml:32-38`, parentRef `argocd/argocd-gateway` with no `sectionName`). cert-manager can also obtain a valid TLS certificate for `auth.know-me.tools`, which is strictly stronger than serving plaintext on port 80.

   Admitting `argocd` therefore adds no principal that is not already trusted with more. Residual: a non-admin subject with namespaced `create httproutes` in `argocd`. RBAC is not visible in the tree (follow-up 6).
2. **What a port-80 route could do.** Today no redirect route exists for the host:
   - `flint-auth-http` has 0 attached routes;
   - the repo has no `RequestRedirect` route for `auth.know-me.tools`, unlike edgequake, onyx, ipfs, knowdesign and realtor;
   - there is no HSTS anywhere in `namespaces/` (grep for strict-transport and hsts finds nothing).

   So `http://auth.know-me.tools/` returns an Envoy 404, and an admitted route could serve plaintext content under the real hostname.
3. **Why that is not useful.**
   - It cannot read Kratos cookies. Kratos runs without `--dev` (`manifests.yaml:369-372`), so its cookies are `Secure` and are never sent over port 80.
   - Phishing over port 80 needs the victim to follow an explicit `http://` link, and the browser marks the page "Not Secure".
   - Only already-trusted principals can attach (point 1).
4. **Renewal.** The deviation is needed. cert-manager creates the solver HTTPRoute in the Certificate's namespace, `argocd`. Its path match on `/.well-known/acme-challenge/<token>` is more specific than any `/` route, so a future redirect route (LOW-10) would not break HTTP-01. The renewal on 2026-11-13 is an operator watch item.

## Assessment: #45 pg_hba

- **Ordering and matching are correct** (`postgres-hba.yaml:75-83`):
  - First match wins. A TCP connection from a pod IP as `hydra` falls through rules 1-6, which are local or loopback only.
  - It hits `host hydra hydra all scram-sha-256` for database `hydra`, and `host all hydra all reject` for every other database, including `postgres`, `template1`, `flint`, `kratos`, `flintgate`, `broker` and `frf`.
  - `host` matches both SSL and non-SSL TCP, so it covers the `sslmode=disable` DSN (#40 INFO-5).
  - `all` in the address field covers IPv4 and IPv6.
  - `all` in the database field does not match replication connections, so remote replication stays rejected, as before.
  - The `hydra` user field is an exact role match, not `+group`. That is what is needed: the threat is a login as `hydra`.
- **Loopback and local trust for `hydra` is not a gap.**
  - The postgres pod has one container, no sidecars and no `hostNetwork` (`kmc-hba/namespaces/flint-core/manifests.yaml:865-944`). 127.0.0.1, ::1 and the Unix socket are reachable only from inside the postgres container.
  - The Hydra pod connects from its own pod IP and can never present as loopback.
  - Anyone who can `kubectl exec` into postgres-0 is already a superuser through rule 1 (pre-existing, upstream default).
  - The loopback trust is very likely what pg_cron depends on: `cron.database_name=flint` (`:871`) with the default `cron.host=localhost`, so jobs connect over loopback without a password. Removing it is a separate change.
  - Pivot check: `hydra` owns database `hydra`, so it can `CREATE EXTENSION` for trusted extensions in it. A loopback pivot would need a libpq-capable extension. `dblink` and `postgres_fdw` are not trusted, and `pg_cron`, `pg_net` and `flint_llm` are preloaded (`:869`). INFO-D asks to confirm none is marked trusted.
- **Fail-closed behavior.**
  - A missing ConfigMap leaves the pod in ContainerCreating. Argo applies ConfigMaps before StatefulSets.
  - A parse error stops start-up, which the body's runbook covers.
  - The mount is read-only and a directory mount, so later edits propagate and take effect only on reload.
- **Fresh-volume init still works.** The upstream entrypoint's temporary server gets the same `-c` args and uses the local socket (rule 1). The `$PGDATA/pg_hba.conf` copy it writes is ignored. This was confirmed by the builder's local PG 18.6 test, not by me.
- **Acceptance mismatch.** The PR body's after-sync step 4 calls the hydra-to-flint rejection "optional". Task 6.0 requires it, and this read marks LOW-2 resolved only on it.

## Assessment: post-logout redirect and PKCE

- **`urls.post_logout_redirect`** (`kmc-hard/namespaces/flint-core/hydra.yaml:103`) is a static, operator-set default. Hydra uses it only when the logout request names no registered `post_logout_redirect_uri`. An RP-initiated logout that names one must exact-match the client's registered `post_logout_redirect_uris` and carry an `id_token_hint`. A request parameter cannot steer the browser elsewhere, so there is no open redirect.
  - A logout-CSRF link (`/oauth2/sessions/logout`) still goes through kratos-ui's GET/POST logout confirmation (4.6 evidence, line 55), and it ends on the console, not on an attacker's host.
  - The client's registered URI is still `https://console.prometheusags.ai/` (#40 INFO-8; evidence line 65). The out-of-band change to `/webui/` in 6.1 is an admin-API write and should be recorded with the unchanged `client_id`.
- **PKCE** adds no redirect surface, and as noted under LOW-4 it fails closed. Side effect: any future authorization-code client, for example a CLI, must use S256. That is intended.

## New items

- **MEDIUM-2 (pre-existing, newly relevant for OG-26): the Kratos session cookie is scoped to `know-me.tools`, and the sibling listeners still admit every namespace.**
  - Evidence: Kratos `cookies.domain: know-me.tools` and `session.cookie.domain: know-me.tools`, `same_site: Lax`, `lifespan: 720h` (`kmc-hard/namespaces/flint-core/manifests.yaml:1291-1296`). These HTTPS listeners on hosts that receive that cookie still say `from: All`:
    - `flint-sso-https`, `flint-gate-https`, `flint-api-https`, `flint-rt-https` (`argocd-config/manifests.yaml:273-324`);
    - `knowme-site-https`, `knowme-www-https`, `knowme-runtime-https`, `design-https` (`:325-376`).
  - Mechanism:
    1. A route from any namespace attaches a path on, say, `rt.know-me.tools`. The browser sends `ory_kratos_session` to it: the cookie is Secure and Lax, and same-site requests and top-level navigations carry it. Kratos `allowed_return_urls` includes `https://*.know-me.tools` (`:1298`), so a login flow with `return_to` set to that host delivers the victim there with the cookie.
    2. Replaying that cookie at `auth.know-me.tools` makes kratos-ui accept the Hydra login challenge. Consent is skipped, so the attacker gets the operator's console session after 6.1, for up to 30 days.
  - Why it matters: INFO-B closed the password-phishing variant of this exposure. This one needs no password, and closing `flint-auth-*` alone does not close it. The same class of attacker as INFO-B (any namespace allowed to create HTTPRoutes) can do it.
  - Action, before OG-26: give each `know-me.tools` HTTPS listener a `from: Selector` naming the namespace that owns its route. For the `flint-*` hosts that is flint-core (and argocd on the `-http` ones, for HTTP-01). Do not narrow the cookie domain: flint-gate's `kratos_session` provider depends on it for `*.know-me.tools` (`:1242-1256`). If the operator accepts it instead, record it in writing as an interim residual, as was done for LOW-2.
- **LOW-9: no offline assertion pins the INFO-B stanzas.** `oauth-listener-check.sh:37` checks only `oauth.know-me.tools` / `flint-oauth-https`, so a later edit back to `from: All` on `flint-auth-*` passes CI. Extend the check, or a sibling check, to assert:
  - `flint-auth-https` is exactly the flint-core Selector;
  - `flint-auth-http` is exactly `In [argocd, flint-core]`;
  - after MEDIUM-2, the other `know-me.tools` listeners as well.
- **LOW-10: plain HTTP on `auth.know-me.tools` is a 404 rather than a redirect, and no host sends HSTS.** Add a flint-core HTTPRoute on `flint-auth-http` with a `RequestRedirect` to `scheme: https` and code 301, following the pattern in `namespaces/ipfs/manifests.yaml:586-599`. The solver's more specific path still wins. Also add `Strict-Transport-Security` on the auth and oauth hosts, for example through a response header modifier. Not blocking.
- **INFO-C (#45): the ConfigMap now controls database authentication.** Before, `pg_hba.conf` lived on the PVC and only the postgres process could write it. Now anyone who can write ConfigMaps in flint-core, or merge to the repo, can relax authentication on the next reload or restart. That is the same trust already placed in the `hydra` and `kratos` ConfigMaps, and Argo self-heal reverts drift. A small offline check is also worth adding: one that asserts the two hydra rules come before the generic host rule and that no non-loopback `trust` exists. The reason: a parse error is invisible until a restart.
- **INFO-D (#45): extension trust.** Confirm that none of `pg_net`, `flint_llm` or `pg_cron` is `trusted` in `pg_available_extension_versions`. If one were, `hydra`, as owner of database `hydra`, could create it there:
  - pg_net would give HTTP requests from the postgres pod, which has no egress policy;
  - pg_cron would give loopback-trusted job connections.
- **INFO-E (for 6.6): Kratos self-registration is open.** `selfservice.flows.registration.enabled: true` (`manifests.yaml:1300-1302`); the operator registered this way in 4.6. Any internet user can get a valid `ipfs-console` ID token. After 6.1, the only barrier is the `sub` allowlist, enforced in both the Envoy `authorization` rule and `ipfs mcp` principal subjects. Allowlist only by `sub`, never by `email`: login does not require a verified email.
- **INFO-F: the evidence README has a stale line.** `2026-10-19-console-session-auth-4.5/README.md:58` says "the UI is on the pinned v26.2.0 digest since #42". The repo pins `oryd/kratos-selfservice-ui-node:v1.3.1@sha256:2d6f9a84...` (`kmc-hard/namespaces/flint-core/manifests.yaml:513`). Correct it with an appended note.
- **Merge interaction.** #44 and #45 edit different hunks of `flint-core/manifests.yaml` (523-546 vs 873-960) from the same base `c577821`, so they should merge cleanly. Each sync rolls a different workload: #44 rolls Hydra (Recreate) and kratos-ui; #45 restarts postgres-0. Do not sync them together. While postgres-0 is down, Hydra and Kratos both fail, so a combined outage would be harder to diagnose.

## Pre-OG-26 gates remaining after #44 and #45 merge

1. **MEDIUM-1 live:** the unlabeled flint-core pod probe fails on 4445, while kratos-ui still gets 200. At the same time, list the live pods carrying `app=kratos` or `app=kratos-ui` and confirm only the two Deployments do.
2. **6.0 live acceptance (LOW-2):** `show hba_file` is the ConfigMap path, with 0 errors in `pg_hba_file_rules`. `hydra` to `hydra` succeeds and `hydra` to `flint` is rejected by pg_hba (required). Password never printed.
3. **6.0a live:**
   - `verify.sh` PASS for flint-core and argocd, and discovery still 200;
   - `flint-auth-https` shows 1 attached route and `flint-auth-http` 0, both Programmed;
   - the live Hydra ConfigMap contains `pkce.enforced`;
   - the live kratos-ui pod spec shows all four securityContext fields and the pod is Ready.
4. **MEDIUM-2:** restrict the remaining `know-me.tools` HTTPS listeners, or accept the gap in writing.
5. **From #42, not visible in these PRs:** the `ipfs mcp` JWKS client caches keys and fails closed on fetch errors.
6. **Out-of-band:** record the `ipfs-console` `post_logout_redirect_uris` change to `/webui/`, with `client_id` unchanged.
7. **Recommended, not blocking:** INFO-A, LOW-9, LOW-10, INFO-7, LOW-7.
8. **Operational:** watch the `wildcard-know-me-tools-tls` renewal Order before 2026-11-13.

## Manual follow-ups

1. Confirm the tree SHAs, and run `oauth-listener-check.sh` (with `--self-test`) and `networkpolicy-check.sh` on both.
2. Read Envoy v1.39.1 `source/extensions/filters/http/oauth2/filter.cc` around lines 1062-1088 and 1307-1337 to confirm PKCE is unconditional. Optional, since failure is closed.
3. After the #44 sync: `kubectl --context know-me -n flint-core get pod -l app=kratos-ui -o jsonpath='{.items[0].spec.securityContext}{.items[0].spec.containers[0].securityContext}'`.
4. After the #45 sync: the task 6.0 acceptance checks.
5. INFO-D: `select name, trusted from pg_available_extension_versions where name in ('pg_net','flint_llm','pg_cron','dblink','postgres_fdw');`.
6. RBAC: `kubectl --context know-me auth can-i create httproutes -n argocd --list` for non-admin subjects. Separately, who can create HTTPRoutes in namespaces other than flint-core; that determines MEDIUM-2's real reach.
7. Live HTTPRoutes in `argocd` today: confirm none would attach to `flint-auth-http` (the 0 count under `from: All` already implies none).

## Lead's read-only checks (2026-10-19, appended by the lead, not part of the review)

- INFO-D: `pg_available_extension_versions` shows `trusted=false` for every version of `pg_net`, `flint_llm`, `pg_cron`, `dblink` and `postgres_fdw`. Closed.
- INFO-F: confirmed. The live kratos-ui image is `oryd/kratos-selfservice-ui-node:v1.3.1@sha256:2d6f9a84456c0e6797664e65f2a743f695794788f1e276f915d247f9e35684a3`; the evidence README line was wrong and is corrected by an appended note.
- RBAC (follow-up 6, partial): `system:serviceaccount:default:default` cannot create HTTPRoutes in `argocd`. Cluster role bindings to `cluster-admin` are `system:masters`, `kube-system/lke-admin` and `velero/velero`; no namespaced binding grants HTTPRoute creation to a non-system subject outside `envoy-gateway-system`. MEDIUM-2's attacker class is therefore admin-equivalent today; the gate stands as defense in depth.
- MEDIUM-1 label holders: only `flint-core/kratos-647859df8f-zz4gv` and `flint-core/kratos-ui-c4b844598-cmt8b` carry `app=kratos` / `app=kratos-ui`.
