# ADR-0013: Session sign-in for the IPFS console

## Status

**Accepted**, 2026-10-17, by the operator. Proposed 2026-10-16 by `ipfs-agent-architect`. Nothing in this ADR has been built, deployed or run against the cluster; implementation is tracked as the OpenSpec change `openspec/changes/console-session-auth/`.

Operator words, verbatim, in order:

- "I set `oauth.prometheusags.ai` to `23.239.29.33` IP address and we should use `prometheusags.ai` for this since kratos, etc. are all parked there.  that is the answer to #1, and all the other answers should be as you recommend but adjusted for that."
- "I set `oauth.know-me.tools` to the same `23.239.29.33`"
- "know-me.tools, defer IDs"

Recorded answers (the first quote was superseded for point 1 by the second and third, after the lead reported that Kratos, sso-broker, flint-gate, forge and frf are all on `know-me.tools`):

1. Hydra host: `oauth.know-me.tools` (issuer `https://oauth.know-me.tools`). DNS A 23.239.29.33.
2. Allowlisted identity IDs and the admin: deferred. Required before subjects are wired (group 4 RBAC) and before the cutover (group 6).
3. Admin path: option (a), TOTP or WebAuthn enabled in Kratos and the admin identity enrolled before cutover. **Superseded 2026-10-20 by option (c), port-forward only; see the amendment of that date.**
4. Non-browser console `/api/v0` clients: none.
5. go-jose `v4.1.6`: approved as the target. Corrected 2026-10-17: it arrives only through an upstream `ipshipyard/p2p-forge` bump (section 4); the verifier is built at the graph's `v4.1.4`.
6. Soak before deleting the interim credentials: seven days.

Placement (console-session-auth before `lke-agent-settings` and `lke-vector-search`) and the Hydra deployment in `flint-core`: approved.

## Date

2026-10-16

## Context

### Operator decisions this ADR implements (verbatim)

- "If I move the console, that fucks up all the URL's I have with IPFS being owned by prometheusags.ai". The console stays at `https://console.prometheusags.ai`. Its session cookie must live on prometheusags.ai.
- "file the issues and add Hydra". Ory Hydra goes next to the existing Ory Kratos as the OAuth2 and OpenID Connect provider. Envoy Gateway's built-in OIDC (`SecurityPolicy.spec.oidc`) signs the user in and sets the console's own cookie. sso-broker is not used: it has a filed security flaw, Prometheus-AGS/flint-infra#9. flint-gate ext_authz is not used either: its pinned image crash-loops, Know-Me-Tools/flint-gate#16.

### What exists today (read-only cluster and code reads, 2026-10-16)

- **Kratos.** `oryd/kratos:v26.2.0` runs in `flint-core`. `kratos-public` is served at `auth.know-me.tools` on `argocd/argocd-gateway` (23.239.29.33). The session cookie domain is `know-me.tools`, with `SameSite=Lax` and a 720h lifespan. `allowed_return_urls` includes `https://*.prometheusags.ai`. The only enabled login method is `password`; registration is enabled. The identity schema has `email` and `name` traits and no roles. The UI is `oryd/kratos-selfservice-ui-node:v1.3.1` at `auth.know-me.tools/ui`. The ConfigMap comment says OSS Kratos cannot set one cookie across both `know-me.tools` and `prometheusags.ai` (two registrable domains). The Kratos session cookie therefore can never reach the console.
- **Database.** Postgres (`ghcr.io/prometheus-ags/flint-forge-postgres18`) runs in `flint-core`. Its bootstrap creates the databases `kratos`, `flintgate`, `broker` and `frf`.
- **Not deployed.** Hydra, Keto and Oathkeeper are not deployed. The only Dex belongs to ArgoCD.
- **The console gateway.** The console and `ipfs.prometheusags.ai` are on Gateway `ipfs/ipfs-gateway-gw` (45.56.67.67), which runs Envoy Gateway v1.9.1 with Envoy v1.39.1. HTTPRoute `ipfs-webui` (host `console.prometheusags.ai`, listener `https-console`) sends `/webui`, `/api/v0` and `/ipfs/<WebUIPath CID>` to `ipfs-api:5001`. The only SecurityPolicy is `ipfs/ipfs-webui-auth`. It applies basicAuth with Secret `ipfs-webui-basicauth`, targets `ipfs-webui`, and is annotated `Prune=false` and sync-wave `-1`. `/mcp` is only on the gateway host, through HTTPRoute `ipfs-mcp`. Routes for `/agui` and the settings route are not yet on the console; `lke-agent-settings` adds them.
- **Network.** The CNI is Calico (`calico-*` pods in `kube-system`). No NetworkPolicy exists in `ipfs` or `flint-core`. Whether Calico enforces one here has not been proven by a probe.
- **Certificates.** `argocd/wildcard-know-me-tools-tls` is not a wildcard (corrected 2026-10-17 by a live read): its SANs are `api`, `auth`, `gate`, `rt` and `sso` `.know-me.tools` (notAfter 2026-12-13). DNS for `know-me.tools` is Cloudflare, and DNS-01 works there through ClusterIssuer `letsencrypt-prod` (`lke-gitops-change` cluster facts, 2026-10-14; issuer declared in `know-me-cluster` `cluster/manifests.yaml`).
- **`ipfs mcp` authentication.** The only verifier is `StaticVerifier` (`mcp/server/auth.go:145-196`). It compares SHA-256 digests of configured bearer tokens in constant time. Principal kinds are `human-mcp`, `ui`, `a2a`, `uar` and `admin` (`mcp/server/principal.go`). The `AuthVerifier` interface (`auth.go:19-35`) was designed as the seam for "Ory Hydra access tokens later, with no change to routes or tool handlers". There is no JWT or JWKS verifier. `agent.origin_allowlist` rejects a listed route's request when its `Origin` is not listed. A request with no `Origin` passes.
- **Kubo RPC.** Kubo checks `Authorization` only when `API.Authorizations` is configured (`core/corehttp/commands.go:144-147`). LKE does not configure it, so a forwarded bearer header reaches kubo and is ignored.

### Interim residuals this ADR ends

These are recorded in the ADR-0009 Round 7 amendment (M1, M2) and the ADR-0004 amendment of 2026-10-16:

- M1: inline Basic credentials in the console, `https://<user>:<password>@console.prometheusags.ai` stored as `ipfsApi` in `localStorage`;
- M2: the chat `ui` token in `localStorage` on the console origin;
- the conditions attached to both: a dedicated htpasswd user, rotation of that user and the `ui` token when session auth lands, clearing the stored API address in every browser profile, no screenshots, and the smallest `write_groups`.

The session-auth carry list in ADR-0009 also applies here: cookie CSRF, trusted identity headers, a NetworkPolicy (`lke-origin-isolation` MEDIUM-4), and admin as a distinct identity with step-up. The Envoy basic-auth gotcha of 2026-10-14 adds one more reason. Its realm is the full request URL, so a browser never reuses credentials across paths.

## Verified sources

Each fact the design depends on, and where it was read on 2026-10-16:

| Fact | Source |
|---|---|
| Hydra latest stable is `v26.2.0` (2026-03-20); `v25.4.0` before it | `gh api repos/ory/hydra/releases` |
| `docker.io/oryd/hydra:v26.2.0` index digest `sha256:ff67c7fb5f95074fa53374d41151713554960504b340cd3f95b09e65deaea2a9` (linux/amd64 `sha256:f59c2f7f4969269b154fa34c57bc4b849263ebedbcaf8114aaeb1658a3007b4b`) | `docker buildx imagetools inspect oryd/hydra:v26.2.0` |
| Official Helm chart `ory/hydra` `0.64.0`, `appVersion: v26.2.0`, depends on `hydra-maester` | `ory/k8s` `helm/charts/hydra/Chart.yaml`; `gh api repos/ory/k8s/releases` (v0.64.0, 2026-09-03) |
| Hydra config keys `urls.self.issuer`, `urls.login`, `urls.consent`, `urls.logout`, `urls.post_logout_redirect`; `ttl.id_token` and `ttl.access_token` default `1h`, `ttl.refresh_token` `720h`, `ttl.authentication_session` `720h`; `strategies.access_token` default `opaque` ("jwt is a bad idea"); `serve.cookies.same_site_mode` default `None` | `ory/hydra` `spec/config.json` at `v26.2.0` |
| Hydra's ID token carries `auth_time` (from `LoginAuthenticatedAt`), `acr` and `amr` from the accepted login | `ory/hydra` `oauth2/handler.go` at `v26.2.0`, lines 1436-1440 |
| Hydra and Kratos both create a `networks` table (and popx `schema_migration`), so they cannot share one database | `ory/hydra` `persistence/sql/migrations/20150101000001000000_networks.*`; `ory/kratos` `persistence/sql/migrations/sql/20150100000001000000_networks.*`, both at `v26.2.0` |
| Kratos acts as Hydra's login provider when `oauth2_provider.url` points at Hydra's admin API (`override_return_to`, `headers` optional) | `ory/kratos` `embedx/config.schema.json` at `v26.2.0` |
| On login Kratos accepts Hydra's challenge with `subject = identity ID`, `remember` and `remember_for` from its session settings (720h here), and `amr` = the session's authentication methods. It sets **no `acr`** | `ory/kratos` `hydra/hydra.go` at `v26.2.0`, `AcceptLoginRequest` |
| Kratos method names: `password`, `oidc`, `totp`, `lookup_secret`, `webauthn`, `code`, `passkey` | `ory/kratos` `identity/credentials.go` at `v26.2.0` |
| `kratos-selfservice-ui-node` `v1.3.1` has `login.ts` (forwards `login_challenge`), `consent.ts` (Hydra consent, skips for `skip`, `client.skip_consent` or `TRUSTED_CLIENT_IDS`) and `logout.ts` (accepts Hydra's `logout_challenge`) | `ory/kratos-selfservice-ui-node` `src/routes/*` at `v1.3.1` |
| The consent route is enabled only when `HYDRA_ADMIN_URL` (or `ORY_SDK_URL`), `CSRF_COOKIE_SECRET` and `CSRF_COOKIE_NAME` are set | same repo, `src/pkg/index.ts` at `v1.3.1` |
| The UI's Hydra logout accepts only the OAuth2 logout. It does **not** end the Kratos session | same repo, `src/routes/logout.ts` at `v1.3.1` |
| The UI repo also tags `v26.2.0` (aligned with Kratos) | `gh api repos/ory/kratos-selfservice-ui-node/tags` |
| SecurityPolicy `oidc` fields: `provider.{issuer (https, required), authorizationEndpoint, tokenEndpoint, endSessionEndpoint, backendRefs}`, `clientID`/`clientIDRef`, `clientSecret` (Secret key `client-secret`, required), `redirectURL` (default `%REQ(x-forwarded-proto)%://%REQ(:authority)%/oauth2/callback`), `logoutPath` (default `/logout`), `scopes` (`openid` always added), `resources`, `cookieNames.{accessToken,idToken}`, `cookieDomain` (default: request host, no subdomains), `cookieConfig.sameSite` (`Lax`/`Strict`/`None`), `forwardAccessToken` (default false), `forwardIDToken.header`, `passThroughAuthHeader` (default false), `refreshToken` (default true), `defaultTokenTTL`, `defaultRefreshTokenTTL` (default one week), `csrfTokenTTL` (default 10 min), `denyRedirect.headers[{name, type Exact/Prefix/Suffix/RegularExpression, value}]`, `disableTokenEncryption` (default false) | `kubectl --context know-me explain securitypolicy.spec.oidc` (Envoy Gateway v1.9.1 CRD on the cluster) |
| Envoy Gateway applies `cookieConfig.sameSite` to **every** OAuth2 cookie, including the nonce (CSRF) and PKCE code-verifier cookies, and scopes those two to the redirect path. When `forwardIDToken.header` is `Authorization`, the inbound `Authorization` header is not preserved, and Envoy sets it to `Bearer <ID token>`. The token-endpoint auth type is client_secret_basic | `envoyproxy/gateway` `internal/xds/translator/oidc.go` at `v1.9.1` |
| Envoy's OAuth2 cookies are set with `secure;HttpOnly` | `envoyproxy/envoy` `source/extensions/filters/http/oauth2/filter.cc` at `v1.39.1` (`CookieTailHttpOnlyFormatString`) |
| HTTP filter order: CORS (3), CSRF (after CORS), ext_authz (6), basic auth (8), OAuth2 (9), JWT authn (10), RBAC (301) | `envoyproxy/gateway` `internal/xds/translator/httpfilters.go` at `v1.9.1` |
| SecurityPolicy also has `jwt.providers[{name, issuer, audiences, remoteJWKS.uri, extractFrom.{headers,cookies,params}, claimToHeaders}]`, `authorization.{defaultAction (default Deny), rules[{action, principal.jwt.{provider, claims[{name, valueType, values}], scopes}}]}` and `csrf.{additionalOrigins, shadowFraction}`. CSRF rejects a mutating request with a missing or non-matching Origin with 403; the destination origin is always allowed | `kubectl --context know-me explain securitypolicy.spec.{jwt,authorization,csrf}` |
| `github.com/go-jose/go-jose/v4` is already in kubo's module graph at `v4.1.4` (indirect; required at v4.1.4 by `p2p-forge@v0.10.1`, a protected stack, and by `grpc@v1.83.1`). `v4.1.5` (2026-09-03) and `v4.1.6` (2026-10-14) fix JWK and JWT parsing issues, including GHSA-mgcc-cww5-fp2q, which caps the RSA modulus | `go.mod`, `go mod why`; `gh api repos/go-jose/go-jose/releases` |

### Not verified from an official source

The design does not guess at any of these. Each one is a named acceptance check in the change.

1. Whether Envoy Gateway v1.9.1 accepts one SecurityPolicy that combines `oidc.forwardIDToken.header: Authorization` with a `jwt` provider that extracts the same header and an `authorization` rule. The CRD note forbids this header only with `passThroughAuthHeader: true`, which this design does not use. Check: server-side dry run, then a live sign-in.
2. Whether Envoy forwards a fresh ID token after it uses the refresh token. Hydra issues a new ID token on refresh with the `openid` scope; Envoy's cookie update on refresh was not read. Mitigation in the design: `ttl.id_token` equals `ttl.access_token`. Check: the session must still work after one access-token lifetime.
3. Whether Envoy's logout sends `id_token_hint` to Hydra's end-session endpoint, and what Hydra shows without it. Check: a live logout.
4. The exact migration command in Hydra v26.2.0 (`hydra migrate sql up -e --yes` in recent releases), and chart 0.64.0's values for it. Check: read from the rendered chart before writing manifests.
5. Hydra's ID-token signing algorithm in this deployment. RS256 is expected; it is read from the discovery document after deploy.
6. Whether Calico enforces NetworkPolicy here, and whether kubelet probes still reach the pod under the policy. Check: an operator-approved probe pod, and pod readiness after the policy lands.
7. Whether `kratos-selfservice-ui-node` v1.3.1 works with Kratos v26.2.0 for the challenge flow. Both run together today for ordinary flows, but not the OAuth2 one. The fallback is the UI tag `v26.2.0`, which exists.
8. Whether any non-browser client calls `https://console.prometheusags.ai/api/v0`. After cutover such a client is redirected to sign in. Only the operator knows.

## Decision

### 1. Architecture

```
browser
  │ GET https://console.prometheusags.ai/webui  (no console cookie)
  ▼
Envoy (ipfs-gateway-gw), SecurityPolicy oidc ──302──► Hydra  https://oauth.know-me.tools/oauth2/auth
                                                        │ login_challenge
                                                        ▼
                                                      Kratos UI  https://auth.know-me.tools/ui/login
                                                        │ Kratos session (cookie on know-me.tools)
                                                        │ Kratos accepts the challenge (sub = identity ID, amr)
                                                        ▼
                                                      Hydra  consent: skipped (first-party client, skip_consent)
                                                        │ 302 code
  ◄──────────────────────────────────────────────────────┘
  GET https://console.prometheusags.ai/oauth2/callback?code=…
Envoy exchanges the code (client_secret_basic, PKCE), sets HttpOnly, Secure, SameSite=Lax,
host-only cookies on console.prometheusags.ai, then redirects to the original URL
  │
  ▼ every later console request: Envoy turns the cookie into  Authorization: Bearer <Hydra ID token>
  ├─ JWT filter: signature (Hydra JWKS), iss, aud = ipfs-console, exp
  ├─ RBAC: sub ∈ operator allowlist, default Deny
  ├─ CSRF: mutating requests need Origin = https://console.prometheusags.ai
  ▼
ipfs-api:5001 (/webui, /ipfs/<WebUIPath>, /api/v0)      ipfs-mcp:5050 (/agui, settings, /index/*, /auth/session)
kubo ignores Authorization (no API.Authorizations)       ipfs mcp JWT verifier maps sub to a principal
```

- **Hydra's host is a new `oauth.know-me.tools`.** The issuer is `https://oauth.know-me.tools`. Hydra runs in `flint-core` next to Kratos and is exposed through `argocd/argocd-gateway` with a new listener. The listener needs its own Certificate for `oauth.know-me.tools`, issued by ClusterIssuer `letsencrypt-prod` (DNS-01, Cloudflare zone `know-me.tools`), because `wildcard-know-me-tools-tls` does not cover the host. The operator adds one Cloudflare A record (23.239.29.33). Reasons:
  - Hydra and the Kratos UI on one site (`know-me.tools`) keep Hydra's login and consent round trips same-site, so Hydra's cookies work with `same_site_mode: Lax` rather than the default `None`.
  - A host of its own avoids path-sharing with Kratos on `auth.know-me.tools`. Both serve `/health/*` and `/.well-known/*`, and future releases of either could add colliding paths.
  - The operator's constraint is that IPFS URLs stay on prometheusags.ai. The console does. Only the login hop goes to know-me.tools, which it already does for Kratos.
  - Rejected: Hydra on `auth.know-me.tools` paths, for the collisions above. Also rejected: Hydra on a prometheusags.ai host, which splits the auth plane across two sites and makes the Hydra and Kratos UI hops cross-site.
- **Envoy reaches Hydra by its public URL.** Discovery and the token endpoint use `https://oauth.know-me.tools`. `provider.backendRefs` to the in-cluster Service would need a cross-namespace ReferenceGrant; it is left as a later optimization.
- **The console cookie** is set by Envoy, host-only on `console.prometheusags.ai`, `Secure; HttpOnly; SameSite=Lax`. `cookieDomain` stays **unset**. Setting it to `prometheusags.ai` would send the session to `ipfs.prometheusags.ai`, where anyone's content is served. "The cookie lives on prometheusags.ai" is satisfied by the console host itself. Tokens in the cookies stay encrypted (`disableTokenEncryption: false`). Cookie names are fixed (`ipfs-console-at`, `ipfs-console-idt`) so the logout and the checks can name them.
  - **Amended 2026-10-19 (token encryption off).** Superseded for encryption only: the console token cookies are **unencrypted** (`disableTokenEncryption: true`) since the operator's decision of 2026-10-19 (operator, verbatim: "Disable token encryption"; `.prometheus/decisions.md`, 2026-10-19, decision 2). Host-only, `Secure`, `HttpOnly`, `SameSite=Lax`, no `cookieDomain` and the fixed cookie names are unchanged.
    - Reason: on `/oauth2/logout` Envoy sends the ID-token cookie's raw value as `id_token_hint` (Envoy v1.39.1 `source/extensions/filters/http/oauth2/filter.cc:60`, `:1399-1424`). Encrypted, that value is Envoy ciphertext, which Hydra v26.2.0 rejects, so logout could not end the Hydra login session. Unencrypted, the hint is the ID token Hydra signed, and Hydra accepts it once `https://console.prometheusags.ai/` is in the `ipfs-console` client's `post_logout_redirect_uris` (done out of band, 2026-10-19).
    - Cost: anyone who already holds the cookies can read and replay the access, ID and refresh tokens outside the browser. Refreshing still needs the `ipfs-console` client secret (`client_secret_basic`), and the backends already received the tokens in clear, because Envoy decrypts before forwarding. The new exposure is the ID token (`aud: ipfs-console`, up to 15 minutes) in the logout URL: browser history, the `argocd-gateway` access log if it records query strings, and Hydra's request logs. That is why the gateway JWT provider reads only the `Authorization` header Envoy sets from the cookie (`extractFrom`, know-me-cluster #48, security read MEDIUM-4), never `?access_token=`.
    - Residual: logout after more than 15 minutes idle sends an empty `id_token_hint`; the console session ends but the Hydra and Kratos sessions survive (security read LOW-17, accepted or fixed in the Web UI by the operator's choice).
    - Off switch: unchanged. `disableTokenEncryption: false` restores encrypted cookies, at the price of Hydra refusing the logout hint again.

### 2. The OAuth client

One confidential Hydra client, `ipfs-console`:

- `token_endpoint_auth_method: client_secret_basic`, which is what Envoy uses;
- grant types `authorization_code` and `refresh_token`, response type `code`;
- scopes `openid`, `offline_access` and `email`;
- `redirect_uris: [https://console.prometheusags.ai/oauth2/callback]` and `post_logout_redirect_uris: [https://console.prometheusags.ai/]`;
- `skip_consent: true`, because the client is first-party and the Kratos UI honours it (verified).

The operator creates the client out of band with the Hydra CLI against the admin API. The secret goes only into Secret `ipfs/ipfs-console-oidc` under key `client-secret`. It is never in git and never printed. `hydra-maester` is not used: its Secret layout does not match the `client-secret` key Envoy reads, and it adds a controller.

### 3. How identity reaches the backends: the ID token as the only carrier

- Envoy forwards the **ID token** as `Authorization: Bearer <id_token>` (`forwardIDToken.header: Authorization`). `forwardAccessToken` stays false. Hydra access tokens are opaque by default and Ory advises against JWT access tokens. Envoy cannot send Hydra's `audience` parameter: it has only `scopes` and `resources`, and whether Hydra honours RFC 8707 `resource` is not verified. The ID token's `aud` is the client ID, which is exactly the audience a verifier needs.
- **No trusted identity headers.** `claimToHeaders` is not used. Neither kubo nor `ipfs mcp` reads any identity from a header other than the signed token. A client-supplied `Authorization` on a session route is overwritten by Envoy (verified in the translator).
- **`/api/v0` (kubo RPC).** Envoy authenticates (OIDC), validates (JWT filter: Hydra JWKS, `iss`, `aud = ipfs-console`, expiry) and authorizes (RBAC: `sub` in the operator's allowlist, default Deny). Kubo receives the bearer and ignores it. The RBAC rule is load-bearing: Kratos registration is open, so any registrant can obtain a valid Hydra ID token. Authentication without the allowlist would hand the full RPC API to anyone who signs up.
- **`ipfs mcp` (`/agui`, the settings route, `/index/*`, `/auth/session`).** The same gateway checks run, and then `ipfs mcp` verifies the token itself with the new verifier (section 4). It maps `sub` to a principal from its own config. The gateway's allowlist and `ipfs mcp`'s mapping are two independent checks; the change adds an offline check that they agree.

### 4. The `ipfs mcp` JWT verifier (recommended, additive)

A second `AuthVerifier` beside `StaticVerifier`, behind a new config section. The contract keys (`mcp/contract/config/ipfs-mcp-config.schema.json`) each carry a default and an off switch:

| Key | Default | Off switch, meaning |
|---|---|---|
| `auth.oidc.enabled` | `false` | `false`: no JWT is accepted anywhere; only static bearers authenticate, as today |
| `auth.oidc.issuer` | none (required when enabled) | Exact `iss` match. No issuer is hardcoded anywhere in kubo |
| `auth.oidc.audiences` | `[]` (required, non-empty when enabled) | `aud` must contain one of them |
| `auth.oidc.jwks_url` | none (required when enabled) | `https` only, except loopback. Fetched only when enabled. This is the one new outbound endpoint, and it is configured, not derived |
| `auth.oidc.algorithms` | `["RS256"]` | Allowed `alg` values. `none` and `HS*` are rejected by the schema |
| `auth.oidc.clock_skew` | `"30s"` (max `"2m"`) | Leeway for `exp`, `nbf` and `iat` |
| `auth.oidc.jwks_cache_ttl` | `"15m"` | An unknown `kid` triggers at most one refetch per 30 s |
| `auth.oidc.routes` | `["agui","settings","index","session"]` | Routes that accept a JWT. `mcp` and `a2a` are not in the default. A JWT on any other route gets 401 |
| `auth.oidc.require_origin` | `true` | For a JWT identity on a non-safe method, `Origin` must be present **and** listed in `agent.origin_allowlist`. `false` turns this off |
| `principals[].subjects` | `[]` | The `sub` values (Kratos identity IDs) that resolve to this principal. A principal needs `credential`, `subjects`, or both |
| `principals[].require_amr` | kind default: `admin` needs any of `totp`, `webauthn`, `passkey`, `lookup_secret`; other kinds none | At least one listed method must be in `amr`. Setting `[]` on an admin entry accepts password-only sessions, a documented weakening |
| `principals[].max_auth_age` | kind default: `admin` `"1h"`; other kinds none | `now - auth_time` must not exceed it. An admin session older than this must sign in again |

Behaviour:

- **Signature, then claims.** The verifier checks the signature against the cached JWKS (`kid` match, allowed `alg`), then `iss`, `aud`, `exp` and `nbf` (with skew), and that `iat` is not in the future. `auth_time` is required when `max_auth_age` applies. Any failure is 401 `invalid_token` with no detail about which check failed.
- **Mapping by `sub` only, never by email.** A Kratos user can change their own `email` trait, and the ID token's `email` comes from those traits through consent. Mapping by email would let a user take over a principal by editing a profile field. `sub` is the immutable Kratos identity ID. Email is used only for display, in `/auth/session`.
- **Route-aware resolution.** A subject may appear in a `ui` principal and an `admin` principal (one person, two capabilities). Each request resolves to the principal whose kind the route accepts: settings resolves to `admin`, and `/agui` and `/index/*` resolve to `ui`. The path-scoped kind rules of ADR-0009 are unchanged. Two principals of the same kind listing one subject is a config error at load.
- **Static bearers keep working.** The static verifier is tried first, with its constant-time match. A token that matches no static credential and is shaped like a JWS is passed to the JWT verifier only when `auth.oidc.enabled` is true. Desktop, the Web UI on loopback, stdio (which has no HTTP auth) and `/mcp` clients are unchanged.
- **Absent JWKS never stops startup.** This follows the CRITICAL-2 rule of ADR-0009. If the JWKS cannot be fetched, JWT requests get 401 and the log names the URL, not any token. Static bearers are unaffected.
- **Audit.** The audit log and the logs record the principal name and `sub`, never the token or the email.
- **`GET /auth/session`** is a new contract route, mounted only when `auth.oidc.enabled`. It returns the resolved principals (name and kind), the `email` claim for display, the token's `exp`, and for admin one of `ok`, `step_up_required` or `not_eligible`. The Web UI uses it to choose session mode and to show "Signed in as", "Sign out" and "Verify again for settings".
- **Library.** The verifier uses `github.com/go-jose/go-jose/v4`, imported at the version already in the module graph, `v4.1.4`, with no bump. go-jose is not a free direct bump: `github.com/ipshipyard/p2p-forge@v0.10.1` (the latest release, a protected transitive stack in AGENTS.md "Transitive dependency bumps") requires `v4.1.4`, as do `google.golang.org/grpc@v1.83.1` and the kubo root module (indirect); `github.com/mholt/acmez/v3@v3.1.6` requires `v4.1.3` and `go-dag-jose@v0.1.1` requires `v4.0.4` (`go mod graph`, 2026-10-17). A kubo pin ahead of p2p-forge would run a combination its CI never tested, so the order is: upstream p2p-forge PR bumping go-jose to `v4.1.6`, green upstream CI, an upstream release, then in kubo `go get github.com/ipshipyard/p2p-forge@vX && make mod_tidy` (all three `go.mod` files move together). The verifier ships off by default until then. The cutover (group 6, OG-26) is gated on that bump, because `v4.1.5` and `v4.1.6` fix JWK and JWT parsing flaws (GHSA-mgcc-cww5-fp2q, a denial of service parsing RSA JWKs, patched in `v4.1.6`) that matter for a JWKS fetched over the network. Its other user, `go-dag-jose` and the `dagjose` plugin, must still build and pass its tests.
  - **Amended 2026-10-17 (fork).** Operator, verbatim: "https://github.com/GQAdonis/p2p-forge/pull/1 was merged with the necessary update.  use the fork". All three `go.mod` files now `replace github.com/ipshipyard/p2p-forge` with `github.com/GQAdonis/p2p-forge` at `d40b57a` (p2p-forge v0.10.1 plus go-jose v4.1.6, `go test ./...` passed; the fork has no CI), and kubo selects go-jose `v4.1.6`. This is a recorded exception to the transitive-bump rule. Cost: `go install github.com/ipfs/kubo/cmd/ipfs@...` no longer works for this fork (Go refuses a module with `replace`); `make build` and the image and Desktop builds are unaffected. Drop the replace when ipshipyard/p2p-forge#96 ships in a release. The cutover no longer waits on upstream.

### 5. Admin: a distinct mapping, with step-up enforced by `ipfs mcp`

- Admin is a separate `admin` principal entry with its own `subjects`. Signing in is never enough by itself.
- **`acr` cannot carry the step-up.** Kratos sets no `acr` when it accepts Hydra's login (verified).
- **`max_age` and `prompt=login` cannot be requested.** Envoy's `oidc` has no field for extra authorization parameters (verified field list).
- **What remains:** `amr`, which Kratos fills from the session's authentication methods (verified), and `auth_time`. The admin mapping therefore requires a second factor in `amr` and an `auth_time` within `max_auth_age` (1h). Both are checked by `ipfs mcp` on every settings request.
- **What this proves, stated plainly.** `amr` proves that the Kratos session behind the token completed a second factor. `max_auth_age` bounds how long a stolen console cookie can be used for admin. After the hour, the thief would need the `know-me.tools` Kratos cookie to sign in again. Neither proves that the person is at the keyboard for this particular write: re-sign-in from a live Kratos session is silent. The existing D2 typed re-confirmation on the settings page and Desktop's Touch ID stay in force.
- **Rejected: a separate admin client driven by the settings page** (browser-side PKCE with `prompt=login`). It needs new OIDC code and a second token store in the Web UI. Kratos's handling of `prompt=login` with a live session was not verified. Reconsider it if per-write presence is required.
- **Consequence today.** Kratos has only `password` enabled. Until the operator enables TOTP, WebAuthn or passkeys, no session satisfies the admin mapping. Behind the console's OIDC policy, Envoy overwrites `Authorization`, so the old static admin bearer cannot be used there either. The operator decides ("Decisions left to the operator", point 3).

### 6. `/api/v0` and basic auth: OIDC replaces it on the console

- The console no longer uses basic auth. `basicAuth` and the `ipfs mcp` bearer both use `Authorization`, and Envoy's OIDC filter overwrites that header with the ID token. Both cannot coexist on a rule, and keeping basic auth would keep its per-URL realm defect.
- **The policy keeps its name.** SecurityPolicy `ipfs/ipfs-webui-auth` keeps its name and annotations (`Prune=false`, sync-wave `-1`), and its `spec` changes from `basicAuth` to `oidc` + `jwt` + `authorization` + `csrf`. Argo updates it in place, and `git revert` of the cutover merge restores basic auth in one sync. A second policy on the same HTTPRoute would conflict.
- **The htpasswd Secret** `ipfs-webui-basicauth` is kept untouched until the operator ends the soak period, because the rollback needs it. Then the operator deletes it out of band and retires the dedicated console htpasswd user.
- **Non-browser clients of console `/api/v0`, if any, lose access.** Their supported path is `kubectl --context know-me port-forward` to `ipfs-api:5001`. The operator confirms there are none (unverified point 8).
- `denyRedirect` matches `Sec-Fetch-Mode: cors` (Exact). A `fetch` or `XMLHttpRequest` with no valid session gets 401, not a 302 the browser would follow cross-origin to Hydra and fail on CORS. Top-level navigations (`navigate`) still redirect to sign in. The Web UI turns a 401 into "Sign in again", which is a top-level navigation.

### 7. CSRF

- **`SameSite=Lax`, not `Strict`.** The settings review suggested `Strict`. Envoy Gateway applies `cookieConfig.sameSite` to the nonce and PKCE cookies too (verified). The callback arrives as a cross-site navigation from `oauth.know-me.tools`, a different site from `prometheusags.ai`, so `Strict` would drop the nonce cookie and every sign-in would fail. `Lax` sends cookies on top-level GET navigations from other sites and not on cross-site `fetch` or POST.
- **SameSite does not protect against the gateway host.** `ipfs.prometheusags.ai` and `console.prometheusags.ai` are the same site, so a page served from anyone's CID on the gateway host can send same-site requests that carry console cookies, even under `Strict`. Origin checks are therefore load-bearing, at three layers:
  1. **Gateway:** the SecurityPolicy `csrf` filter rejects any mutating request whose `Origin` is missing or is not `https://console.prometheusags.ai`, with 403.
  2. **Kubo:** `API.HTTPHeaders` CORS is the console origin only, and kubo's Origin check returned 403 for the gateway origin in the OG-6 evidence. Kubo answers GET with 405 for state-changing commands.
  3. **`ipfs mcp`:** `auth.oidc.require_origin` requires a listed `Origin` on every non-safe request from a JWT identity, closing the "no Origin passes" path for cookie-derived identities only.
- **No ambient cookie on bearer-designed routes.** The OIDC filter runs only on rules the session policy targets. `/mcp` (on the gateway host) is not targeted, and `ipfs mcp` never reads cookies, so a console cookie has no effect there. `auth.oidc.routes` also keeps JWTs off `/mcp` and `/a2a` by default.

### 8. NetworkPolicy (MEDIUM-4): no path around the gateway

Today any pod in the cluster reaches `ipfs-api:5001` (the full RPC API, unauthenticated inside the cluster) and `ipfs-mcp:5050`. Session auth at the gateway does not change that, so the NetworkPolicy is part of this change and not an optional follow-up.

- **`ipfs` namespace**, pod `app.kubernetes.io/name=ipfs`:
  - ingress on 5001, 5050 and 8080 only from Envoy proxy pods in `envoy-gateway-system` with labels `gateway.envoyproxy.io/owning-gateway-name=ipfs-gateway-gw` and `owning-gateway-namespace=ipfs`;
  - ingress on the swarm port 4001 TCP and UDP from anywhere, because peering must keep working;
  - nothing else.
  
  The `ipfs-mcp` sidecar reaches kubo over `127.0.0.1`, inside the pod, which the policy does not affect. The preflight first lists every in-cluster consumer of `ipfs-api` and `ipfs-headless` and stops if one exists.
- **`flint-core`, Hydra:**
  - admin port 4445 only from the `kratos` and `kratos-ui` pods. The admin API has **no authentication** of its own; anyone who reaches it can create clients and accept logins.
  - public port 4444 only from the `argocd-gateway` Envoy proxies.
- **Proof.** Calico is the CNI, but enforcement is an acceptance check: an operator-approved probe pod in another namespace must fail to connect, and the `ipfs` pod must stay Ready.

### 9. Logout and session lifetime

- **Logout.** `logoutPath: /oauth2/logout` (not `/logout`, to stay clear of any future console path). Envoy clears its cookies and calls Hydra's end-session endpoint. Hydra ends its login session and hands the logout challenge to the Kratos UI, which accepts it and redirects to `https://console.prometheusags.ai/`. **The Kratos session on `know-me.tools` survives** (verified in `logout.ts`), so the next console sign-in is silent while it lives. A full sign-out is Kratos's own logout at `auth.know-me.tools`; the Web UI's "Sign out" ends the console session and says that signing out of `know-me.tools` is a separate step (no link: a hardcoded endpoint would break the configurable-endpoint rule; UI review m1, 2026-10-17).
- **Hydra lifetimes:**
  - `ttl.id_token = ttl.access_token = 15m`, kept equal on purpose (unverified point 2);
  - `ttl.refresh_token = 12h`, the longest a console session lives without a new sign-in;
  - `ttl.authentication_session` is left at its default. Kratos sets `remember_for` to its own 720h session lifespan.
- **Envoy:** `refreshToken: true`, `defaultRefreshTokenTTL: 12h` as a fallback, `csrfTokenTTL` default (10 min).
- **Admin:** `max_auth_age` of 1h, on top of the above.

### 10. What happens to today's interim credentials

- **The chat `ui` token** is removed from `localStorage` on the console origin. In session mode, which it detects from `/auth/session`, the Web UI deletes `ipfs-webui:agent-chat:token` and any stored `ipfsApi` value that carries userinfo. It then calls same-origin with `credentials: 'same-origin'` and no `Authorization` header. On LKE the console `ui` principal loses its `credential` and gains `subjects`. Its token Secret is deleted after the cutover.
- **Loopback and Desktop are unchanged.** The Web UI served by kubo on `127.0.0.1:5001` and Desktop keep their existing token handling (ADR-0005 and its amendments). This ADR ends only the console residuals M1 and M2.
- **The htpasswd user** is retired, and its Secret deleted after the soak (section 6). The M1 and M2 conditions end at that point. Clearing the stored API address in every browser profile is done by the Web UI itself on the first session-mode load.

### 11. Off switches, per layer

| Layer | Switch | Effect |
|---|---|---|
| `ipfs mcp` | `auth.oidc.enabled: false` (the default) | No JWT accepted, no JWKS fetched, `/auth/session` not mounted. Static bearers as today |
| `ipfs mcp` | `auth.oidc.routes` narrowed, `principals[].subjects: []` | Fewer routes, or no subjects, accept a session |
| Web UI | No `/auth/session` route answering | Bearer mode as today. No session code path runs |
| Envoy | `git revert` of the cutover merge (same policy name) | Basic auth back on the console in one Argo sync |
| Kratos | Remove `oauth2_provider` | The Hydra login integration is off. Kratos keeps serving its other apps |
| Hydra | Remove its Argo-tracked objects (DB and PVC kept, `Prune=false`) | The provider is gone. The console must be reverted first |
| NetworkPolicy | Delete the object | Back to today's open in-cluster access. Not recommended |

**No hardcoded endpoint.** The issuer, JWKS URL, client ID and audiences exist only in the operator's config (the `ipfs-mcp-config` ConfigMap and the SecurityPolicy). Kubo ships no default for any of them, per the AGENTS.md rule that every hardcoded endpoint must be configurable and possible to turn off. These are new `ipfs mcp` config keys under `docs/mcp/config-reference.md`. The pointer in `docs/config.md` stays as decided under D12.

### 12. Stability

- **No `/api/v0` contract change.** No command, argument, default or response shape changes. There are no `/api/v0` or gateway code changes; kubo's only edits are the WebUIPath bump and the dependency pins. The only effect on kubo's RPC listener is that an `Authorization` header it already ignores is present. (Wording corrected 2026-10-20, 8.2 NIT-2.)
- **No HTTP Gateway change.** `ipfs.prometheusags.ai` routes and `Gateway.PublicGateways` are untouched, so gateway conformance is unaffected.
- **No CID or wire change.**
- **`ipfs mcp` change is additive.** A new optional config section, optional principal keys, a new verifier off by default, and a new optional route. `principals[].credential` becomes optional when `subjects` is present, which only relaxes the schema; every existing config stays valid.

## Decisions left to the operator

1. **Hydra's host name.** `oauth.know-me.tools` is recommended (section 1). Confirm, or name another `*.know-me.tools` host, and add the Cloudflare A record. **Answered 2026-10-17: `oauth.know-me.tools`; A record 23.239.29.33 set.**
2. **The allowlists.** The Kratos identity IDs (`sub`) that may use the console (RBAC and `ui`), and which of them is admin. The operator reads them from Kratos; agents never read identities. **Answered 2026-10-17: deferred ("defer IDs"); required before subjects are wired and before the cutover.**
3. **Admin on the console.** Recommended (a): enable TOTP or WebAuthn in Kratos before cutover (`selfservice.methods.totp.enabled: true`, plus `session.whoami.required_aal: highest_available`) and enroll the admin identity. Alternatives: (b) accept password-only admin (`require_amr: []`, a documented weakening); (c) no console admin, with settings changed through `kubectl port-forward` and the static admin bearer. Until (a) or (b), settings on the console answer 403 `step_up_required`. **Answered 2026-10-17: option (a), TOTP or WebAuthn.** **Superseded 2026-10-20: option (c). Operator, verbatim: "Port-forward only". See the amendment of that date.**
4. **Non-browser `/api/v0` clients on the console.** Confirm there are none, or name them. **Answered 2026-10-17: none.**
5. **The go-jose bump** to `v4.1.6` (section 4). **Answered 2026-10-17: approved, `v4.1.6`. Operator, verbatim, on the corrected plan: "yes, go ahead with 1-3".** The original wording assumed a direct `go get` bump; go-jose is pinned through the protected `p2p-forge` stack, so the approved plan is: (1) an upstream PR on `ipshipyard/p2p-forge` bumping go-jose to `v4.1.6`, (2) the verifier built against `v4.1.4` meanwhile and off by default, (3) this correction. The cutover (group 6, OG-26) waits for kubo's p2p-forge bump to land with go-jose at least `v4.1.6` in all three `go.mod` files. The uncomfortable part: cutover timing now depends on an upstream release we do not control, and the project can stall at group 5 with the verifier built but unusable on the real console.
6. **The soak period** before deleting `ipfs-webui-basicauth` and the console `ui` token Secret. Seven days is suggested. **Answered 2026-10-17: seven days.**

## Amendment (2026-10-20: mcp-oidc-hardening and the LKE admin path)

This amendment records the decisions of the OpenSpec change `openspec/changes/mcp-oidc-hardening/` (design D1 to D6), which answers the phase-boundary reviews of `console-session-auth`: the verifier security review (8.1, LOW-1 to LOW-5) and the compatibility review (8.2, MAJOR-1, MINOR-1 to MINOR-7, NIT-1, NIT-2). It also records the operator's 2026-10-20 rulings, quoted verbatim from `.prometheus/decisions.md`. The verifier sections above (4, 5, 7) stay in force except where a decision below narrows them.

### Operator rulings, 2026-10-20 (verbatim)

- `auth.oidc.jwks_max_stale` default (mcp-oidc-hardening task 1.1). Operator: "24h".
- OQ-1, the admin path on LKE. Operator: "Port-forward only". Decision 3 moves from option (a) to option (c). Admin settings changes on the cloud node are made only through `kubectl port-forward` with a static `admin` bearer read from an out-of-band Secret. No admin principal is reachable through the public console session: no principal of kind `admin` lists `subjects` in the LKE config, and the settings route is not routed on the console. This supersedes the 2026-10-17 answer (a). Kratos TOTP and WebAuthn enrollment is no longer a precondition for anything on LKE. `require_amr` and `max_auth_age` (section 5) stay in the contract for other deployments that choose console admin.
- OQ-2, the image roll. Operator: "Fold into OG-13 PR". The image that carries `mcp-oidc-hardening` ships in the same know-me-cluster PR that adds `/agui` (`lke-agent-settings`), with one planned restart of `ipfs-0`.

### Decisions

- **D1, `azp` and several audiences (8.1 LOW-1).** A token whose `aud` holds more than one value authenticates only if it carries an `azp` that is one of `auth.oidc.audiences`. A token that carries an `azp` authenticates only if that `azp` is one of `audiences`. This applies OpenID Connect Core 1.0, section 3.1.3.7, steps 4 and 5. A failure is the same generic 401 as every other token check. Rejected: requiring exactly one `aud`, which refuses conformant tokens; and doing nothing while Hydra access tokens are opaque, which an operator can change without kubo noticing.
- **D2, staleness cap `auth.oidc.jwks_max_stale` (8.1 LOW-2).** New key, default `24h` (the ruling above). A held key stops verifying once more than `jwks_max_stale` has passed since the last successful key-set fetch; every JWT then gets 401 until a fetch succeeds, static bearers are unaffected, and the expiry is logged once per stale period with no key material or token. The value must be at least `jwks_cache_ttl`, `0` is a load error, and the rule is checked whether or not `auth.oidc.enabled` is true. A config that sets `jwks_cache_ttl` above 24h therefore needs an explicit `jwks_max_stale`; the load error names the effective default when the key is absent. At the minimum, `jwks_max_stale` equal to `jwks_cache_ttl`, there is no stale fallback. No value removes the cap; the off switch of the whole verifier stays `auth.oidc.enabled: false`. Rejected: a "never expire" value, which would recreate the finding, and a cap derived from the TTL, which couples two settings tuned for different reasons.
- **D3, key fetch on `context.WithoutCancel` (8.2 MINOR-4).** A key-set fetch is not cancelled when the request that triggered it is cancelled or its client disconnects. It stays bounded by the existing 10 s fetch timeout, and concurrent waiters still share one fetch. A client that drops mid-fetch can no longer burn the 30 s refetch window.
- **D4, per-subject binding (8.1 LOW-3).** For an identity authenticated by an ID token, every session and confirmation binding keys on the principal ID plus the `sub`: the go-sdk `/mcp` session, confirmation dedupe and answers, confirm tokens, elicitation state, and the AG-UI (`/agui`) agent sessions, threads, interrupts and interrupt approval. Two people mapped to one principal cannot continue each other's MCP session or chat thread, or answer each other's pending confirmations, elicitations or interrupts. The binding key is the principal ID, `#`, and the first 128 bits of the SHA-256 digest of `sub` (hex), so the raw `sub` is never part of the key. Limits stay per principal: the agent quota and the cap of two open out-of-band confirmations are shared by every person on the principal (fail-closed), and one person's runs never evict another person's session. Static bearer identities keep the principal-ID binding. The key is internal and is never written to a response; audit records already carry `sub` (section 9). The fallback, "one subject per write-granted principal" at load, is used only if D4 cannot be built without changing the confirmation contract in `mcp/contract/`, and only after the architect amends the design.
- **D5, default `auth.oidc.routes` is `["session"]` (8.1 LOW-4).** With `routes` absent, a JWT authenticates only on `/auth/session`. Every other route is listed explicitly; `[]` accepts a JWT nowhere. When the list names a route other than `session` and `agent.origin_allowlist` is empty, startup logs one Warn line naming both keys and continues, since `require_origin` already fails closed. This narrows a default and widens nothing. The previous default is restored with `["agui","settings","index","session"]`. The table row for `auth.oidc.routes` in section 4 above records the original default and is superseded by this decision.
- **D6, JWKS fetch and proxy variables (8.2 MINOR-1).** Documented, not changed. The JWKS fetch of `ipfs mcp` uses the default HTTP transport and so honors `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` (`docs/environment-variables.md`). The reason other `ipfs mcp` clients ignore the proxy variables, that they carry content, prompts or keys, does not apply: the JWKS request carries none of them and the response is public keys. Setting `Proxy: nil` could break a deployment whose JWKS egress goes through a proxy.

Also in this change: tests for the existing redirect refusal and 1 MiB key-set bound (8.1 LOW-5, 8.2 MINOR-5); the real-binary disabled-state test keeps the session principal configured (8.2 MINOR-3); a startup failure that names `auth.oidc.enabled` when every principal is session-only and the verifier is off (8.2 MINOR-6); a dag-jose round trip under go-jose v4.1.6 (8.2 MINOR-7); the `/auth/session` encode error logged at Debug (8.2 NIT-1); the `principals[].subjects` off-switch wording (8.2 MINOR-2); the v0.44 changelog disclosure of the p2p-forge fork `replace` (8.2 MAJOR-1); and the Stability wording above (8.2 NIT-2).

### Deferred

- **Rejecting access-token-shaped tokens by `scp`, `client_id` or `typ`** (the optional half of 8.1 LOW-1). Deferred, not blocking `lke-agent-settings`, until the claim names in a live Hydra ID token have been read. It can be added later without changing D1 to D6.
- In-process rate limiting of bearer verification (8.1 INFO-1, the carried MCP-03 item).

### Stability

No `/api/v0`, HTTP Gateway, default CID recipe or wire change. No `go.mod` change and no new `replace`; the p2p-forge fork `replace` (section 4, amended 2026-10-17) is unchanged and is now disclosed in the v0.44 changelog. Every new or changed key has a default, an off switch and a row in `docs/mcp/config-reference.md`.

### The uncomfortable thing

The 24h cap signs every console user out if the issuer is unreachable for more than a day, and with admin now port-forward only, the console has no admin fallback at all: the break-glass path for both is cluster access. D1 is unverified against a live Hydra ID token; if Hydra sets an `azp` other than the configured audience, every console sign-in gets 401 after the image roll, and the first place that shows is the operator's signed-in check in `lke-agent-settings` task 1.8.

## Alternatives considered

- **Fixed sso-broker.** It is already deployed at `sso.know-me.tools` and bridges the two eTLDs. Rejected by the operator because of the filed flaw Prometheus-AGS/flint-infra#9. It is also a bespoke component on the auth path, where a certified OpenID provider (Hydra) plus a vendor-supported Envoy filter has a smaller surface.
- **flint-gate ext_authz.** Rejected: its pinned image crash-loops (Know-Me-Tools/flint-gate#16). The ext_authz pattern also passes identity as trusted headers, which this ADR rules out.
- **Keep basic auth.** Rejected. The per-URL realm makes browsers re-prompt or fail (gotcha 2026-10-14). It forced the inline-credentials workaround (M1). It has no per-user identity, no expiry and no step-up.
- **Move the console to a know-me.tools host,** where the Kratos cookie would work directly. Rejected by the operator: it breaks every IPFS URL on prometheusags.ai.
- **The Kratos session cookie directly at the console.** Not possible: OSS Kratos cannot set one cookie across two registrable domains (Kratos config note; multibrand is an Enterprise feature).
- **Oathkeeper.** Not deployed, and it would be another stateful-ish service. Envoy Gateway's `oidc`, `jwt`, `authorization` and `csrf` cover the need natively.
- **JWT access tokens instead of the ID token.** Ory advises against them (Hydra config schema). Envoy cannot request Hydra's `audience`, so their `aud` would not identify this resource.

## Consequences

- One new OpenID provider and one new host. Five out-of-band Secrets: the Hydra DSN, `secrets.system`, `secrets.cookie`, the Kratos UI CSRF secret, and the console client secret. A new `hydra` database and role in the existing Postgres.
- `ipfs mcp` gains a verifier that existing deployments never turn on unless they configure it.
- The console's identity is a person (`sub`), not a shared credential, and the audit log can name who changed settings.
- The interim residuals M1 and M2 end, together with their conditions.
- `lke-agent-settings` and `lke-vector-search` route their console paths under the session policy instead of "bearer only outside basic auth". The plan revision note lists the amendments.

## The uncomfortable thing

- **A new stateful service on the console's critical path.** Hydra needs Postgres, signing keys stored encrypted under `secrets.system`, migrations on every upgrade, and one more release train to follow. Losing `secrets.system` invalidates every session and every signing key. Rotating it has its own procedure, and nobody here has run it.
- **Console access now depends on Kratos, Hydra, the Kratos UI and the flint-core Postgres being up,** one replica each, on a different gateway (`argocd-gateway`) from the console's own. If any of them is down, nobody can sign in to the IPFS console, including the operator. The break-glass path is `kubectl port-forward` to `ipfs-api:5001`, or a revert to basic auth, and both need cluster access. Before this change the console depended only on its own pod and gateway.
- **Authorization rests on an allowlist of identity IDs, in two places.** Kratos registration is open, so a token proves only that someone has an account. One wrong RBAC rule (`defaultAction: Allow`, or a missing rule on one route) hands the full RPC API to any registrant. The offline check and the security review exist for this. The duplication between the SecurityPolicy and `ipfs mcp` is a drift risk the check only partly covers.
- **The Kratos cookie on `know-me.tools` becomes the root credential for the IPFS console,** including admin. Its 30-day lifespan, and the console logout that does not end it, are wider than the old htpasswd password's reach was.
- **Step-up is weaker than the word suggests.** It proves a second factor in the session and bounds cookie age. It does not prove presence per write.
- **The ID token is used as a bearer to the console's own backends.** This is defensible, because the audience is the console client and the backends are part of that client deployment. It is not textbook OAuth, and a reviewer may reasonably object.

## References

- ADR-0003 (confirmation and auth; the `AuthVerifier` seam), ADR-0004 (principals; R11 and 2026-10-16 amendments), ADR-0005 (chat UI token), ADR-0009 (runtime settings; Round 7 amendment M1, M2 and the session-auth carry list), ADR-0012 (model key scope)
- `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/settings-security-review.md`, `lke-redirect-pr36-security-read.md` (MEDIUM-4)
- `.prometheus/gotchas.md` 2026-10-14 (Envoy basic-auth realm)
- Prometheus-AGS/flint-infra#9, Know-Me-Tools/flint-gate#16
- `openspec/changes/console-session-auth/`
- `openspec/changes/mcp-oidc-hardening/` (2026-10-20 amendment)
