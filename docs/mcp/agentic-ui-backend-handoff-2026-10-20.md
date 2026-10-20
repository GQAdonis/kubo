# Agentic UI backend handoff — 2026-10-20

The operator requested an immediate backend handoff to their Claude Code session. Codex backend authorship has stopped. This is an implementation checkpoint, not acceptance of the full agentic UI phase. Do not treat authored task checkboxes as test evidence.

## Preserved work

- Reviewed-effect binding, execution outcome records, and success receipts, including explicit failure and unknown-outcome semantics.
- Identity-scoped decision history, list/detail reads, bounded authenticated event subscriptions, and existing-authority continuation projections. Native and UAR authority remain distinct.
- Reference-only run context, optional reported run facts, configuration switches, and CLI wiring.
- A2UI catalog negotiation and canonical MCP App view/component mapping.
- Contract schemas, examples, fixtures, unit tests, and the authored CLI boundary lease and disabled-feature tests.

The canonical contracts are in `mcp/contract/confirmation/`, `mcp/contract/decisions/`, `mcp/contract/agentic/run-context.md`, and `mcp/contract/agentic/ui-negotiation.md`. ADRs 0015–0018 and the four corresponding OpenSpec changes record the design. The generated catalog and existing generated MCP App HTML/manifest snapshot accompany this commit because the Go server embeds and validates those artifacts. Their newer UI source work remains in the working tree for the separate UI delivery; this checkpoint does not assert frontend source-generation parity.

## Unfinished work

- Decision task 3.5 remains open. The proposed dedicated compiled `TestDecisionBoundaryLease` driver was not completed. The existing `test/cli/mcp_boundary_lease_test.go` is a different, authored but unexercised provider-backed lease.
- Decision tasks 4.1 and 4.2 remain open, including final documentation/default/off-switch/changelog integration.
- The shell, inbox, context inspector, origin/CID presentation, and Desktop attention work remain incomplete. The broader phase has not reached acceptance testing or Reflect.
- Real built-in/UAR flows, native confirmation hardware, browser/accessibility/visual checks, and independent security/quality review remain outstanding. No production deployment or release is authorized by this checkpoint.

## Validation

- `go fmt ./...`: passed without formatting changes.
- `make build`: passed.
- `gmake -O test_go_lint`: failed. New-work findings: unused `(*cliClient).resolve` (`mcp/server/confirm/cli.go:241`), unused `projectDecision` (`mcp/server/decision_reads.go:157`), and ineffectual assignment (`mcp/server/decision_history_test.go:179`). It also reports ST1005 in the unchanged `fuse/node/mount_darwin.go:29`. These are handed off without another backend edit cycle.
- `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo -timeout 90s`: failed in `mcp/server`. The confirmation, embeddings, contract, and CLI-command packages passed. Six top-level server tests failed:
  - `TestDecisionOIDCExplicitRouteOnlyAndSamePrincipalSubjectsStayPrivate`: expected 401, received 403.
  - `TestElicitationDeclineCancelAndFalseLeaveThePin`, `TestDeniedConfirmationIsFinal`, and `TestExpiredConfirmationIsDenial`: the Outcome schema rejects new effect/resolver metadata.
  - `TestUARRelaysOnlyAllowlistedEventsAndFields` and `TestUARDropsEachUnlistedEventType`: expected event sequences omit the newly emitted CUSTOM event. Review provenance and semantics before changing assertions; do not broadly allow backend CUSTOM events.
- The two new CLI disabled-feature tests (`TestMCPRunFactsConfigOffPreservesOrdinaryRuns` and `TestMCPContextReferencesConfigOffPreservesOrdinaryRuns`) passed against the `make build` binary with an isolated temporary `IPFS_PATH`.
- These checks ran in the shared working tree, including the in-progress frontend files. They do not establish a clean-checkout frontend build or full phase acceptance.

## Ownership after handoff

Claude Code owns subsequent backend implementation and fixes. Codex must not resume writes to `mcp/server/`, backend CLI wiring, or backend integration tests without a new explicit user instruction. Coordinate any shared contract change with that backend owner. The interrupted Codex workers must not resume their previous backend assignments.

The shared checkout branch is retained. This focused checkpoint excludes the remaining frontend source and orchestration-memory edits; those files are preserved locally. No phase or incomplete task is closed by this handoff.
