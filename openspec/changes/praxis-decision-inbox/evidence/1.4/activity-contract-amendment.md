# I/1.4 supporting activity contract amendment

Authoring only, 2026-10-20. No tests, builds, review, server/UI writes or generation ran. Lead owns the already-open canonical boundary.

Shared ActivityId in confirmation.schema.json preserves existing string bounds of 1–128 without a mandatory prefix. DecisionObservation.activity_id and RunFactsEvent.activity_id now reference it. Optional Interrupt.metadata["ipfs-mcp"].activity_id uses that same definition and the AG-UI schema exports its alias. No standard RUN_STARTED field or idle SSE payload was changed.

Exact semantics are in decisions/README.md, run-context.md, ADR0017 and inbox design: fresh server ID per accepted user activity; same ID across authorized resumes; new accepted message gets a fresh ID. Interrupt association works when run facts are off, with no synthetic facts event. The ID grants no authority, lookup/approval right or durable-history guarantee and cannot expose private identity/arguments or be supplied by the client/backend.

Small authored fixtures: nine structural and eleven semantic cases in decisions/fixtures/activity-cases.json; initial/resumed facts examples; DecisionObservation/activity-pending.json; and write-interrupt-activity-facts-off.json. Historical fixtures remain unchanged. BE received exact constraints and field placement before adapter edits. Replay normalization must validate the optional field before omitting only that named addition where an old fixture lacked it.

Generated consumers must reuse ActivityId. The confirmation README's generation closure includes it. All runtime and correlation evidence remains for the adopted final boundary.
