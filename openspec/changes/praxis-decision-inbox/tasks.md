# Tasks: praxis-decision-inbox

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

Plan reference: `.kbd-orchestrator/phases/agentic-ui-implementation/plan.md`, Task model assignments. Full keys are `agentic-ui-implementation/praxis-decision-inbox/<id>`. Stable checkbox titles/IDs are retained for runtime identity; the operative amendments below replace obsolete assertions within historical display titles. No historical title authorizes a superseded architecture.

## Operative implementation amendments

Existing IDs: **1.1–1.5, 2.1–2.3, 3.1–3.5**.

- Architect owns **1.1** and prerequisite ADR: dedicated list/item/feed descriptors, schemas, auth/off switches; do not reuse vector-search-specific `routes/index.json` semantics. Server engineer consumes the contract.
- **1.2–1.3**: at most 200 resolved items or seven days (earliest eviction), in memory; query/filter by current identity BindingKey across its sessions. Test same configured principal with different OIDC subjects. No credentials, argument payloads, native requests from other identities or raw binding keys in list/feed.
- **1.4**: server-assigned Activity identity survives interrupt/continuation and backend-specific resume; no durable-history claim. UAR mapping preserves supported facts and labels missing ones.
- **1.5**: event id/state/revision payload only. Add **1.6**, server engineer: protected idle subscription on existing MCP listener with identity binding, expiry/disconnect handling, bounded subscribers/buffers, reconnect snapshot reconciliation and no durable replay.
- Add **1.7**, server engineer: read-only item/outcome lookup and capability-scoped continuation metadata. No new generic approval endpoint. Owned chat interruptions resume through existing `/agui` ownership/digest/resolver checks. Other items name their authorized host/native surface.
- **2.1–2.3** use all eleven shared presentations. **3.1–3.2** use authenticated fetch streaming, snapshot-first reconciliation and frame-coalesced invalidation; no bearer in URL and no invented EventSource header support.
- **3.3** reuses the existing trusted activation/resume route only where allowed. Re-reading or receiving an event cannot approve. Read-only observer, A2A/UAR and native-required cards get no new approval authority.
- **3.4–3.5** truthful Activity states and feature-off absence; no profile/history extras.

## 1. Implementation tasks

- [x] 1.1 Add the decision list item schema (additive) under `mcp/contract/schemas` and the route to `mcp/contract/routes/index.json`

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/1.1`; lane `AR`; role(s) ipfs_agent_architect; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/agentic-react19-foundation/1.1`; `agentic-ui-implementation/praxis-decision-model-v2/1.1`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.2 Add a resolved-history ring to the broker (200 entries or 7 days, in memory)

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/1.2`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/1.1`; `agentic-ui-implementation/praxis-decision-model-v2/1.2`; `agentic-ui-implementation/praxis-decision-model-v2/1.3`; `agentic-ui-implementation/praxis-decision-model-v2/1.4`; `agentic-ui-implementation/praxis-decision-model-v2/1.5`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.3 Register the read-only decisions endpoint through `h.protect` in `mcp/server/http.go`, scoped to the calling principal

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/1.3`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/1.2`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.4 Assign `activity_id` at the first run and carry it through interrupt and continuation in `agui_resume.go`

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/1.4`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/1.3`.

  Acceptance: final-boundary scenario(s) `UI-107`, `UI-111` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.5 Emit `ipfs.decision.changed` (id and state only) on create and on every state change

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/1.5`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/1.4`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.6 Add protected bounded identity-scoped idle event subscription and reconnect reconciliation

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/1.6`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/1.5`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.7 Expose owned decision outcome and existing-resolver continuation metadata without generic approval

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/1.7`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/1.6`.

  Acceptance: final-boundary scenario(s) `UI-104`, `UI-107`, `UI-111` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 2. Implementation tasks

- [ ] 2.1 Build `DecisionInbox` (Waiting and Recent groups) and `ActivityRow` in `mcp/ui/src/components/`

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/2.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/1.7`; `agentic-ui-implementation/praxis-shell-modes/1.1`; `agentic-ui-implementation/praxis-shell-modes/1.2`; `agentic-ui-implementation/praxis-shell-modes/1.3`; `agentic-ui-implementation/praxis-shell-modes/2.1`; `agentic-ui-implementation/praxis-shell-modes/2.2`; `agentic-ui-implementation/praxis-shell-modes/2.3`; `agentic-ui-implementation/praxis-shell-modes/3.1`; `agentic-ui-implementation/praxis-shell-modes/3.2`; `agentic-ui-implementation/praxis-shell-modes/3.3`; `agentic-ui-implementation/praxis-shell-modes/3.4`; `agentic-ui-implementation/praxis-shell-modes/4.1`; `agentic-ui-implementation/praxis-shell-modes/4.2`; `agentic-ui-implementation/praxis-shell-modes/4.3`; `agentic-ui-implementation/praxis-shell-modes/4.4`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.1`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.2`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.3`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.4`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/2.1`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/2.2`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.1`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.2`; `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.3`; `agentic-ui-implementation/praxis-decision-model-v2/3.1`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 2.2 Add both to the generated A2UI catalog and extend validation

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/2.2`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/2.1`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 2.3 WCAG 2.2 AA (axe, keyboard, reduced motion) in light and dark

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/2.3`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/2.2`.

  Acceptance: final-boundary scenario(s) `UI-107`, `UI-112` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 3. Implementation tasks

- [ ] 3.1 Add `../ipfs-webui/src/activity/` with the Home inbox (count plus waiting decisions) and the `#/activity` route (Activity list: state, last run, linked decisions)

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/3.1`; lane `WU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/2.3`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.2 Subscribe to `ipfs.decision.changed` and re-read changed items, coalesced per frame

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/3.2`; lane `WU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/3.1`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.3 WebMCP modal, chat `InterruptItem` and inbox render the same `ApprovalCard`; answering one moves the others to `resolved_elsewhere` naming the surface

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/3.3`; lane `WU -> SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/3.2`.

  Acceptance: final-boundary scenario(s) `UI-104`, `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.4 Activity states render as words: running, waiting for approval, waiting for input, paused, completed, failed, cancelled, unknown (reconciling); waiting never uses the error tone

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/3.4`; lane `WU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/3.3`.

  Acceptance: final-boundary scenario(s) `UI-107` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.5 With agent features off, the inbox and `#/activity` are absent

  Assignment: `agentic-ui-implementation/praxis-decision-inbox/3.5`; lane `WU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-decision-inbox/3.4`.

  Acceptance: final-boundary scenario(s) `UI-107`, `UI-113` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## Workflow follow-up

- Lead uses canonical one-task boundaries after readiness; no bare apply, guessed ordinals or manual generated state.
- Complete all production before the one final packet and independent reviews; one batched correction cycle. No automatic deploy or archive.
