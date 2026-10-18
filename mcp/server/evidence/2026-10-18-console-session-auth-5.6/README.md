# console-session-auth 5.6: ipfs pod egress (know-me-cluster #41)

Date: 2026-10-18. Operator, verbatim: "yes, approve the exec into ipfs-0.  PR's #40 and #41 are merged" (#41 merge `f7d4133`, 16:42:41Z, merged before its security read returned). Run by the lead with `kubectl --context know-me exec ipfs-0 -c ipfs` (read-only commands only).

## Baseline before the sync (16:43:57Z; Argo still at 750788e)

```
peers=133
findprovs=3
REACHED liter-llm.flint-core.svc:4000
REACHED kratos-ui.flint-core.svc:3000
REACHED 1.1.1.1:443
tools: /bin/nc /bin/wget
```

## After the sync (Argo ipfs Synced f7d4133, op Succeeded 16:47:08Z; checks at 16:47:33Z)

```
NetworkPolicies in ipfs: ipfs-egress (26s), ipfs-ingress
nameserver 10.128.0.10
DNS bootstrap.libp2p.io OK
DNS cluster OK
peers=117 quic=97
findprovs=3
DNSLink resolve OK            (ipfs resolve /ipns/docs.ipfs.tech)
BLOCKED liter-llm.flint-core.svc:4000
BLOCKED kratos-ui.flint-core.svc:3000
BLOCKED 104.237.130.131:443   (own LB IP, VIP hairpin)
BLOCKED 169.254.169.254:80    (metadata)
REACHED 1.1.1.1:443
gateway https://ipfs.prometheusags.ai/ipfs/bafkqaaa -> 200
```

Controls: `liter-llm` and `kratos-ui` have no ingress policy and were reachable from `ipfs-0` three minutes earlier; `kratos-ui:3000` was also reached from three other pods at 14:43Z (5.5 evidence). So the new BLOCKED lines are the egress policy, not the targets.

## Security-read preconditions, checked after the fact

- MEDIUM-1 (DNS): resolv.conf points at the kube-dns ClusterIP 10.128.0.10, and both public and cluster names resolve after the sync. No NodeLocal DNSCache in use. Not triggered.
- LOW-1: `kubectl auth can-i --list --as=system:serviceaccount:ipfs:default -n ipfs` shows only self-review and discovery; no RoleBinding or ClusterRoleBinding names the SA. Stays LOW; `automountServiceAccountToken: false` waits for the next planned restart.
- Live `Swarm.AddrFilters` includes 10/8, 100.64/10, 169.254/16, 172.16/12, 192.168/16 and the other server-profile ranges, so the excepted ranges cost no peer address.

## Peer recheck (15 minutes after the sync)

See the appended section below.

### Recheck at 2026-10-18T17:02:02Z (15 minutes after the sync)

```
peers=123 quic=99      (baseline 133 at 16:43:57Z, 117 right after the sync)
findprovs=3
DNSLink OK
gateway -> 200
ipfs-0 ready=true true restarts=0 2
BLOCKED kratos-admin.flint-core.svc:4434
```

`libp2p.direct addrs=0` is not a regression: this node has no AutoTLS listener (no `tls/sni` address; the custom `/tcp/8081/ws` listener suppresses AutoWSS, #41 read INFO-1), so it never had one. The kratos-admin line is blocked by both `kratos-ingress` (receiving side) and `ipfs-egress`; the egress proof is the liter-llm and kratos-ui lines above.

## Result

PASS: peering, DHT, DNSLink and the gateway work; in-cluster destinations, the cluster's own LB IP and metadata are blocked from the ipfs pod. #38 MEDIUM-1 closed in production. Follow-ups from the #41 read: LOW-1 (`automountServiceAccountToken: false` at the next planned restart), LOW-3, LOW-4, LOW-5 (check widening).
