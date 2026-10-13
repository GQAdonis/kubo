# Proposal: kubo-candidate-image

Phase `agent-backend-settings`, change 24 of 28 in plan revision 3, Round 8. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (C-A, WARNING-1).

## Why

LKE runs this fork's binary 0.44.0-dev-0f15c24, which lacks the new flags and principal kind. A verified linux/amd64 image must reach LKE through an image-only GitOps PR before the agent settings can be configured there.

## What Changes

- 1.1: Pre-image verification on the branch head (exception 5): `make build`, `go test ./mcp/...`, `TestDefaultMatchesExpectedProfile`, `test/cli/gateway_test.go`, `TestTelemetry`.
- 1.2: Build a linux/amd64 image from the branch head with the repository `Dockerfile` (`docker buildx build --platform linux/amd64 --load`), tagged with the short commit in the registry named at OG-1.
- 1.3: Container smoke with its own `IPFS_PATH` on non-default ports.
- 1.4: Image-only know-me-cluster PR (the `image:` line set to the digest from 1.3; no flag or config change) and the OG-12 runbook (planned single-replica restart under the 2026-10-13 gotcha; rollback image 0.44.0-dev-0f15c24).
- 1.5: Security-reviewer read of the image PR before OG-12.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: builds and ships an existing kubo commit; no behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer builds (1.1-1.3); ipfs-agent-architect prepares the cluster PR (1.4); operator pushes and merges (OG-12)
- Scope: local container build from the kubo branch; know-me-cluster image line
- Depends on: mcp-agent-hot-reload, mcp-agui-a2ui-registry, mcp-agent-settings-api, agent-features-default-on, settings-security-review
- Size: M; goals: 1, 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
