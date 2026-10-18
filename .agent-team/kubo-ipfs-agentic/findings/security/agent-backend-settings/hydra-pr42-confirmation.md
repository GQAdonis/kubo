# Security confirmation: know-me-cluster PR #42 (follow-ups to the #40 Hydra read)

Relayed verbatim by the lead (the security-reviewer context has no Write or shell).

- Artifact: Prometheus-AGS/know-me-cluster#42, commit 6121615
- Files read (detached worktree): namespaces/argocd-config/manifests.yaml, namespaces/flint-core/lke-checks/oauth-listener-check.sh, namespaces/flint-core/hydra.yaml, helm-values/hydra.yaml, namespaces/flint-core/manifests.yaml. Prior read: hydra-pr40-security-read.md.
- Not verified here: that the worktree is 6121615; the PR body (no gh); a run of oauth-listener-check.sh or its --self-test; kubeconform or the helm render; any live cluster state, including the kratos-ui digest matching the live pod's imageID.

## Verdict

PASS. No open CRITICAL or HIGH. HIGH-1, LOW-1, LOW-3 and LOW-6 are resolved in the repo. LOW-5 is resolved in the repo; its PR-body part still needs confirming. LOW-2 stays open and gates OG-24. MEDIUM-1 stays open and gates OG-26. LOW-4 and LOW-7 are unchanged and open. One new LOW (LOW-8) and two INFO items, none blocking.

## Finding status

| ID | Status | Evidence and notes |
|---|---|---|
| HIGH-1 | RESOLVED | `namespaces/argocd-config/manifests.yaml:358-370`: `kinds: [{group: gateway.networking.k8s.io, kind: HTTPRoute}]`, `namespaces.from: Selector`, `matchLabels: kubernetes.io/metadata.name: flint-core`. It is the only listener for the host. No port-80 oauth listener exists. |
| MEDIUM-1 | OPEN, gates OG-26 | Not touched by #42. The 4.5 cross-namespace and unlabeled-pod probes are still owed. |
| LOW-1 | RESOLVED | `hydra.yaml:302-322` gives the initContainer the main container's limits (500m / 256Mi) and securityContext: no privilege escalation, drop ALL, read-only root filesystem, uid/gid 65534, seccomp RuntimeDefault. Source values are in `helm-values/hydra.yaml:57-64` and `:111-124`. |
| LOW-2 | OPEN, gates OG-24 | `manifests.yaml:1345-1361` still revokes PUBLIC only on database `hydra`. `00-create-databases.sh` (`:1334-1337`) leaves PUBLIC CONNECT on kratos, flintgate, broker and frf, and `flint` keeps it too. The builder's point that the stock scram-sha-256 host rule makes the passwordless role unusable holds only until OG-24 sets the password. After that the gap is live. |
| LOW-3 | RESOLVED | `hydra.yaml:169-170` is `strategy: type: Recreate` with no `rollingUpdate` key. That matters: the API rejects Recreate if `rollingUpdate` is present. `helm-values/hydra.yaml:97-99` (`rollingUpdate: null`) produces this. The backup and restore runbook text is at `hydra.yaml:25-28` and `helm-values/hydra.yaml:93-96`. |
| LOW-4 | OPEN, unchanged | No `oauth2.pkce` in the ConfigMap (`hydra.yaml:80-102`). Decide in 4.6 as before. |
| LOW-5 | RESOLVED in repo; PR-body part UNCONFIRMED | `hydra.yaml:30-39` and `manifests.yaml:1348-1352` now say `--from-file`, at least 32 random bytes, an offline backup of `secretsSystem`, and `\password hydra`, never a literal. I could not read the rewritten OG-24 steps in the PR body. Lead: confirm they pipe the `hydra create client` output straight into `ipfs/ipfs-console-oidc` without printing it, and record only `client_id`. |
| LOW-6 | RESOLVED (digest match not verified) | `manifests.yaml:513` pins `oryd/kratos-selfservice-ui-node:v1.3.1@sha256:2d6f9a84...a3`. I could not confirm that this is the live pod's imageID, or whether it is an index or a single-platform digest. A single-platform digest is fine on amd64-only LKE. Expect kratos-ui to roll at sync, because the pod template changed. |
| LOW-7 | OPEN, unchanged | `manifests.yaml:501-505`. Not required for OG-24 or OG-26. |

## Regression checks

1. **Does the selector match the namespace?** Yes. The API server sets the label `kubernetes.io/metadata.name=<name>` on every namespace (GA since Kubernetes 1.22), and users cannot change it. Gateway API supports `from: Selector` with `matchLabels` as core, so Envoy Gateway handles it. If it ever failed to match, `hydra-public` would get `Accepted=False` / `NotAllowedByListeners` and the issuer would go unreachable. That fails closed. Live confirmation: listener `flint-oauth-https` shows `attachedRoutes: 1`, HTTPRoute `flint-core/hydra-public` shows `Accepted=True` / `ResolvedRefs=True`, and `kubectl get ns flint-core --show-labels` shows the label.
2. **Recreate with an initContainer and one replica.** This adds a short outage on every pod-template change: an image bump, a ConfigMap checksum change, or a restart annotation. The old pod stops (grace period up to 60s), then the new pod's migration (a no-op when the schema is current) and startup run.
   - During that window, new logins, code exchanges and refreshes fail, and JWKS fetches fail.
   - Envoy's `remote_jwks` cache (5 minutes by default) carries existing sessions through it.
   - A failed migration now leaves zero Hydra pods instead of the old one serving. That also fails closed, and the backup-before-bump runbook covers it.
   - This is acceptable for one replica and a single operator.
   - Group 6: `ipfs mcp`'s JWKS client should cache keys and fail closed on fetch errors, not fail open.
   - New minor risk: the read-only root filesystem on `hydra migrate sql up`. I expect no disk writes. If it does write, the initContainer CrashLoops and the problem shows at OG-24. Confirm the init completes then.
3. **Does the check script fail closed?** Yes, by reading. It does not use yq.
   - `set -euo pipefail` plus `exec python3 -I`: if python3 is missing, the failed exec exits non-zero.
   - Missing PyYAML gives an ImportError and exit 1. `-I` also ignores user site-packages and PYTHONPATH, so a `pip --user` install also gives a FAIL, never a PASS.
   - An empty or wrong root finds zero Gateways and FAILs.
   - Unparseable YAML or a missing `metadata` or `spec` raises an exception and exits 1.
   - The `allowedRoutes` comparison is exact dict equality, so any extra key fails.
   - In the self-test, a mutation whose target is missing changes nothing, so the check still passes on that copy, and the self-test reports that mutation as a FAIL. A missing listener raises `SystemExit` and exits 1.
   - By reading, all 6 mutations fail and the baseline passes.

## New items

- **LOW-8 (pre-existing, newly relevant): kratos-ui has no securityContext.** `manifests.yaml:491-531`: the pod-level `securityContext: {}` and none on the container. This internet-facing Node app can now reach the unauthenticated hydra-admin. Add runAsNonRoot, `allowPrivilegeEscalation: false`, drop ALL and seccomp RuntimeDefault, then check that the image's user works. Recommended before OG-26; it does not block.
- **INFO-A: two gaps in the check script.**
  - Lines 85-92 skip any route that names neither `flint-oauth-https` nor the exact host. Two shapes in `flint-core` would still attach to the oauth listener:
    - a route with a parentRef to `argocd-gateway` that has no `sectionName` and no hostnames;
    - a route with the wildcard hostname `*.know-me.tools`.
  - No such route exists today: every `flint-core` route sets both a `sectionName` and an exact hostname (`manifests.yaml:2296-2432`, `hydra.yaml:342-349`).
  - Hardening: treat a parentRef to `argocd-gateway` without `sectionName` as naming the listener, and match the host against wildcard hostnames.
  - Separately, the docstring at lines 21-25 says "an extra matchLabels key", but `m_extra_label` adds `matchExpressions`. The exact-equality check catches both, so only the comment is wrong.
- **INFO-B: the `flint-auth-https` / `flint-auth-http` listeners are still `from: All`** (`manifests.yaml:241-253`, `:377-383`). From group 6, `auth.know-me.tools/ui/login` is the console's login page. A route attached to it from another namespace could phish the operator's password. It cannot accept Hydra challenges while MEDIUM-1 holds. Apply the same Selector stanza before OG-26.

## Before OG-24 (Secrets and client)

1. LOW-2 is closed or accepted in writing on #42 or #40 before the hydra password is set. Preferred fix, in the postgres image's pg_hba, first-match order, ahead of the generic host rule: `host hydra hydra all scram-sha-256` then `host all hydra all reject`.
2. The lead confirms LOW-5 in the PR body: the client secret is piped and never displayed, and only `client_id` is recorded.
3. The secrets are created as the in-repo comments say: `--from-file`, at least 32 random bytes, an offline backup of `secretsSystem`, and `\password hydra`.
4. After sync:
   - the hydra-automigrate init completes under the read-only root filesystem;
   - HTTPRoute `hydra-public` is Accepted and the listener shows `attachedRoutes: 1`;
   - `kubernetes.io/metadata.name=flint-core` is present on the namespace;
   - kratos-ui rolls onto the pinned digest and stays Ready.
5. Run `oauth-listener-check.sh` and `--self-test` on 6121615 and paste the output (not run here).

## Before OG-26 (group 6 trusts the issuer)

1. MEDIUM-1: the live 4.5 probes FAIL on 4445 from another namespace and from an unlabeled `flint-core` pod, while 4444 answers. Add the namespace assertion to `networkpolicy-check.sh`.
2. LOW-4: PKCE S256 is recorded in 4.6, then `oauth2.pkce.enforced: true`.
3. INFO-B: the `flint-auth-*` listeners are restricted to `flint-core`.
4. LOW-8: kratos-ui is hardened (recommended).
5. INFO-A: the check script hardening (recommended).
6. Group 6: the `ipfs mcp` JWKS client caches keys and fails closed on fetch errors.
7. Carried from #40 and still open: INFO-7 (tighten Kratos `allowed_return_urls`) and LOW-7.

## Manual follow-ups

1. Confirm the worktree is 6121615.
2. Read the OG-24 steps in the PR body (LOW-5).
3. Run the check script and its `--self-test`.
4. Check the kratos-ui digest against the live imageID (`kubectl -n flint-core get pod -l app=kratos-ui -o jsonpath='{..imageID}'`).
5. The live Gateway listener status and the namespace label.
6. The pg_hba in the flint-forge postgres image (LOW-2).
