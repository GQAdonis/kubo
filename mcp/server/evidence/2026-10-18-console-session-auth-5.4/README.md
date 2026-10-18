# console-session-auth 5.4: NetworkPolicy enforcement (know-me-cluster #38)

Date: 2026-10-18. Operator approval, verbatim: "yes to both" (two busybox probe pods, created then deleted). Context `know-me`, run by the lead.

## Probes (02:48:03Z, deleted 02:48:26Z)

Each pod (`busybox:1.36`, label `purpose=csa-probe-5-4`, no other labels) ran `nc -z -w3` against the ipfs Services and a control fetch to the public gateway host.

| From | `ipfs-api.ipfs.svc:5001` | `ipfs-mcp.ipfs.svc:5050` | control `ipfs.prometheusags.ai:443` |
|---|---|---|---|
| `default` | BLOCKED | BLOCKED | REACHED |
| `envoy-gateway-system`, no gateway labels (AND/OR selector test, security read INFO-5) | BLOCKED | BLOCKED | REACHED |

Afterwards: no pod left with `purpose=csa-probe-5-4`; `ipfs-0` Running, ready, restarts `0 2` (unchanged; the sidecar restarts predate the sync).

## Other checks after the sync (posted on #38)

`verify.sh ipfs` PASS; `origin-isolation.sh unauth` PASS; swarm TCP 4001 on the LoadBalancer open; console `/webui/` 401 without credentials; gateway `/ipfs/bafkqaaa` 200 (Envoy to 8080 allowed).

## Result and limits

- PASS for the blocking side: neither a pod in another namespace nor an unlabelled pod in the gateway namespace reaches 5001 or 5050.
- Limit: the probe printed only connect success or failure. A DNS failure would also read BLOCKED. The probes had no in-cluster positive control (for example `ipfs-swarm.ipfs.svc:4001`, which the policy allows from anywhere), so "BLOCKED" is attributed to the policy by the before/after design, not proven against DNS. Envoy reaching 8080 is proven (gateway 200); Envoy reaching 5001 is not proven by the 401 (Envoy answers 401 before the backend); the operator's credentialed console check proves it.
- Not covered: `hydra-admin.flint-core.svc:4445` (Hydra does not exist until group 4; moved into group 4's verification).

## Follow-up (02:53:48Z, deleted 02:53:58Z): positive control and operator check

Operator approval, verbatim: "yes, approve the probe pod; console loads".

One more `busybox:1.36` pod in `default`, same label, deleted afterwards (no pod left with the label):

```
BLOCKED ipfs-api.ipfs.svc:5001
BLOCKED ipfs-mcp.ipfs.svc:5050
REACHED ipfs-swarm.ipfs.svc:4001
CONTROL ipfs.prometheusags.ai:443 REACHED
```

The probe also printed a `dns=` field; its awk filter picked the resolver line (`10.128.0.10:53`), not the Service IP, so that field carries no information. The control is the 4001 line: the same pod resolved and reached an `ipfs` Service on the port the policy allows from anywhere, so the 5001 and 5050 failures are the policy, not DNS or routing.

Operator: "console loads" (credentialed console in a browser), so the `ipfs-gateway-gw` proxies still reach 5001 through the policy.

## Result

PASS for the `ipfs` policy: blocked from other namespaces and from unlabelled pods in the gateway namespace, allowed for the gateway, swarm open. `ipfs-0` ready; container restart counts `0 2` unchanged by the sync (the `ipfs-mcp` sidecar restarts predate it). Hydra 4445 is carried into group 4's verification (Hydra does not exist yet).
