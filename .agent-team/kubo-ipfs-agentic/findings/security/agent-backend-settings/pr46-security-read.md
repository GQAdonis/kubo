# Security read: know-me-cluster PR #46 (console-session-auth 6.0b, MEDIUM-2 and LOW-9)

Relayed verbatim by the lead (the security-reviewer context has no Write or shell).

- Artifacts: diff `scratchpad/pr46.diff`, body `pr46-body.md`, tree `scratchpad/kmc-medium2`, live evidence `m2live/live-sim.txt`, `m2live/selftest.txt`, `m2live/evidence/argocd/preflight-20261019T115455Z.txt`.
- Prior reads: `pre-og26-pr44-pr45-security-read.md` (MEDIUM-2, LOW-9), `pre-og26-pr44-pr45-confirmation.md`.
- Not verified here:
  - the tree SHA;
  - any run of `know-me-listener-check.sh` (I read the script and the builder's self-test output only);
  - the live-simulation script itself (only its output is in evidence);
  - live Gateway, HTTPRoute and RBAC state after the cut-off of the lead's read;
  - the Cloudflare `know-me.tools` record set.

## Verdict

- **PASS.** No CRITICAL or HIGH.
- **MEDIUM-2: CLOSED in repo** for every listener that serves a `know-me.tools` host. It moves to RESOLVED on the post-sync live checks below.
- **LOW-9: CLOSED.** The new check pins the INFO-B stanzas and all 21 `know-me.tools` listeners, and its self-test mutations are meaningful.
- New: LOW-11 (do not use an empty selector for runtime), LOW-12 (check scope is one Gateway; dormant `matrix` solver; DNS not enumerated), INFO-H (the cookie and Kratos CORS reach every workload behind an admitted host), INFO-I (live-only knowme routes), INFO-J (self-test gaps).

## Finding status

| ID | Status | Evidence and notes |
|---|---|---|
| MEDIUM-2 | CLOSED in repo; RESOLVED on post-sync live checks | `kmc-medium2/namespaces/argocd-config/manifests.yaml:254-612`: every `know-me.tools` listener has `kinds: [HTTPRoute]` and `from: Selector` on `kubernetes.io/metadata.name`. The values are flint-core (`:254-348`, `:425-443`), `In [argocd, flint-core]` on the five wildcard-cert `-http` listeners (`:444-528`), knowme (`:349-405`, `:529-567`), knowdesign (`:406-424`, `:568-580`) and realtor-demo (`:581-612`). The API server sets `kubernetes.io/metadata.name` and it cannot be changed, so the selector cannot be spoofed with a label. Only `onyx-*` and `edgequake-*` stay `from: All` (`:214-253`). Their hostnames are `*.prometheusags.ai`, which never intersect a `know-me.tools` route hostname, so they cannot serve a host that receives the cookie. `live-sim.txt:1-26` shows all 25 listeners keep their attached-route counts. |
| LOW-9 | CLOSED | `know-me-listener-check.sh:54-76` (EXPECTED) includes `flint-auth-https` (flint-core) and `flint-auth-http` (`In [argocd, flint-core]`). `:125-127` requires exact dict equality with `want_allowed()` (`:84-90`). Any `from: All`, `Same`, extra namespace, extra kind or missing `kinds` therefore fails. `selftest.txt:1-13`: the baseline passes and all 11 mutations fail, each with exactly 1 failing assertion, so each mutation hits its intended assertion and nothing else. Like its sibling checks, it runs by convention rather than in CI (the repo has no `.github/`). That is the same posture as `oauth-listener-check.sh`, not a regression. |

## Assessment: is MEDIUM-2 closed for every host that receives the cookie?

1. **argocd-gateway.** Yes. Every listener whose hostname is `know-me.tools` or a subdomain now admits only the namespace that owns its routes. The gateway has no hostless or wildcard listener (`manifests.yaml:214-612`; asserted at `know-me-listener-check.sh:129-135`).
2. **Other Gateways in git.** `ipfs/ipfs-gateway-gw` (`namespaces/ipfs/manifests.yaml:502-539`) has only `prometheusags.ai` listeners, and they are `from: Same`. No other Gateway object exists under `namespaces/`.
   - Envoy Gateway gives each Gateway its own load balancer here: the two Gateways have different addresses, 23.239.29.33 and 45.56.67.67 (`cluster-facts.md:9-34`). There is no `mergeGateways` in the tree.
   - So a rogue Gateway with a `*.know-me.tools` listener would need both Gateway-create rights (admin-equivalent, per the lead's RBAC read) and a DNS record pointing at its address.
3. **Outside the check: the dormant `matrix` solver** (LOW-12). `cluster/manifests.yaml:12-31`: ClusterIssuer `letsencrypt-http01` still has a solver for `chat`, `livekit` and `matrix.know-me.tools` on `matrix/{cinny,livekit,matrix}-gateway`. `cluster-facts.md:81` says those Gateways are missing.
   - If those names still have DNS records, they are hosts that receive the cookie and are outside every check.
   - If such a record points at a released Linode IP, this is a subdomain-takeover path to the 30-day session cookie that needs no cluster access at all.
   - Preflight checks DNS only for listener hostnames (`preflight-20261019T115455Z.txt:20-83`). It does not enumerate the zone, and the zone has no CAA record (`cluster-facts.md:40`).
4. **The remaining exposure is workloads, not routes** (INFO-H). MEDIUM-2 as written (a route from an unrelated namespace) is closed. The cookie still reaches every backend behind an admitted host, including realtor-demo and the live-only knowme site. Kratos CORS also allows `https://*.know-me.tools` with credentials (`flint-core/manifests.yaml:1298`).

## Assessment: admitting `knowme` and `knowdesign`

Correct. Each admitted namespace is the one whose routes attach today (`live-sim.txt:10-13,20-23`). That is the least privilege that keeps traffic flowing.

Who can create routes there is the question that matters. The lead's RBAC read found no namespaced binding that grants HTTPRoute creation to a non-system subject outside `envoy-gateway-system`, so knowme and knowdesign are admin-only today. One gap remains: that read did not test the namespaces' own ServiceAccounts or any aggregated `edit`/`admin` ClusterRole. See follow-up 3.

## Assessment: runtime.know-me.tools

**Recommend admit `knowme`, as shipped.** knowme already serves the apex `know-me.tools`, which receives the same cookie, so admitting it adds no principal.

**Do not take the PR body's alternative of "an empty selector"** (LOW-11). In Kubernetes, an empty LabelSelector (`selector: {}`) matches every namespace. That would turn this listener back into `from: All` and reopen MEDIUM-2 on that host.

If the operator wants nothing admitted, delete both `knowme-runtime-*` listeners. The certificate's runtime SAN is harmless, and a TLS connection with an unmatched SNI fails the handshake. Then remove the two entries from EXPECTED.

## Assessment: is the check sound?

- **What it catches.** It cannot pass while one of the 21 listed listeners is open, because of the exact-equality check (`:125-127`). It catches an unlisted `know-me.tools` listener (`:133-135`) and any hostless or wildcard listener on argocd-gateway (`:131-132`).
- **Route checks** (`:137-159`):
  - A route without `sectionName` (which includes routes bound by `port`) must name only hosts outside the zone and must name at least one host. A hostless, unpinned route fails, which is correct because it would inherit every listener's hostname.
  - A pinned route must come from an admitted namespace and name only that listener's host.
  - `p.get("namespace", ns)` (`:145`) correctly defaults to the route's own namespace.
- **Scope gaps** (LOW-12, INFO-J):
  - It loads only `namespaces/*/*.y*ml` (`:95`) and inspects only `argocd/argocd-gateway` (`:112-114`). A second Gateway in git with a `*.know-me.tools` or hostless listener passes.
  - It cannot see live-only objects (INFO-I).
  - The self-test has no mutation for a wildcard listener hostname: the hostless mutation (`:210-213`) exercises only the `bool(h)` half of `:131`. It also has none for `selector: {}` or for a route pinned to an unlisted `sectionName` with a zone host. These are covered by the logic, but not proven.

## Assessment: live-only knowme routes (INFO-I)

The routes are a provenance gap, not an admission gap. After this PR, only principals that can write in `knowme` can attach to the knowme hosts, and they are admin-equivalent. But knowme's routes and workloads have no git review trail, Argo self-heal does not cover them, and the offline check cannot see them. The site behind the apex receives the cookie on every request and is inside the Kratos CORS allowlist.

Recommend:
- adopt `knowme` into git under an Argo Application; or
- at minimum, commit the live-simulation script as an `lke-checks` live check and run it from `verify.sh argocd`.

## New items

- **LOW-11: an empty selector for runtime would reopen MEDIUM-2.** See the runtime assessment. Keep `knowme`, or delete the listeners. Never use `selector: {}`.
- **LOW-12: the check covers one Gateway, a dormant solver names three cookie-receiving hosts, and the zone is not enumerated.**
  1. Extend assertion 3 to every Gateway in the tree: no listener outside argocd-gateway may have a `know-me.tools`, wildcard or hostless hostname unless it is listed.
  2. Remove the stale `matrix` solver from `cluster/manifests.yaml:12-31`, or record why it stays.
  3. Enumerate the `know-me.tools` zone. Confirm there is no `*` record and no `chat`, `livekit` or `matrix` record, and that every A or CNAME points at 23.239.29.33 or another address the operator owns.
  4. Consider a CAA record limiting issuance to Let's Encrypt.
- **INFO-H: the remaining trust boundary is the workloads behind admitted hosts.** This is pre-existing and outside this PR.
  - `ory_kratos_session` is HttpOnly, so page scripts cannot read it. But every backend behind a `*.know-me.tools` host receives it, including realtor-demo, knowdesign and the live-only knowme site.
  - Kratos CORS `allowed_origins: https://*.know-me.tools` (`flint-core/manifests.yaml:1298`) lets script on any of those origins make credentialed requests to the Kratos public API. An XSS on realtor-demo could start self-service settings flows inside the privileged-session window.
  - Recommend narrowing Kratos CORS to the origins that call Kratos from the browser. Where a route serves a static or demo site, consider dropping the `Cookie` request header with a RequestHeaderModifier. Do not narrow the cookie domain (flint-gate depends on it).
  - Not blocking for OG-26. The operator's session is the only session that matters today.
- **INFO-I: live-only knowme routes.** See the assessment above.
- **INFO-J: self-test gaps.** Add mutations for a `*.know-me.tools` listener hostname, `selector: {}` on a listed listener, and a route pinned to an unlisted `sectionName` with a zone host.

## Post-sync acceptance (MEDIUM-2 moves to RESOLVED on these)

1. Argo `argocd-config` is Synced and Healthy. All 25 listeners are `Programmed`, and their attached-route counts match `live-sim.txt`.
2. **Negative probe (required):** in a scratch namespace, create an HTTPRoute for `rt.know-me.tools` with `sectionName: flint-rt-https`. Expect `Accepted=False` with reason `NotAllowedByListeners`, and expect `https://rt.know-me.tools/<probe-path>` not to reach the probe backend. Then delete the namespace.
3. `verify.sh argocd` PASS, with an external fetch of each `know-me.tools` host.

## Manual follow-ups

1. Confirm the tree SHA, and run `know-me-listener-check.sh` and `--self-test` on it.
2. Run `kubectl --context know-me get gateways -A -o wide` and confirm only `argocd/argocd-gateway` and `ipfs/ipfs-gateway-gw` exist (`cluster-facts.md` is from 2026-10-14).
3. RBAC in admitted namespaces: `kubectl auth can-i create httproutes -n {knowme,knowdesign,realtor-demo,flint-core} --as=system:serviceaccount:<ns>:default`. Also check whether any ClusterRole that grants `httproutes` create carries the `rbac.authorization.k8s.io/aggregate-to-edit` or `aggregate-to-admin` label.
4. Export the Cloudflare `know-me.tools` record set for LOW-12.
5. Commit the live-simulation script, or record where it lives (INFO-I).

## Lead's read-only checks (2026-10-19, appended by the lead, not part of the review)

- Follow-up 2: only `argocd/argocd-gateway` (23.239.29.33) and `ipfs/ipfs-gateway-gw` (45.56.67.67) exist.
- Follow-up 3: the `default` ServiceAccount cannot create HTTPRoutes in knowme, knowdesign, realtor-demo or flint-core. The only ClusterRole granting HTTPRoute create is `cert-manager-controller-challenges`, with no aggregate-to-edit/admin label.
- **LOW-12 item 3, live DNS (raises LOW-12 to a pre-OG-26 gate, lead's call pending the operator):**
  - `chat`, `livekit`, `matrix` `.know-me.tools` resolve to `172.169.221.204` (RIPE-registered; not a cluster gateway). HTTPS to them times out.
  - A wildcard record exists: an unknown name (`zz-probe-<epoch>.know-me.tools`) resolves to `35.238.217.92` (`bc.googleusercontent.com`, Google Cloud). HTTPS times out.
  - `runtime.know-me.tools` resolves to 23.239.29.33 (argocd-gateway).
  - No CAA record on `know-me.tools`.
  - Every one of those names receives the `know-me.tools` Kratos cookie (Secure, 30 days). If either address is released or reassigned, its new holder receives the operator's session on any request to those names, with no cluster access. Unless the operator confirms both addresses are theirs and in use, the wildcard record and the three matrix records should be deleted in Cloudflare before OG-26.
