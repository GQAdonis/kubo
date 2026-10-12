# Tasks: mcp-trusted-peers

Phase `ipfs-mcp-server`; operator decision of 2026-10-12 on R13 security L5. Design: `design.md`.

## 1. Peer-supplied data marker

- [x] 1.1 (architect) Contract: `peerSuppliedFields` in the catalog schema and on `swarm_peers`, `routing_findpeer`, `routing_findprovs`; `PeerSuppliedUntrusted` in `common.json` and the optional `untrusted` field in the three output schemas; the `any:` rule and `peerSupplied` field; WebMCP `untrustedContentHint` rule `any:returnsUntrustedContent,peerSupplied`; config key `trusted_peers` with `PeerId`, an example and an invalid example. Verify: `go test ./mcp/contract/...` passes, including `TestPeerSuppliedFieldsAreMarked`, `TestEveryPeerAddressIsDeclared`, `TestAnyRule`, `TestDerivedWebMCPAnnotations` and the config example tests
- [x] 1.2 (mcp-server-engineer) Read `trusted_peers`; set `untrusted` on each entry of the three tools (`false` only for a listed peer ID); set `_meta["ai.ipfs/untrusted-content"] = true` when any entry is untrusted. Verify: unit tests for an empty list (all untrusted, flagged), a mixed result (flagged) and an all-trusted result (not flagged); a `test/cli` case on `swarm_peers` with two harness nodes, one listed; `ipfs_node_info` unchanged; remove the "Not read by the server yet" note from `docs/mcp/config-reference.tests.json` and cite the new tests
- [x] 1.3 (mcp/webmcp owner) Support the `in:` and `any:` rule forms in `scripts/lib/generate.mjs`, regenerate `src/generated/tools.ts`, and set `untrusted: true` on every peer-supplied entry. Verify: the generator check passes; the three tools carry `untrustedContentHint: true`; a test shows each entry marked and the result under `data` with `untrusted_content: true`
- [x] 1.4 (ui-ux-engineer) Regenerate `mcp/ui` from the contract. Verify: `pnpm gen:check` passes
