# Tasks: mcp-agent-principal-enforcement

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-agent-principal-enforcement`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-http-transport-auth`, `mcp-pinning-with-confirmation`. Scheduled in Round 5.

## 1. Principal enforcement

- [x] 1.1 Implement configured principals with kinds `human-mcp`, `ui`, `a2a` and `uar`, read-only defaults and named write and destructive grants from the contract config schema. Verify: unit tests show an unknown token gets 401 and a read-only principal lists no write tool
- [x] 1.2 Implement the egress acknowledgement for `contentEgress` tools and for backend and embedder hosts. Verify: unit tests return `egress_not_acknowledged` for every `contentEgress` tool without `egress_ack`
- [x] 1.3 Implement `ipfs mcp check-uar` and its re-check per session and on reported settings change (published-release version minimum, auth probe, effective provider, embedders and governance, write groups refused with `terminal_exec` or a non-Kubo MCP server, fail closed, `check: assert`). Verify: unit tests against recorded UAR fixtures refuse UAR defaults and accept the shipped profile (HG-2). Contract inputs (architect, 2026-10-09, ADR-0007 and design.md "Contract gaps resolved"): settings shape `mcp/contract/agentic/uar-effective-settings.schema.json`, path, version pin and recheck interval from `contract.LoadUARPreflight()`, fixtures `mcp/contract/fixtures/uar/`
- [x] 1.4 Implement the `shell_capable: false` requirement under `--confirm=cli` and the stdio `clientInfo` guard. Verify: unit tests refuse write groups without the assertion and refuse `contentEgress` tools to a stdio client identifying as UAR
- [x] 1.5 Add test/cli acceptance tests: an unconfigured principal gets nothing; no `contentEgress` tool without `egress_ack`; check-uar refuses a fixture UAR with defaults; disabled state: with no `--mcp-config` and no `IPFS_MCP_CONFIG`, no principal of kind `ui`, `a2a` or `uar` exists and no check-uar connection is made. Verify: `make build && go test ./test/cli/... -run TestMCPPrincipal -v` passes
