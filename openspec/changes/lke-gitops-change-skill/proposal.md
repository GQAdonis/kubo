# Proposal: lke-gitops-change-skill

## Why

Four know-me-cluster PRs (#24-#27) failed on live state that offline validation and an artifact-only review could not see. The operator asked for the procedure to be captured as a reusable agentskills.io skill.

## What changes

See the task list and the plan section reproduced here.

### 1. `lke-gitops-change-skill`

An agentskills.io-compliant skill for any know-me-cluster change.

- 1.1 `SKILL.md`: frontmatter (`name`, `description` that triggers on any know-me-cluster, LKE, Argo, Gateway, cert-manager or SecurityPolicy change); the procedure: (1) inventory and read the facts, (2) write the change, (3) offline checks (render, `kubeconform`, the namespace's own check scripts), (4) `preflight` and post its report on the PR, (5) stop for the operator's merge, (6) after sync `verify` and post its report, (7) restart rules, (8) rollback as a forward fix. Acceptance: the procedure names each of the four failures in the assessment and the step that blocks it.
- 1.2 `scripts/preflight.sh <namespace> [manifest...]`, read-only config checks: referenced Secrets and keys; zone apex NS, A record against the owning gateway address, CAA; TLS coverage (a Secret in the namespace whose public `tls.crt` SANs match the host, or an issuer whose solver type fits the zone and whose solver parentRefs name an existing gateway with an HTTP listener for the host); listeners and LoadBalancer ports; route `sectionName` pinning; the Argo app's sync and health (a non-Healthy app FAILs unless the caller passes `--expect-unhealthy <kind>/<name>[:<listener>]`; the exception holds only when that named resource, and nothing else in the app, is the unhealthy one, and it is printed in the report so it is visible on the PR); `argocd app diff` when the CLI is configured, otherwise reported as skipped. Writes `evidence/preflight-<UTC>.txt`, exits nonzero on any failure. Acceptance: every check prints PASS, FAIL or SKIP with the object it read.
- 1.3 `scripts/verify.sh <namespace> <host>...`, read-only outcome checks after a sync: every listener and route `Programmed` and `ResolvedRefs`; each SecurityPolicy `Accepted` per target; certificates `Ready`; external HTTPS and HTTP fetches of each host from this machine with expected status codes (passed as `host=path:code`); each host's DNS A record equals its gateway's LoadBalancer address; StatefulSet and PVC readiness; leftover cert-manager objects (Challenges, Orders, solver routes) in the namespace. Same evidence and exit rules. Acceptance: runs with no cluster write verb anywhere in the script (grep).
- 1.4 `scripts/inventory.sh` regenerates `references/cluster-facts.md` (dated): zones and DNS providers, certificates with SANs, issuers and expiry, issuers and solver attachment, gateways, listeners and addresses, routes, SecurityPolicies, Argo apps and sync policy, workloads per namespace. Secrets are read with field selection only (names, key names, public `tls.crt`); Secret annotations are never output. Acceptance: grep of the scripts finds no `-o yaml`/`-o json` on Secrets without a field filter, and the generated file contains no `last-applied-configuration`.
- 1.5 Live self-test against the cluster as it is now. Acceptance, per script: `verify.sh ipfs console.prometheusags.ai=/api/v0/version:401 ipfs.prometheusags.ai=/api/v0/version:401` exits nonzero, its report shows FAIL for listener `https-console` (`Programmed=False`) and for the console HTTPS fetch, and PASS for listener `https` and the `ipfs.prometheusags.ai` fetch; `preflight.sh ipfs` reports the dangling `matrix` gateway references as a FAIL line on issuer `letsencrypt-http01` (the overall exit is nonzero for that reason, which is correct). A run that passes the broken console is a failed task.
- 1.6 Validation and packaging: validate against the agentskills.io spec with the installed `validate-skill` skill (listed in this harness); mirror to `.agents/skills/`; a short pointer in the repo README. Acceptance: validator passes; both copies are byte-identical (`diff -r`).

## Impact

- Repository: `Prometheus-AGS/know-me-cluster` (namespace `ipfs` and, for the skill, `.claude/skills/` and `.agents/skills/`). No change to Kubo code, `/api/v0`, the gateway behaviour, the CID recipe or wire behaviour.
- Cluster mutations are operator gates, except the OG-6 restart recorded in the parent `execution.md`.
