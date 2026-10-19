# Security read: know-me-cluster PRs #50 (image and Web UI CID roll, task 6.3) and #48 (console cutover, tasks 6.1, 6.2, 6.4, 6.5)

Relayed verbatim by the lead. The security-reviewer context has no Write tool and no shell. This is task 6.6, and the 6.3 image PR is in scope with it.

- Artifacts:
  - #50: diff `scratchpad/pr50.diff`, body `pr50-body.md`; image evidence `mcp/server/evidence/2026-10-19-kubo-candidate-image-session-webui/README.md` and the `1.2-*` files there; `core/corehttp/webui.go` at HEAD.
  - #48: diff `scratchpad/pr48.diff`, body `pr48-body.md`, tree `scratchpad/kmc-cutover`, and the builder's upstream sources under `scratchpad/src/` (`envoy-oauth2-filter.cc` from Envoy v1.39.1, `eg-oidc.go`, `eg-securitypolicy.go`, `eg_ir.go` from Envoy Gateway v1.9.1).
  - Context: `mcp/server/evidence/2026-10-19-console-session-auth-6.0/README.md`, `.prometheus/decisions.md` (2026-10-19 entries), `.prometheus/gotchas.md` (2026-10-16), and the prior reads in this folder.
- Not verified here:
  - the tree SHAs, and the kubo diff of `1140ff10c` (no git);
  - any run of the check scripts, kubeconform, the dry run or the preflight (I read the scripts and the builder's reported output only);
  - Envoy `jwt_authn` source and Envoy Gateway's JWT and CSRF translators, which are not in `src/`. That covers the token extraction defaults (MEDIUM-4) and the HTTP filter order;
  - any live cluster, registry or browser state.

## Verdict

- **#50: PASS with one pre-merge gate.** No CRITICAL or HIGH.
  - The route stays pinned to exactly one Web UI CID, which is correct.
  - The 404 window fails closed.
  - `1140ff10c` changes no RPC, gateway or CID-recipe behavior.
  - Gate: LOW-13. Nothing in the evidence links the registry digest `sha256:3467dea8…` to the tested image `sha256:a50f357c…`.
- **#48: PASS with gates.** No CRITICAL or HIGH.
  - There is no window without auth on `/webui` or `/api/v0`, in the cutover or in its revert.
  - The `sub` allowlist is a default-Deny RBAC rule on one identity.
  - A client-supplied `Authorization` header cannot reach kubo 5001.
  - Two new MEDIUMs gate OG-26:
    - MEDIUM-3: the console now inherits the trust of every host that receives the `know-me.tools` Kratos cookie, including off-cluster third-party hosts. It needs written acceptance.
    - MEDIUM-4: the JWT provider takes Envoy's default token locations, which include the `access_token` query parameter. With token encryption off, the operator's ID token also lands in logout URLs. Pin `extractFrom`, or prove it safe with a negative probe.
  - The 6.5 script has a defect that will FAIL every run and miss a `Domain` attribute (LOW-14). Fix it in this PR.
  - New LOWs: LOW-15 to LOW-18. New INFO: INFO-K to INFO-O.
- **Carried:** MEDIUM-2 is RESOLVED on the 6.0b post-sync evidence. LOW-12 is narrowed; what remains of it moves into MEDIUM-3.

## Finding status

| ID | Status | Evidence and notes |
|---|---|---|
| MEDIUM-1 | RESOLVED (live) | 6.0 README:45-55. From an unlabeled flint-core pod, 4445 and 4444 are BLOCKED; the postgres positive control is REACHED; kratos-ui gets 200 on 4445; only the two Deployments hold the labels. With the 4.5 cross-namespace probe, the live part is complete. |
| MEDIUM-2 | **RESOLVED** | 6.0 README:69-81 meets the three post-sync acceptance items in `pr46-security-read.md`. All 25 listeners are Programmed, with counts equal to `live-sim.txt`. The required negative probe got `Accepted=False` / `NotAllowedByListeners`, and the probe path still reached the existing backend. `verify.sh argocd` had 0 failing. I accept these as the lead's observations. |
| LOW-9 | CLOSED (unchanged) | The listener check and its self-test PASS on `c5bc0ef` (README:78). |
| LOW-12 | Narrowed; the remainder becomes MEDIUM-3 | 13 dead records deleted (README:95-112). The operator kept `admin` (GCP, 80/443 open), `flint` and `the-boss` (Lovable), and `agent-runtime` and `agentic-social-networking` (CNAME `cname.manus.space`). Every one of them receives the Kratos cookie (README:110). |
| `ipfs mcp` JWKS gate (#42 item 6) | ACCEPTED on the lead's code read | README:57-59. Unknown keys fail closed (`jwt.go:184-191`). Serving cached keys through an issuer outage is bounded by the 15m ID token and the 30 s refetch window. On the console the gateway enforces first, so `ipfs mcp` is defense in depth (INFO-O). |
| Post-logout URIs (out of band) | DONE | README:85-93: `["https://console.prometheusags.ai/", "https://console.prometheusags.ai/webui/"]`; `client_id` and `redirect_uris` unchanged. |
| go-jose >= v4.1.6 (task 1.5) | MET | `go.mod:102` `v4.1.6`, through the p2p-forge fork replace (`go.mod:288-289`), per the 2026-10-17 decision. |

## #50 assessment

1. **Pinned to one CID: correct.**
   - `pr50.diff:41-46` moves the console `/ipfs/<CID>` PathPrefix to `bafybeihd7…mpa`, which equals `WebUIPath` (`core/corehttp/webui.go:20`). The `postStart` pin (`pr50.diff:18-19`) moves with it.
   - Gateway API PathPrefix matches on path-segment boundaries. Envoy normalizes the path before matching. So `/ipfs/<CID>/..` cannot reach another CID on this credentialed origin.
   - The old fork CIDs stay in `WebUIPaths` (`webui.go:25-26`). Kubo still serves them on 5001, but no console rule routes to them.
   - Keeping the old prefix alongside the new one would expose a build without session mode on the console origin. Declining that is the right call.
2. **The ~1 minute `/webui` 404 fails closed.**
   - The route moves first. The old daemon then redirects to the old CID, which no rule matches, so Envoy answers 404 itself.
   - Nothing is exposed, and the reverse order also ends in a 404. Accepted.
3. **`1140ff10c` and Kubo's guarantees.**
   - The only Go change is the `WebUIPath` constant and the `WebUIPaths` list (`webui.go:14-27`, evidence README:23). Its only effect is the target of the 302 from `/webui/` (`webui.go:154-155`).
   - No `/api/v0`, gateway or CID-recipe change. In the evidence, `TestGateway` has 75 PASS, and `TestDefaultMatchesExpectedProfile` and `TestWebUI` pass.
   - `RepoVersion` is 18 (`version.go:36`), as it was at `3d8a128c5` (lead's check in `kubo-candidate-image-security-read.md`), so a revert under `--migrate=true` is safe.
   - The CAR import and the pin are operator-directed local causes, so there is no silent CID injection. The CID binds the content, so the build's integrity reduces to the reviewed source commit `81674a5e`.
4. **Provenance: LOW-13.**
   - The evidence README still says "NOT pushed … Local digest: none" (README:5, `1.2-image-inspect-summary.txt`).
   - The #50 body claims "The pushed digest is the tested image (local ID `sha256:a50f357c…`)". No artifact records the check; the session note only says "I'll confirm it's the tested image".
   - The image ID and the registry manifest digest are different hashes, so the link has to be shown. One of these is enough:
     - `docker image inspect ghcr.io/gqadonis/kubo:1140ff10c --format '{{.Id}} {{json .RepoDigests}}'` shows `a50f357c…` together with `@sha256:3467dea8…`;
     - `docker buildx imagetools inspect --raw ghcr.io/gqadonis/kubo@sha256:3467dea8…` has `config.digest` = `sha256:a50f357c…`. If it is an index, check its linux/amd64 manifest.
   - Append the output to the evidence README.
   - After the merge, `ipfs version --commit` = `1140ff1` proves the commit, not the tested build.
5. **Sequencing with #48.** `ipfs-mcp` restarts on its own (the pre-existing start race: "restarts 2"). ConfigMap volume updates propagate to the mounted directory. So if #48's ConfigMap lands while the sidecar still runs `38d453a5…`, which has no verifier and whose schema rejects `auth.oidc`, the sidecar crash-loops. The pod then goes NotReady, and the console and the gateway go with it. #50 must be synced, with both containers on `3467dea8…`, before #48 merges. The #48 body says so (Dependencies 1, Merge order 1).

## #48 assessment

### 1. No window without auth (sync order, Prune=false, the 2026-10-16 gotcha)

- **The swap is one object updated in place.**
  - `ipfs-webui-auth` keeps its name, `targetRefs` and annotations (`kmc-cutover/namespaces/ipfs/manifests.yaml:723-755`). `basicAuth` is replaced by `oidc`, `jwt`, `authorization` and `csrf` in a single update at sync-wave -1.
  - The API server applies it atomically, and the CEL schema accepted it in the server dry run.
- **A translation failure fails closed.** If Envoy Gateway cannot translate the policy (missing secret, discovery fetch, schema), it sets a 500 direct response on every rule of the targeted route (`src/eg-securitypolicy.go:1412-1471`). Nothing falls back to "no auth".
- **The gotcha does not apply.** It concerns narrowing `targetRefs` and pruning a route. #48 keeps `targetRefs`, deletes nothing, and only adds route rules at wave 0, after the policy. Between the waves `/oauth2/callback` has no rule, so a login cannot complete for a few seconds. That fails closed.
- **The revert is symmetric.** `basicAuth` returns at wave -1 first. The extra rules (`/auth/session` to `ipfs-mcp`) are removed at wave 0 while already covered by basic auth.
- **Prune=false is retained** (`:733`), and Secret `ipfs-webui-basicauth` stays until 7.1.
- **Residual: a field-manager edge.** If the live object ends up with both `basicAuth` and `oidc`, it fails closed (double auth or not Accepted). `console-session.sh` check 3 flags a leftover Basic challenge.

### 2. The allowlist

- `authorization.defaultAction: Deny` with one Allow rule: jwt principal `hydra`, claim `sub`, `valueType: String`, one value `461e5d70-…6881` (`:817-829`). No `email` claim, no CIDR, no header condition.
- A valid `ipfs-console` token for any other Kratos identity, which open registration makes easy to obtain (INFO-E), matches no rule and gets 403. The PR states the same and operator step b checks it (`console-session.sh:245-247`).
- `ipfs-mcp-config` maps the same `sub` to `console-ui` (`:896-905`). `session-allowlist.sh` keeps the gateway list a superset.
- The decision is only as good as the JWT payload that RBAC reads. See MEDIUM-4.

### 3. Route coverage

- The policy targets HTTPRoute `ipfs-webui` without `sectionName` (`:752-755`), so every rule on it is covered: `/webui`, `Exact /` redirect, `Exact /oauth2/callback` and `/oauth2/logout`, `Exact /auth/session`, `/api/v0`, and `/ipfs/<CID>` (`:632-721`).
- Envoy's HTTP filters run before the router's redirect, so the `/` redirect is also behind sign-in and RBAC.
- **`/oauth2/logout`** is matched before any cookie check and answered locally (`src/envoy-oauth2-filter.cc:943-946`, `:1355-1430`).
- **`/oauth2/callback`** is always answered locally:
  - signed in: redirect or 401 (`:956-990`);
  - not signed in: CSRF and state validation, then the token exchange (`:1049-1097`).
  - The only Continue is an OAuth client `FailureContinue` (`:1090-1091`). Even then there is no forwarded JWT, so RBAC denies before kubo.
  - The Exact matches keep every other `/oauth2/...` path off kubo.
- **`/auth/session`**: OAuth2, JWT and RBAC run at the gateway, then `ipfs mcp` re-verifies the forwarded ID token (routes `[session]`, `:890-892`).
- The gateway host's `/mcp` is a separate route with no session policy, bearer-only (`:952-977`). With routes `[session]`, a JWT there gets 401 (task 2.4).

### 4. CSRF and `denyRedirect`

- **CSRF.** `csrf: {}` (`:833`) compares Origin (or Referer) with the destination on mutating methods. A page on `ipfs.prometheusags.ai` is same-site, so Lax alone would not stop it; this filter does.
- **CSRF filter order: not verified.**
  - If CSRF runs after OAuth2, an unauthenticated POST without Origin gets a 302 or 401 rather than 403. That is still a refusal, but checks 5 and 6 (`console-session.sh:202-215`) would report FAIL.
  - An authenticated cross-origin POST is refused in either order.
  - So the order changes the script's expectations, not the security outcome.
- **`denyRedirect`** on `Sec-Fetch-Mode: cors` (`:800-804`; `src/eg-oidc.go:227-228`; `src/envoy-oauth2-filter.cc:1035-1038`) only chooses between 401 and a 302 to Hydra. Both stop the request, so it cannot bypass auth.

### 5. Cookie scope and the cost of `disableTokenEncryption`

- **Every cookie is Secure, HttpOnly and host-only.** Each is written with `;path=…;Max-Age=…;secure;HttpOnly` plus SameSite (`src/envoy-oauth2-filter.cc:57`, `:1547-1553`). There is no `;domain=` because `cookieDomain` is unset (`src/eg-oidc.go:210-212`; `:775-777` here), so the cookies are host-only on `console.prometheusags.ai`.
- **Lax is applied to all seven cookies** (`src/eg-oidc.go:294-303`). The nonce and code-verifier cookies are scoped to `/oauth2/callback` (`:283-291`).
- **The HMAC cookie binds the host, the expiry and all three tokens** (`src/envoy-oauth2-filter.cc:811-815`), so a token cookie cannot be swapped.
- **Encryption off adds little exposure.**
  - The upstreams (kubo, `ipfs mcp`) already received the access and ID tokens in clear: Envoy decrypts the cookies before forwarding (`:915-919`), and `removeOAuthFlowCookies` keeps them (`:1958-1991`).
  - Refreshing with the 12h refresh token needs the client secret (`client_secret_basic`, 6.0 README:67).
  - Hydra's access token is opaque.
- **The real new exposure is the ID token in the logout URL.** `id_token_hint` carries the raw cookie value (`:60`, `:1404-1408`), so a JWT for `aud: ipfs-console`, valid up to 15m, lands in browser history, the argocd-gateway access log (query string) and Hydra's request logs. That feeds MEDIUM-4.
- ADR-0013 section 1 still says the cookies are encrypted. The architect should amend it (INFO-K).

### 6. `forwardIDToken: Authorization` with `passThroughAuthHeader: false`

Safe. A client-supplied `Authorization` header cannot reach kubo 5001.

- `passThroughAuthHeader` is false, so no pass-through matchers are built (`src/eg-oidc.go:223-225`). A request carrying `Authorization` therefore never skips the OAuth2 filter (`src/envoy-oauth2-filter.cc:905-908`).
- Forwarding on `Authorization` forces `PreserveAuthorizationHeader=false` (`src/eg-oidc.go:243-248`), so the filter removes the client's header (`src/envoy-oauth2-filter.cc:921-927`).
- The filter sets `Authorization` only from an HMAC-valid cookie (`:1125-1136`, `:1181-1190`).
- If someone later sets `passThroughAuthHeader: true`, a bare ID token in `Authorization` would skip the cookie check entirely. Nothing pins it today (LOW-15).

### 7. The egress rule

- `networkpolicy-egress.yaml` (diff `:962-973`) allows TCP 10443 to pods labelled `owning-gateway-name=argocd-gateway`, `owning-gateway-namespace=argocd` in `envoy-gateway-system`.
- The policy selects the whole `ipfs` pod, so both the kubo daemon and the sidecar get it.
- What it reaches is every argocd-gateway HTTPS listener, the same set the internet reaches. Only 10443 is open: not the HTTP or admin ports, and not Hydra admin.
- The one difference is the client IP: Envoy sees a pod IP, not the LB path. No SecurityPolicy in git authorizes by `clientCIDRs` (the only SecurityPolicy under `namespaces/` is `ipfs-webui-auth`). Live-only objects exist (INFO-I), so confirm this live (follow-up 4; INFO-L).
- The `ipfs-gateway-gw` Envoy also has to reach the issuer (discovery, token, JWKS) through the argocd-gateway hairpin. That is proven only by the first post-sync login. A failure fails closed.

### 8. Logout residual after 15 minutes idle (LOW-17)

- The sign-out path runs before any refresh (`src/envoy-oauth2-filter.cc:943-946`), and the ID-token cookie's Max-Age follows `exp` (15m). An idle logout therefore sends an empty `id_token_hint`, and Hydra refuses it (PR body).
- Envoy has already cleared the console cookies, so the console session ends. The Hydra login session and the 30-day Kratos session survive.
- The next console visit signs in silently, with no password. On a shared device, a user who believes they signed out is still signed in.
- Accept it in writing as a residual, or take the PR body's fix: the session-mode Web UI makes one same-origin request through the filter just before navigating to `/oauth2/logout`.

### 9. Check scripts

- **Secrets are handled correctly.**
  - `console-session.sh` prints only cookie names (`:139`), the decoded state `url` and parameter names (`:99-131`), and HTTP codes.
  - Header and body files sit in `mktemp -d`, removed on EXIT (`:75-76`). `set +x` is set (`:60`), and nothing is followed (no `-L`).
  - `session-allowlist.sh` prints Kratos identity IDs, which are not secrets.
- **One soundness defect in `console-session.sh`: LOW-14** (below).
- **`session-allowlist.sh` is sound for what it asserts.**
  - Exactly one policy and one ConfigMap (`:61-66`).
  - Each Allow rule is a jwt principal only, with exactly one claim `sub` of type String (`:101-114`).
  - Issuer, audience and JWKS agree between the policy and `ipfs mcp` (`:127-139`).
  - 12 meaningful mutations (`:161-199`).
- **Gaps in `session-allowlist.sh` and `webui-auth-targeting.sh`: LOW-15.** The carrier settings are not pinned, and `webui-auth-targeting.sh` counts only `ipfs-api` backends (`webui-auth-targeting.sh:6-9`).
- **`origin-isolation.sh`** `check_refused` accepts 401 or 403, rejects kubo's body, and prints at most 200 characters of the gateway body. Fine.

### 10. Rollback and break glass

- **The rollback is sound.**
  - `git revert` brings `basicAuth` back at wave -1 in one sync, the htpasswd Secret is kept, and no `lifecycle:` key is added.
  - The ConfigMap only takes effect at the next sidecar start. A reverted ConfigMap with no restart is harmless, because the `/auth/session` rule is gone.
- **Break glass works only on local port 5001.**
  - `port-forward svc/ipfs-api 5001` works, because kubo always safelists `http://127.0.0.1:<port>` and `http://localhost:<port>` with the listener's port (`core/corehttp/commands.go:40-46`, `:90-121`).
  - A different local port would make the Web UI's POSTs fail kubo's origin check. If the operator's own node already holds 5001 (AGENTS.md warns of this), the forward fails.
  - Add a fallback to the runbook: `ipfs --api=/ip4/127.0.0.1/tcp/<port> …` (the CLI sends no Origin), or stop the local node first.

## New items

- **MEDIUM-3: after the cutover, a `know-me.tools` Kratos session is a console login, and off-cluster third-party hosts receive that cookie.**
  - Evidence: Kratos `session.cookie.domain: know-me.tools`, 720h (`pre-og26-pr44-pr45-security-read.md` MEDIUM-2). Consent is skipped for `ipfs-console` (task 4.6). The `ui` principal has no step-up.
  - The 6.0 README (:110-112) lists names the operator kept: `admin` (GCP 35.194.48.152, 80/443 open), `flint` and `the-boss` (Lovable 185.158.133.1), `agent-runtime` and `agentic-social-networking` (CNAME `cname.manus.space`).
  - Mechanism: the browser sends `ory_kratos_session` on every same-site request to those hosts, and the hosting platforms terminate TLS. Anyone holding the cookie can replay it at `auth.know-me.tools` and complete a silent `ipfs-console` login. The allowlist does not help, because the stolen session *is* the operator's. The result is full Kubo RPC for up to 30 days.
  - A CNAME to a platform hostname is also a classic subdomain-takeover target if the platform project is ever deleted.
  - Before #48, basic auth was independent of Kratos. This is the trust #48 newly imports.
  - Action before OG-26: the operator accepts this in writing as a residual (as with LOW-2 and MEDIUM-2), or removes or repoints those five names. Narrowing the cookie domain stays off the table (flint-gate depends on it).
- **MEDIUM-4 (NEEDS VERIFICATION, gate): the `hydra` JWT provider uses Envoy's default token locations, which include `?access_token=`.**
  - Evidence: `jwt.providers[0]` sets no `extractFrom` (`:805-812`). Envoy Gateway's own IR states that an empty `ExtractFrom` "defaults to extract JWT token from the Authorization HTTP request header using Bearer schema or access_token from query parameters" (`src/eg_ir.go:1449-1452`). The OAuth2 filter removes only `Authorization`.
  - Attack shape:
    1. A non-allowlisted Kratos user (registration is open) signs in to the console and holds valid cookies, so the OAuth2 filter passes and forwards their own ID token.
    2. They add `?access_token=<operator ID token>`.
    3. `jwt_authn` then holds two valid tokens from the same provider. Which payload RBAC reads is the open question.
    4. If it is the query token, RBAC sees the operator's `sub` and allows full `/api/v0`.
  - The operator's ID token is now readable in logout URLs: browser history, the gateway access log, Hydra logs (section 5). It is valid for up to 15 minutes.
  - I could not read Envoy's `jwt_authn` multi-token handling or Envoy Gateway's JWT translator. Severity is MEDIUM because the leak precondition is narrow, but the impact is RPC takeover and the fix is one stanza.
  - Fix in this PR: on provider `hydra`, add `extractFrom: {headers: [{name: Authorization, valuePrefix: "Bearer "}]}`. The carrier stays `Authorization`, so this is not the design.md header fallback. Confirm with the server dry run and `Accepted` after sync.
  - Either way, add a 6.7 negative probe. In the non-allowlisted profile, request `GET /webui/` and `POST /api/v0/version` with console Origin and `?access_token=<the operator's current ID token>`, read from the operator's own devtools; expect 403. Never print or record the token.
- **LOW-13 (#50, gate): the provenance link between registry digest and tested image is unrecorded.** See the #50 assessment, point 4.
- **LOW-14 (gate for 6.7, fix in #48): `console-session.sh` check 8 cannot pass and cannot see a `Domain` attribute.**
  - `check_cookies` matches `"; secure"`, `"; httponly"` and `"; domain="`, each with a space (`console-session.sh:141-144`).
  - Envoy writes `;path=…;Max-Age=…;secure;HttpOnly;SameSite=Lax` and `;domain=…` with no space (`src/envoy-oauth2-filter.cc:57-58`, `:83`, `:1549-1553`).
  - Result: every run reports "no Secure" and "no HttpOnly" and ends `RESULT: FAIL … rollback`. That invites either a needless revert or the habit of ignoring this script.
  - A real `Domain` attribute would pass silently.
  - Fix: split each Set-Cookie on `;`, trim, lower-case, and compare attribute names exactly. Add a `--self-test` with Envoy-format sample lines, both with and without `;domain=`.
  - The unauthenticated part sees only the nonce and verifier cookies. The five session cookies are set at the callback, so add an operator step that confirms their attributes in devtools: names and attributes only, never values.
- **LOW-15 (recommended now; required before `lke-agent-settings`): the checks do not pin the token carrier.**
  - Add to `session-allowlist.sh`:
    - `oidc.passThroughAuthHeader` absent or false;
    - `forwardAccessToken` false;
    - `forwardIDToken.header` == `Authorization`;
    - `jwt.providers[*].extractFrom` exactly the Authorization header (after MEDIUM-4);
    - no `cookieDomain`;
    - `cookieConfig.sameSite` in {Lax, Strict}.
  - Add a self-test mutation for each.
  - Extend `webui-auth-targeting.sh` so that any console-host rule with a backendRef to `ipfs-mcp` must also be covered. Today `/auth/session` is covered only because it sits on the same route. `/agui` arrives next and is write-capable.
- **LOW-16 (recommended): same-site sibling surface.** `ipfs.prometheusags.ai` serves arbitrary CID content and is same-site with the console.
  - **(a) Framing.** Lax session cookies are sent in an iframe from that host. The Web UI has no `frame-ancestors` or `X-Frame-Options` (carried Round 5 L3, still OPEN), so the console's one-click actions can be clickjacked. Add a ResponseHeaderModifier on route `ipfs-webui` with `X-Frame-Options: DENY` and `Content-Security-Policy: frame-ancestors 'none'`. Cross-site framing is already improved: Lax cookies are not sent in cross-site iframes, whereas basic-auth credentials were.
  - **(b) Cookie tossing.** Script there can set `Domain=prometheusags.ai` cookies under the console's cookie names, including a complete valid set from the attacker's own login (the HMAC binds the host, which matches). The outcome is 403 or a login loop for the operator. It is not escalation: tokens are HMAC-bound and RBAC is by `sub`. `/oauth2/logout` cannot delete tossed cookies. Accept as a DoS-only residual. `__Host-` names are possible only for the access and ID token cookies, because Envoy Gateway fixes the HMAC and refresh names.
- **LOW-17 (residual, accept in writing or fix in the Web UI): logout after 15 minutes idle leaves Hydra and Kratos signed in.** See section 8.
- **LOW-18 (gate): the image was smoke-tested with a different config.**
  - `cfg-oidc.json` (evidence) has `groups ["*"]`, an `admin` principal and `agent.agui`. It has no `routes`, no `algorithms` and no `require_origin`.
  - The PR's `config.json` has `routes ["session"]`, `algorithms ["RS256"]`, `require_origin: true`, `groups: []` and no admin (`:879-905`).
  - Schema validation is not binary acceptance. If the binary refuses this config, the sidecar crash-loops and takes the pod NotReady.
  - Before the merge, run `ghcr.io/gqadonis/kubo@sha256:3467dea8…` offline with exactly this `config.json`. Expect startup and the log line naming route `session` only, and `/auth/session` answering 401 without a token.
- **INFO-K: ADR-0013 section 1 says the cookies stay encrypted.** Decision 2 (2026-10-19) changed that. The architect should append an amendment. This is record-keeping, not a security gate.
- **INFO-L: egress client-IP difference.** See section 7 and follow-up 4.
- **INFO-M: logout CSRF.** A cross-site link to `/oauth2/logout` is a top-level GET, so Lax cookies are sent and the console session ends. Hydra's logout then goes through kratos-ui's confirmation. Nuisance only.
- **INFO-N: no open redirect through `state`.** Envoy Gateway sets no `allowed_redirect_domains`, and `isHostAllowedDomain` returns true for an empty list (`src/envoy-oauth2-filter.cc:318-321`). But a state is honored only when its `csrf_token` matches the HMAC-signed nonce cookie in the victim's own browser (`:1901-1915`, `:1936-1949`), so an attacker cannot plant a state URL.
- **INFO-O: task 8.1 (security review of the merged `ipfs mcp` verifier) is still open.** Today the verifier guards only the read-only `/auth/session`, behind the gateway's own JWT and RBAC, so it is not an OG-26 gate. It must complete before `lke-agent-settings` exposes `/agui` or settings to JWT identities.

## MCP note (`ipfs mcp`, network-exposed through the gateway)

- MCP-01:
  - `/auth/session` is authenticated twice: gateway OIDC + JWT + RBAC, then the server's own JWT check with `sub` mapping.
  - `/mcp` on the gateway host stays bearer-only, and JWT is refused there (routes `[session]`).
  - The PASS is by configuration; the verifier review is still owed (INFO-O).
- MCP-03: still no per-identity rate limit (carried from `kubo-candidate-image-security-read.md` S7).
- MCP-02, MCP-04 and MCP-05: unchanged by these PRs.

## Gates before OG-26 (the operator's merge of #48)

1. **#50 first:**
   - before the #50 merge, record the provenance link (LOW-13);
   - after its sync, `ipfs-0` is Ready with both containers on `sha256:3467dea8…` and `ipfs version --commit` = `1140ff1`;
   - `verify.sh ipfs` and `origin-isolation.sh unauth` PASS;
   - console `/webui/` 302 to `/ipfs/bafybeihd7…`.
2. **MEDIUM-4:** pin `extractFrom` on provider `hydra` in #48 (preferred), or bring it to the operator with the 6.7 negative probe as a required check.
3. **MEDIUM-3:** written acceptance on #48 or in `decisions.md`, or the five off-cluster names removed or repointed.
4. **LOW-14:** fix the cookie-attribute check in `console-session.sh` (with a self-test) before the 6.7 run.
5. **LOW-18:** the exact-config smoke of the deployed image.
6. **Rebase #48 onto main after #50 merges.** Re-run `webui-auth-targeting.sh`, `session-allowlist.sh` (and `--self-test`), `networkpolicy-check.sh` and the dry run on the merged tree, and post the results.
7. **Already met:** MEDIUM-1 live, MEDIUM-2 resolved, the JWKS fail-closed code read, the post-logout URIs, and go-jose v4.1.6.

### Post-sync acceptance (6.7)

- `ipfs-webui-auth` is `Accepted`, and no route answers 500.
- `console-session.sh unauth` PASS.
- Operator steps a through e. In step b, also check `/auth/session` returns 403 for the second account.
- Two new probes:
  - `GET /api/v0/version` with `Sec-Fetch-Mode: cors` and `Authorization: Bearer x` and no cookies answers 401 from the gateway, not kubo's body;
  - the MEDIUM-4 `?access_token=` probe from the non-allowlisted profile answers 403.
- On any FAIL, run the 6.4 revert and record it.

### Recommended, not blocking

- LOW-15, LOW-16, LOW-17 (Web UI fix);
- INFO-K;
- carried: INFO-H, INFO-7, LOW-7, LOW-10, INFO-A, INFO-G, INFO-J, the LOW-4 negative PKCE test, Round 5 L3.

## Manual follow-ups

1. Confirm the tree SHAs, and run each check script and its self-test on the rebased #48 tree.
2. Read Envoy v1.39.1 `source/extensions/filters/http/jwt_authn/authenticator.cc` and `filter.cc`: when one provider extracts two valid tokens, which payload is written to `payload_in_metadata`? Also read Envoy Gateway v1.9.1 `internal/xds/translator/jwt.go`: are `from_headers` and `from_params` left empty when `extractFrom` is nil? Then settle MEDIUM-4's severity.
3. Read Envoy Gateway's HTTP filter order for `csrf` relative to `oauth2` (`internal/xds/translator/httpfilters.go`). Adjust `console-session.sh` checks 5 and 6 if CSRF runs after OAuth2.
4. Live: `kubectl --context know-me get securitypolicy -A -o yaml | grep -n clientCIDRs` returns nothing for argocd-gateway routes (INFO-L).
5. Check whether the argocd-gateway access log records query strings. If it does, note that the `id_token_hint` value appears there, and consider a path-only log format for the oauth host.
6. After the sync, confirm the token cookies' attributes in devtools (names and attributes only), and confirm that no `ipfs-console-*` cookie has a `Domain`.

## Lead's checks (2026-10-19, appended by the lead, not part of the review)

- LOW-13 closed: `docker image inspect ghcr.io/gqadonis/kubo:1140ff10c` gives Id `sha256:a50f357c…` with RepoDigest `@sha256:3467dea8…`, and `docker buildx imagetools inspect --raw …@sha256:3467dea8…` gives a single-platform manifest whose `config.digest` is `sha256:a50f357c…`. Recorded in `mcp/server/evidence/2026-10-19-kubo-candidate-image-session-webui/README.md`.
