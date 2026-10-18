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
