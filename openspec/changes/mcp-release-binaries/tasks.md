# Tasks: mcp-release-binaries

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-release-binaries`, backend task IDs 1.1, 1.2, 1.3. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-server-core-stdio`, `mcp-pinning-with-confirmation`, `mcp-http-transport-auth`, `native-confirmation-prompt`, `mcp-agent-principal-enforcement`, `mcp-llm-proxy-loop`, `mcp-agui-endpoint`, `mcp-a2a-endpoint`, `mcp-embeddings-surrealdb`, `mcp-embedding-index-lifecycle`, `mcp-app-views`, `webui-cid-delivery`, `mcp-tools-content`, `mcp-tools-mfs`, `mcp-tools-ipns-keys`, `mcp-tools-dag-car`, `mcp-tools-routing-diag`. Scheduled in Round 9.

## 1. Local release build

- [x] 1.1 Write the cross-compile script via `make build` (GOOS/GOARCH) producing archives and SHA-256 checksums. Verify: the script produces all targets, and the checksum verification passes
- [x] 1.2 Write the release-notes template with the stock-Kubo guidance ("use the fork's `ipfs` binary as an MCP client to your existing node", minimum daemon version, per-tool degradation). Verify: the template renders with the current version
- [x] 1.3 Smoke test: the host-architecture binary runs `ipfs mcp` against a harness daemon. Verify: the smoke script lists tools and calls `ipfs_node_info` successfully
