# Tasks: mcp-pinning-with-confirmation

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS (plan revision 2), phase path `ipfs-mcp-server`, change ID `mcp-pinning-with-confirmation`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-server-core-stdio`. Scheduled in Round 3.

## 1. Confirmation gate and pinning tools

- [x] 1.1 Implement the confirmation gate: elicitation detection, HMAC single-use token with TTL, structured `confirmation_required` result, never silent. Verify: the unit tests in 1.3 pass
- [x] 1.2 Implement `pin_add`, `pin_ls`, `pin_verify`, `pin_update` and `pin_rm` via CoreAPI `Pin()`. Verify: unit tests with a fake RPC match the contract output schemas
- [x] 1.3 Write unit tests for token replay, tampering, expiry and cross-session reuse, and for elicitation accept/decline using a go-sdk client with an elicitation handler. Verify: `go test ./mcp/server/...` passes
- [x] 1.4 Write test/cli real-daemon tests: an unconfirmed `pin_rm` leaves the pin, a confirmed one removes it, plus contract conformance for the pin tools. Verify: `make build && go test ./test/cli/... -run TestMCP -v` passes against a harness daemon
- [x] 1.5 Extend the gate state machine for external resolution (pending to approved, denied or expired by the native prompt, an AG-UI interrupt answer or another out-of-band resolver; A2A input-required waits) and refuse the two-step token from principals marked as unable to elicit, so no agent can confirm itself. Verify: unit tests show a token presented by such a principal is rejected and an externally approved call runs exactly once
