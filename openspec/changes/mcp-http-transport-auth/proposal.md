# Proposal: mcp-http-transport-auth

Phase `ipfs-mcp-server`, change 7 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The operator wants Streamable HTTP to do everything stdio does (answer 4), authenticated by a static bearer token now and by Ory Kratos/Hydra OAuth2 later, without reworking tool handlers.

## What Changes

- `ipfs mcp --http[=<addr>]`. It binds loopback by default, and refuses a non-loopback bind without a configured verifier.
- Request hardening: go-sdk Host check, cross-origin protection, body limit, session timeout.
- An `AuthVerifier` interface mapped onto go-sdk `auth.TokenVerifier`, with scope-to-tool-group authorization and an RFC 9728 protected-resource-metadata hook.
- A static bearer verifier: token read from a file or environment variable, compared in constant time.
- Confirmation tokens bound to the HTTP session and principal.
- A `--print-rpc-allowed-paths` helper that prints a least-privilege `API.Authorizations` entry.

## Capabilities

### New Capabilities

- `mcp-http-transport`: Streamable HTTP transport for `ipfs mcp` with loopback default, bearer authentication through a pluggable verifier, and request hardening

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*_test.go`. No Kubo config keys. HTTP is off unless `--http` is given.

## Plan revision 2

Only tasks 1.1-1.2 were registered before the interruption; 1.3-1.5 are unchanged on disk and are registered as written. Tasks 1.6-1.7 add configured principals and the route-mount interface used by `mcp-agui-endpoint` and `mcp-a2a-endpoint`. Default listen address: `127.0.0.1:5050` (operator, 2026-10-09; 4000 is the liter-llm proxy). No component defaults to 4000, 1906-1908, 50051, 8081, 8181, 11434, 23001, 28000, 4545, 4546, 5173, 5432, 6379 or 8080. Tests choose free ports dynamically.
