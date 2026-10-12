# Tasks: webmcp-host-spike

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `webmcp-host-spike`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `team-ownership-grant`. Scheduled in Round 1.

## 1. Host-support spike

- [x] 1.1 Build the probe page in `mcp/webmcp/spike/`: feature detection, an `echo` tool returning a per-load nonce, and secure-context, user-agent and permissions-policy readout, with no IPFS calls. Verify: opening it in a desktop Chrome shows the readout and makes no network requests besides its own files
- [x] 1.2 The operator runs the probe in Codex desktop and records screenshots and the agent transcript. Verify: evidence files are saved under `mcp/webmcp/spike/evidence/codex/`
- [x] 1.3 The operator runs the probe in Kimi desktop and records screenshots and the agent transcript. Verify: evidence files are saved under `mcp/webmcp/spike/evidence/kimi/`
- [x] 1.4 Record fallback evidence: whether each host supports MCP stdio/HTTP and renders MCP Apps. Verify: the evidence is recorded separately from the WebMCP criteria
- [x] 1.5 Write `mcp/webmcp/spike/REPORT.md` with GO/NO-GO per host against the four criteria (`document.modelContext` present, `echo` registers, the host agent invokes it, the transcript contains the nonce). MCP/MCP App support is not GO. Verify: each criterion has cited evidence for each host
