// Package server implements the IPFS MCP server behind the `ipfs mcp`
// command.
//
// The server is a client of a running Kubo daemon. It reaches the daemon only
// through client/rpc (the /api/v0 RPC API), it never opens the repo or takes
// repo.lock, and it has no offline mode. Its tools, schemas, annotations and
// default selection come from the embedded contract in mcp/contract.
//
// Tools are registered per group: each tools_<group>.go file adds a factory
// with registerGroup in an init function, so groups added by different changes
// do not touch a shared file.
package server
