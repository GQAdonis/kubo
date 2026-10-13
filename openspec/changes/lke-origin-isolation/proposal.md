# Proposal: lke-origin-isolation

Phase `agent-backend-settings`, change 7 of 28 in plan revision 3, Round 2. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (D1; the real D0 fix; needs no new binary, but the CORS change needs a pod restart).

## Why

On LKE the gateway, Web UI and `/api/v0` share the origin `ipfs.prometheusags.ai`, so any opened CID page can drive the RPC API with cached basic-auth credentials. A dedicated console host, a reduced CORS list and an in-git basic-auth policy remove that exposure with the current binary.

## What Changes

- 1.1: Add an HTTPS listener and certificate for the console host to Gateway `ipfs-gateway-gw` (today one listener for `ipfs.prometheusags.ai` with cert `ipfs-tls`, manifest 502-524).
- 1.2: Adopt the live basic-auth policy into git as SecurityPolicy `ipfs-webui-auth` (same name, so Argo adopts it), with `targetRefs` `ipfs-webui` and `ipfs-webui-legacy` and the htpasswd Secret referenced by name only (name from the operator; no value read).
- 1.3: Reduce `API.HTTPHeaders.Access-Control-Allow-Origin`, written by `configure-ipfs.sh` in ConfigMap `ipfs-config` (manifest 180-199, 268-278), to the console origin only, keeping `Access-Control-Allow-Credentials`.
- 1.4: Rollback and planned-restart runbook (R9; W9).
- 1.5: PR body file and offline render of every changed manifest.
- 1.6: Security-reviewer read of the PR diff before OG-6 (C-B).
- 1.7: Post-merge check script `lke-checks/origin-isolation.sh` with two parts.
- 1.8: After OG-6, record the operator's credentialed output and run the unauthenticated part (read-only) into a new dated evidence folder.
- 1.9: After 1.8 passes, prepare a separate redirect PR: on the gateway host, `/webui` and `/api/v0` redirect to the console host, and `ipfs-webui-legacy` is removed.
- 1.10: Security-reviewer read of the redirect PR before OG-15.
- 1.11: After OG-15, run the unauthenticated redirect check (read-only).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: know-me-cluster deployment configuration outside this repository; no kubo behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect
- Scope: know-me-cluster feature branch, `namespaces/ipfs/**`
- Depends on: team-ownership-agent-backend; OG-2
- Size: M-L; goals: 1, 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
