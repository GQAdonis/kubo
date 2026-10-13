# Tasks: mcp-embedding-index-lifecycle

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-embedding-index-lifecycle`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-embeddings-surrealdb`, `mcp-tools-content`, `mcp-tools-routing-diag`, `mcp-pinning-with-confirmation`. Scheduled in Round 8.

## 1. Indexing and lifecycle

- [x] 1.1 Implement the chunker, policy (MIME allowlist, size cap, chunking, `embeddings.auto`) and the post-add trigger for `ipfs_add` on every transport. Verify: unit tests show no indexing when off and a warning, not a failure, when the embedder is down
- [x] 1.2 Implement `ipfs_index_search`, `ipfs_index_cid`, `ipfs_index_status` and `ipfs_index_reconcile` per the contract. Verify: unit tests match the contract output schemas
- [x] 1.3 Implement pin-aware deletion after `pin_rm` and `repo_gc`, and paginated reconciliation. Verify: unit tests keep vectors when another pin remains or the pin check fails
- [x] 1.4 Add test/cli tests with Ollama embeddings and embedded `surreal`: an added text is indexed by CID and found; double pin keeps vectors (G12-4); reconcile after a CLI unpin deletes them; and write the indexing section of `docs/mcp`. Verify: `make build && go test ./test/cli/... -run TestMCPEmbeddingIndex -v` passes
- [x] 1.5 (Round 8 contract amendment, see design.md) mcp-server-engineer: return pin_rm/pin_update/repo_gc/ipfs_index_reconcile warnings in the structured `warnings` field and remove `_meta["ai.ipfs/warnings"]`; `ipfs_index_cid` skipped_reason `not_a_file` and `empty`; `ipfs_index_search` reads hit text with `cat` offline. ui-ux-engineer: regenerate mcp/ui contract and catalog, add skip-reason copy, show warnings on pin and node-status outputs, rebuild mcp/app. Verify: go test ./mcp/... and TestMCPEmbeddingIndex pass; mcp/ui gen:check passes; mcp/app check:dist passes
