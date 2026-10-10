# Proposal: uar-conformance-request

Phase `ipfs-mcp-server`, plan revision 2 (new change). N1: request the UAR conformance and safe-default changes that topology B and the UAR backend need. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The operator chose two topologies. Topology 2 (UAR hosts AG-UI and A2A and calls `ipfs mcp` as a tool server) is supported only once UAR ships its conformance fixes in a published release. UAR is another team's code: Kubo files a precise request and does not edit UAR. Filing it in Round 1 gives the UAR team the most time before the delivery gate.

## What Changes

- `mcp/uar/requests/CONFORMANCE.md`: each requested UAR item with a falsifiable acceptance check: A2A 1.0 agent card at `/.well-known/agent-card.json`, spec TaskState wire values, tool approval surfaced as input-required, an AG-UI `RunAgentInput` endpoint with spec interrupts, a configurable CORS allowlist, MCP elicitation in UAR's MCP client, safe defaults (no default model, local memory embedder, loopback bind), readable effective settings (model, embedders, governance, `terminal_exec`, configured MCP servers), and the UAR version in the agent card.
- The delivery-gate rule: only items contained in a published, installable UAR release (tag or image digest) count. Merged-but-unreleased items do not.
- The operator files the request with the UAR team and records the link.
- Topology-B gate (addendum revision 3): UAR's conformance items contained in a published, installable UAR release (tag plus image digest or native package) by the end of Round 4. Merged-but-unreleased items do not count.

## Capabilities

### New Capabilities

None. Spike or request with no product behavior (`skip_specs: true`).

### Modified Capabilities

None.

## Impact

New `mcp/uar/requests/**` (architect). No Kubo code. No edits to the UAR repository.
