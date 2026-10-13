# Proposal: mcp-architecture-adrs

Phase `ipfs-mcp-server`, change 2 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The placement decision (F4), the WebMCP approach and the confirmation/auth design need durable records in `docs/mcp/` before implementation depends on them. The lead adjudication requires operator approval before ADR-0001 is marked Accepted.

## What Changes

- ADR-0001: MCP ships as `ipfs mcp` in the fork's `ipfs` binary, as a daemon RPC client. It records the rejected alternatives and the flip conditions.
- ADR-0002: WebMCP in the GQAdonis/ipfs-webui fork. Its status (Accepted or Deferred) is set from the `webmcp-host-spike` report.
- ADR-0003: the confirmation protocol (elicitation first, two-step confirm token as fallback, never silent) and the `AuthVerifier` interface, with the path to an Ory Hydra OAuth2 resource server.

## Capabilities

### New Capabilities

None. Documentation only (`skip_specs: true`). The behavior is specified in the implementing changes.

### Modified Capabilities

None.

## Impact

New `docs/mcp/0001-*.md`, `0002-*.md` and `0003-*.md`. No code.

## Plan revision 2

Tasks 1.1-1.3 are registered and unchanged. Tasks 1.4-1.6 add ADR-0004 (agentic endpoints and LLM provider, two topologies), ADR-0005 (chat UI), and ADR-0006 (embeddings, both SurrealDB modes), from the addendum revision-3 ADR drafts; the human gate is part of ADR-0004. ADR-0006's component choice comes from plan revision 2's re-score (section D7).
