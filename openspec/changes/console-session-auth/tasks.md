# Tasks: console-session-auth

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md`, "Plan revision note (2026-10-16): console-session-auth", phase path `agent-backend-settings`, change ID `console-session-auth`. ADR: `docs/mcp/0013-console-session-auth.md`.

Change-level prerequisites: `lke-origin-isolation` complete, and the plan revision note approved by the operator. The cutover (group 6) additionally waits for task 1.5 (go-jose at least `v4.1.6` through the p2p-forge bump). Scheduled before `lke-agent-settings` and `lke-vector-search`. Cluster tasks follow the know-me-cluster skill `lke-gitops-change`: offline checks, a live preflight posted on the PR, then stop for the operator's merge, then verify. Agents never apply, sync, restart or read Secret values.

## 1. Architect: ADR, contract, dependency (ipfs-agent-architect)

- [x] 1.1 Operator review of ADR-0013 and its six open decisions. Acceptance: the ADR status line reads `Accepted` with the operator's words quoted verbatim, and each of the six points has a recorded answer in `.prometheus/decisions.md`.
- [x] 1.2 Add `auth.oidc` (`enabled`, `issuer`, `audiences`, `jwks_url`, `algorithms`, `clock_skew`, `jwks_cache_ttl`, `routes`, `require_origin`) and `principals[].subjects`, `require_amr` and `max_auth_age` to `mcp/contract/config/ipfs-mcp-config.schema.json`. Each key gets `default`, `offSwitch` and `ownerChange`. `credential` becomes optional when `subjects` is present. Add one valid example and three invalid examples (missing `jwks_url`, `HS256`, a duplicate subject in two `admin` principals). Acceptance: `go test ./mcp/contract/...` passes and the three invalid examples are rejected with the offending key named.
- [x] 1.3 Add the `/auth/session` route and its response schema (principals, email, exp, admin status, no token and no `sub`) under `mcp/contract/routes/`, and the `step_up_required` error code. Acceptance: `go test ./mcp/contract/...` passes with a fixture that holds a `sub` field failing schema validation.
- [x] 1.4 Regenerate `docs/mcp/config-reference.md` with the existing generator `docs/mcp/config_reference.py`. Acceptance: the generator's consistency check passes, and the generated page lists every new key with its default and off switch.
- [x] 1.5 Track the go-jose bump through the protected `p2p-forge` stack (ADR-0013 decision 5, corrected 2026-10-17). Open an upstream PR on `ipshipyard/p2p-forge` bumping `github.com/go-jose/go-jose/v4` to `v4.1.6`, and track it to release; the verifier (group 3) builds against the graph's `v4.1.4` meanwhile and stays off by default. Do not `go get` go-jose directly in kubo. Acceptance (done when): kubo's `go get github.com/ipshipyard/p2p-forge@vX && make mod_tidy` has landed, all three `go.mod` files show go-jose at least `v4.1.6`, and `make build` plus `go test ./plugin/plugins/dagjose/...` pass.
  - 2026-10-17: met through the operator-directed fork replace (ADR-0013 section 4 amendment): all three modules resolve go-jose `v4.1.6` (`test/dependencies` carries no go-jose line; `go list -m` there shows v4.1.6); `make build`, `go test ./mcp/server/ ./mcp/contract/... ./core/node/libp2p/... ./cmd/ipfs/kubo/` and the `test/cli` MCP and CID-profile tests pass. `plugin/plugins/dagjose` has no test files.

## 2. `ipfs mcp` JWT verifier (mcp-server-engineer)

- [x] 2.1 A JWT `AuthVerifier` implementing the spec's token validation: signature and `kid` against a cached JWKS, the allowed `alg`, exact `iss`, `aud`, `exp`, `nbf` and `iat` with skew, and an unknown-`kid` refetch limited to once per 30 s. Acceptance: table tests against a local `httptest` JWKS cover a valid token, a wrong `aud`, a wrong `iss`, an expired token, a future `nbf`, `alg: none`, an `HS256` token signed with the public key, an unknown `kid`, and a tampered payload. Only the valid token authenticates.
- [x] 2.2 Subject mapping and route-aware principal selection, with email never used. Acceptance: tests show (a) a token whose `email` matches but whose `sub` does not gets 403, (b) one subject in `ui` and `admin` resolves to `ui` on `/agui` and to `admin` on settings, and (c) a duplicate subject in two `admin` principals fails config load.
- [x] 2.3 Admin step-up: `require_amr` and `max_auth_age` with the kind defaults, and 403 `step_up_required` on failure. Acceptance: tests show `amr: ["password"]`, `amr: ["password","totp"]` with a 2h-old `auth_time`, and a missing `auth_time` each give `step_up_required`, and `amr: ["password","totp"]` with a 10-minute-old `auth_time` passes.
- [x] 2.4 Route scope (`auth.oidc.routes`) and the Origin rule for JWT identities. Acceptance: tests show a mapped JWT on `/mcp` gets 401 by default, a JWT POST to `/agui` with no `Origin` gets 403, and a static-bearer POST to `/agui` with no `Origin` still succeeds.
- [x] 2.5 Chain the verifiers: static first, then JWT only when enabled. With OIDC disabled, nothing is fetched. A JWKS that cannot be fetched does not stop startup. Acceptance: a test with `auth.oidc` absent counts zero requests to the `httptest` JWKS server, and a test with the JWKS server stopped shows startup succeeds, a static bearer authenticates and a JWT gets 401.
- [x] 2.6 The `GET /auth/session` handler per the contract, mounted only when enabled. Acceptance: its responses validate against the 1.3 schema in tests, and it answers 404 when `auth.oidc.enabled` is false.
- [x] 2.7 Redaction: audit records name the principal and the `sub`, and no log line, audit record or error contains a token. Acceptance: a test captures slog and the audit sink across 2.1-2.6 and finds zero occurrences of any test token's signature segment.
- [x] 2.8 CLI integration test through the real binary: an `ipfs mcp --http` instance on a non-default port, configured against an `httptest` issuer, accepts a mapped token on `/agui` and refuses it on `/mcp`. Acceptance: `make build && go test ./test/cli/... -run TestMCPSessionToken -v` passes.
- [x] 2.9 Document the feature and its off switch in `docs/mcp/` (user guide section) and in the v0.44 changelog Highlights with a disable example. Acceptance: the changelog entry contains `"enabled": false` for `auth.oidc`, and the docs state that the Kratos session survives a console logout.

## 3. Web UI session mode (ui-ux-engineer, `../ipfs-webui`)

- [x] 3.1 Session-mode detection through same-origin `GET /auth/session`. Acceptance: a unit test with the route answering 404 leaves the existing chat-token code path untouched, and one with a 200 report switches to session mode.
- [x] 3.2 On entering session mode, delete `ipfs-webui:agent-chat:token` and any stored API address with userinfo, and never write a credential afterwards. Acceptance: an e2e test seeds both values, loads the page in session mode, and asserts both keys are absent and that `localStorage` and `sessionStorage` contain no value matching the seeded token or password.
- [x] 3.3 Same-origin calls with `credentials: 'same-origin'` and no `Authorization` for `/api/v0`, `/agui`, settings and `/index/*`. Acceptance: an e2e network capture in session mode shows zero requests carrying an `Authorization` header set by the page.
- [x] 3.4 The 401 path ("Sign in again", top-level navigation), the 403 `step_up_required` explanation, and "Signed in as", sign-out (`/oauth2/logout`) and the separate `know-me.tools` sign-out note. Acceptance: e2e tests for each state pass, with screenshots at 320 and 1440 px in both themes attached to the PR.
  - Held (UI review M3, 2026-10-17): the acceptance e2e run of `test/e2e/console-session.test.js` has not executed; it runs at the phase integration gate and 3.4 is unverified until it passes there.
- [x] 3.5 UI review through `prometheus-ui-review` after 3.1-3.4. Acceptance: a findings file exists with no open blocking item.
- [x] 3.6 UI review carry items (confirmation `console-session-auth-ui-review-confirmation.md`): M4 visible focus ring on accent-filled primary buttons (WCAG 2.2 SC 2.4.7) with re-captured step-up and sign-in-again focus states; m2 single announcement when the session has ended on the settings page, with a test rendering `AgentSettingsPage` and the banner; M1 residual: 401s from the async resource bundles raise "Sign in again"; m7 unsent chat text is not lost silently on expiry. Acceptance: each item fixed with a test, jest, eslint and tsc pass, and the reviewer confirms in one pass before the phase gate.
  - Verified by the lead 2026-10-18 at ipfs-webui c2193fdd: eslint exit 0, tsc exit 0, jest 1171 passed / 1 failed / 6 skipped. The failure is `src/bundles/peer-bandwidth.test.js`, a pre-existing flake: file unchanged since upstream f8568b9e, the parent 661fb813 run fails the same suite (3 failed), and alone at HEAD it passed, failed (2 of 6), passed. Reviewer confirmation PASS (`console-session-auth-3.6-confirmation.md`).
- [x] 3.7 Focus-ring follow-ups from the 3.6 confirmation: N1 the modal close button ring is clipped by `.prom-modal` `overflow: hidden` (inset ring that beats the reset rule), plus the nits (inset edge on `pm-switch-track` and `sr-chip` kept through the halo; `box-shadow: none` on `hoverable-button` and `button-inside-focus`). Acceptance: a focused modal close captured at 320 and 1440 in both themes shows the full ring; jest, eslint and tsc pass.

## 4. Cluster: provider and wiring (ipfs-agent-architect, know-me-cluster via `lke-gitops-change`)

- [ ] 4.1 Hydra in `flint-core`: manifests rendered from Helm chart `ory/hydra` 0.64.0 and committed in the repo's raw-manifest style. Requirements:
  - image `oryd/hydra:v26.2.0@sha256:ff67c7fb5f95074fa53374d41151713554960504b340cd3f95b09e65deaea2a9`;
  - one replica;
  - public 4444 and admin 4445 as ClusterIP;
  - the migration command read from the rendered chart (ADR unverified point 4);
  - `urls.self.issuer: https://oauth.know-me.tools`, `urls.login`, `urls.consent` and `urls.logout` on `https://auth.know-me.tools/ui/...`, and `serve.cookies.same_site_mode: Lax`;
  - `ttl.id_token` and `ttl.access_token` both `15m`, `ttl.refresh_token` `12h`;
  - DSN, `secrets.system` and `secrets.cookie` referenced from out-of-band Secrets by name;
  - a `hydra` database and role added to `postgres-bootstrap`;
  - `Prune=false` on any PVC.
  
  Acceptance: the offline render and `kubeconform` pass, `git grep` finds no secret value, and the preflight posted on the PR shows every referenced Secret and key present (created at OG-24).
  - Added 2026-10-18 (#40 security read LOW-2, operator accepted option (b)): role `hydra` keeps the default PUBLIC `CONNECT` on `flint`, `kratos`, `flintgate`, `broker` and `frf` once OG-24 sets its password. Accepted as an interim residual; the fix is the 6.0 pg_hba change, before cutover.
- [ ] 4.2 Route `oauth.know-me.tools` on `argocd-gateway`: a Certificate for `oauth.know-me.tools` (ClusterIssuer `letsencrypt-prod`, DNS-01 on the Cloudflare zone `know-me.tools`; `wildcard-know-me-tools-tls` is not a wildcard and does not cover this host), a new listener on that Certificate's Secret, and an HTTPRoute to `hydra-public:4444` pinned by `sectionName`. Acceptance: the preflight reports DNS A = 23.239.29.33, issuance path DNS-01 on a Cloudflare zone PASS, no redundant Certificate, and route pinning PASS.
- [ ] 4.3 Kratos wiring. In `kratos.yml`, set `oauth2_provider.url: http://hydra-admin.flint-core.svc.cluster.local:4445`. On `kratos-ui`, set `HYDRA_ADMIN_URL`, `CSRF_COOKIE_NAME`, and `CSRF_COOKIE_SECRET` from an out-of-band Secret. If 4.6 fails on v1.3.1, also change the `kratos-ui` image to the `v26.2.0` tag pinned by digest. Acceptance: the diff shows exactly these keys, and the preflight shows the CSRF Secret key present.
- [x] 4.4 Security-reviewer read of the 4.1-4.3 PR before OG-23. Acceptance: a findings file under `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/` with no open CRITICAL.
- [ ] 4.5 After OG-23 (merge and sync) and OG-24 (Secrets and client): read-only verification. Checks: `https://oauth.know-me.tools/.well-known/openid-configuration` returns `issuer` equal to `https://oauth.know-me.tools`, and records `id_token_signing_alg_values_supported` (unverified point 5); the JWKS URL returns at least one key; `hydra-admin` has no route on any Gateway. Acceptance: a README in a new dated evidence folder marks each check PASS or FAIL.
  - Added 2026-10-18 (carried from 5.4): a throwaway probe pod in another namespace fails to reach `hydra-admin.flint-core.svc:4445` while `hydra-public` 4444 answers, with operator approval for the pod.
- [ ] 4.6 Operator-run browser check of the provider alone, through a throwaway authorization request for `ipfs-console` built by the agent. Kratos login completes, consent is skipped, and the callback URL receives a `code`. Acceptance: the evidence README records the operator's result. On FAIL with UI v1.3.1, task 4.3's image switch is made and this check is re-run.
  - Added 2026-10-18 (#39 security read LOW-2): with `HYDRA_ADMIN_URL` set, run login, consent-skip and logout and confirm kratos-ui makes no request to Kratos admin 4434 or to any `oryapis.com` host. If it does, set `KRATOS_ADMIN_URL=http://kratos-admin:4434/` and add `app=kratos-ui` (not flint-gate) to a 4434 rule in `kratos-ingress`, with the check updated.

## 5. Cluster: NetworkPolicy (ipfs-agent-architect)

- [x] 5.1 Read-only inventory of in-cluster consumers of `ipfs-api`, `ipfs-headless` and `ipfs-mcp` (Services, Endpoints, any manifest referencing them outside `namespaces/ipfs`). Acceptance: the inventory is in the PR body, and the PR stops if any consumer other than the gateway exists.
- [x] 5.2 NetworkPolicies:
  - `ipfs`: 5001, 5050 and 8080 only from Envoy proxies labelled `gateway.envoyproxy.io/owning-gateway-name=ipfs-gateway-gw`; 4001 TCP and UDP from anywhere;
  - `flint-core`: `hydra-admin` 4445 only from `kratos` and `kratos-ui`; `hydra-public` 4444 only from the `argocd-gateway` proxies.
  
  Acceptance: an offline `yq` check asserts these exact selectors and ports, and the preflight passes.
- [x] 5.3 Security-reviewer read of the 5.2 PR. Acceptance: findings file, no open CRITICAL.
- [x] 5.4 After OG-25 (merge, plus an operator-approved probe pod): verify enforcement. Acceptance: the evidence README shows the probe pod in another namespace failing to connect to `ipfs-api.ipfs.svc:5001` and `hydra-admin.flint-core.svc:4445`; the `ipfs` pod Ready with zero restarts; console `/webui` and the swarm still answering.
- [x] 5.5 Kratos ingress NetworkPolicy (security read of #38, HIGH-1): `app=kratos` 4434 (admin) only from `flint-gate` (and `kratos-ui` only if it calls the admin API, checked first), 4433 only from the `argocd-gateway` proxies and `kratos-ui`. Acceptance: one cluster PR through `lke-gitops-change` with an offline check and preflight posted, a security read with no open CRITICAL, and live proof after merge that a probe pod in another namespace cannot reach `kratos-admin.flint-core.svc:4434`. Prerequisite for group 6.
  - Correction 2026-10-18 (5.5 inventory, `mcp/server/evidence/2026-10-17-console-session-auth/5.5-inventory.md`): nothing calls Kratos admin 4434 today; the `base_url: http://kratos-admin:4434/` cited by HIGH-1 is Kratos's own `serve.admin.base_url`. flint-gate and sso-broker call public 4433, so they are allowed there; flint-gate stays allowed on 4434 per the task text, for the security read to confirm or drop. PR know-me-cluster#39.
- [x] 5.6 Egress limits for the `ipfs` pod (security read of #38, MEDIUM-1): allow `0.0.0.0/0` except the pod and Service CIDRs, plus kube-dns 53 UDP/TCP. Acceptance: cluster PR with preflight; after merge, peering and DHT still work (`ipfs swarm peers` count and `ipfs routing findprovs` on a known CID through the RPC) and the pod cannot reach `kratos-admin.flint-core.svc:4434`.

## 6. Cluster: console cutover (ipfs-agent-architect)

Prerequisite: task 1.5 is done (go-jose at least `v4.1.6` in all three `go.mod` files). Do not start 6.1 before it.

- [ ] 6.0 Close #40 LOW-2 (operator-accepted interim residual, 2026-10-18). In the `flint-forge-postgres18` image's pg_hba, first-match order and ahead of the generic host rule: `host hydra hydra all scram-sha-256`, then `host all hydra all reject`; keep the upstream `scram-sha-256` host rule. Acceptance: after the image rollout, a connection as `hydra` to `hydra` succeeds and one to `flint` is rejected by pg_hba (operator-run, password never printed), and the security-reviewer marks LOW-2 resolved. Gates OG-26.

- [ ] 6.1 Cutover PR. Contents:
  - `ipfs-webui-auth` keeps its name and annotations; its spec becomes `oidc`:
    - issuer `https://oauth.know-me.tools`, `clientID: ipfs-console`, `clientSecret` Secret `ipfs-console-oidc`;
    - `redirectURL https://console.prometheusags.ai/oauth2/callback`, `logoutPath /oauth2/logout`;
    - scopes `openid offline_access email`, `cookieConfig.sameSite: Lax`, no `cookieDomain`, cookie names `ipfs-console-at` and `ipfs-console-idt`;
    - `forwardIDToken.header: Authorization`, `forwardAccessToken: false`, `passThroughAuthHeader: false`, `refreshToken: true`;
    - `denyRedirect` on `Sec-Fetch-Mode` Exact `cors`;
  - a `jwt` provider (issuer, audience `ipfs-console`, the remote JWKS);
  - `authorization` with `defaultAction: Deny` and one Allow rule on `sub` from the operator's list;
  - `csrf: {}`;
  - a console rule for `/oauth2/`;
  - `ipfs-mcp-config` gains `auth.oidc` and the `subjects` of the `ui` and `admin` principals from OG-26.
  
  Acceptance: the offline render passes, and `kubectl --context know-me apply --dry-run=server` of the SecurityPolicy, run read-only by the agent, is accepted (ADR unverified point 1). If it is rejected, stop and bring the header fallback in `design.md` to the operator.
- [ ] 6.2 Allowlist agreement script `lke-checks/session-allowlist.sh`. Acceptance: it exits nonzero when a principal subject in `ipfs-mcp-config` is absent from the SecurityPolicy rule, and zero on the PR's files.
- [ ] 6.3 Image check. The running `ipfs` sidecar image must contain the 2.x verifier commit (the schema rejects `auth.oidc` otherwise). Acceptance: the preflight records the running image digest and the commit it was built from, and the commit contains the 2.x merge.
- [ ] 6.4 PR body: "Rollback" (literal `git revert <merge-sha>`, the Argo app, the planned restart under the 2026-10-13 gotcha, no `lifecycle:` key) and "Break glass" (`kubectl --context know-me port-forward svc/ipfs-api 5001`). Acceptance: both sections are present, and `git diff` adds no `lifecycle:` key.
- [ ] 6.5 Check script `lke-checks/console-session.sh`, read-only and never printing tokens or cookies. Unauthenticated part:
  - a `/webui/` navigation gets 302 to the issuer;
  - a `fetch`-style request (`Sec-Fetch-Mode: cors`) to `/api/v0/version` gets 401;
  - a POST with `Origin: https://ipfs.prometheusags.ai` gets 403;
  - `/mcp` on the gateway host still answers 401 without a bearer.
  
  Operator part: a signed-in allowlisted browser reaches `/webui` and chat; a second, non-allowlisted Kratos account gets 403; a session still works 20 minutes after sign-in (unverified point 2); `/oauth2/logout` clears the session. Acceptance: the script exists and prints PASS or FAIL per check.
- [ ] 6.6 Security-reviewer read of the cutover PR before OG-26. Scope: RBAC default, route coverage, cookie scope, CSRF, the token carrier, the allowlist script, and rollback. Acceptance: findings file, no open CRITICAL.
- [ ] 6.7 After OG-26 (merge, sync, planned restart): record the script's unauthenticated output and the operator's part in a new dated evidence folder. Acceptance: README with PASS or FAIL per check. On any FAIL, the 6.4 revert was run and is recorded.

## 7. Retire the interim credentials (ipfs-agent-architect, operator)

- [ ] 7.1 After the soak (OG-27): a PR removing the console `ui` principal's `credential`, and the token Secret reference it used, from `ipfs-mcp-config`. The operator deletes `ipfs-webui-basicauth` and that token Secret out of band. Acceptance: the preflight shows no manifest referencing either Secret, and `kubectl --context know-me get secret -n ipfs` (names only) no longer lists them.
- [ ] 7.2 Close the residuals. Append amendments to ADR-0009 (M1) and ADR-0004 (M2) stating that they ended, with the 6.7 evidence path, and log the decision. Acceptance: both ADRs carry a dated amendment naming this change, and `.prometheus/decisions.md` has the entry.

## 8. Independent review at the phase boundary (security-reviewer, reviewer)

- [ ] 8.1 Security review of the merged `ipfs mcp` verifier (2.x) and the contract (1.2, 1.3), in a separate context from the builders. Acceptance: a findings file with no open CRITICAL or HIGH, or the operator's verbatim ruling on each one.
- [ ] 8.2 Quality and compatibility review. It confirms there is no `/api/v0`, gateway or CID change and that every new key has an off switch. Acceptance: findings file under `.agent-team/kubo-ipfs-agentic/findings/review/` with no blocking finding.
