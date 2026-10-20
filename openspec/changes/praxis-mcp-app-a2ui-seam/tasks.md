# Tasks: praxis-mcp-app-a2ui-seam

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

Plan reference: `.kbd-orchestrator/phases/agentic-ui-implementation/plan.md`, Task model assignments. Full keys are `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/<id>`. Stable checkbox titles/IDs are retained for runtime identity; the operative amendments below replace obsolete assertions within historical display titles. No historical title authorizes a superseded architecture.

## Operative implementation amendments

Existing IDs: **1.1–1.3, 2.1–2.2, 3.1–3.3**. Add **1.4**, architect, for catalog negotiation/view-component mapping contract and ADR. Execute 1.4 before 1.1 and 1.2 despite its display order.

- Architect adds contract negotiation and view/component derivation rules under existing contract authority; UI engineer owns `mcp/app/src/view-specs.ts`, app host adapter and `mcp/ui` renderer; server engineer owns `agui_a2ui.go` and validated AG-UI input.
- **1.1** derives server/client mapping from one contract source. A second independently maintained TS map must not become a second authority.
- **1.2** explicitly accepts a bounded advertised catalog identifier/version, validates it, selects server delivery and keeps older MCP hosts working. A flag may not select arbitrary HTML.
- **2.2** must target actual SDK message transport (`mcp-port.ts` / main SDK setup), not `port.ts` interface. Validate source window and trusted origin/bootstrap plus standard request correlation using pinned SDK capabilities. Do not add a private field external hosts cannot echo. Record any precise spec amendment and evidence before implementation.
- **3.1** fallback is inert text for unknown component, unsupported catalog version and schema-invalid result. No executable markup, action, navigation or remote fetch.
- **3.2** starts after S host placement is available; **1.1–2.2** can precede shell completion when file ownership is disjoint.

## 1. Implementation tasks

- [x] 1.1 Add `VIEW_COMPONENT` next to `VIEW_IDS` in `mcp/app/src/view-specs.ts` and extend `mcp/app/tests/views.test.ts` to check contract `appView` → view → catalog component

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.4`; `agentic-ui-implementation/agentic-react19-foundation/1.7`; `agentic-ui-implementation/praxis-decision-model-v2/2.3`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.2 In `mcp/server/agui_a2ui.go`, deliver `appView` results as A2UI surface updates when the run input advertises the IPFS catalog id; keep the `ui://` link for other MCP hosts

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.2`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.1`; `agentic-ui-implementation/praxis-decision-model-v2/1.2`; `agentic-ui-implementation/praxis-decision-model-v2/1.3`; `agentic-ui-implementation/praxis-decision-model-v2/1.4`; `agentic-ui-implementation/praxis-decision-model-v2/1.5`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.3 A2UI fixtures for all five views drive both the A2UI renderer test and the iframe view test

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.3`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.2`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.4 Define additive catalog negotiation and single-source view/component mapping contract

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.4`; lane `AR`; role(s) ipfs_agent_architect; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/agentic-react19-foundation/1.1`; `agentic-ui-implementation/praxis-decision-model-v2/1.1`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 2. Implementation tasks

- [x] 2.1 Add the exception declaration (`exception: "iframe"`, reason, correlation rule) to each view resource's metadata

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/2.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/1.3`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 2.2 In `mcp/app/src/host/port.ts`, issue a per-render correlation id, pin the host origin from the first host message, and drop messages that do not match

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/2.2`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/2.1`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 3. Implementation tasks

- [x] 3.1 Inert labelled fallback in `mcp/ui` for unknown component, off-catalog version and schema-failing output, with the copy "This result can't be shown here. Nothing in it runs." Verify: component tests cover the three cases and find no button or link other than copy text

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/2.2`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.2 Register the `mcp/ui` A2UI renderer in the Web UI pane and canvas (`../ipfs-webui/src/shell/**`, `src/agent-chat/**`)

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.2`; lane `SU -> WU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.1`; `agentic-ui-implementation/praxis-shell-modes/1.1`; `agentic-ui-implementation/praxis-shell-modes/1.2`; `agentic-ui-implementation/praxis-shell-modes/1.3`; `agentic-ui-implementation/praxis-shell-modes/2.1`; `agentic-ui-implementation/praxis-shell-modes/2.2`; `agentic-ui-implementation/praxis-shell-modes/2.3`; `agentic-ui-implementation/praxis-shell-modes/3.1`; `agentic-ui-implementation/praxis-shell-modes/3.2`; `agentic-ui-implementation/praxis-shell-modes/3.3`; `agentic-ui-implementation/praxis-shell-modes/3.4`; `agentic-ui-implementation/praxis-shell-modes/4.1`; `agentic-ui-implementation/praxis-shell-modes/4.2`; `agentic-ui-implementation/praxis-shell-modes/4.3`; `agentic-ui-implementation/praxis-shell-modes/4.4`.

  Acceptance: final-boundary scenario(s) `UI-106` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.3 WCAG 2.2 AA and the no-remote-assets scan on the five surfaces and the fallback, light and dark

  Assignment: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.3`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/3.2`.

  Acceptance: final-boundary scenario(s) `UI-106`, `UI-112` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## Workflow follow-up

- Lead uses canonical one-task boundaries after readiness; no bare apply, guessed ordinals or manual generated state.
- Complete all production before the one final packet and independent reviews; one batched correction cycle. No automatic deploy or archive.
