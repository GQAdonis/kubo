# Proposal: mcp-embedding-index-lifecycle

Phase `ipfs-mcp-server`, plan revision 2 (new change). N7b: opt-in indexing trigger, index tools and pin-aware deletion (split from N7 for session size). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G12 asks for configuration-driven automatic embedding of content added through the agent surfaces, and the addendum requires pin-aware deletion. The addendum's N7 covers store, trigger and lifecycle in one change; plan revision 2 splits it because the combined change is about nine tasks and does not fit one agent session (planning rule 4).

## What Changes

- Trigger: when indexing is configured (`embeddings.auto`, off by default), a successful `ipfs_add` from any `ipfs mcp` transport (stdio, HTTP, AG-UI chat, A2A, and UAR in topology B) is chunked and embedded under the MIME allowlist and size cap. Failures are warnings; the add succeeds. WebMCP adds in the browser are not indexed automatically (stated limitation).
- Tools per the contract: `ipfs_index_search` (read, `contentEgress`), `ipfs_index_cid` (explicit indexing, write class), `ipfs_index_status` (read) and `ipfs_index_reconcile` (operator-triggered, write class).
- Deletion: a CID's vectors are deleted only when `pin ls --type=all <cid>` shows no direct, recursive or indirect pin; after `repo_gc`, only CIDs GC reported removed; reconciliation for content unpinned outside `ipfs mcp` (G12-4).
- The indexing and deletion section of the docs.

## Capabilities

### New Capabilities

- `mcp-embedding-index-lifecycle`: N7b: opt-in indexing trigger, index tools and pin-aware deletion (split from N7 for session size)

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`. Off unless configured. No daemon-wide hook (refused under User Agency).
