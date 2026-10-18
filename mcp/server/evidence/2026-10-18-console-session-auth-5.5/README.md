# console-session-auth 5.5: Kratos ingress enforcement (know-me-cluster #39)

Date: 2026-10-18. Operator, verbatim: "yes, approve the probe pods", then "PR #39 is merged" (merge `750788e`, 14:41:26Z). Context `know-me`, run by the lead.

Argo `flint-core`: Synced `750788e`, operation Succeeded 14:43:18Z, health Degraded (pre-existing `flint-gate` rollout, waived; being fixed in another session). `kratos-ingress` (`app=kratos`) present next to `hydra-ingress`. Kratos pod `kratos-647859df8f-zz4gv` ready, restarts 0 before and after.

## Probes (14:43:51Z, all deleted by 14:44:05Z; no pod left with `purpose=csa-probe-5-5`)

`busybox:1.36`, only label `purpose=csa-probe-5-5`, `nc -z -w3`:

| From | `kratos-admin:4434` | `kratos-public:4433` | `kratos-ui:3000` (no policy, DNS control) | `auth.know-me.tools:443` |
|---|---|---|---|---|
| `default` | BLOCKED | BLOCKED | REACHED | REACHED |
| `flint-core`, unlabelled | BLOCKED | BLOCKED | REACHED | REACHED |
| `envoy-gateway-system`, unlabelled | BLOCKED | BLOCKED | REACHED | REACHED |

External: `https://auth.know-me.tools/health/ready` -> 200 (the argocd-gateway path to 4433 is allowed).

## Result

PASS. The unauthenticated Kratos admin API is unreachable from every pod source tested, including an unlabelled pod in Kratos's own namespace, so security read #38 HIGH-1 is closed in production (node-local sources remain, #38 LOW-1). The control column shows DNS and routing work from each pod.

Not proven here: the allowed in-cluster callers on 4433 (`kratos-ui`, `flint-gate`, `sso-broker`) still reach Kratos. Checking that needs `exec` into those pods, which was not approved; the policy's selectors for them are pinned by `networkpolicy-check.sh`, and the public sign-in path answers 200.
