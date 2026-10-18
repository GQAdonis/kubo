# Security read: know-me-cluster PR #40 (console-session-auth 4.1-4.3, Ory Hydra on oauth.know-me.tools)

Relayed verbatim by the lead (the security-reviewer context has no Write tool).

- Task: `agent-backend-settings › console-session-auth › 4.4` (acceptance: a findings file with no open CRITICAL)
- Artifact: Prometheus-AGS/know-me-cluster#40, branch `feat/flint-hydra`, commit c2029fd
- Reviewer: security-reviewer, separate context, read-only, no shell
- Files read (detached worktree at c2029fd provided by the lead): `namespaces/flint-core/hydra.yaml`, `helm-values/hydra.yaml`, `namespaces/argocd-config/manifests.yaml` (Certificates, Gateway `argocd-gateway`), `namespaces/flint-core/manifests.yaml` (kratos and kratos-ui Deployments, postgres StatefulSet, `kratos-config`, `postgres-bootstrap`), `namespaces/flint-core/networkpolicy-hydra.yaml`, `cluster/manifests.yaml` (ClusterIssuers), `apps/flint-core.yaml`, `namespaces/ipfs/lke-checks/origin-isolation.sh` (credential handling only). Also kubo `openspec/changes/console-session-auth/tasks.md` group 4, ADR `docs/mcp/0013-console-session-auth.md` sections 1-3, 5, 7-9 and "The uncomfortable thing", and the #39 read.
- Not verified here:
  - that the worktree equals c2029fd, or which lines the PR changes (no git); "pre-existing" below is inferred from the #39 read;
  - that the image digest really is `oryd/hydra:v26.2.0` (no network);
  - Hydra v26.2.0 config defaults, which come from Ory's documented defaults, not this image;
  - the rendered chart output and kubeconform;
  - the live cluster state, Calico enforcement, and the Secret values.
- Scope note: this PR deploys an OAuth2/OIDC provider, not an MCP server. The MCP-01..05 baseline is N/A here. It applies to `ipfs mcp`'s JWT verifier, which consumes this issuer in group 6.

## Verdict

**PASS WITH FINDINGS. No CRITICAL, one HIGH.**

- What holds:
  - hydra-admin has no route on any Gateway.
  - Secrets come only from `secretKeyRef`, and no value is in git.
  - The main container's pod security is strong.
  - The Hydra config sets only safe keys and leaves the security-relevant ones at secure defaults.
  - The postgres role cannot sign in until the operator sets a password.
  - Rollback prunes cleanly.
- HIGH-1 is the issuer listener's `allowedRoutes: from: All`. Any namespace that can create an HTTPRoute can attach a more specific path rule to `oauth.know-me.tools`, for example `/.well-known/jwks.json` or `/oauth2/token`. Gateway API precedence lets that rule win over Hydra's `/` prefix. Once group 6 trusts this JWKS, that rule lets someone forge ID tokens. The fix is one stanza and breaks nothing, so make it in this PR.
- MEDIUM-1: the unauthenticated admin API's only control is a NetworkPolicy whose enforcement is still unproven (ADR unverified point 6). That is a hard gate on OG-26, not on OG-23.

## Summary

| ID | Severity | One line |
|---|---|---|
| HIGH-1 | HIGH | Listener `flint-oauth-https` accepts HTTPRoutes from every namespace. An Exact or longer-prefix route for the JWKS, discovery or token path beats Hydra's `/`. Restrict it to `flint-core`. |
| MEDIUM-1 | MEDIUM | hydra-admin 4445 has no authentication, and Hydra serves it on every pod interface. The only barrier is `hydra-ingress`, and enforcement is unproven. Reaching it means minting a console session for any `sub`. The 4.5 probe must pass before OG-26. |
| LOW-1 | LOW | The `hydra-automigrate` initContainer has no container `securityContext` (privilege escalation allowed, capabilities kept, root filesystem writable) and no `resources`. |
| LOW-2 | LOW | Role `hydra` keeps PUBLIC `CONNECT` on the other databases (`flint`, `kratos`, `flintgate`, `broker`, `frf`). The REVOKE covers only database `hydra`. |
| LOW-3 | LOW | A RollingUpdate with 1 replica (surge 1, unavailable 0) runs the new version's migrations while the old Hydra still serves on the same schema. |
| LOW-4 | LOW | PKCE is not enforced (`oauth2.pkce.enforced` unset, so false). Enforce it once 4.6 shows that Envoy sends S256. |
| LOW-5 | LOW | OG-24 handling: `ALTER ROLE ... PASSWORD '<literal>'`, `hydra create client` printing `client_secret`, and `kubectl create secret --from-literal` all leave secrets in history or scrollback. |
| LOW-6 | LOW | `kratos-ui` (internet-facing Node app, now able to reach the unauthenticated hydra-admin) runs `oryd/kratos-selfservice-ui-node:v1.3.1` by tag, with no digest. |
| LOW-7 | LOW | Pre-existing: kratos-ui `COOKIE_SECRET` reuses Kratos `secrets.cookie` (`flint-kratos/secrets-cookie`). One key serves two systems. |
| INFO-1..12 | INFO | Hydra config review, no admin route, secrets, the TLS and HTTP listener, Kratos wiring and return URLs, the client spec, `secrets.system`, `sslmode`, egress, rollback, the Degraded waiver, and `origin-isolation.sh`. |

## Findings

### HIGH-1: the issuer listener accepts routes from any namespace

- Evidence:
  - `namespaces/argocd-config/manifests.yaml:358-370`: listener `flint-oauth-https`, hostname `oauth.know-me.tools`, `allowedRoutes.namespaces.from: All`, the same as every neighbour (`:201-357`).
  - `namespaces/flint-core/hydra.yaml:306-334`: Hydra's route matches only `PathPrefix /`.
  - In the repo, no route without a `sectionName` attaches to `argocd-gateway`. The only parentRef without one is the cert-manager HTTP-01 solver template (`cluster/manifests.yaml:32-38`), and the oauth Certificate uses the DNS-01 issuer (`cluster/manifests.yaml:62-74`). So nothing hijacks this host today.
- Mechanism:
  - Any principal that can create an HTTPRoute in any namespace can attach to `sectionName: flint-oauth-https` with hostname `oauth.know-me.tools`. That includes a namespace editor, an Argo Application for another namespace (including third-party Helm charts), a compromised workload with HTTPRoute RBAC, and the cert-manager controller.
  - Gateway API merges rules across routes. Exact beats prefix, and a longer prefix beats a shorter one, so `Exact /.well-known/jwks.json` or `/oauth2/token` takes precedence over Hydra's `/` whatever the creation time.
  - Consequences once group 6 lands:
    - (a) A forged JWKS lets the attacker sign ID tokens for the operator's `sub`. Envoy's jwt provider (task 6: "the remote JWKS") and `ipfs mcp` `auth.oidc.jwks_url` both fetch it through this listener (ADR section 1, "Envoy reaches Hydra by its public URL"). The result is full `/api/v0` and the settings routes.
    - (b) Capturing `/oauth2/token` yields the `ipfs-console` client secret, which Envoy sends with `client_secret_basic`, plus the codes.
    - (c) A forged `/oauth2/auth` gives login phishing.
  - This is the root of trust for the console. The issuer's integrity should not depend on RBAC over HTTPRoute objects in unrelated namespaces.
- Why HIGH and not CRITICAL: exploitation needs write access to HTTPRoutes somewhere in the cluster, which today means the operator, Argo, or an already-compromised controller. It is dormant until group 6 points Envoy and `ipfs mcp` at this JWKS.
- Action (recommended in this PR, required before OG-26): on `flint-oauth-https` set the following.

  ```yaml
  allowedRoutes:
    kinds:
    - kind: HTTPRoute
    namespaces:
      from: Selector
      selector:
        matchLabels:
          kubernetes.io/metadata.name: flint-core
  ```

  This does not break anything: the only route for this host is in `flint-core`, and the Certificate uses DNS-01, so no solver route is needed. Extend the 4.2 "route pinning" preflight to assert this listener's `allowedRoutes`. As a separate change, apply the same restriction to `flint-auth-https` and `flint-auth-http` (`:241-253`, `:371-377`). The Kratos login host has the same exposure today (pre-existing).

### MEDIUM-1: unauthenticated hydra-admin, protected only by an unproven NetworkPolicy

- Evidence:
  - `hydra.yaml:96-119` (ClusterIP Service), `:195-210` (`serve all`; container port 4445 on every pod interface).
  - `networkpolicy-hydra.yaml:39-48` (4445 only from `app=kratos` and `app=kratos-ui`, flint-core only).
  - ADR section 8 (lines 221-224) and unverified point 6 (line 91).
  - Tasks 4.5 sub-bullet (line 58).
- Mechanism: if Calico does not enforce the policy, any pod in any namespace can call the admin API. It can:
  - accept a login challenge for any `subject`, the operator's Kratos identity ID included. The attacker starts an `ipfs-console` authorization in their own browser, the admin accept returns a code to the console callback, and Envoy exchanges it and issues the attacker an operator session;
  - create clients with arbitrary redirect URIs;
  - read client metadata and keys.

  The label-claim residual inside flint-core is the same as #39 LOW-1. The policy also lacks the `metadata.namespace` assertion that #39 LOW-3 noted for hydra-ingress.
- Action:
  - The 4.5 probe ("a pod in another namespace fails 4445, 4444 answers") must PASS before OG-26. Add an unlabeled pod in flint-core to the probe as a must-FAIL case.
  - Add the namespace assertion for hydra-ingress to `networkpolicy-check.sh`.
  - This does not block OG-23 or OG-24: nothing trusts Hydra until group 6.

### LOW-1: the migration initContainer lacks hardening

- Evidence: `hydra.yaml:274-296` has no `securityContext` and no `resources`. The pod level (`:297-304`) gives runAsNonRoot, uid 65534 and seccomp RuntimeDefault, but not `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]` or `readOnlyRootFilesystem`.
- Mechanism: the initContainer holds the DSN, and its role owns the DB. It runs with a weaker profile than the main container. The risk is low, since it is the same image and runs only once.
- Action: give `hydra-automigrate` the same container `securityContext` as `:258-271`, and requests and limits (the chart's `deployment.automigration` / initContainer security-context values, or a post-render patch). Use the same values in `helm-values/hydra.yaml`.

### LOW-2: the hydra role can connect to the other databases

- Evidence: `manifests.yaml:1345-1360`. `REVOKE ALL ON DATABASE hydra FROM PUBLIC` covers only `hydra`. `00-create-databases.sh` (`:1334-1337`) creates `kratos`, `flintgate`, `broker` and `frf` with default PUBLIC `CONNECT`. `flint` is the `POSTGRES_DB` (`:891-892`).
- Mechanism: once the password is set, a compromised Hydra can connect to `flint`. There it can run PUBLIC-executable functions, including any `SECURITY DEFINER` ones in the flint bootstrap, and read anything granted to PUBLIC. Postgres has no NetworkPolicy, so the reachability is the same from anywhere.
- Action: in the custom image's pg_hba (or `-c hba_file`), add `host hydra hydra all scram-sha-256` and `host all hydra all reject`. Alternatively, `REVOKE CONNECT ON DATABASE flint, kratos, flintgate, broker, frf FROM hydra`. A role-specific revoke does not override a PUBLIC grant, so this needs either PUBLIC revokes plus explicit grants or the pg_hba approach; pg_hba is the clean one. Also confirm that the flint-forge postgres image keeps the upstream entrypoint's `scram-sha-256` host rule. That is what makes "LOGIN, no password" unusable over TCP (`:893-898` sets `POSTGRES_PASSWORD`, so upstream uses scram for host connections).

### LOW-3: migration while the old version is serving

- Evidence: `hydra.yaml:159-165` (replicas 1, maxSurge 25%, which rounds up to 1, and maxUnavailable 25%, which rounds down to 0) and `:274-296`.
- Mechanism: on an image bump, the new pod's initContainer migrates the schema while the old pod is still serving. Ory does not guarantee that the old binary works on the new schema. Expect token or consent errors during the overlap, or worse after a failed rollback, since down-migrations are not run. There is no concurrent-migration race at 1 replica.
- Action: use `strategy: Recreate`, or document an upgrade runbook (scale to 0, migrate, scale to 1) and that image rollbacks need a DB restore. Back up the `hydra` DB before every Hydra bump.

### LOW-4: PKCE not enforced

- Evidence: `hydra.yaml:72-94` has no `oauth2.pkce`. ADR section 1, line 114 says Envoy uses "client_secret_basic, PKCE".
- Mechanism: a confidential client without enforced PKCE relies on the client secret alone against code injection.
- Action: in 4.6, record whether Envoy's redirect carries `code_challenge_method=S256`; the agent-built throwaway request should include PKCE as well. If it does, set `oauth2.pkce.enforced: true` (Hydra has one client) in a follow-up.

### LOW-5: secret handling at OG-24

- Evidence: `hydra.yaml:26-31` and `manifests.yaml:1348-1350` tell the operator to run `ALTER ROLE hydra PASSWORD ...`. The PR body has the operator run `hydra create client` through a port-forward and store the secret in `ipfs/ipfs-console-oidc`.
- Mechanism:
  - A literal `ALTER ROLE ... PASSWORD` lands in psql history inside the postgres pod, and in the server log if `log_statement` is ever raised.
  - `hydra create client` prints `client_secret` to stdout.
  - `kubectl create secret --from-literal` puts values in shell history.

  ADR section 2 says the secret is "never printed".
- Action:
  - Use psql `\password hydra`, which sends only a SCRAM verifier.
  - Generate `secretsSystem` and `secretsCookie` with at least 32 random bytes into files and use `--from-file`.
  - Pipe the client creation straight into the Secret without displaying it, for example `hydra create client ... --format json | jq -r .client_secret | kubectl -n ipfs create secret generic ipfs-console-oidc --from-file=client-secret=/dev/stdin`.
  - Record `client_id`, not the secret.

### LOW-6: kratos-ui image not digest-pinned

- Evidence: `manifests.yaml:513`. With `HYDRA_ADMIN_URL` (`:499-500`) it can now accept login, consent and logout on the unauthenticated admin API, which makes it effectively an admin credential holder.
- Action: pin `kratos-selfservice-ui-node:v1.3.1@sha256:...`, the same as Hydra (`hydra.yaml:191`). If 4.6 forces the v26.2.0 switch, pin that digest instead.

### LOW-7: shared cookie secret (pre-existing)

- Evidence: kratos-ui `COOKIE_SECRET` comes from `flint-kratos/secrets-cookie` (`manifests.yaml:501-505`), the same key Kratos uses as `secrets.cookie` (`:386-390`, `:1305`). `CSRF_COOKIE_SECRET` is a separate key (`:508-512`).
- Mechanism: a kratos-ui compromise or log leak discloses Kratos's cookie key, and rotating one forces rotating the other.
- Action: give kratos-ui its own key when the Secret is next touched. This is not required for OG-23.

### INFO-1: Hydra config (`hydra.yaml:72-94`, `helm-values/hydra.yaml:57-82`)

- The issuer is exact and the same in three places: `urls.self.issuer` and `urls.self.public` are `https://oauth.know-me.tools`, and env `URLS_SELF_ISSUER` matches (`:234-235`). There is no trailing slash. The kubo verifier's exact-`iss` match needs it exactly as written.
- `urls.login`, `urls.consent`, `urls.logout` and `urls.error` all point at `https://auth.know-me.tools/ui/*`, as required by 4.1.
- `serve.cookies.same_site_mode: Lax`. No `domain`, so cookies are host-only on `oauth.know-me.tools`. Secure is the non-dev default; check it in 4.6 devtools: `ory_hydra_*` cookies should be Secure, HttpOnly, SameSite=Lax, host-only.
- TTLs are 15m for the ID and access tokens and 12h for the refresh token, matching ADR section 9.
- Absent, so at their secure defaults: CORS (disabled), `oauth2.expose_internal_errors` (false), `log.leak_sensitive_values` (false), `strategies.access_token` (opaque), `oidc.subject_identifiers` (public only), `oidc.dynamic_client_registration` (disabled), and any token or refresh hook. These rest on Ory's documented defaults, not on this image. 4.5 can confirm that the discovery document has no `registration_endpoint`.
- `log.level: info`, JSON.

### INFO-2: hydra-admin is never routed

Across the worktree, `hydra-admin` and `4445` appear only in the Service, the probes, the policy, the check script, kratos-ui `HYDRA_ADMIN_URL` and `kratos.yml`. No HTTPRoute, Gateway listener or `backendRef` points at them. The only HTTPRoute for this host targets `hydra-public:4444` (`hydra.yaml:325-329`). 4.5 re-checks this live.

### INFO-3: secrets

- `secret.enabled: false` (`helm-values/hydra.yaml:32-34`). DSN, `SECRETS_SYSTEM` and `SECRETS_COOKIE` come only from `secretKeyRef flint-hydra` (`hydra.yaml:236-250`, `:291-296`).
- A grep for `postgres://hydra:` followed by anything other than a placeholder, and for `secretsSystem:`, `secretsCookie:` and `client-secret:` values, found nothing.
- The ServiceAccount and pod have `automountServiceAccountToken: false` (`:57`, `:187`), and no RBAC is rendered.
- The main container runs as uid 65534 with no privilege escalation, all capabilities dropped, a read-only root filesystem and seccomp RuntimeDefault (`:258-271`), with limits of 500m CPU and 256Mi memory.

### INFO-4: `secrets.system` rotation

Hydra encrypts its signing keys in the DB with `secrets.system` (`hydra.yaml:33-34`). Rotation means prepending a new value to the list (env `SECRETS_SYSTEM`, comma-separated, newest first), and nobody has run that procedure yet (ADR, "uncomfortable thing" bullet 1). Back up the Secret value offline. If `flint-hydra` is lost or recreated with a new value, Hydra cannot decrypt its keys, and every session and client must be re-established. Record the procedure in the 4.5 evidence or a runbook.

### INFO-5: `sslmode=disable`

The DSN (`hydra.yaml:27`) sends the DB password exchange (SCRAM, so no cleartext password) and all Hydra data, including encrypted keys and session rows, unencrypted pod-to-pod. That is acceptable inside one cluster only if node-to-node traffic is trusted. Note whether Calico WireGuard is on. If it is not, this matches the existing Kratos posture (pre-existing).

### INFO-6: TLS and HTTP

- The Certificate `oauth-know-me-tools-tls` uses `letsencrypt-prod`, which is DNS-01 on Cloudflare (`cluster/manifests.yaml:62-74`). That matches 4.2.
- No port-80 listener exists for `oauth.know-me.tools`, so there is no plaintext route to Hydra. Every consumer uses `https://` URLs.
- HSTS is not set. That is optional for an issuer host.

### INFO-7: Kratos wiring (`manifests.yaml:1307`, `:499-500`)

- `oauth2_provider.url` makes Kratos fetch and accept login challenges from Hydra admin. A challenge must exist in Hydra, so Kratos cannot be driven for a client that the admin API did not create. Client creation is admin-only (MEDIUM-1).
- `oauth2_provider.override_return_to` is not needed for this flow and stays at its default.
- Pre-existing: Kratos `allowed_return_urls` includes `https://*.know-me.tools` and `https://*.prometheusags.ai` (`:1283-1284`). The second covers `ipfs.prometheusags.ai`, which serves arbitrary CIDs. That makes it an open redirect to attacker-controlled content after Kratos self-service flows. It does not affect the Hydra code flow, whose redirect is the client's exact `redirect_uris`. Tighten it separately.
- `CSRF_COOKIE_NAME __HOST-kratos-ui-csrf` (`:506-507`): older browsers match the `__Host-` prefix case-sensitively, so they would not apply the prefix's protections to this cookie. Prefer `__Host-` (pre-existing, cosmetic).
- Group 4 still needs the #39 LOW-2 check: kratos-ui must make no call to Kratos admin 4434 or to `oryapis.com` during login, consent and logout (tasks.md line 60).

### INFO-8: the `ipfs-console` client (OG-24, not in git)

- Create it as ADR section 2 specifies:
  - `token_endpoint_auth_method: client_secret_basic`;
  - grants `authorization_code` and `refresh_token`, response type `code`;
  - scopes `openid`, `offline_access` and `email` only;
  - exactly one `redirect_uris` entry, `https://console.prometheusags.ai/oauth2/callback`;
  - `post_logout_redirect_uris: [https://console.prometheusags.ai/]`;
  - `skip_consent: true`;
  - no `client_credentials`, no `audience`, no wildcard or extra redirect.
- Confirm the resulting `client_id`. If Hydra assigns a UUID instead of `ipfs-console`, the Envoy `jwt` audience and `ipfs mcp` `auth.oidc.audiences` must use that value. A mismatch fails closed.
- Before group 6 the client is inert, but any Kratos registrant can already complete a flow and get a code at the console callback. That is harmless until Envoy redeems codes, and the allowlist RBAC is what makes it safe afterwards (ADR section 3).

### INFO-9: Hydra egress is unrestricted

`hydra.yaml:36-39` acknowledges it. Hydra's outbound fetches (client `jwks_uri`, `request_uri`, `sector_identifier_uri`, back-channel logout) are driven by client metadata, which is admin-only. Not a finding while MEDIUM-1 holds. An egress policy to postgres 5432 plus DNS would be defense in depth.

### INFO-10: rollback and prune

`apps/flint-core.yaml:15-18` has `prune: true` and `selfHeal: true`, and every object is new without `Prune=false`, so a revert removes them. The following survive a revert by design: the `hydra` DB and role, `flint-hydra`, `ipfs/ipfs-console-oidc`, and the cert-manager-issued `oauth-know-me-tools-tls`. A later redeploy must reuse the same `secretsSystem` (INFO-4). Reverting the `kratos.yml` and `HYDRA_ADMIN_URL` lines turns off only the OAuth2 paths. Ordinary Kratos login does not depend on Hydra.

### INFO-11: the flint-core Degraded waiver

Between OG-23 and OG-24, Hydra sits in `CreateContainerConfigError` (`hydra.yaml:28-29`), and flint-gate is already Degraded (another session). Both fail closed and expose nothing. Degraded health does not stop Argo applying objects (#38 LOW-4 and #39 INFO-6 reasoning). Carry the waiver on #40, and confirm after sync that `hydra-ingress` exists before the Hydra pod first becomes Ready (it merged earlier, so it should).

### INFO-12: the builder's `origin-isolation.sh` run

`namespaces/ipfs/lke-checks/origin-isolation.sh:26-59`, `:133-136` and `:205-213`:

- The credentialed part runs only if `IPFS_CONSOLE_USER` and `IPFS_CONSOLE_PASSWORD` are already in the environment. Otherwise mode `all` prints `SKIP: credentialed checks` and exits 0.
- Credentials go to curl only as a config on stdin, never on argv or in output, and only to exactly `https://console.prometheusags.ai`.
- Not security-relevant if the builder's output shows `SKIP`. If it shows credentialed PASS or FAIL lines, the basic-auth console credentials were present in the builder's shell environment. That would be an unrequested use of operator credentials by an agent, and the operator should be told and decide whether to rotate the htpasswd user. Check the builder's transcript for which it was.

## Must be true before OG-23 (merge and sync)

1. HIGH-1 fixed in this PR (recommended), or accepted by the operator in writing on #40 with OG-26 as the hard deadline.
2. kubeconform and the offline render pass, and `git grep` finds no secret value (4.1 acceptance). Not re-run here.
3. The preflight shows DNS A = 23.239.29.33 for `oauth.know-me.tools`, DNS-01 issuance, and route pinning PASS, extended to assert the listener's `allowedRoutes` (4.2).
4. The Degraded waiver recorded on #40 (INFO-11).
5. Recommended: LOW-1 (initContainer securityContext) and LOW-6 (kratos-ui digest). Both are cheap and in this diff's area.

## Must be true at and after OG-24 (Secrets and client)

1. Postgres password set with `\password hydra`. `flint-hydra` has keys `dsn` (`sslmode=disable` accepted, INFO-5), `secretsSystem` and `secretsCookie`, each at least 32 random bytes, created `--from-file`. An offline backup of `secretsSystem` exists (LOW-5, INFO-4).
2. The `ipfs-console` client created exactly as in INFO-8, and its secret piped into `ipfs/ipfs-console-oidc` key `client-secret` without being displayed. `client_id` recorded.
3. LOW-2: pg_hba limits `hydra` to database `hydra`, or the gap is recorded as accepted.
4. 4.5 checks: discovery `issuer` is exactly `https://oauth.know-me.tools` with no `registration_endpoint`, the JWKS returns at least one key, hydra-admin has no route, and the cross-namespace and unlabeled-pod probes FAIL on 4445 while 4444 answers (MEDIUM-1).
5. 4.6: login, consent-skip and logout work; kratos-ui makes no 4434 or `oryapis.com` calls (tasks.md line 60); Hydra cookies have the right attributes (INFO-1); whether Envoy or the test request uses PKCE S256 is recorded (LOW-4).
6. Hard gates for OG-26 (group 6), not OG-23 or OG-24: HIGH-1 closed and MEDIUM-1 proven by the live probe.

## Manual follow-ups

1. Confirm the worktree is c2029fd and that the env lines kratos-ui had before (`COOKIE_SECRET`, `CSRF_*`) predate this PR; this read assumed so from #39.
2. Verify the Hydra image digest against Docker Hub (`docker buildx imagetools inspect oryd/hydra:v26.2.0`).
3. Check the flint-forge postgres image's pg_hba (LOW-2).
4. Check the builder transcript for the `origin-isolation.sh` credentialed output (INFO-12).
5. Separate changes: restrict `flint-auth-*` listeners (HIGH-1 note); tighten Kratos `allowed_return_urls` (INFO-7); a separate kratos-ui cookie key (LOW-7); a Hydra upgrade runbook (LOW-3).
