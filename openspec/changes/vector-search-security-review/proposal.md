# Proposal: vector-search-security-review

Phase `agent-backend-settings`, change 36 of 36 in plan revision 4 (proposed, pending OG-16), Round 7, beside `settings-security-review`. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011.

## Why

Whole-node search adds new egress paths (extractor, describer, gateway), an unauthenticated local service (xberg), a new HTTP read surface on the console, stored copies of derived text, untrusted model output shown in the Web UI and fed to agents, and a long-running job that reads every pinned file. These need an independent security pass before the candidate image (Round 8) carries them to LKE.

## What Changes

- 1.1: Independent review by `security-reviewer` on Claude Code.
- 1.2: One batched correction of CRITICAL and HIGH findings by the owning builders.
- 1.3: Confirmation pass.

Full task text and acceptance checks are in `tasks.md`.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: read-only review with batched corrections in the owning changes.

### Modified Capabilities

None.

## Impact

- Owners: security-reviewer (1.1, 1.3, Claude Code, read-only); owning builders on Kimi Code (1.2)
- Scope: read-only review; findings in `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/`
- Depends on: mcp-content-extractor, mcp-media-describer, mcp-whole-node-indexing, webui-vector-search, desktop-search-sidecar
- Size: M; goals: 6
- Stability: review only; corrections keep the Stability and User Agency rules.
