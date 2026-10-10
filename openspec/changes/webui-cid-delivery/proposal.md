# Proposal: webui-cid-delivery

**CONDITIONAL:** this change is applied only if `webmcp-host-spike` reports GO for at least one host, AND the operator chooses a delivery path (Q3) after the spike. D2 (the `WebUIPath` bump) is executed only on explicit operator approval. D4 (path-gateway origin with RPC CORS) stays rejected.

Phase `ipfs-mcp-server`, change 18 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

A WebMCP-enabled Web UI build has to reach the hosts. Kubo serves only pinned Web UI CIDs on the RPC origin (`core/corehttp/webui.go:15`, `cmd/ipfs/kubo/daemon.go:870-877`), so delivering it either changes Kubo (D2) or uses Desktop bundling (D1) or an opt-in subdomain gateway (D3).

## What Changes

- The operator chooses D1, D2 or D3 based on how the spike showed Codex desktop and Kimi desktop load pages.
- If D2: bump `WebUIPath`/`WebUIPaths` in this fork, with test/cli coverage of the `/webui` redirect and the `Gateway.NoFetch` behavior, plus a changelog Highlights entry.
- If D1 or D3: documentation only.

## Capabilities

### New Capabilities

None at emission time (`skip_specs: true`). If the operator selects D2, a spec delta for `/webui` serving is added to this change before apply, because D2 changes observable Kubo behavior.

### Modified Capabilities

None.

## Impact

D2: `core/corehttp/webui.go`, `test/cli`, `docs/changelogs/v0.44.md`. D1/D3: `docs/mcp/**` only.
