# Security read: know-me-cluster PR #41 (console-session-auth 5.6, ipfs pod egress)

Relayed verbatim by the lead (the security-reviewer context has no Write tool and no shell).

Lead note (not part of the read): the operator merged #41 at 2026-10-18T16:42:41Z before this read returned. The lead ran the pre-sync baseline (16:43:57Z) and post-sync checks (16:47:33Z) from inside `ipfs-0`; see `mcp/server/evidence/2026-10-18-console-session-auth-5.6/README.md`. MEDIUM-1's failure mode did not occur: DNS resolved after the sync.

- Task: `agent-backend-settings › console-session-auth › 5.6`
- Artifact: Prometheus-AGS/know-me-cluster#41, commit 6897df2. Files read from the detached worktree `/private/tmp/claude-501/-Users-gqadonis-Projects-references-kubo/43b10494-d504-43f2-9ba1-af8ce53bff62/scratchpad/kmc-pr41-review`: `namespaces/ipfs/networkpolicy-egress.yaml`, `namespaces/ipfs/networkpolicy.yaml`, `namespaces/ipfs/lke-checks/networkpolicy-check.sh`, `namespaces/ipfs/manifests.yaml`, `apps/ipfs.yaml`.
- Kubo source read: `config/profile.go:39-72` (server profile filters), `core/node/groups.go:122-184` (AutoTLS/AutoWSS), `mcp/server/embeddings/service.go:317` and `extractor_supervisor.go:260` (`HF_ENDPOINT`), `plugin/plugins/telemetry/telemetry.go`.
- Prior read: `networkpolicy-pr38-security-read.md` (MEDIUM-1 and LOW-3 are addressed here).
- Reviewer: security-reviewer, separate context, read-only.
- Not verified here: that the worktree equals the PR head; the check script's output (not run); every live cluster fact (see "Rests on the builder's word").

## Verdict

**PASS WITH FINDINGS. Nothing CRITICAL or HIGH.**

The policy closes #38 MEDIUM-1 for every pod-backed service. It blocks:

- direct pod IPs (`10.2.0.0/16`);
- ClusterIPs whose backends are pods (evaluated after DNAT);
- this cluster's own LoadBalancer IPs (ipMode VIP, hairpinned to the Envoy pods);
- node InternalIPs and the Linode private network (`192.168.128.0/17`);
- the Linode metadata service (`169.254.0.0/16`).

That covers kratos-admin 4434, flint-core postgres 5432, hydra-admin 4445 and liter-llm, including liter-llm through its public hostname.

DNS rule semantics are correct (namespace AND pod selector, with ports). Nothing a peer can see gets worse. Rollback is safe.

Two lateral paths remain, both outside the pod CIDR:

- the kube-apiserver, both public and through `kubernetes.default.svc`, with a token mounted in the pod (LOW-1);
- anything listening on the host network on node public IPs (LOW-2).

The one gating item is MEDIUM-1. Unlike the earlier CORS PRs, a NetworkPolicy applies to the running pod the moment Argo syncs. If the CoreDNS assumptions are wrong, the merge itself causes a DNS outage on the production node. Verify them before merging.

## Summary

| ID | Severity | One line |
|---|---|---|
| MEDIUM-1 | MEDIUM (precondition) | The DNS rule rests on live facts no one has verified: CoreDNS pod labels, CoreDNS listening on 53 after DNAT, no NodeLocal DNSCache (169.254.20.10 is excepted). A wrong guess means an immediate DNS outage at sync: DNSLink, `/dnsaddr` bootstrap, AutoConf, AutoTLS and delegated routing all fail. |
| LOW-1 | LOW (residual; MEDIUM if the default SA has grants) | The kube-apiserver stays reachable with the auto-mounted SA token: publicly at 45.33.120.221:6443, and also through `kubernetes.default.svc`, because its endpoint is public after DNAT. The policy comment implies the Service CIDR exception covers this. It does not. |
| LOW-2 | LOW (residual) | Host-network listeners on node public IPs (kubelet 10250, any hostNetwork exporter or agent) are still reachable, including on the pod's own node. Only the InternalIPs are excepted. |
| LOW-3 | LOW | Check 1d is narrow. It misses short or `svc.ns` names, this cluster's public hostnames and LB IPs (blocked by the hairpin), container env and args (for example `HF_ENDPOINT`), and Secrets. These gaps cost availability (fail closed), not security. |
| LOW-4 | LOW (hardening) | The exceptions are tied to today's single IPPool and ServiceCIDR. A second pool, or a VPC subnet in 10/8 or 172.16/12, would open a new lateral path silently. Kubo never dials RFC 1918 or CGNAT addresses anyway (server profile), so excepting the whole ranges costs nothing. |
| LOW-5 | LOW (carried over from #38 LOW-2) | Only partly closed. `metadata.namespace` is now asserted (check script lines 203-208). A repo-wide scan and Calico `GlobalNetworkPolicy`/`NetworkPolicy` kinds are still not checked, so an additive allow committed elsewhere would pass. |
| INFO-1 | INFO | #38 LOW-3 is closed by check 1c (lines 210-230). |
| INFO-2 | INFO | No self-dial, hairpin, AutoTLS, ACME, relay, DCUtR, AutoNAT, QUIC or telemetry dependency is broken. |
| INFO-3 | INFO | Wire compatibility (AGENTS.md): no regression a peer can see. |
| INFO-4 | INFO | IPv6 and dual-stack: fails closed, documented. |
| INFO-5 | INFO | Rollback and prune are correct. |
| INFO-6 | INFO | How an operator adds an in-cluster exception, including the Envoy container-port detail. |

## Rests on the builder's word (not visible in git)

- The Calico IPPool set is exactly `10.2.0.0/16` (ipipMode Always, natOutgoing). Tunnel addresses come from it.
- ServiceCIDR is `10.128.0.0/16`.
- Node InternalIPs are `192.168.212.x`.
- The LB IPs 104.237.130.131, 45.56.67.67 and 23.239.29.33 use ipMode VIP.
- The apiserver is at 45.33.120.221:6443.
- CoreDNS labels and port (MEDIUM-1).
- The live kubo config carries the server-profile `Swarm.AddrFilters` and `Addresses.NoAnnounce`. The init container applies `--profile=server` only on first init (`manifests.yaml:181-186`), so a repo initialised earlier, or re-initialised, may lack them.
- Calico enforces after DNAT. This is standard for the iptables and eBPF dataplanes; it is not proven live.

The values match LKE defaults, and `config/profile.go:40-47` confirms that 10/8, 169.254/16 and 192.168/16 are in `defaultServerFilters`. Still, each fact gets a read-only check in the pre-merge steps below.

## Findings

### MEDIUM-1: the DNS rule's live assumptions must be verified before merge

- Evidence: `networkpolicy-egress.yaml:55-66` (to kube-system pods `k8s-app: kube-dns`, ports UDP and TCP 53); `:51` (pod CIDR excepted); `:53` (169.254/16 excepted); `manifests.yaml:177` (`dnsPolicy: ClusterFirst`).
- Mechanism: Calico matches the destination after DNAT. The DNS rule works only if three things hold:
  - the `kube-dns` Service's endpoints are pods carrying `k8s-app=kube-dns` in a namespace labelled `kubernetes.io/metadata.name=kube-system`;
  - their targetPort is 53, not 5353 or 8053;
  - the pod's `/etc/resolv.conf` points at the kube-dns ClusterIP, not a NodeLocal DNSCache address such as `169.254.20.10`. That address is excepted at `:53`, and its host-network listener would not match the pod selector.
  If any of these fails, every lookup fails on the running pod at sync time, without a restart. Kubo loses DNSLink, `/dnsaddr/bootstrap.libp2p.io` bootstrap, AutoConf, AutoTLS/p2p-forge and HTTP delegated routing. The public gateway degrades.
- Selector semantics are right: `namespaceSelector` and `podSelector` sit in one `to` element (AND), and `ports` restricts that rule only. That means the rule does not open CoreDNS's other ports (9153 metrics, 8080 health), which the proof below uses.
- Action: run pre-merge steps B1-B3. Merge only inside an announced window, with the revert commit ready.

### LOW-1: kube-apiserver residual, including through the Service ClusterIP

- Evidence: `manifests.yaml:51` (`automountServiceAccountToken: true`). There is no `serviceAccountName`, so both containers get the `ipfs` namespace's `default` SA token. Neither kubo nor `ipfs mcp` uses the Kubernetes API.
- Correction to the policy comment (`networkpolicy-egress.yaml:19-22`): "a ClusterIP … is matched as the backend pod IP and denied" holds only for Services backed by pods. `kubernetes.default.svc` (10.128.0.1) DNATs to its endpoint 45.33.120.221:6443, which is public, so the `0.0.0.0/0` rule allows it. The same applies to any Service with manually set public endpoints. The `10.128.0.0/16` exception (`:52`) is effectively redundant after DNAT. It is harmless, but it is not what blocks Services.
- Mechanism: an RCE in kubo or the sidecar, or an SSRF that can read files, gets a valid in-cluster identity and reaches the apiserver by either path. The impact depends on RBAC. A default SA normally holds only discovery permissions.
- Action:
  - Verify with `kubectl auth can-i --list --as=system:serviceaccount:ipfs:default -n ipfs`, and check that no RoleBinding or ClusterRoleBinding names that SA.
  - Set `automountServiceAccountToken: false` on the pod template. This changes the template and restarts the pod, so do it in the next planned restart window, not in this PR.
  - Fix the comment.
  - Raise this to MEDIUM if the SA has any grant beyond discovery.

### LOW-2: host-network listeners on node public IPs remain reachable

- Evidence: `networkpolicy-egress.yaml:47-54` excepts only InternalIPs (`192.168.128.0/17`) and tunnel and pod addresses. Every LKE node also has a public IPv4, which `0.0.0.0/0` allows. That includes the pod's own node (workload-to-host traffic goes through egress policy first).
- Mechanism: NodePorts on public node IPs are DNAT'd to pods and blocked. Host-network sockets bound to `0.0.0.0` are not blocked: kubelet 10250 (needs authn and authz, so with the default SA it answers 403), any hostNetwork exporter (for example 9100), the CSI node plugin, kube-proxy healthz 10256, and BGP 179 (BIRD accepts only configured peer IPs).
- Action: `kubectl get pods -A -o json | jq -r '.items[]|select(.spec.hostNetwork)|.metadata.namespace+"/"+.metadata.name'`, then list their listeners. If anything sensitive listens on the public interface, add each node's public /32 to `except`. Kubo does not peer with nodes of this cluster.

### LOW-3: check 1d misses most of the ways an in-cluster destination could be configured

- Evidence: `networkpolicy-check.sh:233-238`. It scans only the `.data` of `ipfs-config` and `ipfs-mcp-config`, with the regex `*.svc`, `cluster.local`, and `10.(2|128).x.x`.
- Gaps:
  1. Short names (`http://liter-llm:4000`) and `name.namespace` names (`liter-llm.liter-llm`), which resolve through the resolv.conf search path.
  2. This cluster's public hostnames (`*.prometheusags.ai`, `*.know-me.tools`) and the three LB IPs. These are blocked too, because of the VIP hairpin, which the policy's own comment says (`networkpolicy-egress.yaml:20-23`).
  3. StatefulSet container `env`, `args` and `command`. The sidecar is configured by flags (`manifests.yaml:137-143`), and kubo's embeddings code reads `HF_ENDPOINT` from the environment (`mcp/server/embeddings/service.go:317`, `extractor_supervisor.go:260`).
  4. Secrets.
  5. The 192.168.128/17 and 169.254/16 ranges.
- Stakes: the policy fails closed, so a miss is a silent outage of a future LLM or embeddings integration, not a leak. That is the scenario 5.6 promised to catch before merge.
- Action: extend 1d to (a) the ipfs StatefulSet's container and initContainer `env[].value`, `args` and `command`, (b) the cluster's domain suffixes and LB IPs, (c) any `scheme://host` whose host has no dot or has exactly one label before a namespace name, and (d) the other two excepted ranges.

### LOW-4: tie the exceptions to address classes, not to today's pool

- Evidence: `networkpolicy-egress.yaml:51-54`. In kubo, `config/profile.go:40-51` (server profile `Swarm.AddrFilters` and `Addresses.NoAnnounce`) already refuses to dial 10/8, 100.64/10, 172.16/12, 192.168/16, 169.254/16 and others.
- Mechanism: a second Calico IPPool, a pool resize, or LKE VPC addressing (10/8) would put new in-cluster workloads outside `10.2.0.0/16` and silently reopen MEDIUM-1.
- Action: except 10.0.0.0/8, 100.64.0.0/10, 172.16.0.0/12, 192.168.0.0/16 and 169.254.0.0/16, and update `EXPECTED_IPFS_EGRESS` (`networkpolicy-check.sh:161-182`) in the same commit. A peer never notices, because kubo already filters these ranges. Optionally also except the node public /32s (LOW-2).

### LOW-5: additive policies elsewhere (#38 LOW-2), partly closed

- Evidence: `networkpolicy-check.sh:203-208` now asserts `metadata.namespace`. `:254-255` scans only `namespaces/ipfs/*.y*ml` and only `kind: NetworkPolicy`.
- Mechanism: NetworkPolicies are additive. An egress allow for `ipfs` committed in another app directory, or a Calico `NetworkPolicy`/`GlobalNetworkPolicy`, would reopen lateral paths and the check would still pass.
- Action: grep the whole repo for any `*NetworkPolicy*` kind with `namespace: ipfs`, or with a selector that matches `app.kubernetes.io/name: ipfs`. Live, also run `kubectl get networkpolicies.crd.projectcalico.org -A` and `kubectl get globalnetworkpolicies.crd.projectcalico.org`.

### INFO-1: #38 LOW-3 is closed by check 1c

- Evidence: `networkpolicy-check.sh:210-230` parses the one `Addresses.Swarm` line (`manifests.yaml:280-282`).
- Results on that line: `/tcp/4001`, `/udp/4001/quic-v1` and `/ip6/::/...4001` pass; `/tcp/8081/ws` passes as intentionally closed; any other port or protocol fails. WebTransport and webrtc-direct on `/udp/4001` correctly pass, since UDP 4001 is open.
- AutoWSS appends no hidden listener here: a custom `/ws` exists, so the append condition is false (`core/node/groups.go:169`). If 8081/ws is ever removed, AutoWSS adds `/tcp/4001/tls/sni/*.libp2p.direct/ws`, which is still on an open port.

### INFO-2: kubo network functions that keep working

- Hairpin and self-dial: libp2p refuses to dial its own peer ID, and nothing in the pod fetches from its own gateway or LB IP. The Web UI pin and the probes use 127.0.0.1 (`manifests.yaml:72-78`), which a NetworkPolicy does not affect. Blocking the hairpin to the pod's own LB IP costs nothing.
- AutoTLS: the p2p-forge client registers with `registration.libp2p.direct` over public HTTPS. The forge dials back inbound on 4001, which `ipfs-ingress` allows. ACME DNS-01 talks to Let's Encrypt over the public internet. All of it is allowed.
- Relay (`manifests.yaml:290-292`), DCUtR and AutoNAT:
  - Relay client dials go to public relays.
  - Relay service and dial-back traffic is inbound on 4001.
  - DCUtR's simultaneous dials are outbound to public addresses.
  - AutoNAT v2 server dial-backs go to public addresses.
  - Calico conntrack lets replies to allowed flows through, both UDP (QUIC) and TCP, in both directions.
  - Inbound connections are governed by the ingress policy, and their replies are not affected by egress rules.
- Telemetry: the plugin's destination is a public HTTPS endpoint set at build time (`plugin/plugins/telemetry/telemetry.go:12`). This PR neither adds nor removes a reporting path, and the opt-outs are untouched.
- Delegated routing and AutoConf use public HTTPS, so they are allowed.

### INFO-3: AGENTS.md wire-compat and user-agency rules

- Ingress is unchanged, and outbound to every public IPv4 address is unchanged.
- The excepted ranges are ones the server profile already refuses to dial and to announce (`config/profile.go:68-69`), so peers see the same reachability and the same addresses.
- No protocol behaviour changes, so no IPIP is needed. No tracking or logging is added. No CIDs are added to the blockstore or provide queue.
- Caveat: if the live config lacks the server-profile filters (see "Rests on the builder's word"), kubo would now time out dialing peers' private LAN addresses instead of failing some other way. Those dials were useless from a data centre in any case.

### INFO-4: IPv6

The cluster is single-stack (`ipFamilyPolicy: SingleStack` on every Service in `manifests.yaml`), and the pod has no IPv6 address, so no IPv6 egress is possible today. If the cluster becomes dual-stack, the policy denies all IPv6 egress until a `::/0` rule is added. That fails closed and is documented (`networkpolicy-egress.yaml:28-31`). When that happens, except the IPv6 pod and Service CIDRs, `fc00::/7` and `fe80::/10`.

### INFO-5: rollback and prune

- `apps/ipfs.yaml:15-18` sets `prune: true`, and the new object has no `Prune=false`. A revert deletes it on the next sync, and egress is open again immediately with no pod restart (`networkpolicy-egress.yaml:33-34`).
- A label drift on the ipfs pod would make the policy select nothing (fails open), but check 4a (`networkpolicy-check.sh:260-263`) guards against that.
- A label drift on CoreDNS fails closed (an outage). See MEDIUM-1.
- Calico does not tear down flows that were already established, so the proof below must use new connections.

### INFO-6: adding an in-cluster exception (future `ipfs mcp` LLM or embeddings endpoint)

- Add a third `to` element to `ipfs-egress`: a `namespaceSelector` plus `podSelector` for the backend pods, with the port the backend container actually listens on (the post-DNAT port). Update `EXPECTED_IPFS_EGRESS` in the same commit. The exact-spec check and check 3 then force a reviewed change, rather than letting someone add a second policy.
- If the endpoint is this cluster's public hostname, the hairpin lands on the Envoy proxy pods. The exception must select `envoy-gateway-system` pods with that gateway's `owning-gateway-*` labels, on Envoy's container port (often 10443 or 10080, not 443). Read the port live.
- Prefer the internal Service path to the specific backend over the Envoy path. The Envoy path opens every route on that gateway.

## Check script assessment (`namespaces/ipfs/lke-checks/networkpolicy-check.sh`)

- 1b (`:161-182,202`): the canonical exact-spec comparison catches any widened CIDR, removed exception, added port, selector split (AND turned into OR), or `policyTypes` change. Arrays are not sorted, so reordering the `except` list fails closed. This is strong.
- Namespace assertion (`:203-208`): closes part of #38 LOW-2.
- 1c: see INFO-1.
- 1d: see LOW-3.
- Check 3 (`:254-255`): covers only k8s `NetworkPolicy` kinds in this directory (LOW-5).
- Everything is checked offline. Nothing proves enforcement, the CoreDNS facts or the IPPool set; that belongs to the probes below.
- Not executed by this reviewer, so its PASS is unverified here.

## Pre-merge baseline (read-only, run from inside `ipfs-0`)

Rules for all probes:

- Use new TCP connections only, with a 3-second timeout.
- Send no credentials, write no state, and never use the SA token.
- Record "connected", "refused" and "timeout" separately. A timeout after merge, where the baseline connected, is the enforcement proof.

B0. Tools and context:

- Find out which tools exist: `kubectl -n ipfs exec ipfs-0 -c ipfs -- sh -c 'command -v nc wget nslookup; cat /etc/resolv.conf'`. Upstream kubo images are busybox-based; this fork image is unverified.
- If `nc` or `wget` is missing, use a pinned debug image in the pod's network namespace: `kubectl -n ipfs debug ipfs-0 --target=ipfs --image=busybox@sha256:<pinned> -it -- sh`. An ephemeral container shares the pod IP, so the policy applies to it. It stays in the pod spec until the next restart and is harmless there.

B1. DNS facts (MEDIUM-1):

- `kubectl -n kube-system get svc kube-dns -o jsonpath='{.spec.clusterIP} {.spec.ports}'`: targetPort must be 53, and 9153 should be listed.
- `kubectl -n kube-system get pods -l k8s-app=kube-dns -o wide`: there must be at least one pod.
- `kubectl get ns kube-system --show-labels`.
- The resolv.conf nameserver from B0 must equal the kube-dns ClusterIP, not `169.254.20.10`.
- `kubectl get ds -A | grep -i nodelocal` must be empty.

B2. Address facts:

- `kubectl get ippools.crd.projectcalico.org -o custom-columns=N:.metadata.name,C:.spec.cidr,IPIP:.spec.ipipMode`: exactly one pool, `10.2.0.0/16`.
- `kubectl get servicecidrs`.
- `kubectl get nodes -o wide`: note the InternalIPs and public IPs.
- `kubectl get svc -A -o jsonpath` on the LoadBalancer Services: `status.loadBalancer.ingress[].ipMode` must be VIP.

B3. Kubo facts: `kubectl -n ipfs exec ipfs-0 -c ipfs -- ipfs config Swarm.AddrFilters`. It must include 10/8, 169.254/16 and 192.168/16. Record `ipfs swarm peers | wc -l`, and the same count piped through `grep -c quic-v1`, as the peer baseline.

B4. Receiving-side controls:

- `kubectl -n kube-system get networkpolicy` and `kubectl -n flint-core get networkpolicy`. This confirms that CoreDNS and postgres have no ingress policy, so a timeout after merge can only come from egress.
- Postgres has no ingress policy, which is #38 HIGH-1's sibling. Note it.

B5. Baseline probes. All of these should connect today. Replace `nc -w3 H P </dev/null; echo $?` with `wget -T3 -qO- http://H:P/...` where that is all the image has.

| # | Target | Proves after merge | Expected now | Expected after |
|---|---|---|---|---|
| P1 | CoreDNS pod IP :9153 (`wget -T3 -qO- http://<coredns-podIP>:9153/metrics \| head -1`) | pod CIDR denied, and the DNS rule is port-scoped (AND) | connect | timeout |
| P2 | `kube-dns.kube-system.svc:9153` (ClusterIP) | post-DNAT evaluation of Services backed by pods | connect | timeout |
| P3 | `<postgres-svc>.flint-core.svc:5432`, TCP connect only, no startup packet | the named MEDIUM-1 target | connect | timeout |
| P4 | liter-llm Service, its port, TCP connect | named target | connect | timeout |
| P5 | `104.237.130.131:443` (cluster LB IP) | VIP hairpin denied; confirms the documented breakage | connect | timeout |
| P6 | `<node InternalIP>:10250`, TCP connect only | Linode private range denied | connect | timeout |
| P7 | `169.254.169.254:80`, TCP connect only (no PUT, no token request) | metadata denied | connect | timeout |
| P8 | `<another node's public IP>:10250`, TCP connect only | LOW-2 residual (record, not a pass/fail) | connect | connect |
| P9 | `45.33.120.221:6443` and `kubernetes.default.svc:443`, TCP connect only | LOW-1 residual (record) | connect | connect |
| P10 | `nslookup kubernetes.default.svc.cluster.local`, `nslookup bootstrap.libp2p.io`, `nc -w3 <coredns-podIP> 53` (TCP) | DNS UDP and TCP still allowed | ok | ok |
| P11 | `nc -w3 1.1.1.1 443` | public egress intact | connect | connect |

The kratos-admin 4434 probe is omitted on purpose: kratos-ingress already blocks it on the receiving side, so it cannot tell egress enforcement apart from anything else. P1 and P2 are the cleanest proof. They are read-only, unauthenticated, have no ingress policy at the receiver, and target the exact pods the DNS rule selects. That proves pod-CIDR denial, post-DNAT matching and the AND port scoping in one step.

## Post-merge proof (immediately after Argo sync, in the announced window)

1. `kubectl -n ipfs get networkpolicy ipfs-egress -o yaml` exists and matches git (`argocd app diff ipfs` is empty).
2. Re-run P10 first. If DNS fails, revert at once.
3. Re-run P1-P11 and compare against the table. P1-P7 must change from connect to timeout. P8, P9, P10 and P11 must stay unchanged.
4. Control, so a timeout in P1 and P2 is attributable to the policy and not to the target being down. This creates a short-lived pod, so it needs operator approval. From a pod outside `ipfs`: `kubectl run -n default egress-ctl --rm -it --restart=Never --image=busybox@sha256:<pinned> -- wget -T3 -qO- http://kube-dns.kube-system.svc:9153/metrics | head -1`. It must still connect.
5. Kubo health over 15-30 minutes:
   - `ipfs swarm peers | wc -l` stays within the baseline band, and the quic-v1 count is non-zero;
   - `ipfs id` still lists the public addresses;
   - `ipfs routing findprovs` on a well-known CID returns providers;
   - `ipfs resolve /ipns/docs.ipfs.tech` (DNSLink) succeeds;
   - the public gateway serves a known CID;
   - `/mcp` answers through the gateway.
6. Rollback trigger: any of P10, step 5's DNSLink or gateway check, or the peer count falling below about 50% of baseline after 15 minutes. Rollback is a revert commit; the prune removes the policy with no restart.
7. Attach the B0-B5 and post-merge outputs to the 5.6 evidence. This is the live enforcement proof that offline checks cannot give.

## MCP-specific note

This is an infrastructure-only change, so the MCP-01 to MCP-05 control table does not apply. Its effect on the `ipfs mcp` sidecar: the SSRF vector (OWASP MCP RCE table) is now confined to public destinations plus the LOW-1 and LOW-2 residuals. The sidecar still runs with `read_only: false` and `confirmation.modes: []` (`manifests.yaml:745-749`), behind the `IPFS_MCP_TOKEN` bearer token. That exposure is unchanged by this PR and was covered in earlier reads.

## Follow-ups

1. Before merge: B0-B5. Merge only if B1 and B2 confirm the facts.
2. In the next planned restart window: `automountServiceAccountToken: false` (LOW-1), plus the comment fix at `networkpolicy-egress.yaml:19-22`.
3. Follow-up PR: widen the exceptions to the RFC 1918, CGNAT and link-local classes (LOW-4), extend 1d (LOW-3), and add the repo-wide and Calico policy scan (LOW-5).
4. List hostNetwork listeners on node public IPs (LOW-2).
