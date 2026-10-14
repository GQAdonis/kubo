# Plan — agent-backend-settings › lke-deploy-discipline

Plan revision 1, 2026-10-14. Input: [`assessment.md`](assessment.md) (revised after an independent review). Change backend: OpenSpec (`openspec/changes/<id>/`), driven through `kbd-apply` begin-task and end-task.

## Lessons applied

- "External integrations require at least one real-service test." The skill's acceptance runs it against the live cluster, and it must fail on today's broken console listener before it is trusted (change 1, task 1.5).
- "Preserve production evidence in dated review folders." Every preflight and verify run writes a new dated file; nothing is overwritten.
- "Operator controls PR merges." The skill stops at "PR ready with preflight posted"; merges and cluster mutations stay with the operator.
- "Boundary gates must launch the packaged artifact." The proof (change 2) uses the skill's scripts exactly as committed, not ad-hoc commands.

## Scope and route

- Repository for the skill: `Prometheus-AGS/know-me-cluster` (`.claude/skills/lke-gitops-change/`, mirrored at `.agents/skills/lke-gitops-change/`). Neither path is owned by a team role; the lead builds it, because building and testing it needs read-only live cluster access, which builders are deliberately not given. Recorded as a deviation from team routing.
- The lead runs only read-only cluster commands, with one exception: the OG-6 restart (`rollout restart`), approved by the operator in the lead session and recorded verbatim in the parent `execution.md` ("Operator gate OG-6 (answered 2026-10-14)"), and run only under the conditions in task 2.5. All other cluster mutations are operator gates: merges, Argo syncs, recovery of a hung pod on the node, Secret annotation removal, key rotation, orphan cleanup.

## Changes (in order)

### 1. `lke-gitops-change-skill`

An agentskills.io-compliant skill for any know-me-cluster change.

- 1.1 `SKILL.md`: frontmatter (`name`, `description` that triggers on any know-me-cluster, LKE, Argo, Gateway, cert-manager or SecurityPolicy change); the procedure: (1) inventory and read the facts, (2) write the change, (3) offline checks (render, `kubeconform`, the namespace's own check scripts), (4) `preflight` and post its report on the PR, (5) stop for the operator's merge, (6) after sync `verify` and post its report, (7) restart rules, (8) rollback as a forward fix. Acceptance: the procedure names each of the four failures in the assessment and the step that blocks it.
- 1.2 `scripts/preflight.sh <namespace> [manifest...]`, read-only config checks: referenced Secrets and keys; zone apex NS, A record against the owning gateway address, CAA; TLS coverage (a Secret in the namespace whose public `tls.crt` SANs match the host, or an issuer whose solver type fits the zone and whose solver parentRefs name an existing gateway with an HTTP listener for the host); listeners and LoadBalancer ports; route `sectionName` pinning; the Argo app's sync and health (a non-Healthy app FAILs unless the caller passes `--expect-unhealthy <kind>/<name>[:<listener>]`; the exception holds only when that named resource, and nothing else in the app, is the unhealthy one, and it is printed in the report so it is visible on the PR); `argocd app diff` when the CLI is configured, otherwise reported as skipped. Writes `evidence/preflight-<UTC>.txt`, exits nonzero on any failure. Acceptance: every check prints PASS, FAIL or SKIP with the object it read.
- 1.3 `scripts/verify.sh <namespace> <host>...`, read-only outcome checks after a sync: every listener and route `Programmed` and `ResolvedRefs`; each SecurityPolicy `Accepted` per target; certificates `Ready`; external HTTPS and HTTP fetches of each host from this machine with expected status codes (passed as `host=path:code`); each host's DNS A record equals its gateway's LoadBalancer address; StatefulSet and PVC readiness; leftover cert-manager objects (Challenges, Orders, solver routes) in the namespace. Same evidence and exit rules. Acceptance: runs with no cluster write verb anywhere in the script (grep).
- 1.4 `scripts/inventory.sh` regenerates `references/cluster-facts.md` (dated): zones and DNS providers, certificates with SANs, issuers and expiry, issuers and solver attachment, gateways, listeners and addresses, routes, SecurityPolicies, Argo apps and sync policy, workloads per namespace. Secrets are read with field selection only (names, key names, public `tls.crt`); Secret annotations are never output. Acceptance: grep of the scripts finds no `-o yaml`/`-o json` on Secrets without a field filter, and the generated file contains no `last-applied-configuration`.
- 1.5 Live self-test against the cluster as it is now. Acceptance, per script: `verify.sh ipfs console.prometheusags.ai=/api/v0/version:401 ipfs.prometheusags.ai=/api/v0/version:401` exits nonzero, its report shows FAIL for listener `https-console` (`Programmed=False`) and for the console HTTPS fetch, and PASS for listener `https` and the `ipfs.prometheusags.ai` fetch; `preflight.sh ipfs` reports the dangling `matrix` gateway references as a FAIL line on issuer `letsencrypt-http01` (the overall exit is nonzero for that reason, which is correct). A run that passes the broken console is a failed task.
- 1.6 Validation and packaging: validate against the agentskills.io spec with the installed `validate-skill` skill (listed in this harness); mirror to `.agents/skills/`; a short pointer in the repo README. Acceptance: validator passes; both copies are byte-identical (`diff -r`).

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

## Operator gates in this child

- G-1: merge PR-A (after 2.2's preflight). G-1b: merge PR-B (after its preflight).
- G-2 (decision): strip `kubectl.kubernetes.io/last-applied-configuration` from `ipfs/ipfs-tls`, `argocd/wildcard-prometheusags-ai-tls`, `onyx/prometheusags-wildcard-tls` (removes the stored private key copy; no service impact). The lead prepares the exact commands; the operator runs or approves them.
- G-3 (decision): rotate the wildcard certificate and key with ZeroSSL, and who owns renewal before 2027-02-13.
- G-4 (cleanup): delete leftover cert-manager objects if they remain after PR-A (Challenge, Order, solver route) and the ACME account Secret `ipfs/letsencrypt-http01-ipfs-account` after PR-B. The lead lists the exact objects; the operator deletes them.
- Recovery of a hung restart (2.5 step 3) is the operator's.

## Task model assignments

| Phase path | Change | Task | Task class | Route | Model | Reason | Prerequisites |
|---|---|---|---|---|---|---|---|
| agent-backend-settings/lke-deploy-discipline | lke-gitops-change-skill | 1.1-1.6 | Ops procedure and read-only tooling against a live cluster | Lead, Claude Code native (this session) | claude-opus-5-5 | Needs read-only live cluster access, which builder sessions are not given | none |
| agent-backend-settings/lke-deploy-discipline | lke-console-tls-wildcard | 2.1-2.2, 2.4 | Manifest edit plus live preflight | Lead, Claude Code native | claude-opus-5-5 | Same | change 1 |
| agent-backend-settings/lke-deploy-discipline | lke-console-tls-wildcard | 2.3, 2.5 | Post-merge verify and bounded restart | Lead, Claude Code native; operator gates G-1, G-4 | claude-opus-5-5 | Same | G-1 |

Review: an `artifact-critic` read of the skill (fresh context) before change 2 starts, and a `security-reviewer` read of each PR diff before its merge gate.

Plan review (2026-10-14, `artifact-critic`, fresh context): two CRITICAL (restart approval not traceable; no abort path for a hung restart) and six WARNING findings; revised twice: the approval is now recorded verbatim in the parent `execution.md`; 2.5 adds a snapshot, a bounded wait and a stop-and-hand-over rule; the console change is split into PR-A and PR-B; preflight takes a visible expected-health exception; verify checks LoadBalancer address, DNS and leftover cert-manager objects; orphan cleanup is gate G-4. Round 2: no CRITICAL; two WARNINGs (health exception too broad; restart could overlap PR-B) and one suggestion (failed post-restart check) applied.

## Exit criteria

The skill is committed in know-me-cluster with a passing validator and a recorded live self-test that failed on the broken console; the console host serves the wildcard and answers 401 unauthenticated; OG-6's restart and unauthenticated checks have passed; `handoff-out.md` tells the parent that later cluster changes (`lke-agent-settings`, the vector-search deployment) must use the skill.
