# Security read: know-me-cluster PR #39 (console-session-auth 5.5, Kratos ingress NetworkPolicy)

Relayed verbatim by the lead (the security-reviewer context has no Write tool).

- Task: `agent-backend-settings › console-session-auth › 5.5` (acceptance: a security read with no open CRITICAL)
- Artifact: Prometheus-AGS/know-me-cluster#39, branch `feat/flint-kratos-networkpolicy`, commit 02cf6e8, base main 4f329e9
- Reviewer: security-reviewer, separate context, read-only, no shell
- Files read: `namespaces/flint-core/networkpolicy-kratos.yaml`, `namespaces/ipfs/lke-checks/networkpolicy-check.sh`, `namespaces/flint-core/manifests.yaml` (Deployments flint-gate, kratos, kratos-ui, sso-broker; Services kratos-admin, kratos-public, kratos-ui; ConfigMaps flint-gate-config, kratos-config), `namespaces/flint-core/networkpolicy-hydra.yaml`, `apps/flint-core.yaml`, kubo `mcp/server/evidence/2026-10-17-console-session-auth/5.5-inventory.md`, the #38 read `networkpolicy-pr38-security-read.md`, and ADR `docs/mcp/0013-console-session-auth.md` sections 1, 8 and 9.
- Not verified here: that the working tree equals 02cf6e8 (no git); the check script's output and the six negative cases (not run; the inventory reports them); live Calico enforcement; the values of Secrets loaded through `envFrom`.

## Verdict

**PASS WITH FINDINGS. No CRITICAL, no HIGH.** For the spec, the policy closes #38 HIGH-1: pods outside flint-core, and flint-core pods without an allowed label, can no longer reach `kratos-admin:4434`. Selector semantics are correct. The probes still work. A revert or prune reopens access and never locks anyone out. Inventory callers are covered. One MEDIUM: the 4434 allow for `app=flint-gate` is unused privilege on an internet-facing proxy that is currently being changed. Drop it before merge; it costs one rule in the policy and one case in the check script. HIGH-1 counts as closed in production only after a live enforcement probe (manual follow-up 2).

## Summary

| ID | Severity | One line |
|---|---|---|
| MEDIUM-1 | MEDIUM | `app=flint-gate` may reach the unauthenticated admin API (4434), which nothing uses. That leaves a path from an internet-facing reverse proxy, now crash-looping and under change, to identity creation and recovery links. Drop it. |
| LOW-1 | LOW | Label claims: any pod created in flint-core with `app=flint-gate`, `app=kratos-ui` or `app.kubernetes.io/name=sso-broker` gets the allow. The check script does not assert that only the expected Deployment carries each allowed label. |
| LOW-2 | LOW | Group 4 follow-up: kratos-ui has no `KRATOS_ADMIN_URL`. Upstream falls back to `ORY_SDK_URL`, then the Ory playground. Before cutover, confirm that the consent and logout routes make no admin (IdentityApi) call. If they do, set `KRATOS_ADMIN_URL` and add `app=kratos-ui` to the 4434 rule. |
| LOW-3 | LOW | The consumer guard (check 4h) misses `configMapKeyRef`, `secretKeyRef`/Secret values, projected ConfigMap volumes and `initContainers[].envFrom`. It also exempts `app=kratos` in every namespace. All of these misses fail closed (outage), not open. |
| LOW-4 | LOW | Unread `envFrom` Secrets (`edgequake-api-secret`, `uar-secrets`, `site-alert-mail`) could hide a `kratos-public` URL. This is break risk only. |
| INFO-1 | INFO | Selector AND/OR is correct. |
| INFO-2 | INFO | Hydra needs no Kratos port. The Hydra-to-Kratos UI hop runs through the browser. Kratos and kratos-ui call Hydra admin 4445 as egress, which this policy does not affect. |
| INFO-3 | INFO | Kubelet probes on 4434 come from the pod's own node, which the spec always allows. Nothing scrapes Kratos metrics. |
| INFO-4 | INFO | The 4433 restriction adds little secrecy: 4433 is public through `auth.know-me.tools`. It is defense-in-depth that carries break risk. Acceptable as built. |
| INFO-5 | INFO | Rollback and prune are safe. |
| INFO-6 | INFO | The flint-core preflight FAIL (flint-gate rollout) is unrelated to this policy's safety. The waiver carries over from #38 LOW-4. |

## Findings

### MEDIUM-1: unused 4434 allow for flint-gate (least privilege)

- Evidence: `namespaces/flint-core/networkpolicy-kratos.yaml:48-54` (rule 1: `app=flint-gate` to TCP 4434). The policy's own comment at `:14-17` says "no workload today … drop it if it stays unused". Inventory row "flint-gate | 4434 | no reference in git or live". `flint-gate-config` (`manifests.yaml:1227-1232`) names only `kratos-public:4433` and `sso-broker:8080`. Check script `:127-128` (expected spec) and `:279` (`allowed_on 4434`) encode the same allow.
- Mechanism: the Kratos admin API has no authentication. Whoever reaches it can create identities and mint recovery links for the operator, which leads to a Kratos session, then a Hydra login, then the console (#38 HIGH-1). flint-gate is a reverse proxy that takes internet traffic for `*.know-me.tools` and `*.prometheusags.ai` and forwards cookies (`manifests.yaml:1226-1241`). ADR-0013 records that flint-gate is not used for console auth because its pinned image crash-loops (Know-Me-Tools/flint-gate#16). The inventory shows two flint-gate ReplicaSets with different images, one in CrashLoopBackOff, and another session is changing it. Any RCE or SSRF in flint-gate, or a future upstream/route config pointing at `kratos-admin`, reaches the admin API. As built, HIGH-1 is narrowed to "flint-gate pods", not closed.
- Stakes: the allow buys nothing today. Without it, HIGH-1 is closed for every pod except node-local sources (LOW-1 from #38). If a future flint-gate admin integration needs 4434, it should be its own reviewed change that names the caller and the endpoints it uses. The policy comment's argument ("does not need a second policy change") trades exactly the review that change should get.
- Action (before merge): delete `networkpolicy-kratos.yaml:48-54`. Delete the matching block from `EXPECTED_KRATOS` (`networkpolicy-check.sh:123-128`) and the `4434)` arm of `allowed_on` (`:279`), so any `kratos-admin` reference fails. Update the comment at `:14-17` and the header at `:18-23`. With no rule for 4434, the port is closed to every pod source. Kubelet probes are unaffected (INFO-3).

### LOW-1: allowed labels can be claimed inside flint-core

- Evidence: `networkpolicy-kratos.yaml:49-51,63-71` use bare `podSelector`s, which are same-namespace only. Check `:261-267` asserts only that the Deployments flint-gate and sso-broker carry their labels, and `:223-228` does the same for kratos-ui. Nothing asserts that no other workload carries them.
- Mechanism: anyone who can create a pod in flint-core (the Argo `flint-core` app, any Deployment editor, a compromised workload with pod-create RBAC) gets 4433, and with MEDIUM-1 unfixed also 4434. Other namespaces cannot claim the labels. The same holds for the Envoy rule (`:56-62`): a pod in `envoy-gateway-system` with the two owning-gateway labels gets 4433. Both namespaces are privileged, so RBAC carries this. It is the same residual as #38 INFO-1. The flint-gate Deployment selector is `app=flint-gate` (`manifests.yaml:22-24`), so a claimed label also puts the pod into the flint-gate Service. That is a separate traffic-hijack concern.
- Action: in check 4g, fail if any Deployment, StatefulSet, DaemonSet, Job or CronJob in `namespaces/flint-core/` other than the expected one carries `app=flint-gate`, `app=kratos-ui`, `app.kubernetes.io/name=sso-broker` or `app=kratos`. Record the RBAC dependency in the 5.5 evidence.

### LOW-2: kratos-ui admin access and consent (required check before the group 4 cutover)

- Evidence: kratos-ui env `manifests.yaml:492-510` has `KRATOS_PUBLIC_URL`, the CSRF settings and no `KRATOS_ADMIN_URL`, `HYDRA_ADMIN_URL` or `ORY_SDK_URL`. The inventory reports that upstream v1.3.1 `src/pkg/sdk/index.ts` builds the IdentityApi client from `KRATOS_ADMIN_URL`, falling back to `ORY_SDK_URL` and then the Ory playground. ADR-0013 lines 71-73: the consent route turns on only once `HYDRA_ADMIN_URL` is set (group 4).
- Mechanism: today kratos-ui needs no 4434. Once group 4 sets `HYDRA_ADMIN_URL`, the consent and logout routes come alive. The inventory says `consent.ts` reads the identity from `req.session`, and that claim was not re-verified here. If any route enabled in group 4 does call the IdentityApi, one of two things happens. Either it calls the Ory playground, a hardcoded third-party endpoint (AGENTS.md configurable-endpoint rule; it would also send identity lookups off-cluster). Or, once `KRATOS_ADMIN_URL` is set, this policy blocks it and consent breaks, failing closed.
- Hydra: no Kratos port is needed from Hydra pods. Per ADR section 1 (lines 100-112) and section 9 (line 228), Hydra hands the login and logout challenges to the Kratos UI through browser redirects to `https://auth.know-me.tools/ui/*`, which enter through argocd-gateway (allowed on 4433, and kratos-ui has no policy). Kratos accepts the challenge through `oauth2_provider.url` to Hydra admin 4445, and kratos-ui accepts consent and logout the same way. Both are egress from the Kratos pods, and `hydra-ingress` (`networkpolicy-hydra.yaml:39-48`) allows them. No follow-up is required for Hydra to kratos-public.
- Action: add to the group 4 (4.x) acceptance: with `HYDRA_ADMIN_URL` set, run the login, consent-skip and logout flow and confirm kratos-ui makes no request to 4434 or to any `oryapis.com` host. If it does, set `KRATOS_ADMIN_URL=http://kratos-admin:4434/`, add `app=kratos-ui` (not flint-gate) to a 4434 rule, and update the check.

### LOW-3: consumer guard (check 4h) coverage gaps

- Evidence: `networkpolicy-check.sh:302-312`. The searched text is `env[].value`, `args`, `command` of `containers` and `initContainers`. ConfigMaps come only from `volumes[].configMap.name` and `containers[].envFrom[].configMapRef.name` (`:307`). `:288` skips any workload labeled `app=kratos` in any namespace.
- Mechanism: a caller that names `kratos-public` or `kratos-admin` through `env[].valueFrom.configMapKeyRef`, a `projected` volume's ConfigMap source, `initContainers[].envFrom`, any Secret, or a renamed Service selecting `app=kratos` passes the guard. So does a pod in another namespace labeled `app=kratos`. The policy then blocks that caller, so each miss is an outage the guard should have flagged, not an exposure. The exact-spec comparison (`:117-149`, `:170`) remains the security assertion. It catches added sources, added ports, an AND split into OR (the inventory reports a namespaceSelector-only negative failing), and `metadata.namespace` drift (`:171-174`).
- Action: add `configMapKeyRef.name` and projected `sources[].configMap.name` to the ConfigMap list, include `initContainers[].envFrom`, and make the `app=kratos` exemption flint-core only. Optional: also fail on any Service other than kratos-admin and kratos-public selecting `app=kratos`. The `metadata.namespace` assertion added for kratos-ingress is not applied to hydra-ingress (#38 LOW-2). Add it there too.

### LOW-4: break risk from unread Secrets

- Evidence: inventory, "Other live facts": the values of `edgequake-api-secret`, `uar-secrets` and `site-alert-mail` loaded through `envFrom` were not read.
- Mechanism: a Kratos URL inside one of those values would be blocked after merge. Callers that use the public `https://auth.know-me.tools` URL are unaffected, because they enter through the allowed argocd-gateway Envoy proxies.
- Action: before merge, grep the values without printing them, for example `kubectl get secret <name> -n <ns> -o json | jq -r '.data[]|@base64d' | grep -c -e kratos-public -e kratos-admin -e ':443[34]'`. A count of 0 closes this.

### INFO-1: selector semantics

- `networkpolicy-kratos.yaml:56-62`: `namespaceSelector` and `podSelector` sit in one `from` element, so they combine with AND: only argocd-gateway proxies in `envoy-gateway-system`. This matches the live labels in the inventory and the `hydra-ingress` pattern.
- `:63-71`: three separate bare `podSelector` elements, meaning kratos-ui OR flint-gate OR sso-broker, flint-core only. This is the intended OR.
- `:42-46`: the target `app=kratos` selects only the kratos Deployment's pods (`manifests.yaml:353-366`). The `kratos-migrate` Job pods carry `job-name`/`controller-uid` (inventory), so they are not selected. That would be harmless anyway, since they take no ingress.
- Ports are TCP only, matching the container ports at `manifests.yaml:408-414`. Service targetPorts are the named ports `admin` and `public` (`:1084-1088`, `:1107-1111`). NetworkPolicy matches the resolved pod port, so 4433 and 4434 are correct.

### INFO-2: Hydra flow (see LOW-2)

No Hydra pod to Kratos ingress is required by ADR sections 1, 8 or 9. Both Kratos-to-Hydra calls are egress, and `policyTypes: [Ingress]` (`:45-46`) leaves egress open. Return traffic is stateful.

### INFO-3: probes and scrapers

The kratos liveness and readiness probes are httpGet on port `admin` (`manifests.yaml:398-423`) from the kubelet, which is node-local and always allowed. No ServiceMonitor or PodMonitor or scrape annotation targets Kratos (repo grep: none in flint-core). Residual, same as #38 LOW-1: hostNetwork pods and processes on the kratos node can still reach 4434.

### INFO-4: value of the 4433 restriction

Kratos public is served to the internet through HTTPRoute `kratos-public` (`manifests.yaml:2365-2391`), so any pod can reach it by hairpinning through `auth.know-me.tools`. The in-cluster restriction on 4433 limits only direct ClusterIP access, for example spoofed `X-Forwarded-*` headers that skip Envoy. Its main cost is break risk, which the inventory and the guard cover. Acceptable. An open 4433 rule would also be defensible.

### INFO-5: rollback and prune

`apps/flint-core.yaml:11,17` (`path: namespaces/flint-core`, `prune: true`). The object is new and has no `Prune=false`, so a revert deletes it in one sync and access reopens (`networkpolicy-kratos.yaml:34-35`). Label drift on an allowed caller fails closed. flint-gate's `kratos_session` and the sso-broker whoami would then fail, which is an availability risk for `*.know-me.tools` and `*.prometheusags.ai` sign-in, not an exposure.

### INFO-6: pre-existing flint-core preflight FAIL

The policy never selects flint-gate. It selects only `app=kratos`, and Kratos is healthy per the inventory. Degraded health does not stop Argo applying the object (#38 LOW-4 reasoning). Carry the waiver. As a hard precondition, confirm `kubectl -n flint-core get networkpolicy kratos-ingress` after sync. If MEDIUM-1 is not applied, that allow sits on a component with an unsettled rollout; one more reason to drop it.

## Check script assessment

- Strong: the exact canonical-spec match (`:117-149`, `:151-165`); the namespace assertion (`:171-174`); the additivity regex (`:184`); the container ports (`:256-260`); the allowed-label presence (`:261-267`); `set -euo pipefail` (`:39`). These catch the inventory's negatives: sso-broker removed, namespaceSelector-only (AND to OR), flint-gate label drift, an onyx `kratos-admin` caller, frf calling `kratos-public` (a cross-namespace caller always fails at `:296`), and the wrong namespace. Not executed by this reviewer.
- Gaps: LOW-1 (label uniqueness) and LOW-3 (reference coverage). After MEDIUM-1, the `4434)` arm must go so any `kratos-admin` reference fails.

## Manual follow-ups

1. Apply MEDIUM-1 (policy, check script, comments) and re-run `networkpolicy-check.sh`. Attach the output.
2. Live enforcement probe after sync (#38 INFO-5 pattern). These must FAIL: a pod in another namespace, an unlabeled pod in flint-core, and an unlabeled pod in `envoy-gateway-system`, each connecting to `kratos-admin.flint-core.svc:4434` and `kratos-public.flint-core.svc:4433`. These must PASS: kratos-ui and sso-broker `/sessions/whoami` against 4433; `https://auth.know-me.tools/health/ready` from outside; kratos Ready with no new restarts. Only after this is HIGH-1 closed in production.
3. LOW-4 Secret grep (count only).
4. Add the LOW-2 kratos-ui admin and consent check to the group 4 acceptance in `openspec/changes/console-session-auth/tasks.md`.
5. Record the preflight waiver (INFO-6) on #39.
