# Security read: know-me-cluster PR #38 (console-session-auth 5.2 NetworkPolicies)

Relayed verbatim by the lead (the security-reviewer context has no Write tool).

- Task: `agent-backend-settings › console-session-auth › 5.3`
- Artifact: Prometheus-AGS/know-me-cluster#38, branch `feat/ipfs-flint-networkpolicy`, commit bba654d, base main 24ab586
- Reviewer: security-reviewer, separate context, read-only
- Requirements: kubo `openspec/changes/console-session-auth/tasks.md:60-64` (5.2); `docs/mcp/0013-console-session-auth.md:211-224` (section 8); AGENTS.md wire-compat rule
- Evidence base: working-tree files `namespaces/ipfs/networkpolicy.yaml`, `namespaces/flint-core/networkpolicy-hydra.yaml`, `namespaces/ipfs/lke-checks/networkpolicy-check.sh`, `namespaces/ipfs/manifests.yaml`, `namespaces/flint-core/manifests.yaml`, `apps/ipfs.yaml`, `apps/flint-core.yaml`, `helm-values/eg.yaml`; inventory `mcp/server/evidence/2026-10-17-console-session-auth/5.1-inventory.md`
- Not verified here: working tree == bba654d (no git); the check script's real output (not run); the PR body and preflight comments (no gh); Calico enforcement (that is task 5.4).

## Verdict

**PASS, no open CRITICAL.** Both policies match 5.2 and ADR section 8: right selector semantics, complete ports and protocols, no swarm regression for network peers, and rollback goes in the safe direction. One HIGH finding is pre-existing and outside this PR's scope (Kratos admin 4434 is open to the whole cluster). It undercuts the purpose of the 4445 restriction and must be tracked before the console session cutover. It does not block this merge.

## Summary

| ID | Severity | One line |
|---|---|---|
| HIGH-1 | HIGH (pre-existing, out of PR scope) | `kratos-admin:4434` (no auth) is reachable from every pod, which is a path around the 4445 restriction: mint a recovery link for the operator, then a Hydra login. |
| MEDIUM-1 | MEDIUM (residual, not a regression) | Ingress-only: the internet-facing ipfs pod (kubo plus `ipfs mcp`) can still reach every in-cluster service, including kratos-admin and flint-core postgres. |
| LOW-1 | LOW | Pods using the host network on the ipfs or Hydra node, and the node itself, can still reach 5001/5050/8080 and 4445. The NetworkPolicy spec always allows node-local traffic. |
| LOW-2 | LOW | The check script's "no other policy" assertion only scans the two directories and k8s `NetworkPolicy` kinds. It does not see policies in other paths, Calico `GlobalNetworkPolicy` or AdminNetworkPolicy, and it does not assert `metadata.namespace`. |
| LOW-3 | LOW | The check script does not tie `Addresses.Swarm` to the allowed ports, so a future swarm listener (new port or transport) would be silently blocked without the check failing. |
| LOW-4 | LOW | The flint-core preflight FAIL (flint-gate ProgressDeadlineExceeded, Degraded since 2026-10-13) is unrelated to this PR's safety. It does mean the 5.2 "preflight passes" acceptance needs an explicit waiver and a check that `hydra-ingress` is live before Hydra (4.1) lands. |
| INFO-1 | INFO | Selector semantics are correct: ipfs uses AND (namespaceSelector and podSelector in one element); Hydra admin uses an intended OR (kratos or kratos-ui, same namespace). |
| INFO-2 | INFO | Closing 8081 (`/tcp/8081/ws`) causes no regression for peers. It never had a Service, so only in-cluster pods could reach it. Kubo still announces it. |
| INFO-3 | INFO | Kubelet probes are fine: ipfs uses exec probes; Hydra's httpGet probes on 4445 come from the pod's own node, which the spec allows. |
| INFO-4 | INFO | Rollback and prune are safe: apps use `prune: true` and the policies have no `Prune=false`, so a revert deletes them and access reopens. If labels drift, the failure is closed (outage), not open. |
| INFO-5 | INFO | Recommendation for 5.4: also probe from a pod in `envoy-gateway-system` without the gateway labels, and from the argocd Envoy pod to 5001. This is the live proof against the AND/OR mistake. |

## Findings

### HIGH-1: Kratos admin API open cluster-wide; a path around the Hydra 4445 restriction (pre-existing, out of PR scope)

- Evidence: `namespaces/flint-core/manifests.yaml:1072-1092` (Service `kratos-admin`, ClusterIP 4434, selector `app: kratos`); `:412-413` (container port 4434 `admin`); `flint-gate-config` `:1274-1275` (`base_url: http://kratos-admin:4434/`). There is no NetworkPolicy in flint-core other than `hydra-ingress` (`namespaces/flint-core/networkpolicy-hydra.yaml:33-35` selects only `app.kubernetes.io/name=hydra`). Inventory section 6: no policies in flint-core.
- Mechanism: like Hydra's 4445, the Kratos admin API has no authentication. Any pod in any namespace can create or modify identities and generate recovery links or codes for an existing identity (for example the operator's). After that it can sign in through Kratos, complete the Hydra login for `ipfs-console`, and get a console session. A fresh recovery login would also satisfy the admin `max_auth_age` step-up (ADR section 9).
- Stakes: ADR section 8 says "anyone who reaches [4445] can … accept logins". 4434 gives the same result one hop earlier, so restricting 4445 alone does not achieve "no path around the gateway" for console session auth.
- Scope: not introduced by PR #38, and not part of the 5.2 requirement text. It does not block this merge.
- Action: add a follow-up task before the console session cutover: a NetworkPolicy on `app=kratos` allowing 4434 only from `flint-gate` (and `kratos-ui` if it uses admin), and 4433 only from the argocd-gateway proxies and `kratos-ui`. Record it in the OpenSpec change.

### MEDIUM-1: ipfs pod egress is unrestricted toward in-cluster services

- Evidence: `namespaces/ipfs/networkpolicy.yaml:14-16,30-31` (`policyTypes: [Ingress]`; the comment explains egress stays open for peers). The pod runs internet-facing `ipfs mcp` with `read_only: false` and `confirmation.modes: []` (`namespaces/ipfs/manifests.yaml:727-750`).
- Mechanism: the SSRF-capable surfaces of `ipfs mcp` (configurable LLM proxy and embeddings endpoints), or a kubo compromise, can reach any ClusterIP, including kratos-admin 4434 (HIGH-1) and postgres 5432 in flint-core. Leaving egress open is correct for public peers, but kubo does not need to reach the pod and Service CIDRs, apart from DNS.
- Stakes: this is not a regression and not part of 5.2. It is the remaining lateral path from the most exposed pod.
- Action (follow-up): add an egress rule that allows `0.0.0.0/0` except the pod CIDR (10.2.0.0/16 per the inventory IPs) and the Service CIDR (10.128.0.0/16), plus kube-dns 53 UDP/TCP. Verify that DHT and peering still work, per the wire-compat rule.

### LOW-1: node-local sources bypass both policies (spec behavior)

- Evidence: the Kubernetes NetworkPolicy spec: when a pod is isolated for ingress, "the only allowed connections into the pod are those from the pod's node and those allowed by the ingress list". The ipfs pod sits on `lke653426-965151-9k2gk` (inventory section 1).
- Mechanism: processes on that node and pods using the host network there (calico-node, kube-proxy, CSI node plugins, any future node agent) can reach 5001, 5050, 8080 and later 4445 without matching any rule. Cross-node traffic is not exempt.
- Stakes: this needs node or system-component compromise. Note it as residual risk; Calico host endpoints or eBPF would be needed to close it. The same exemption is what keeps the Hydra httpGet probes working (INFO-3).

### LOW-2: check script's additivity guard has blind spots

- Evidence: `namespaces/ipfs/lke-checks/networkpolicy-check.sh:33-34` (globs only `namespaces/{ipfs,flint-core}/*.y*ml`), `:128-136` (only `.kind == "NetworkPolicy"`), `:49-53,107-121` (compares `.spec` only, not `metadata.namespace`).
- Mechanism: an allow policy for namespace `ipfs` committed under another app's directory, a Calico `GlobalNetworkPolicy` or AdminNetworkPolicy, or a copy of `ipfs-ingress` with `metadata.namespace` pointing elsewhere would all pass. The exact-spec comparison is otherwise strong: any widening or AND/OR reshaping of the two policies changes the canonical JSON and fails, and since arrays are not sorted, reordering also fails closed.
- Action: also grep the whole repo for `namespace: ipfs` / `namespace: flint-core` on any `*NetworkPolicy*` kind, and assert `metadata.namespace`. Live coverage of the cluster-wide kinds belongs in 5.4.

### LOW-3: no guard tying swarm listeners to the allowed ports

- Evidence: `namespaces/ipfs/lke-checks/networkpolicy-check.sh:144-150` checks only that 4001/TCP, 4001/UDP, 5001, 5050 and 8080 are present. `Addresses.Swarm` lives in `namespaces/ipfs/manifests.yaml:280-282`.
- Mechanism: adding a WebTransport or WebRTC-direct listener, or moving the swarm port, would be blocked by `ipfs-ingress` and the script would still pass. That would quietly reduce reachability for peers, which the AGENTS.md wire-compat rule cares about.
- Action: parse `Addresses.Swarm` from `configure-ipfs.sh` and fail if any `tcp` or `udp` port other than 4001 appears (8081 allowed, since it is closed on purpose).

### LOW-4: flint-core preflight FAIL (flint-gate rollout stuck) does not affect this PR's safety, but the acceptance needs a waiver

- Evidence: `hydra-ingress` selects only `app.kubernetes.io/name=hydra` (`namespaces/flint-core/networkpolicy-hydra.yaml:33-35`). `flint-gate` pods carry `app: flint-gate` (`namespaces/flint-core/manifests.yaml:24,35`). Hydra is absent (inventory section 7). `apps/flint-core.yaml:15-18` uses automated prune and selfHeal.
- Judgment: today the policy selects zero pods, so it changes nothing at runtime in flint-core, and it never touches flint-gate. A Degraded health status does not stop Argo from auto-syncing a new revision, and the policy has no sync wave, so it is applied with everything else. The re-applied, unchanged flint-gate spec does not start a new rollout.
- Residual: if flint-core's sync operation itself is failing (not just health), `hydra-ingress` may never be applied, and Hydra would then land with 4445 open. Landing the policy before Hydra is the right order, so keep it.
- Action: record the preflight FAIL as an explicit, unrelated waiver against 5.2's "preflight passes". In 5.4, and again as a hard precondition in the 4.1 PR preflight, confirm `kubectl -n flint-core get networkpolicy hydra-ingress` exists and the app's last sync succeeded.

### INFO-1: selector semantics correct

- `namespaces/ipfs/networkpolicy.yaml:33-40`: `namespaceSelector` (`kubernetes.io/metadata.name=envoy-gateway-system`) and `podSelector` (`owning-gateway-name=ipfs-gateway-gw`, `owning-gateway-namespace=ipfs`) sit in the same `from` element, so they combine with AND. The inventory confirms the live labels (5.1 section 5). Only pods in ipfs carry `app.kubernetes.io/name=ipfs` (StatefulSet `:43-47`), so the target is exactly `ipfs-0`.
- `namespaces/flint-core/networkpolicy-hydra.yaml:39-48`: two `podSelector` elements with no namespaceSelector, meaning `app=kratos` or `app=kratos-ui`, in flint-core only. This is the intended OR. `:49-59` uses AND for the argocd-gateway proxies.
- Port coverage: 5001, 5050 and 8080 are TCP (`:41-47`). The second rule (`:48-52`) has ports and no `from`, so 4001 TCP and UDP are open to all sources, including SNATed NodePort and LB traffic. That matches `ipfs-swarm` (LB TCP, `externalTrafficPolicy: Cluster`) and `ipfs-swarm-udp` (NodePort UDP), whose targetPort is 4001 (`manifests.yaml:453-458,490-495`).
- Bypass via Gateway API: `ipfs-gateway-gw` listeners use `allowedRoutes: Same` (`manifests.yaml:512-514,525-527`). The cluster has no ReferenceGrant (inventory section 3), and Envoy Gateway's Backend API is not enabled (`helm-values/eg.yaml:39` `extensionApis: {}`). No other namespace can make the allowed Envoy proxies forward to 5001. The argocd-gateway proxies are allowed only to 4444, which is public by design.
- Residual: anyone who can create pods in `envoy-gateway-system` (with the gateway labels) or in flint-core (with `app=kratos`) gets the allow. Both are privileged namespaces, so this falls to RBAC.

### INFO-2: 8081 closure and swarm reachability

- `manifests.yaml:98-100` (containerPort 8081) and `:280-282` (`/ip4/0.0.0.0/tcp/8081/ws`). No Service exposes 8081, and the 5.1 inventory found no consumer, so internet peers could never reach it and nothing regresses. Kubo still advertises the pod-IP `/ws` address, so an in-cluster dialer would now time out instead of connecting. Optional: drop it from `Addresses.Swarm` (needs a planned restart). The cluster is single-stack IPv4, so the IPv6 listeners do not matter.

### INFO-3: probes

- ipfs: exec probes against 127.0.0.1 (`manifests.yaml:73-83,101-111`). The sidecar and postStart use loopback (`:72,141`), which NetworkPolicy does not affect.
- Hydra: chart httpGet probes on 4445 from the kubelet, i.e. the pod's own node, which the spec always allows (LOW-1). The inventory notes enforcement is unproven; 5.4 covers that.

### INFO-4: rollback and prune

- `apps/ipfs.yaml:15-18` and `apps/flint-core.yaml:15-18` use `prune: true`. Neither policy carries `Prune=false`, so a revert deletes it in one sync and restores today's open access. Rollback opens rather than locks out, which is the right direction for an availability rollback, and the comments at `networkpolicy.yaml:18-19` and `networkpolicy-hydra.yaml:25-26` match. If Envoy Gateway ever changes the proxy labels or switches to merged gateways, `ipfs-ingress` would block all gateway traffic. That fails closed (an outage, not an exposure).

### INFO-5: 5.4 negative tests

- Beyond the planned probe pod in another namespace, add two probes: one from a pod in `envoy-gateway-system` without the gateway labels, and one from the `argocd-gateway` Envoy pod (via `kubectl debug`), both against `ipfs-api.ipfs.svc:5001` and `ipfs-mcp.ipfs.svc:5050`. Both must fail. These are the live tests that tell AND from OR.

## Check script assessment (`namespaces/ipfs/lke-checks/networkpolicy-check.sh`)

- The exact canonical-spec comparison (`:46-121`) detects the negatives that matter: any added source, a port missing or added, a TCP/UDP swap, a `policyTypes` change, or the namespaceSelector and podSelector split into separate elements.
- It fails closed when yq is missing or errors (`set -euo pipefail`, `:29`; the `got=$(...)` assignment at `:110` is not masked).
- Route-to-port regression guard (`:157-171`) and the Hydra label and port guard (`:181-203`, which excludes maester; ADR line 144 says maester is not used) are sound for raw manifests under `namespaces/flint-core/`, which is the style 4.1 mandates.
- Gaps: LOW-2 and LOW-3. Not executed by this reviewer, so its PASS output is unverified here.

## Manual follow-ups

1. Confirm the reviewed working tree equals bba654d (`git -C know-me-cluster diff bba654d -- namespaces/`).
2. Run `bash namespaces/ipfs/lke-checks/networkpolicy-check.sh` and attach the output to the PR.
3. 5.4: enforcement probes, including INFO-5; `ipfs-0` Ready with zero restarts; console `/webui` and swarm (TCP and QUIC) answering; `hydra-ingress` present in flint-core.
4. Open the HIGH-1 follow-up (Kratos ingress policy) and the MEDIUM-1 follow-up (ipfs egress to cluster CIDRs) in `openspec/changes/console-session-auth/`.
5. Record the flint-core preflight FAIL waiver (LOW-4) in the PR or the evidence.
