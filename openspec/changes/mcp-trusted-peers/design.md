# Design: mcp-trusted-peers

## Context

See proposal.md. Today the server marks untrusted data only per result: a tool with `returnsUntrustedContent` puts `_meta["ai.ipfs/untrusted-content"] = true` on every result, and the agent loop wraps such results in `<untrusted-content>` tags. `mcp/webmcp` puts results of tools with `untrustedContentHint` under `data` with `untrusted_content: true`. The contract ties `returnsUntrustedContent` to `contentEgress` (`TestUntrustedContentRequiresEgress`), and the operator kept peer and routing tools `contentEgress: false`.

## Goals / Non-Goals

**Goals:** mark peer-supplied entries per entry, trusted only by peer ID, without changing `contentEgress` or breaking existing output consumers.

**Non-Goals:** a trusted list in the browser; trusting by agent version or address; verifying signed peer records.

## Decisions

1. **One optional boolean per entry, `untrusted`.** The smallest additive change. Existing fields (`addr`, `addrs`, `id`) keep their names and types. It is optional, and absent means untrusted, so an older server or a consumer that drops unknown fields fails safe. Rejected: wrapping untrusted entries in a separate array, which changes the shape; renaming `addrs`, which breaks consumers; a JSON Schema extension keyword on the field, which the MCP bundle and the browser validator would not carry reliably.
2. **Tool-level declaration in the catalog, `peerSuppliedFields`.** Generators and tests learn which tools carry the marker without parsing descriptions. It is separate from `returnsUntrustedContent`, so OQ-EGRESS-ENUM is untouched.
3. **Result-level flag reused.** When any entry is untrusted, the server sets the existing `_meta["ai.ipfs/untrusted-content"]`, so the agent loop's framing works with no new code path. A result in which every entry is trusted carries no flag.
4. **WebMCP derivation gains an `any:` rule form** (`any:returnsUntrustedContent,peerSupplied`), interpreted the same way by the Go validator and the `mcp/webmcp` generator.
5. **Trusted list in config only.** `trusted_peers` uses the same `PeerId` grammar as the tool contract. No CLI flag or environment variable.

## Risks / Trade-offs

- [A listed peer's addresses from routing were relayed by other DHT nodes, not by the peer itself] → For `routing_findpeer` and `routing_findprovs`, "trusted" means "this peer ID is one the operator trusts", not "these addresses are authentic". Kubo's RPC does not say whether addresses came from a signed peer record. Documented in the config description and in ADR-0004.
- [Operators list peers casually to silence the marker] → The default is empty and the description says what trust means; nothing else changes with the list.
- [Agents ignore per-entry markers] → The result-level meta flag makes the loop frame the whole result as data whenever any entry is untrusted.
