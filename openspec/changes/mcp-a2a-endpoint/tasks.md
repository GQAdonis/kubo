# Tasks: mcp-a2a-endpoint

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-a2a-endpoint`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-llm-proxy-loop`, `native-confirmation-prompt`. Scheduled in Round 7.

## 1. A2A endpoint

- [x] 1.1 Verify A2A 1.0 (card path, methods, task-state wire values, security schemes) from primary sources, record them and the implementation decision in `mcp/server/a2a/SOURCES.md`. Verify: each wire value cites the specification; any proposed module is listed with license and binary delta for an operator answer
- [x] 1.2 Serve the agent card from contract A2A metadata behind `--a2a` with the bearer scheme. Verify: the card validates against the A2A 1.0 schema and is 404 when disabled
- [x] 1.3 Implement `SendMessage`, `SendStreamingMessage`, `GetTask`, `CancelTask` (A2A 1.0 names, formerly `message/send`, `message/stream`, `tasks/get`, `tasks/cancel`) over the N11 loop and direct data-part calls per design.md "Binding to the implemented loop" (loop change L4, shared L2 and L3), with input-required, auth-required, completion after out-of-band approval and no peer approval. Verify: unit tests show an "approve" message leaves the confirmation pending and an out-of-scope request becomes auth-required
- [x] 1.4 Add test/cli tests with a Go A2A peer: delegated read; delegated `pin_add` exists only after the test resolver approves (G9-2); deny leaves the node unchanged; the G9-4 cross-path token matrix; and the disabled state (without `--a2a`, or without a backend, the agent card and `/a2a` are 404). Verify: `make build && go test ./test/cli/... -run TestMCPA2A -v` passes

## 2. Round 7 reconciliation (architect decisions in design.md, "Recorded behavior (Round 7)")

- [x] 2.1 Loop change L5 and the lost-context rule: a text message for a context that already ran a text task and whose loop session is gone gets `-32001` `CONTEXT_NOT_FOUND`, runs nothing, and drops the A2A context. Verify: a `mcp/server` unit test evicts the loop session (fake clock past the idle TTL, or the 64-session cap filled by other sessions), sends a text message on the old `contextId`, asserts the error, that the fake model received no request, and that `GetTask` for the context's earlier task is then `-32001`
- [x] 2.2 Replay the corrected contract fixture `fixtures/a2a/stream-read.json`: the first event is a Task in `TASK_STATE_SUBMITTED`, no later event is a Task, and the last event's `statusUpdate.status.state` equals the fixture's last `statusUpdate`. Verify: `go test ./mcp/server -run 'TestA2AFixturesReplay/stream-read' -count=1` passes
- [x] 2.3 Pin the destructive rule: with only the `cli` resolver, a destructive data-part call from an `a2a` principal ends `TASK_STATE_REJECTED` with `confirmation_unavailable` and opens no confirmation. Verify: a `mcp/server` unit test, plus the existing presence-resolver test showing `requires_native_prompt: true` in the task metadata
