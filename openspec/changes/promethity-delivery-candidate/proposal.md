# Proposal: promethity-delivery-candidate

Phase `agent-backend-settings`, change 26 of 28 in plan revision 3, Round 9. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (local, reversible part of UI-70).

## Why

The phase gate must launch the packaged artifacts, so the Web UI build, its CID, the `WebUIPath` bump and the Desktop package are produced before the gate.

## What Changes

- 1.1: Production Web UI build and its CID.
- 1.2: Local retrieval check of the CID on a non-default-port node.
- 1.3: After OG-7, set `WebUIPath` to the new CID on the branch and `make build`.
- 1.4: Package Desktop with the candidate Web UI and fork binary (not installed).

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: builds, pins and installs already-specified artifacts; the only kubo edit is the operator-approved WebUIPath value.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer (1.1, 1.2, 1.4); ipfs-agent-architect (1.3)
- Scope: Web UI build, `core/corehttp/webui.go`, Desktop package
- Depends on: every UI change, agent-features-default-on, mcp-agui-a2ui-registry, settings-security-review, kubo-candidate-image; OG-7 for 1.3
- Size: M; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
