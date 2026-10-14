# Proposal: lke-console-tls-wildcard

## Why

`https://console.prometheusags.ai` has never worked: listener `https-console` references a certificate that was never issued. The wildcard `ipfs-tls` already covers the host. This is also the first change carried by the new skill.

## What changes

See the task list and the plan section reproduced here.

### 2. `lke-console-tls-wildcard`

The first change carried by the skill, and the proof. Two PRs, because pruning the Issuer in the same sync as the Certificate can leave the finalizer-held Challenge and its solver route behind.

- 2.1 PR-A from `origin/main`: listener `https-console` references `ipfs-tls`; remove Certificate `console-tls` (this also stops cert-manager's retries, which spend Let's Encrypt's failed-validation allowance); keep the Issuer and the port-80 listeners for now. Acceptance: offline render and `kubeconform` pass; `webui-auth-targeting.sh` passes; nothing in the namespace references Secret `console-tls` (it was never created).
- 2.2 Run `preflight.sh ipfs --expect-unhealthy Gateway/ipfs-gateway-gw:https-console` on PR-A and post the report before asking for the merge (gate G-1). Acceptance: report attached; zero FAIL other than the named expected-unhealthy line (only listener `https-console` not Programmed) and the pre-existing `matrix` issuer line.
- 2.3 After G-1 and the Argo sync, run `verify.sh` and post it. Acceptance: listener `https-console` `Programmed=True`; unauthenticated `POST https://console.prometheusags.ai/api/v0/version` returns 401 (and returned TLS failure before, per 1.5); the `ipfs` app is Healthy; `console.prometheusags.ai` and `ipfs.prometheusags.ai` A records still equal 45.56.67.67. Record whether the Challenge `console-tls-1-…` and the solver route `cm-acme-http-solver-…` are gone; if either remains after 10 minutes, its cleanup becomes an operator step (G-4) before PR-B.
- 2.4 PR-B, after the Challenge and solver route are gone: remove Issuer `letsencrypt-http01-ipfs` and the `http` and `http-console` listeners; preflight posted; merge (gate G-1b); then `verify.sh`. Acceptance: listeners `https` and `https-console` Programmed; LoadBalancer Service `envoy-ipfs-ipfs-gateway-gw-…` exposes 443 only and keeps address 45.56.67.67; both hosts' external checks pass. The ACME account Secret `ipfs/letsencrypt-http01-ipfs-account` is not managed by Argo and is left for operator cleanup (G-4).
- 2.5 OG-6 restart, by the lead under the recorded operator approval, only when 2.3 has passed and no PR-B is open or unsynced (the restart and PR-B's sync never overlap, so a failed check can be traced to one change):
  1. Pre-restart snapshot to a dated evidence file: StatefulSet `ipfs` status and revision, pod `ipfs-0` phase and start time, PVCs bound, `verify.sh` output.
  2. `kubectl --context know-me -n ipfs rollout restart statefulset/ipfs`, then `kubectl --context know-me -n ipfs rollout status statefulset/ipfs --timeout=10m`.
  3. If the rollout does not finish within 10 minutes, or the new pod sits in `PodInitializing` or a crash loop: stop, take no further cluster action, record the pod's state and events, and hand over to the operator with the 2026-10-13 recovery notes (`.prometheus/gotchas.md`): no force delete; recovery is a node debug pod stopping the orphaned daemon, an operator action.
  4. On success: `verify.sh` again and `lke-checks/origin-isolation.sh` (unauthenticated part); the credentialed part is the operator's. Results go to `lke-origin-isolation` 1.8 as a new dated evidence folder. If either check fails after a successful rollout, the same rule as step 3 applies: stop, record, hand over; the rollback is the forward fix in `PR_BODY.md`, run by the operator.

## Impact

- Repository: `Prometheus-AGS/know-me-cluster` (namespace `ipfs` and, for the skill, `.claude/skills/` and `.agents/skills/`). No change to Kubo code, `/api/v0`, the gateway behaviour, the CID recipe or wire behaviour.
- Cluster mutations are operator gates, except the OG-6 restart recorded in the parent `execution.md`.
