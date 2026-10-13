# Proposal: uar-conformance-release

Phase `agent-backend-settings`, change 3 of 28 in plan revision 3, Round 1. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: none.

## Why

Goal 2 needs UAR features that do not exist at UAR HEAD `b4244aea` (per-agent MCP binding with a run-time credential, skill upload, elicitation, an AG-UI agent path) and a published release. The request has the longest lead time, so it starts first.

## What Changes

- 1.1: Draft the request as a follow-up to `Prometheus-AGS/universal-agent-runtime#379`: commit the working tree and release `v1.1.0` with an OCI digest; bind an external MCP server to an agent with a run-time credential; remote skill upload; elicitation for globally registered servers (and `shared_transports_enabled`); an AG-UI agent path and interrupt shapes; verify #379 items 2, 3, 7.
- 1.2: After the operator files it (OG-3), record the URL and poll with read-only `gh issue view` until it is visible.
- 1.3: When OG-4 confirms a release, record tag and digest.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: a cross-team request document; no kubo behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect drafts and records; operator files (OG-3)
- Scope: `mcp/uar/requests/**`
- Depends on: none
- Size: S; goals: 2
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
