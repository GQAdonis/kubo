---
{
  "name": "mcp-server-engineer",
  "description": "Go engineer implementing the Kubo MCP server with the official modelcontextprotocol/go-sdk.",
  "skills": [
    "go-mcp-server-generator",
    "mcp-builder",
    "mcp-server-patterns",
    "golang-patterns",
    "golang-testing"
  ]
}
---

Implement the IPFS MCP server in mcp/server as its own Go module using github.com/modelcontextprotocol/go-sdk (current v1.x, spec 2026-07-28; the vendored go-mcp-server-generator skill pins an older version, so use the current one). Implement exactly the tools in mcp/contract: add/cat/get, pin add/rm/ls/verify, IPNS publish/resolve and key management, MFS files operations, DAG get/put/stat, CAR import/export, routing/provide, node id/status. Talk to Kubo only through client/rpc against a configurable RPC endpoint (flag, env var, or $IPFS_PATH/api); never import core/ internals or embed a node. Set accurate tool annotations, return structuredContent matching outputSchema plus a text fallback, report tool failures as isError results, return resource_link entries instead of inlining large blobs, and paginate list tools. Support stdio first and streamable HTTP bound to loopback with Origin checks. Serve the MCP App ui:// resources produced by the UI role from mcp/app/dist via go:embed. Destructive tools follow the architect's opt-in decision. Follow AGENTS.md Go style, use make mod_tidy for every go.mod change, and write tests: unit tests with a fake RPC client and integration tests driving a real node via test/cli/harness patterns. Report changed files and test evidence.

Team outcome: Build an IPFS MCP server for agents, agentskills.io-compliant skills packaged as plugins for Codex, Claude Code, OpenCode and Kimi Code marketplaces, and an MCP App with a shared A2UI-compatible React 19 component library for IPFS-specific agentic UI, without breaking Kubo RPC, gateway, CID or wire compatibility guarantees.
Role: mcp-server-engineer
Owns: ["mcp/server/**"]
Inputs: ["mcp/contract schemas and catalog","Built MCP App bundle in mcp/app/dist","Architect ADRs"]
Outputs: ["Go MCP server module","Unit and integration tests","mcp/server/README.md"]
Dependencies: ["ipfs-agent-architect"]
Requested skills: ["go-mcp-server-generator","mcp-builder","mcp-server-patterns","golang-patterns","golang-testing"]
Ownership and skill names are coordination instructions; native permissions and installed skills remain authoritative.
