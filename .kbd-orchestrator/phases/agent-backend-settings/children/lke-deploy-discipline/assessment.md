# Assessment — agent-backend-settings › lke-deploy-discipline

Date: 2026-10-14. Assessor: the lead (Claude Code), read-only against the live cluster (`kubectl --context know-me`; the default kubectl context on this machine is `orbstack`). Raw evidence: [`evidence/live-inventory-20261014T0910Z.txt`](evidence/live-inventory-20261014T0910Z.txt) and [`evidence/followup-20261014T0913Z.txt`](evidence/followup-20261014T0913Z.txt) (listener and route status, certificate subjects and issuers from the public `tls.crt` only, apex NS and CAA, external fetches). No cluster object was changed by this stage. Revised once after an independent artifact review (findings folded in below; see "Review").

## Production state right now (read this first)

- **`https://console.prometheusags.ai` is down and has never worked.** Listener `https-console` is `Programmed=False` (`InvalidCertificateRef`): it references Secret `console-tls`, which was never issued. The cert-manager Order and Challenge are `invalid` (Let's Encrypt timed out fetching the challenge earlier); the solver route reports `ResolvedRefs=False` (`BackendNotFound`). Port 80 now answers from outside (404 on a probe path), so a retry may succeed, but the simpler fix is to reference the wildcard Secret.
- `ipfs.prometheusags.ai` is up: listener `https` Programmed; unauthenticated `/api/v0` answers 401.
- **The wildcard private key sits in an annotation.** All three wildcard Secrets (`ipfs/ipfs-tls`, `argocd/wildcard-prometheusags-ai-tls`, `onyx/prometheusags-wildcard-tls`) carry `kubectl.kubernetes.io/last-applied-configuration`, which holds the full Secret including `tls.key`. Not in the know-me-cluster git tree or its history (searched by file name, values never printed). Anyone who can read those Secrets can already read the key; the added risk is any metadata dump (logs, evidence files, PR comments) carrying it. Removing the annotation and rotating the key are operator decisions.

## Why this child exists

Four know-me-cluster PRs in a row (#24-#27) were prepared from offline manifests and an artifact-only review, and each met live state it had not checked. The operator asked why deployment kept failing and asked for the procedure to be captured as a reusable agentskills.io skill. This child records what the cluster actually is, why the PRs failed, and what the skill must enforce.

## Prior lessons that apply

- "External integrations require at least one real-service test: For every external integration, run at least one test against the real service before claiming the work is done. Fakes and mocks can hide service-specific preconditions." The cluster is the real service; offline render and `kubeconform` are the fakes. Every failure below is a live precondition the fakes could not see.
- "Preserve production evidence in dated review folders." Live checks go into a new dated evidence file per run; earlier runs are never overwritten.
- "Operator controls PR merges." The skill prepares, checks and documents; merges stay with the operator.

## Live conventions (facts, 2026-10-14)

### DNS and certificates

| Zone | Authoritative DNS | Hosts on this cluster | TLS source |
|---|---|---|---|
| `prometheusags.ai` | GoDaddy (apex NS `ns57/ns58.domaincontrol.com`); no CAA record | `ipfs`, `console` → 45.56.67.67; `onyx`, `edgequake` → 23.239.29.33 | **A ZeroSSL DV wildcard `*.prometheusags.ai` (plus the apex), notAfter 2027-02-13**, copied as a plain TLS Secret per namespace: `ipfs/ipfs-tls`, `argocd/wildcard-prometheusags-ai-tls`, `onyx/prometheusags-wildcard-tls`. No cert-manager Certificate object manages it; `ipfs-tls` carries a stale `cert-manager.io/issuer-name: letsencrypt-http01` annotation, so the annotation does not describe the certificate. How it was obtained (manual or ACME outside the cluster) and who renews it are unverified. |
| `know-me.tools` | Cloudflare (apex NS `abdullah/yolanda.ns.cloudflare.com`); no CAA record | `know-me.tools`, `www`, `runtime`, `design`, `auth`, `sso`, `gate`, `api`, `rt` → 23.239.29.33 | cert-manager: `letsencrypt-prod` (Cloudflare DNS-01) for `design`, site and runtime; `letsencrypt-http01` for the flint hosts |

Method note: query NS at the zone apex (`dig NS prometheusags.ai`), not at the parent of a host name; the first inventory queried `tools.` for `know-me.tools` and printed the TLD servers.

Consequences:

- **A new `*.prometheusags.ai` host needs no new certificate.** Reference the namespace's copy of the wildcard Secret. cert-manager DNS-01 cannot work for this zone (the Cloudflare token cannot see a GoDaddy zone), and HTTP-01 works only through a gateway that has a port-80 listener for the host.
- The wildcard expiring on 2027-02-13 breaks `ipfs`, `console`, `onyx` and `edgequake` at once unless every copy is replaced. Nothing in the cluster renews it.

### Issuers

| Issuer | Type | Where the HTTP-01 challenge route attaches |
|---|---|---|
| ClusterIssuer `letsencrypt-prod`, `letsencrypt-staging` | DNS-01 Cloudflare | not applicable |
| ClusterIssuer `letsencrypt-http01` | HTTP-01 | references `matrix/{cinny,livekit,matrix}-gateway` for three matrix hosts, but namespace `matrix` does not exist (dangling); otherwise `argocd/argocd-gateway` |
| ClusterIssuer `letsencrypt-http01-staging` | HTTP-01 | `envoy-gateway-system/acme-http01-gateway` |
| Issuer `ipfs/letsencrypt-http01-ipfs` (added by #27) | HTTP-01 | `ipfs/ipfs-gateway-gw` |

### Gateways and load balancers

- Only two Gateways exist. `argocd/argocd-gateway` (23.239.29.33): HTTPS and HTTP listeners per host, `allowedRoutes: All`; most apps pair an HTTPS route with an HTTP→HTTPS redirect route, but the five flint-core routes are HTTPS-only.
- `ipfs/ipfs-gateway-gw` (45.56.67.67): `https` (Programmed), `https-console` (**not Programmed**, see above), and since #27 HTTP `http` and `http-console` (Programmed); `allowedRoutes: Same`. Its LoadBalancer Service now exposes 443 and 80.
- Envoy Gateway derives each LoadBalancer Service's ports from its Gateway's listeners. There is no ReferenceGrant anywhere, so a listener can only use a TLS Secret in its own namespace.
- Every application HTTPRoute on `ipfs-gateway-gw` pins `sectionName` (`https` or `https-console`); the ACME solver route has none.

### Auth, sync and workloads

- SecurityPolicy `ipfs/ipfs-webui-auth`: targets `ipfs-webui` and `ipfs-webui-legacy`, basic auth from Secret `ipfs-webui-basicauth` (key `.htpasswd`; `ipfs-webui-basic-auth` with key `auth` is a stale ingress-era Secret), `Accepted=True` on both listeners, `Prune=false`.
- Argo CD: every app from `Prometheus-AGS/know-me-cluster` syncs automatically with `prune: true, selfHeal: true`. A hand edit to a tracked object is reverted; a hand-created object that git later stops listing is pruned unless it carries `Prune=false`. **The `ipfs` app itself is `Health=Progressing`** (consistent with the unprogrammed console listener). Other apps: `velero` OutOfSync, `flint-core` Degraded, `firecrawl` Missing, `hotseaters-platform` sync Unknown.
- `ipfs` StatefulSet: one replica, `RollingUpdate`, image `ghcr.io/gqadonis/kubo@sha256:9f94890e…2a8e52` in `configure-ipfs` (init), `ipfs` (with a detached `postStart` pin hook) and `ipfs-mcp`; grace period 30 s. A restart is a full outage of the gateway, the API and swarm until the new pod is Ready.
- No NetworkPolicy in namespace `ipfs` (only `argocd`, `dot17-uar`, `knowme`, `onyx` have any).

## The four failures, as cases

| PR | What it assumed | What was true | A preflight check that would have caught it |
|---|---|---|---|
| #24 | Secret `ipfs-webui-auth` holds the htpasswd | It is `ipfs-webui-basicauth`; Envoy marked the policy Invalid and answered 500 (fail closed); observed by the lead at 08:24Z-08:4xZ (unauthenticated `POST /api/v0/version` and `GET /webui/` returned 500 until #25 synced; not captured in an evidence file) | Every Secret a manifest references exists in the namespace with the expected key (key names only) |
| #24 | `letsencrypt-prod` can issue `console.prometheusags.ai` | Its DNS-01 solver is Cloudflare; the zone is on GoDaddy | Apex NS of the zone compared with the issuer's solver type |
| #26 | `letsencrypt-http01` works for `ipfs-gateway-gw` hosts | Its challenge route attaches to `argocd-gateway`; `ipfs-gateway-gw` had no port 80 | The issuer's solver parentRefs name a gateway that exists and owns the host, with an HTTP listener for it |
| #27 | A namespaced HTTP-01 Issuer plus port-80 listeners would issue `console-tls` | Config was all present (listener Programmed, LB port 80, parentRef matching), yet the challenge was `invalid` (Let's Encrypt timed out fetching it) and the solver route is `BackendNotFound` | Only an outcome check catches this: listener and route `Programmed`/`ResolvedRefs` status, and an external fetch of the challenge path from outside the cluster |
| #24, #26, #27 | The console needs its own certificate | The wildcard `ipfs-tls` already covers it | A Secret in the namespace whose certificate SANs match the new host |

Common cause: the builder had no cluster access (by design), the reviewer saw only the artifact, and the lead passed the PR to the operator without running the PR's own "Before merge" checklist, though read-only `kubectl` access to `know-me` was available. A second cause, shown by #27: checks that config exists and matches are not checks that the result works. The skill needs both.

## Gaps

1. **No procedure.** Nothing tells a builder or the lead which live facts to collect before writing a cluster change, nor that the lead must post them on the PR before asking for a merge.
2. **No live preflight tool.** Each check above was typed by hand during an incident.
3. **No recorded cluster facts.** The zone-to-DNS-provider map, the wildcard certificate and its expiry, the gateway ownership of hosts, and the Secret names exist only in the live cluster and in this file.
4. **Console TLS is broken.** `https-console` references the never-issued `console-tls`. The wildcard `ipfs-tls` covers the host; the fix is `https-console` → `ipfs-tls`, then remove the Issuer, the Certificate and the two port-80 listeners that exist only because of #26 and #27. Not changed in this stage; it is the first change the skill must carry.
5. **Wildcard renewal is unowned.** `*.prometheusags.ai` expires 2027-02-13; nothing in the cluster renews it, and its origin is unverified.
6. **Wildcard private key in annotations** (see "Production state"). Operator decisions: remove the `last-applied-configuration` annotation from the three Secrets, and whether to rotate the key.
7. **Dangling issuer config.** `letsencrypt-http01` references gateways in the nonexistent `matrix` namespace.
8. **OG-6 is not finished.** The planned restart (which arms the console-only CORS list) and the post-merge checks have not run; they need a working console host first.

## What the skill must do (input to Plan)

- A `SKILL.md` procedure for any know-me-cluster change: collect live facts, write the change, offline checks, a live preflight posted on the PR, the operator's merge, post-merge checks, restart rules from the 2026-10-13 outage, and rollback as a forward fix.
- A read-only `preflight` script with two kinds of checks, writing a dated evidence file and exiting nonzero on any failure:
  - **Config checks**, before merge: referenced Secrets exist with the expected keys; zone apex NS and A record against the owning gateway's address; TLS coverage (an existing Secret whose certificate SANs match, read from the public `tls.crt` only, or an issuer whose solver type fits the zone and whose solver parentRefs name an existing gateway with an HTTP listener for the host); CAA records; gateway listeners and LoadBalancer ports; route `sectionName` pinning; the target Argo app's current sync and health; `argocd app diff` where available; image references pullable by digest.
  - **Outcome checks**, after sync: every listener and route `Programmed` and `ResolvedRefs`; SecurityPolicy `Accepted` per target; certificates `Ready`; external HTTPS and HTTP fetches of each affected host from outside the cluster with expected status codes; for restarts, StatefulSet and PVC readiness before and after.
  - **Never prints Secret data.** Secret objects are read with field selection only (names, key names, the public `tls.crt`); annotations on Secrets are never output, because the wildcard Secrets carry their private key in `last-applied-configuration`.
- Restart and rollback rules from the 2026-10-13 outage: planned single-replica restart, no force delete, forward-fix rollback.
- Let's Encrypt hygiene: after an `invalid` order, fix the cause before retrying (rate limits); clean up stale solver routes and Services.
- `references/cluster-facts.md` generated from a live inventory, dated, with the regeneration command.
- Placement: the know-me-cluster repo (`.claude/skills/lke-gitops-change/`, mirrored for other harnesses under `.agents/skills/`). Neither directory exists yet.

## Knowledge gaps

None recorded in `prior-context.md`. Unverified: whether a Linode Cloud Firewall sits in front of the LoadBalancers (not visible from the cluster); who renews the wildcard certificate and where its source is kept.

## Review

Independent artifact review (`artifact-critic`, fresh context, 2026-10-14): verdict "defects found" with three CRITICAL and five WARNING findings, all confirmed live and folded in above: the console listener is not Programmed and the ACME order is invalid (the first draft called it merely an unnecessary path); the preflight lacked outcome checks and would have passed #27; the wildcard private key sits in `last-applied-configuration` annotations; the wildcard is ZeroSSL, not shown to be purchased; the `matrix` gateways do not exist; flint-core routes have no HTTP redirect; the `ipfs` app is Progressing; the first inventory queried the TLD for `know-me.tools`. The reviewer's uncomfortable point stands as written: the first draft was produced with live data that already showed the console failure and still under-ranked it.
