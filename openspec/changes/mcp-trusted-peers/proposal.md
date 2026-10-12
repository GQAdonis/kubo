# Proposal: mcp-trusted-peers

Phase `ipfs-mcp-server`. Operator decision of 2026-10-12 on R13 security finding L5 (`.agent-team/kubo-ipfs-agentic/findings/security/r13-webmcp.md`): network-supplied multiaddrs were returned as trusted data.

## Why

`swarm_peers`, `routing_findpeer` and `routing_findprovs` return addresses that remote peers chose. An agent treats tool results as facts. A peer can supply addresses crafted to steer the agent (for example toward an attacker's host), and nothing in the result says they came from a stranger. The only untrusted marker the contract had, the tool-level `returnsUntrustedContent`, is tied to `contentEgress`, which the operator kept `false` for these tools (OQ-EGRESS-ENUM, 2026-10-10).

## What Changes

- **Trust by peer ID.** New `ipfs mcp` config key `trusted_peers`: a list of libp2p peer IDs, default `[]`. Peer IDs are authenticated by libp2p; agent versions are self-reported and are never a trust signal.
- **Field-level untrusted marker in the contract.**
  - Catalog field `peerSuppliedFields` (per tool): slash paths of output fields supplied by remote peers. Set for `swarm_peers` (`/peers/*/addr`), `routing_findpeer` (`/addrs`) and `routing_findprovs` (`/providers/*/addrs`). `ipfs_node_info` returns the node's own addresses and is not listed.
  - Each object holding such a field gets an optional boolean `untrusted` (`common.json` `PeerSuppliedUntrusted`): `false` only when the entry's peer ID is in `trusted_peers`; `true` otherwise; absent is read as `true`.
  - When any entry of a result is untrusted, the server also sets `_meta["ai.ipfs/untrusted-content"] = true`, so the agent loop frames the result as data.
  - The marker is independent of `contentEgress`; OQ-EGRESS-ENUM is not reopened.
- **WebMCP.** No MCP config exists in the browser, so `mcp/webmcp` marks every peer-supplied entry `untrusted: true` this phase. The derivation rule for WebMCP `untrustedContentHint` becomes `any:returnsUntrustedContent,peerSupplied`, so these three tools also carry the hint and their results go under `data` with `untrusted_content: true`.
- Off switch: the default `[]` already marks every peer address untrusted; there is nothing to turn on. Listing a peer is the only way to mark its addresses trusted.

## Capabilities

### New Capabilities

- `mcp-peer-supplied-data`: how `ipfs mcp` and `mcp/webmcp` mark data supplied by remote peers, and the `trusted_peers` config key.

### Modified Capabilities

None.

## Impact

- `mcp/contract/**` (architect, done 2026-10-12): `catalog.schema.json`, `catalog.json`, `schemas/common.json`, three output schemas, `derivations/webmcp.json` and its rule schema, `derive.go`, `contract.go`, config schema, examples and tests.
- `mcp/server/**` (`mcp-server-engineer`): read `trusted_peers`, set `untrusted` per entry, set the meta flag.
- `mcp/webmcp/**` (owner): support the `any:` rule in the generator, regenerate, set `untrusted: true` on every peer-supplied entry.
- `mcp/ui/**` (ui-ux-engineer): `pnpm gen:contract`.
- No `/api/v0`, gateway, CID or wire change. Output change is additive (an optional field).
