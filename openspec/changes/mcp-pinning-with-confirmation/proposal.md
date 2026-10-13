# Proposal: mcp-pinning-with-confirmation

Phase `ipfs-mcp-server`, change 6 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The operator wants writes and destructive tools on every transport, each requiring confirmation and never executed silently (answer 5). This change builds that gate once, transport-agnostic, and proves it with the pinning group: the first writes and the first destructive tool.

## What Changes

- Confirmation gate. It uses MCP elicitation when the client advertises the capability. Otherwise it issues a two-step confirm token: HMAC over session, principal, tool, canonical arguments and expiry; single-use; short TTL. The first call has no side effect.
- Pin tools: `pin_add`, `pin_ls` (paginated), `pin_verify`, `pin_update`, and `pin_rm` (destructive).

## Capabilities

### New Capabilities

- `mcp-confirmation`: confirmation of write and destructive MCP tool calls through elicitation or a server-issued two-step token; never silent
- `mcp-pin-tools`: MCP tools for listing, adding, verifying, updating and removing pins on the connected node

### Modified Capabilities

None.

## Impact

`mcp/server/**` and `test/cli/mcp_*_test.go`. No Kubo RPC change.

## Plan revision 2

Tasks 1.1-1.4 are registered and unchanged. Task 1.5 adds the third gate mode required by operator answer Q4: confirmations resolved out of band (the resolvers themselves are `native-confirmation-prompt` (N14)), and no two-step token accepted from principals that cannot elicit.
