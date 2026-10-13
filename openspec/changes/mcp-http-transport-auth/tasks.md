# Tasks: mcp-http-transport-auth

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS (plan revision 2), phase path `ipfs-mcp-server`, change ID `mcp-http-transport-auth`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-pinning-with-confirmation`. Scheduled in Round 4.

## 1. Streamable HTTP and authentication

- [x] 1.1 Implement the `--http` listener: loopback default, non-loopback refusal without a verifier, Host check, cross-origin protection, body limit, session timeout. Verify: unit tests cover each rejection
- [x] 1.2 Implement the `AuthVerifier` interface, a constant-time static bearer verifier from file or env, scope-to-group authorization and the RFC 9728 metadata hook. Verify: unit tests cover valid, invalid and missing tokens and scope denial
- [x] 1.3 Bind confirmation tokens to the HTTP session and principal. Verify: a unit test shows a token from session A is rejected in session B
- [x] 1.4 Implement `--print-rpc-allowed-paths` from the contract's required command paths. Verify: the output for `--read-only` lists only read command paths
- [x] 1.5 Write test/cli real-daemon HTTP tests: 401/403, DNS rebinding, cross-origin, stdio parity, and a confirmed destructive call, using a go-sdk HTTP client. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes against a harness daemon
- [x] 1.6 Load principals as configured entities (kind, groups, credentials by env or file) from the `ipfs mcp` config file (`--mcp-config` or `IPFS_MCP_CONFIG`), validated against the contract config schema. Verify: unit tests reject a token with no principal and a config with a literal secret
- [x] 1.7 Add the route-mount interface and path-scoped authorization for `/mcp`, `/agui`, `/a2a` and `/.well-known/agent-card.json`, sharing the AuthVerifier, Host and Origin policy and body limits, so a token for one path's principal is never accepted on another for a tool outside its groups (G9-4), and set the default listen address to `127.0.0.1:5050`, overridable by flag and env. Verify: unit tests mount a test route that inherits 401/403 behavior; no test hardcodes a port
