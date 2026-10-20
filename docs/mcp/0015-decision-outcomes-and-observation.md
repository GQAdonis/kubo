# ADR-0015: Separate approval, operation outcomes and observation

## Status

Accepted for implementation, 2026-10-20, under operator decisions UI-05/06/07/11/12/13 and the adopted `agentic-ui-implementation` contract revision `3`, digest `648adf5c5bfe4b3bb17d31517a3b22737182d0f85a20236acaa6a4bb08cf846a`. This is D/1.1's contract tranche; it supplies no running endpoint or acceptance receipt. ADR-0003/0010/0013 and F4/G10/E6 remain authoritative.

## Date

2026-10-20

## Context

`Broker.claim` changes an approved record to legacy `executed` before `registry.go` calls the tool implementation. It establishes exactly-once consumption, not successful completion. The old staged `executed -> committed` mapping would falsely report Done after failed or ambiguous operations. The operator added the eleventh presentation, Failed after approval, and required honest uncertainty.

The inbox also needs current-identity observations across sessions, while native-required decisions and paired local-app requests retain their existing authority domains. UAR's `cf_uar_...` IDs represent its backend approvals, not this node's broker confirmations. A single unqualified decision state or a browser-reported resolver would confuse observation with permission.

The uncomfortable limit is evidence: successful approval, stream completion and textual tool output cannot prove that a write succeeded. Some UAR operation outcomes cannot currently be established. Those cards must stay unreported/uncertain even when a more satisfying completion message would be easy to fabricate.

## Decision

### Additive contract and immutable review binding

Amend the actual [confirmation schema](../../mcp/contract/confirmation/confirmation.schema.json), retaining all existing required fields, broker state values, token/request-state v1 claims and acceptance tables. Add optional effect digest, safe configured destination, resolving mechanism, separate surface attribution and real execution data. The original legacy pending fixture remains unchanged. Missing optional metadata never skips existing owner/session/tool/argument/expiry/resolver/single-use checks.

`effect_digest` binds `canonicalJSON({arguments, tool})` using the existing canonicalizer and validated supplied arguments, excluding only top-level `confirm_token`. Do not normalize omitted defaults or change legacy token hashing bytes. `ReviewedEffect` is an optional resolver-transport echo, not a new tool argument or consent. Any supplied mismatch is `confirmation_digest_mismatch`, non-retryable. A changed operation receives a fresh ID/review; an approved record's immutable arguments never change. Legacy callers remain gated through retained authoritative binding even without a new echo.

The normative [contract semantics](../../mcp/contract/confirmation/README.md) name exactly what is bound and what is outside the envelope. D/1.2–1.3 maps this through the existing resolver adapters; it must not invent a public generic confirmation resolver. No handle changes or early UI generation occur in this task.

### Approval and execution are separate

Keep the old `pending -> approved|denied|expired`, `approved -> executed` lifecycle. A single winning invocation owns additive `ExecutionRecord`: `not_started -> running -> succeeded|failed|unknown`. Observation/serialization errors cannot erase an already recorded operation success. Token-grace cleanup cannot make retained history replayable.

Only a real successful result produces `SuccessfulReceipt`, bound to the decision/tool/effect digest and actual completion time/result references. Background job acceptance says job accepted. Known unsuccessful outcomes carry bounded safe error detail and map to Failed after approval without claiming zero effects. Ambiguous transport/deadline/partial outcomes stay unknown. Neither uncertainty nor rereading a receipt retries a write.

Use one eleven-state shared presentation enum and precedence: successful matching receipt -> committed; definite failure -> failed_after_approval; unknown -> uncertain. The source contract's bare executed never means Done. Preserve resolving attribution alongside outcomes; answered-elsewhere is not a stronger execution verdict. Unknown post-restart pending cards require a new user request, not old handle replay. Default/off-switch work remains assigned to D/4.2; disabling observation never disables the gate.

### Identity-safe observation and bounded capabilities

`DecisionObservation` is the reusable safe item shape for I/1.1's later list/item/feed contract. Store ownership privately by the full BindingKey, including OIDC subject, and filter across only that identity's sessions. Expose safe configured authority names, bounded prepared summaries/targets, actual egress facts and semantic destinations; never raw binding keys, JWT subjects, tokens, grants, sensitive args or backend bodies.

Distinguish `source/source_kind` pairs: `kubo/node_confirmation` versus `uar/backend_approval`, with UAR tool labels and IDs kept distinct. Never resolve a synthetic UAR ID through the node broker or merge by tool/target. UAR text TOOL_CALL_RESULT, RUN_FINISHED and approval acknowledgement do not supply a successful node receipt. A validated correlated operation report is required before reporting a backend success.

`actions` contains server-derived hints with existing authenticated rechecks. Capability-scoped continuation identifies only the owner's exact open thread/interrupt at existing `POST /agui`; it contains no token or arguments. Other items identify the known authorized surface. Receiving a snapshot/feed event cannot grant approval authority. Native-required cards remain deny-only in Desktop/Web UI, and paired local-app requests stay separate from current-identity Web observations.

### Resolver mechanism is not surface attribution

`resolved_by` uses the existing Resolver enum; legacy `resolver` agrees when both appear. `resolved_surface` describes chat/inbox/MCP host/Desktop/native/CLI plus an optional opaque instance, so two chat panes can differ. Surface claims are not principals. Trusted adapters determine the actual mechanism and stamp their native/Desktop kind; browser input cannot claim it. Do not invent resolver attribution for automatic expiry. D/1.5 clarification: retain genuine prior human approval attribution as historical provenance when an unclaimed approval later expires or is automatically withdrawn; current terminal state/outcome controls presentation and disabled actions. Unanswered records keep attribution absent. The old approver is not presented as the actor of an automatic terminal transition.

Only macOS LocalAuthentication/Linux polkit count as the server's native-presence factor. Desktop Touch ID is not that factor. Preserve main-process 1.5-second focused arming, rearming, blur, IPC-sender and expiry checks. No new approval authority is introduced by the shared model.

## Alternatives considered

- Interpret executed as success: contradicts the observed pre-invocation claim and hides failures.
- Replace the existing broker states or token formats: unnecessary compatibility risk; additive lifecycle preserves clients and one-use semantics.
- Make the digest mandatory for all clients immediately: breaks existing confirmation callers without improving their retained immutable server binding.
- Treat a resolving-surface string or read access as authorization: untrusted display attribution cannot supply owner/resolver/native presence.
- Infer UAR success from a completed stream: no correlated operation proof and no basis for a node receipt.

## Consequences and implementation handoff

### D/1.4 supporting amendment: ordinary MCP result delivery

An external MCP App host may have no HTTP inbox credentials. The existing standard tool-result channel therefore carries optional `CallToolResult._meta["ipfs-mcp"] = {confirmation_id, effect_digest, execution}` for the authorized current winning node invocation. `ToolOutcomeMetadata` and `ToolOutcomeResultMeta` are defined in the confirmation schema. The only receipt remains `execution.receipt`; it must match ID, tool, digest and completion time. The host forwards the ordinary CallToolResult to the app, with no private handshake or credential acquisition. Preserve tool output schemas, content/structuredContent/isError and all other metadata. No raw arguments/results, identity bindings, tokens or grants appear in this projection.

Actual operation success survives a later delivery error; the interface may show both independently. Missing/malformed metadata cannot create Done or automatic retry. D/3.4 consumes the same shared model through the existing SDK. Pinned go-sdk v1.8.0 CallToolResult.Meta is the supported server extension point; the standard MCP Apps tool-result notification carries it.

Timestamps are nondecreasing: `started_at <= completed_at`, including equal observed instants. Clamp a backwards wall-clock completion to start, never add a fictional nanosecond. Receipt completion equals the stored execution completion. The schema cannot compare arbitrary field values; the semantic model enforces this relation. The additional examples and `confirmation/fixtures/tool-outcome-cases.json` are authored evidence only; no QA or runtime acceptance ran.

The contract is larger because approval, execution and display truth are distinct. Server code must propagate the winning claim to the shared invocation boundary across token, tokenless and elicitation paths. Only actual outcomes finalize the record; callbacks/notifications must not hold broker locks or execute operations.

SU D/2.1 extends generation to the new definition closure and faithfully handles source/native/execution conditions. The current converter supports only two special allOf patterns; silently dropping the new constraints is forbidden. Contract fixtures include positives, structural negatives and semantic cases, with legacy fixtures retained. No tests, generation, builds or independent review run during this tranche.

Next architect contracts A/1.4, I/1.1 and C/1.1 may reference these definitions for App negotiation, identity-scoped reads/feed and context/run facts. I/1.1 still owns routes, retention/cursors/epoch and idle subscription bounds; this ADR does not implement them. The final packet evaluates UI-103/104/111 after all production tasks. Existing RPC/gateway/default CID/wire behavior, off switches and egress rules stay intact.

## References

- [Operator decisions](../../.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md), [backend source research](../../.kbd-orchestrator/phases/agentic-ui-implementation/research-backend.md).
- [Canonical OpenSpec decision change](../../openspec/changes/praxis-decision-model-v2/design.md).
- [Confirmation/auth ADR](0003-confirmation-and-auth.md), [UAR authority ADR](0010-uar-agent-behind-agui.md), [identity/session ADR](0013-console-session-auth.md).
