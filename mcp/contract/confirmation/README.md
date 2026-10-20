# Decision and confirmation contract

Canonical source: `confirmation.schema.json` (JSON Schema 2020-12). ADR-0015 defines the architecture and compatibility amendment. This document specifies producer/consumer semantics that structural schema validation cannot establish. It is part of D/1.1; schema generation belongs to SU D/2.1 and server implementation to BE D/1.2–1.5. No generated UI file is authored here.

## Existing compatibility boundary

`PendingConfirmation`, `ConfirmationRequired`, confirmation token/request-state claims, signatures and acceptance tables retain their existing required fields and meanings. The incumbent `examples/PendingConfirmation/ui-destructive.json` intentionally contains none of the optional additions. `principal` remains a configured display name, never an internal `BindingKey` or raw OIDC subject. The legacy `resolver` identifies a mechanism; if the new `resolved_by` is also present, both must agree.

Optional additions on pending records are `effect_digest`, `destination`, `resolved_by`, `resolved_surface` and `execution`. Required-error payloads can add the first four. Absence means unreported, never approved, successful or exempt from confirmation. An older `state: executed` without an execution result cannot become Done. The token's `args_sha256` remains an args-only binding; do not rename it, substitute the new envelope hash or change its canonicalization/signing bytes.

The server retains immutable tool/validated arguments, session, full identity binding, resolver constraints, expiry and the single-use claim. Metadata fields, card display state and reads grant no rights. Schema validity alone cannot prove authorization. Browser code cannot choose a resolver mechanism by sending `resolved_by`, and a supplied surface instance is only attribution.

## Reviewed operation and digest

Compute `effect_digest` as `sha256:` plus lowercase hexadecimal SHA-256 of the UTF-8 bytes returned by the **same existing** server `canonicalJSON` implementation for this envelope:

```json
{"arguments":{"example":"validated arguments actually supplied"},"tool":"canonical_tool_name"}
```

Decode/validate the same arguments used to prepare the broker record; remove only the top-level `confirm_token`. Do not insert schema defaults that the caller omitted, rewrite explicit defaults, fetch content, resolve a path differently, change numeric handling or introduce a second canonicalizer. All other valid tool inputs, including destination-selecting inputs, are bound. Session/auth/AG-UI resume fields, requestState, inputResponses and surface/review metadata are transport fields outside `arguments`, not extra tool arguments to strip opportunistically. If material implicit execution facts can change the effect, freeze them with the prepared operation or require a fresh confirmation; never silently substitute them after review.

`ReviewedEffect` specifies an optional transport-level echo of the reviewed digest. D/1.2–1.3 carries it through the existing resolver adapters; it is not a new MCP tool argument, generic resolve route, token format or permission. An upgraded renderer echoes exactly what it displayed; the server compares any supplied echo and its retained operation before approval/claim. Bind an absent echo through all existing immutable checks for legacy clients. Absence must never skip a binding, ownership, resolver, expiry or single-use check. Missing server digest is rendered as unreported; never compute a substitute from displayed/truncated text.

Mismatch returns `confirmation_digest_mismatch` with `retryable: false` and no execution. Authenticate/authorize the caller before disclosing a decision mismatch. It invalidates the stale presentation, not the immutable broker record's tool/arguments. Changed arguments create a new ID and a fresh human review; do not overwrite arguments/digest behind an existing approved ID. A schema-accepted legacy payload remains subject to the same gate.

Error precedence preserves the legacy acceptance tables: token/requestState signature, use, session, identity, tool or argument-binding failures remain `confirmation_invalid` (including InputRetry `state_check: other_call`). Only after authorized ownership and those bindings pass does an explicitly supplied ReviewedEffect echo mismatch return `confirmation_digest_mismatch`. An additive internal retained-effect consistency failure also fails closed, even when no echo was supplied; it never weakens or replaces the old binding checks. Both failure paths perform no tool call. Arbitrary changed arguments do not newly promise the distinct digest error.

### Reviewed-effect wire placement amendment (C/1.1, 2026-10-20)

Pinned go-sdk v1.8.0 `mcp/protocol.go` declares supported `_meta` on ElicitParams, ElicitResult, CallToolParams and CallToolParamsRaw (lines2122,2179,219,241); `mcp/shared.go` defines Meta as map[string]any. These actual extension points, not new SDK constructor options, define the following placements:

| Existing transport | Display facts / optional reviewed echo |
|---|---|
| MCP elicitation request | `inputRequests.confirm.params._meta["ipfs-mcp"] = {effect_digest, destination?}`; ordinary legacy elicitation uses the same ElicitParams metadata. Omit unknown destination. |
| MCP elicitation answer | `inputResponses.confirm._meta["ipfs-mcp"].reviewed_effect = {effect_digest}` on ElicitResult; the legacy ElicitResult has the same metadata placement. |
| MCP tools/call retry | Params `_meta["ipfs-mcp"].reviewed_effect = {effect_digest}` for optional echo on the ordinary repeat or InputRetry; never put it in tool arguments. |
| AG-UI owned interruption | `resume[i].reviewed_effect = {effect_digest}` outside the existing `payload`; original payload remains exactly approved:boolean or wait:true. Outbound display facts live in Interrupt.metadata["ipfs-mcp"]. |
| Existing private native channel | Optional `reviewed_effect = {effect_digest}` beside id/approve in authenticated POST /v1/resolve, covered by the existing signed request body; no new public endpoint or pairing protocol. |

Keep elicitation requestedSchema/content's confirm boolean unchanged. No bare top-level ElicitResult.reviewed_effect, unknown content field or private MCP App handshake is required. Preserve unrelated `_meta` keys. If both answer-level and call-level echoes exist, validate both and require identical digests; do not silently choose one. Malformed explicitly supplied echo is refused without consuming approval, never treated as absent; valid-but-different echo follows the distinct mismatch/error precedence above. Older clients omit echoes safely through the retained original binding/gate. On deny/cancel/wait, supplied malformed/mismatched metadata must not grant or consume approval; a safe denial may retain its existing authority without synthesizing success.

ReviewDisplayMetadata, ReviewResponseMetadata and NativeResolveRequest are canonical schema definitions. Native UI/channel wiring belongs to its assigned adapter tasks; this amendment grants no Desktop factor or browser resolver authority. A2A's existing metadata["ipfs-mcp"] may show digest/destination but does not create a message-based approval path. C's run context/surface metadata is independent of this operation review echo.

UAR synthetic backend approvals currently have no trusted retained operation digest. Their UI must omit ReviewedEffect, never synthesize a digest/receipt from backend text or reuse a node decision's echo. A malformed explicit echo is `invalid_argument` at decoding. A well-formed explicit echo is refused with nonretryable `confirmation_digest_mismatch` after full owner/thread/open-interrupt ID and ordinary payload checks; do not forward the approval to UAR, enter the node broker, or consume the continuation. An absent echo retains the existing UAR approval path. This does not fabricate a trusted digest for backend operations.

## Facts and read-side projection

`DecisionObservation` supplies the shared shape for later list/item/feed schemas. It contains no raw principal, ownership key, JWT subject, session secret, confirmation token, requestState, backend grant or argument payload. `summary` provides prepared tool/class/object text and bounded targets as scope. `authority` gives only safe configured display facts. `destination` identifies the affected configured node/pinning service/IPNS name, never a model-chosen URL or a credentialed endpoint. Unknown names/destinations/egress are omitted or explicitly unreported.

Filter privately by the full `Principal.BindingKey()` including OIDC subject, across that identity's sessions; do not expose that key. Current-identity node observations and paired local-app native requests remain separate domains. Feed notifications will carry only minimal ID/state/revision facts; reading a decision does not authorize a resolver.

Optional `activity_id` uses ActivityId: a bounded opaque server-assigned correlation string, fresh per accepted user activity and retained across its authorized resumes. I/1.4 defines the matching optional run-facts value and interrupt metadata locations in the decisions README. It is never a client authority/lookup token or a durable-history promise. Consumers/generators reuse ActivityId rather than maintaining a separate ID rule.

`source: kubo` with `source_kind: node_confirmation` is a node broker record. `source: uar` with `source_kind: backend_approval` uses a synthetic `cf_uar_...` ID and `uar:` tool label; it is UAR's own approval only. It must never enter `Broker.Resolve` or imply that this node's later write gate was approved. Do not merge sources by matching tool/target/untrusted backend call ID. Without trusted correlated operation evidence, UAR reports no successful node receipt.

`actions` contains current server-derived capability hints; the destination answering surface is separate from the effect's `destination`. `continuation` may identify only the current identity's exact open interruption through existing `POST /agui`, using `thread_id`/`interrupt_id` as locators. No raw args or authorization handles travel in it. I/1.1 defines the read routes and authenticated resume mapping; the resolver rechecks ownership, open interrupt, effect, expiry and allowed mechanism. Native-required decisions always have `approve: false`; their deny/wait path stays as already authorized. Other records name their known authorized surface rather than offering a new approval button. Missing capability facts do not default to allow.

## Resolver versus surface

`resolved_by` is the existing `Resolver` enum: elicitation, agui-interrupt, desktop, macos-localauth, linux-polkit or cli. `resolved_surface` is optional `{kind, instance?}` display attribution. Distinguish two same-kind panes using a bounded opaque instance ID, not a JWT/session credential. Trusted adapters stamp Desktop/native/elicitation mechanisms; browser kinds never claim them. Expiry or automatic withdrawal invents no human resolver or surface. If no human answered, attribution remains absent. If an actual human approved before the unclaimed record later expires or is automatically withdrawn, retain that genuine attribution as historical approval provenance; do not clear it or relabel it as the actor of the automatic terminal transition. Legacy `resolver` remains unchanged and agrees with retained `resolved_by`. A claimed operation preserves genuine attribution alongside its actual outcome.

Current approval state and real execution outcome control presentation and actions. A prior approving surface cannot turn expired/withdrawn into still-approved or resolved_elsewhere, and it cannot leave approval actions enabled. Where useful, label retained attribution “Previously approved by” rather than “Expired by” or implying that the prior approver performed the automatic withdrawal.

Only macos-localauth/linux-polkit supply the server's required native-presence factor. Desktop Touch ID does not make the paired Desktop mechanism a native-presence resolver. Main retains its 1.5-second focused arm, content/digest rearming, blur disarming, sender and expiry checks. The Web UI cannot gain approval rights from observing the paired local-app queue.

## Execution and receipt

Approval state is independent of operation lifecycle. `executed` is the atomic consumed claim before invocation. Exactly one winning invocation owns `ExecutionRecord`; callbacks, notification or rendering cannot run the handler. `not_started -> running -> succeeded | failed | unknown` describes actual execution. A later reliable correlated report may refine unknown without ever invoking again. Keep lifecycle/history apart from token-grace cleanup so observation does not enable replay.

Only `succeeded` contains `SuccessfulReceipt`, bound semantically to the same decision ID, tool and effect digest. Completion time equals the execution record's completion time. When both times exist, `started_at <= completed_at`: equal observed instants are valid. Clamp a backwards wall-clock completion to the observed start; never fabricate +1ns to force a strict inequality. References come from real returned identifiers, not guesses from input or arbitrary backend prose. Empty `result_refs` is appropriate when the operation returned no identifier. Async work records `effect: job_accepted`; it must not imply all indexing/work finished. Store the successful fact before later delivery/serialization steps, which cannot undo an operation that succeeded.

### Current-call result metadata amendment (D/1.4, 2026-10-20)

The exact optional transport is `CallToolResult._meta["ipfs-mcp"] = {confirmation_id, effect_digest, execution}`, definition `ToolOutcomeMetadata`. `ToolOutcomeResultMeta` describes the outer metadata map; a missing namespace remains valid. `execution` is the existing ExecutionRecord and its succeeded branch contains the only SuccessfulReceipt at `execution.receipt`. Do not duplicate a top-level receipt. This node-operation envelope does not carry synthetic UAR approvals or claim a backend operation was executed by this node.

Pinned go-sdk v1.8.0 `mcp/protocol.go:266–269` embeds Meta with `json:"_meta,omitempty"` on CallToolResult. The standard [MCP Apps tool-result notification](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx) carries CallToolResult, including its metadata, to the view; existing `ontoolresult` / tool-call response delivery suffices. An external host needs no HTTP inbox credential, private endpoint, nonce or custom handshake. An app must not obtain credentials or guess resolver rights from this observation. Ordinary host SDK compatibility and ui:// fallback remain unchanged.

The server attaches a detached authoritative snapshot to the authorized current winning invocation's result after actual outcome recording, including a later result-delivery/serialization error if a result can still be returned. Preserve existing `content`, `structuredContent`, `isError`, output schemas and all unrelated metadata, including `ai.ipfs/tool` and `ai.ipfs/untrusted-content`. These fields are not inserted into tool output JSON or arguments. Do not attach another owner's record, metadata guessed from tool text, or a stale record to an invalid/replayed/losing claim. Legacy results and read tools may omit the namespace entirely.

Consumers validate the namespace, bind its ID/digest to the shown decision when present, and bind a success receipt's ID/tool/digest/completion time to this envelope and the known current tool call. A received tool marker may identify the call but cannot override a contradiction. Only a valid matching success receipt supports Done. Definite failure maps to failed_after_approval; unknown, malformed, conflicting or missing execution evidence never synthesizes success or permits a write retry. Absence of metadata does not invalidate ordinary legacy tool output; its decision outcome remains unreported/uncertain where no other evidence exists. A snapshot is not a subscription, so later refinements require an already authorized read channel.

If a handler succeeded but output delivery subsequently failed, preserve the successful receipt. The UI shows that operation outcome separately from the delivery error; `isError` alone cannot erase established success or prove that an operation did not run. Copy/View receipt uses only the bounded safe receipt; never include raw arguments/results, BindingKey, tokens, requestState, credentials, prompt text or host filesystem data in this namespace. Result references retain the existing safe actual-identifier policy.

`failed` means a definite unsuccessful operation result after approval; it has bounded public error detail and never asserts zero partial effects. Ambiguous RPC disconnect, deadline/cancellation after dispatch, unproven partial effects or absent evidence are `unknown`, not a definite failure or denial. Denied/expired records cannot also carry an execution record. Neither read-side lookup nor retryable error copy may automatically retry a write. Repeated resume/token/elicitation attempts remain subject to the existing one-use claim.

UAR success requires a correlated, validated backend operation report. Text TOOL_CALL_RESULT, RUN_FINISHED success and approval acknowledgement alone are insufficient. `provenance: validated_backend_operation` must identify real verified evidence internally; it is not a client assertion. If backend facts are missing, omit them honestly.

## Eleven presentations and precedence

The presentation is derived once by the shared model; it is not another server approval state. `DecisionPresentation` supplies the eleven canonical values.

| Presentation | Label | Evidence/control rule |
|---|---|---|
| `pending` | Awaiting a person | Pending and currently authorized; Deny focused; web Approve only after arm. |
| `waiting_system_prompt` | Waiting for system prompt | Native-required pending record; Deny only. |
| `approved` | Approved, not yet run | Approved without final outcome; no receipt. While dispatch/running is reported, show that supporting fact without claiming the operation has not started. |
| `committed` | Done | Successful matching receipt only; View receipt. For async success, supporting text says job accepted. |
| `denied` | Denied. Nothing was changed. | Gate denied before invocation; no execution record. |
| `expired` | Expired. Nothing was changed. | Gate expired before invocation; Ask again creates a fresh request. |
| `invalidated` | Changed since you saw it | Stale reviewed digest/identity-bound request; Review again prepares a fresh immutable proposal, never edits approved arguments. |
| `resolved_elsewhere` | Answered on another screen | Known actual different surface instance and no higher-priority terminal outcome; preserve attribution. |
| `uncertain` | Outcome unknown, checking | Explicit unknown or outcome timeout without reliable result; Check now reads only. Legacy executed without a result is uncertain immediately. |
| `needs_resubmit` | Can't resume. Ask again to create a new request. | Former pending request missing after epoch/reset/eviction; Ask again makes a fresh user request with no old handle replay. |
| `failed_after_approval` | Failed after approval | Definite unsuccessful operation result; safe detail, no zero-effect claim and no automatic retry. |

Actual execution outcome takes precedence: successful receipt -> committed; definite failure -> failed_after_approval; unknown -> uncertain. Never hide one behind resolved_elsewhere, denied or expired. Keep resolver/surface attribution visible alongside that outcome. Bare approved/executed is not success. A missing old completed receipt means unavailable history, not proof of a failed operation. The configured outcome timeout controls uncertainty display only; it must not mutate broker truth or create retry authority. D/4.2 owns default/off-switch documentation.

## Consumer generation handoff (SU D/2.1)

Extend the existing `CONFIRMATION_DEFS` closure in `mcp/ui/scripts/lib/generate-contract.mjs` to include every referenced addition: EffectDigest, Destination, ResolvingSurface, DecisionId, ActivityId, DecisionApprovalState, DecisionPresentation, SafeExecutionError, ReceiptReference, SuccessfulReceipt, ExecutionRecord, DecisionAuthority, DecisionEgress, DecisionContinuation, DecisionActions, DecisionSummary, DecisionObservation, ReviewedEffect, ReviewDisplayMetadata, ReviewResponseMetadata, NativeResolveRequest, ToolOutcomeMetadata and ToolOutcomeResultMeta. Preserve existing exports. Generate into the existing `mcp/ui/src/contract/generated/confirmation.ts`; never maintain a second handwritten schema/state set. D/3.4 consumes current-call outcome metadata through the ordinary host SDK.

The current schema-to-Zod converter handles only two specialized allOf patterns. The new PendingConfirmation execution, DecisionActions continuation and DecisionObservation source/native/execution/provenance conditions require explicit faithful conversion or equivalent generated refinements; do not ignore unsupported keywords or weaken the JSON Schema to fit the converter. ExecutionRecord uses disjoint oneOf branches with complete objects. Add semantic model checks for matching receipt ID/tool/digest/time and valid source/continuation capability; JSON Schema cannot compare arbitrary field values. Generator/type/test authorship belongs to SU; execution waits for the final phase boundary.

Positive examples remain under `examples/<Definition>/` for the existing contract example walker. `fixtures/decision-cases.json` lists structural negatives and semantic mapping/gate cases separately so negative fixtures are not mistaken for positive examples. Existing legacy fixtures stay unchanged. All data is synthetic fixture data. Fixtures are authored expectations, not validation or real-operation receipts; final UI-103/104/111 coverage supplies actual gate/outcome evidence.
