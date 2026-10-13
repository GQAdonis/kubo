# Design: mcp-embedding-index-lifecycle

## Context

See proposal.md and the plan revision 2 decisions (ADR-0004 to ADR-0007 drafts in `mcp-architecture-adrs`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.

## Goals / Non-Goals

**Goals:**
- Deliver the proposal's behavior with an off switch and no default endpoint.

**Non-Goals:**
- Any change to `/api/v0`, the gateway, the default CID recipe, wire behavior, the daemon, `config/` or `docs/config.md`.

## Decisions

- **Never delete on a guess.** If the pin check fails, vectors stay and a warning is returned.
- **Per-item, opt-in trigger.** Only content added through `ipfs mcp` with indexing on, or indexed explicitly.

### Round 8 contract amendment (architect, 2026-10-09)

Gaps found while implementing, decided in `mcp/contract` (all additive; no Kubo RPC, gateway, CID or wire surface is touched):

- **Lifecycle warnings are structured output, not `_meta`.** `pin_rm`, `pin_update`, `repo_gc` and `ipfs_index_reconcile` outputs gain an optional `warnings` array (`common.json#/$defs/Warnings`), the same shape as `ipfs_add.warnings`. The `_meta["ai.ipfs/warnings"]` key the first implementation used is not ratified and is removed. Reasons: clients pass `structuredContent` (and its text rendering) to the model but generally not `_meta`, so a warning that vectors were kept and `ipfs_index_reconcile` should run would never reach the agent that has to act on it; `ipfs_add` already reports indexing warnings in its output, and one channel for one kind of message is the contract rule. Alternative rejected: a contract constant for `ai.ipfs/warnings`, like `ai.ipfs/untrusted-content`. That key is a framing flag for the client and the agent loop; warnings are information for the caller. Hyrum risk: the field is optional and absent when empty, so a consumer that ignores it sees the same output as before.
- **`ipfs_index_cid.skipped_reason` gains `not_a_file` and `empty`.** A directory (or another non-file node) is `not_a_file`, not `mime_not_allowed`; an empty or whitespace-only file is `empty` with `chunks_indexed` 0, instead of a result with no reason. The schema now describes every value.
- **Retryable `index_unavailable` and `egress_not_acknowledged` are named in the `ipfs_index_*` descriptions**, so a model that reads only `tools/list` knows to retry the first and to change principal for the second.
- **RPC lists match the calls.** `ipfs_index_search` reads hit text with `cat`, so `rpc` is `["cat"]` (it was empty). `pin_rm`, `pin_update` and `repo_gc` call `pin/ls` for the pin-aware deletion when embeddings are configured, so each has `rpcOptional: ["pin/ls"]`. Without this, an `API.Authorizations` entry printed by `--print-rpc-allowed-paths` denies those calls: search silently drops every hit, and deletion keeps vectors with a warning.
- **Config schema.** `embeddings.remote.credential` and `embeddings.embedder.api_key` now carry a `description` of the credential format: `user:password` (non-empty user) is HTTP Basic; any other value is a bearer token.

Follow-ups this creates: `mcp-server-engineer` moves the warnings from `_meta` into the `warnings` field and returns the new skip reasons (tasks 1.2 and 1.3); `ui-ux-engineer` regenerates `mcp/ui` (`pnpm gen:contract`), adds copy for the two skip reasons and renders `warnings` on the pin and node-status outputs, then rebuilds the `mcp/app` bundle. The `mcp/ui` regeneration lands before or with the server change: the UI parses outputs with strict objects and enums, so an output with `warnings` or a new skip reason fails to parse against the old generated types.

Open recommendation for `mcp-server-engineer` (not yet a contract rule): `ipfs_index_search` reads hit text with `cat` without `offline`, so a hit whose blocks were garbage-collected makes the node ask the network for them for up to 20 seconds. That announces interest in content the operator deleted. Passing the root `offline: true` option on that read keeps search local; `ipfs_index_cid`, an explicit confirmed request, keeps `ipfs_cat` semantics.

## Risks / Trade-offs

- [Index drift from out-of-agent unpins] -> operator-run reconciliation; immediate deletion would need a Kubo pin event source (out of this phase).
