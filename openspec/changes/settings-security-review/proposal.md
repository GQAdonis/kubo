# Proposal: settings-security-review

Phase `agent-backend-settings`, change 23 of 28 in plan revision 3, Round 7. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (pre-LKE pass; round-2 W1; scope extended per W7).

## Why

The settings API is a new remote write surface that can redirect where node content goes. An independent security pass runs before anything reaches LKE.

## What Changes

- 1.1: Review: the settings API, `admin` principal, test-connection (SSRF), egress extension, overlay and secret storage, admin-token storage in the page, the Desktop bridge, the cloud console design (ADR-0009), and per W7: `mcp-agent-hot-reload` secret lifetime and interrupt resume across swaps, `desktop-mcp-sidecar` token generation and environment passing, and the chat `ui` token kept in `localStorage` (`src/agent-chat/settings-store.js`) on the console origin.
- 1.2: One batched correction of CRITICAL and HIGH findings by the owning roles.
- 1.3: Confirmation pass.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: independent review and verification; no behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: security-reviewer on Claude Code (1.1, 1.3); owning builders on Kimi Code (1.2)
- Scope: read-only review; findings in `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/`
- Depends on: agent-settings-contract, mcp-agent-hot-reload, mcp-agent-settings-api, desktop-mcp-sidecar, webui-agent-settings-page, agent-chat-island-integration, desktop-agent-settings-bridge, agent-features-default-on
- Size: M; goals: 1, 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
