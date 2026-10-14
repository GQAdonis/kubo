# Proposal: lke-vector-search

Phase `agent-backend-settings`, change 35 of 36 in plan revision 4 (proposed, pending OG-16), Round 9. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011 section V8 (LKE). Notes: task 1.6 is stacked on `lke-agent-settings` and its merge gate OG-21 comes after OG-13 (task-level wait inside Round 9).

## Why

The cloud node at `ipfs.prometheusags.ai` needs the services behind whole-node search: a SurrealDB store (the Kubo image carries no `surreal`, so embedded mode is not available in the pod), an xberg extractor, an embedder, optionally a liter-llm gateway and, only if the operator approves, the Xiaomi hosted describer. All of them stay inside the cluster except an approved hosted describer, none is exposed on a Gateway listener, and the console search routes are bearer-only like `/agui`.

## What Changes

- 1.1: SurrealDB StatefulSet (pinned digest, PVC, ClusterIP, out-of-band credential).
- 1.2: xberg Deployment (pinned digest, ClusterIP, model cache PVC, optional one-off warm Job), with NetworkPolicies after verifying the cluster enforces them.
- 1.3: Embedder per OG-18, and liter-llm only if OG-19 approves it (master key, ClusterIP).
- 1.4: `ipfs-mcp` sidecar config: `embeddings` in remote mode, extractor and optional describer, `ui` principal `egress_ack` equal to the configured host set; console HTTPRoute rules for `/index/*`, bearer-only.
- 1.5: Rollback and disable path.
- 1.6: PR body file and offline render.
- 1.7: Security-reviewer read of the PR diff before OG-21.
- 1.8: Post-merge check script `lke-checks/vector-search.sh`.
- 1.9: After OG-21 and OG-22, record the operator's output and the first bounded bulk run.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: know-me-cluster deployment configuration outside this repository; no kubo behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect (know-me-cluster feature branch, `namespaces/ipfs/**`); security-reviewer (1.7); operator (OG-17 to OG-22)
- Scope: `../../travis/personal/know-me-cluster/namespaces/ipfs/**`, `docs/mcp/chat-setup.md`
- Depends on: lke-origin-isolation, kubo-candidate-image (the image must contain the extractor, describer and bulk code), vector-search-security-review; task-level: lke-agent-settings 1.9 before 1.6's merge gate; OG-17, OG-18, OG-19, OG-20 before 1.1-1.4 are final
- Size: L; goals: 6
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change; no Kubo repo config change on the node. Every new component can be removed by deleting the `embeddings` block and its manifests (AGENTS.md off-switch rule). Each merge is a planned single-replica restart under the 2026-10-13 gotcha.
