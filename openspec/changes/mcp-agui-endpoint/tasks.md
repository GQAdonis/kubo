# Tasks: mcp-agui-endpoint

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-agui-endpoint`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-llm-proxy-loop`, `native-confirmation-prompt`. Scheduled in Round 7.

## 1. AG-UI endpoint

- [x] 1.1 Verify the AG-UI protocol (RunAgentInput, events, interrupts, resume) from primary sources and record URLs and dates in `mcp/server/agui/SOURCES.md`. Verify: every emitted event cites a source and validates against the published `@ag-ui/core` schemas (G9-1)
- [x] 1.2 Implement `POST /agui` with SSE over the N11 loop per design.md "Binding to the implemented loop" (thread mapping, error table, event table, loop change L1), behind `--agui`, mounted only with a configured backend, path-scoped auth and the origin allowlist. Verify: unit tests cover 404 when disabled, 401, 403 and event order against contract fixtures
- [x] 1.3 Implement interrupt and resume per the spec and design.md (one interrupt per run, resume table, `{wait: true}` observation, loop changes L2 and L3): `ui` principal approval counts, destructive calls also need the native prompt, other principals are resolved by the native prompt. Verify: unit tests show a resume from a non-`ui` principal does not approve, a destructive call waits for the prompt, a resume naming another thread's interrupt resolves nothing, and the two `two-calls-*` fixtures replay
- [x] 1.4 Add test/cli tests against a harness daemon and the Ollama backend on free ports: event schema validation, unconfirmed write leaves the node unchanged, the G9-4 cross-path token matrix, and the disabled state (without `--agui`, or without a backend, `/agui` is 404 and no model call is made). Verify: `make build && go test ./test/cli/... -run TestMCPAGUI -v` passes

## 2. Round 7 contract follow-ups (architect decisions in design.md, "Errors before the stream" and "Recorded behavior")

- [x] 2.1 (mcp-server-engineer) Map pre-stream errors to the shared codes added in Round 7: `ErrSessionBusy` -> 409 `session_busy` retryable; `ErrInterruptOpen`, `ErrSessionFull` -> 409 `failed_precondition`; `ErrTooManySessions` -> 503 `resource_exhausted` retryable with `Retry-After`; `ErrMessageTooLarge` -> 413 `size_limit_exceeded`; add the matching `Code*` constants. Verify: unit tests assert `error.code` for each row of the table
- [x] 2.2 (mcp-server-engineer) Add replay scenarios for the new fixtures `run-error-step-limit.json` (`LoopLimits{MaxSteps: 1}`), `run-error-backend-failure.json` (no scripted model step) and `two-calls-native-wait-approved.json` (after the two `two-calls-*` fixtures, approving in the presence resolver after `RUN_STARTED`). Verify: `go test ./mcp/server/ -run TestAGUIFixturesReplay -count=1` passes with no "has no replay scenario" failure
- [x] 2.3 (ui-ux-engineer) Add the same three fixtures to the `mcp/chat` fixture replay, render `RUN_ERROR` by `code` (limit reasons and `backend_error`) and treat 409 `session_busy` as retryable and `failed_precondition` as "answer the open interrupt first". Verify: the chat replay test passes for every file in `mcp/contract/fixtures/agui/`
