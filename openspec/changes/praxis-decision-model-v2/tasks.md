# Tasks: praxis-decision-model-v2

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

Plan reference: `.kbd-orchestrator/phases/agentic-ui-implementation/plan.md`, Task model assignments. Full keys are `agentic-ui-implementation/praxis-decision-model-v2/<id>`. Stable checkbox titles/IDs are retained for runtime identity; the operative amendments below replace obsolete assertions within historical display titles. No historical title authorizes a superseded architecture.

## Operative implementation amendments

Existing IDs: `1.1–1.3`, `2.1–2.3`, `3.1–3.5`, `4.1`.

- Architect owns amended **1.1** in `mcp/contract/confirmation/confirmation.schema.json`, generated-type specification and ADR; UI engineer owns generation in `mcp/ui`. Correct the obsolete `mcp/contract/schemas` path and server ownership claim.
- Server engineer owns **1.2–1.3** in `mcp/server/broker.go`, `gate.go`, `elicit.go`, `registry.go`, transport payload mappers and confirmation channel. Bind `effect_digest` to the exact normalized operation; carry reviewed digest through each supported resolver without making absent legacy fields an approval bypass.
- D/1.2 operative acceptance: preserve `confirmation_invalid` for legacy token/requestState signature/use/session/identity/tool/argument-binding failures, including `other_call`. Return distinct `confirmation_digest_mismatch` for an explicit reviewed-effect echo mismatch only after authorized valid legacy binding; retained-effect inconsistency always fails closed. No tool call occurs in either case. The historical title does not promise a new error for arbitrary changed arguments.
- Add **1.4**, server engineer: execution lifecycle and success receipt. Preserve the one-time claim used by `executed`; record actual handler start/outcome separately. Receipt appears only after confirmed success; known failure maps to “Failed after approval”; timeout/partial/transport ambiguity stays unknown. No replay or automatic write retry.
- Add **1.5**, architect/server engineer serial handoff: resolver mechanism versus resolving surface identity and precedence. Surface IDs are attribution, not authorization. Outcome facts survive an “answered elsewhere” presentation; two same-kind chat panes are distinguishable without exposing another identity.
- Amend **2.1–2.3, 3.1, 3.3, 3.5, 4.1** to eleven states. Preserve the original ten labels and add “Failed after approval.” Never infer “nothing changed” from a post-start failure.
- **2.2, 3.2** must define Ask again as a fresh proposal, Check now as a read-only lookup and Review again as a newly reviewed digest. No old token resend, hidden execution retry or fabricated restart success.
- **3.4** uses the same shared model in every MCP App view. Desktop consumption belongs to T below.
- D/1.4 supporting contract amendment supplies optional `CallToolResult._meta["ipfs-mcp"] = {confirmation_id, effect_digest, execution}` (ToolOutcomeMetadata), with the only receipt at execution.receipt, to the authorized current winning node call. Standard host delivery, unchanged tool output schemas and other metadata, no inbox credentials/private handshake or sensitive payloads. **3.4** validates this ordinary result channel before deriving real outcomes. Missing/malformed metadata cannot invent Done or retry authority. Actual timestamps allow started_at <= completed_at with equal instants and matching receipt time; never fabricate +1ns. See confirmation README and authored tool-outcome cases.
- Add **4.2**, architect: defaults/off switches and changelog Highlights/TOC disclosure for new outcome/decision behavior; preserve generated changelog sections.

## 1. Implementation tasks

- [x] 1.1 Add optional `effect_digest` (`sha256:` + 64 hex), `destination` (`kind`: `node` | `pinning_service` | `ipns_name`, `name`) and `resolved_by` (Resolver enum) to the pending-confirmation schema in `mcp/contract/schemas`, and regenerate `mcp/ui/src/contract/generated/confirmation.ts`

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/1.1`; lane `AR`; role(s) ipfs_agent_architect; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/agentic-react19-foundation/1.1`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.2 Server computes the effect digest over the canonical JSON of tool name and arguments, emits it with `destination`, and rejects a confirm whose digest no longer matches with a distinct digest-mismatch error

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/1.2`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/1.1`.

  Acceptance: final-boundary scenario(s) `UI-103`, `UI-104` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.3 Server sets `resolved_by` on every state change it reports

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/1.3`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/1.2`.

  Acceptance: final-boundary scenario(s) `UI-103`, `UI-104` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.4 Record actual execution lifecycle and successful outcome receipts after the single-use claim

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/1.4`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/1.3`.

  Acceptance: final-boundary scenario(s) `UI-103`, `UI-104`, `UI-111` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.5 Implement resolver versus resolving-surface attribution and terminal-outcome precedence

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/1.5`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/1.4`.

  Acceptance: final-boundary scenario(s) `UI-103`, `UI-104` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 2. Implementation tasks

- [x] 2.1 Extend `APPROVAL_PHASES`, `PHASE_WORD` and `phaseSentence` in `mcp/ui/src/approval/model.ts` to the ten states with the badge words from the spec table; add a `muted` tone for `resolved_elsewhere`

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/2.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/1.5`; `agentic-ui-implementation/agentic-react19-foundation/1.7`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 2.2 Map contract states to model states (`executed` → `committed`; `pending` + `requires_native_prompt` → `waiting_system_prompt`) and derive `invalidated`, `resolved_elsewhere`, `uncertain` (configurable outcome timeout, default 30s) and `needs_resubmit`

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/2.2`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/2.1`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 2.3 Add the card fields to `ApprovalModelSchema`: object, destination, scope, class in words, named authority, egress, short digest formatter

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/2.3`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/2.2`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 3. Implementation tasks

- [x] 3.1 Update `ApprovalCard`, `ApprovalPanel` and `ConfirmationDialog` to render the ten states, the card fields and the controls (Deny focused, Approve after 1s arm, Ask again, Review again, Check now, View receipt)

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/3.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/2.3`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 3.2 "Ask again" requests a new confirmation and never re-sends the old confirm token

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/3.2`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/3.1`.

  Acceptance: final-boundary scenario(s) `UI-103`, `UI-104` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 3.3 Regenerate `mcp/ui/catalog/ipfs-mcp.catalog.json` with `pnpm gen:catalog` and extend catalog validation to require all ten states on `ApprovalCard` and `ConfirmationDialog`

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/3.3`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/3.2`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 3.4 Update the MCP App views in `mcp/app/src/**` that render a confirmation to use the new model

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/3.4`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/3.3`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.5 Run WCAG 2.2 AA (axe, keyboard, reduced motion) on all ten states in light and dark, and the no-remote-assets scan

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/3.5`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/3.4`.

  Acceptance: final-boundary scenario(s) `UI-103`, `UI-112` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 4. Implementation tasks

- [ ] 4.1 Replace the five-row "Confirmation states (trust surface)" table in `DESIGN.md` with the ten-row table and the card field list, and change the `approved` copy rule to "Approved, not yet run"

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/4.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/3.5`.

  Acceptance: final-boundary scenario(s) `UI-103` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 4.2 Document new defaults, off switches and changelog Highlights

  Assignment: `agentic-ui-implementation/praxis-decision-model-v2/4.2`; lane `AR`; role(s) ipfs_agent_architect; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-model-v2/4.1`.

  Acceptance: final-boundary scenario(s) `UI-103`, `UI-113` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## Workflow follow-up

- Lead uses canonical one-task boundaries after readiness; no bare apply, guessed ordinals or manual generated state.
- Complete all production before the one final packet and independent reviews; one batched correction cycle. No automatic deploy or archive.
