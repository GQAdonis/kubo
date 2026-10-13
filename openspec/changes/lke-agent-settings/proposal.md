# Proposal: lke-agent-settings

Phase `agent-backend-settings`, change 25 of 28 in plan revision 3, Round 9. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (C-A).

## Why

With the candidate image rolled, the LKE sidecar can serve `/agui` and the settings route on the console host, with out-of-band Secrets, a standalone settings PVC and bearer-only routes.

## What Changes

- 1.1: Sidecar and config: `--agui`, `--settings-dir=/mcp-settings`, `admin` and `ui` principals reading their tokens from the out-of-band Secrets created at OG-14, `agent.origin_allowlist` = console origin, `agent.settings.accept_secrets: false`, `llm.api_key` as a file reference to the out-of-band LLM key Secret (S1: the cloud key is set by the operator out-of-band, never through the browser, never in git).
- 1.2: Console routes for `/agui` and the settings route to `ipfs-mcp:5050`, bearer-only, plus a streaming timeout policy.
- 1.3: Document LKE confirmation behavior (S2) in `docs/mcp/chat-setup.md`: with `confirmation.modes: []`, MCP clients on `/mcp` need elicitation for writes and destructive tools; chat writes are confirmed by the island interrupt answered with a user gesture; destructive tools from chat are refused with `confirmation_unavailable`.
- 1.4: Standalone RWO PersistentVolumeClaim `ipfs-mcp-settings` (1Gi) mounted at `/mcp-settings` in the sidecar; the StatefulSet `volumeClaimTemplates` stay unchanged because they are immutable (WARNING-5); pod `fsGroup` 1000 already applies.
- 1.5: Rollback and safety (R9; W9).
- 1.6: PR body file and offline render.
- 1.7: Security-reviewer read of the PR diff before OG-13 (C-B).
- 1.8: Post-merge check script `lke-checks/agent-settings.sh`.
- 1.9: After OG-13, record the operator's bearer output and run the unauthenticated part (read-only).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: know-me-cluster deployment configuration outside this repository; no kubo behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect
- Scope: know-me-cluster feature branch, `namespaces/ipfs/**`, `docs/mcp/chat-setup.md`
- Depends on: lke-origin-isolation, kubo-candidate-image, agent-features-default-on, settings-security-review; OG-12 and OG-14 before OG-13
- Size: M-L; goals: 1, 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
