# Tasks: praxis-context-inspector

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

Plan reference: `.kbd-orchestrator/phases/agentic-ui-implementation/plan.md`, Task model assignments. Full keys are `agentic-ui-implementation/praxis-context-inspector/<id>`. Stable checkbox titles/IDs are retained for runtime identity; the operative amendments below replace obsolete assertions within historical display titles. No historical title authorizes a superseded architecture.

## Operative implementation amendments

Existing IDs: **1.1–1.2, 2.1–2.5, 3.1–3.2**.

- Architect owns **1.1** schema extension in the actual AG-UI schema/contract definitions; UI engineer regenerates consumers. Effective facts come from principal-filtered offered tools and retained per-run prompt/backend snapshot, not global `Server.ToolNames()`.
- **1.2** supports built-in and UAR: emit known facts only; no synthesized prompt digest/tool count for UAR when unreported.
- Add **1.3**, architect then server engineer: bounded typed route/CID/IPFS-path references accepted by AG-UI decoder and built-in/UAR adapter. No filesystem access, implicit content fetching, uploads or authority from context strings. Resolve content only through existing authorized tools/egress checks. Frame incoming references as untrusted data.
- **2.1–2.2, 3.1–3.2**: visible removable reference chips; removed context absent on every subsequent request until route changes. “Files attached” prototype language becomes accurate reference language; no unavailable upload control.
- **2.3** current-run-only event list, virtualized after 200; no prompt text/secrets/raw sensitive args. **2.4** only event-supported lifecycle phases; cancellation and unknown backend facts are distinct.

## 1. Implementation tasks

- [x] 1.1 Add `offered_tools`, `catalog_size` and `system_prompt_digest` to the AG-UI run-start payload schema (additive) and regenerate `mcp/ui/src/contract/generated/agui.ts`

  Assignment: `agentic-ui-implementation/praxis-context-inspector/1.1`; lane `AR`; role(s) ipfs_agent_architect; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/agentic-react19-foundation/1.1`; `agentic-ui-implementation/praxis-decision-model-v2/1.1`.

  Acceptance: final-boundary scenario(s) `UI-108` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.2 Emit the fields from `agui_stream.go` using `Server.ToolNames()` and the prompt built in `agent_frame.go`

  Assignment: `agentic-ui-implementation/praxis-context-inspector/1.2`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/1.1`.

  Acceptance: final-boundary scenario(s) `UI-108`, `UI-111` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [x] 1.3 Ingest bounded untrusted reference context through built-in and UAR paths

  Assignment: `agentic-ui-implementation/praxis-context-inspector/1.3`; lane `BE`; role(s) mcp_server_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/1.2`.

  Acceptance: final-boundary scenario(s) `UI-108`, `UI-111` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 2. Implementation tasks

- [ ] 2.1 Add `ContextStrip.tsx` with container, files attached, model placement, memory scope ("None") and tools offered, each with a provenance label

  Assignment: `agentic-ui-implementation/praxis-context-inspector/2.1`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/1.3`; `agentic-ui-implementation/praxis-shell-modes/1.1`; `agentic-ui-implementation/praxis-shell-modes/1.2`; `agentic-ui-implementation/praxis-shell-modes/1.3`; `agentic-ui-implementation/praxis-shell-modes/2.1`; `agentic-ui-implementation/praxis-shell-modes/2.2`; `agentic-ui-implementation/praxis-shell-modes/2.3`; `agentic-ui-implementation/praxis-shell-modes/3.1`; `agentic-ui-implementation/praxis-shell-modes/3.2`; `agentic-ui-implementation/praxis-shell-modes/3.3`; `agentic-ui-implementation/praxis-shell-modes/3.4`; `agentic-ui-implementation/praxis-shell-modes/4.1`; `agentic-ui-implementation/praxis-shell-modes/4.2`; `agentic-ui-implementation/praxis-shell-modes/4.3`; `agentic-ui-implementation/praxis-shell-modes/4.4`.

  Acceptance: final-boundary scenario(s) `UI-108` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 2.2 Removal: container and files removable, removed items absent from the next request; tools and model items link to `#/settings/agent`

  Assignment: `agentic-ui-implementation/praxis-context-inspector/2.2`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/2.1`.

  Acceptance: final-boundary scenario(s) `UI-108` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 2.3 Add `ContextInspector.tsx` with Overview, Context, Prompt plan and Events; Events is read-only and current run only, virtualized above 200 rows

  Assignment: `agentic-ui-implementation/praxis-context-inspector/2.3`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/2.2`.

  Acceptance: final-boundary scenario(s) `UI-108` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 2.4 Add `LifecycleStrip.tsx` mapping AG-UI events to Planning, Calling `<tool>`, Waiting for approval, Running, Done, Failed, Cancelled

  Assignment: `agentic-ui-implementation/praxis-context-inspector/2.4`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/2.3`.

  Acceptance: final-boundary scenario(s) `UI-108` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 2.5 WCAG 2.2 AA (axe, keyboard, reduced motion) in light and dark; no horizontal overflow at 320 and 375

  Assignment: `agentic-ui-implementation/praxis-context-inspector/2.5`; lane `SU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/2.4`.

  Acceptance: final-boundary scenario(s) `UI-108`, `UI-112` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## 3. Implementation tasks

- [ ] 3.1 Send `{route, cid?, path?}` for the current route through `../ipfs-webui/src/agent-chat/bridge.js`; re-send on route change only

  Assignment: `agentic-ui-implementation/praxis-context-inspector/3.1`; lane `WU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/2.5`.

  Acceptance: final-boundary scenario(s) `UI-108` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

- [ ] 3.2 Respect removal: after the person removes the container, no container is sent until the route changes

  Assignment: `agentic-ui-implementation/praxis-context-inspector/3.2`; lane `WU`; role(s) ui_ux_engineer; OpenAI `gpt-6.1-sol/high`, native fresh-context `collaboration.spawn_agent`; recheck availability at dispatch.

  Prerequisites: `agentic-ui-implementation/praxis-context-inspector/3.1`.

  Acceptance: final-boundary scenario(s) `UI-108` in phase `adopted-contract.json`, including negative controls and exact command receipts. Author the necessary fixtures/tests/docs during this task; evaluate after all production.

## Workflow follow-up

- Lead uses canonical one-task boundaries after readiness; no bare apply, guessed ordinals or manual generated state.
- Complete all production before the one final packet and independent reviews; one batched correction cycle. No automatic deploy or archive.

## C/1.1 canonical contract amendment (2026-10-20)

The historical checkbox titles remain stable. Operative acceptance is the additive standard `CUSTOM ipfs.run.facts` event, **not custom RUN_STARTED properties**. The latter can be stripped by ordinary AG-UI SDK schemas. `mcp/contract/agentic/run-context.schema.json` and `run-context.md` define exact optional fields, source provenance, typed references, bounds and backend handoff; `mcp/contract/fixtures/agui.schema.json` dispatches the event. See ADR0018.

- **1.1** authors those contracts plus reviewed-effect wire placements in the confirmation schema/README; it does not generate UI or implement server adapters.
- **1.2** constructs actual facts for both backend paths and emits at most once after RUN_STARTED and optional immediate a2ui.catalog, before tool/text. Use Agent.offer(eff) and the retained effective run snapshot, never raw Server.ToolNames() or a reconstructed prompt. UAR facts not actually reported remain absent. Clients bind thread_id/run_id and show Not reported for absent facts.
- **1.3** validates forwardedProps.ipfs.context independently of the A2UI branch: maximum 16 unique references and 16,384 encoded UTF-8 bytes, syntax-only CID/logical IPFS/IPNS/MFS paths, visible 422 invalid_argument before either backend for invalid references, and no implicit read/fetch/name resolution. MFS always means the connected node's logical MFS, never host files. Frame accepted references as untrusted data and preserve existing egress/confirmation/grants.
- **2.1** owns generated consumers and faithful external-reference/conditional-schema conversion. **2.2, 3.1–3.2** omit removed references across sends until a visible route change; a resume retains its accepted turn and rejects new nonempty references. Generic context/tools/state never supply a hidden fallback.
- **2.3** retains only current-run safe event projections, at most 1,000 entries/524,288 encoded bytes, virtualizes after 200, discloses truncation and resets on inspected run switch. **2.4** uses actual lifecycle evidence and unknown status; approval is not success. No token-budget guesses or raw prompt/args.
- `agent.context_references=false` rejects nonempty references while ordinary messaging works; `agent.run_facts=false` suppresses only the fact projection. D/4.2 integrates generated config references, default examples, changelog and disabled-state checks.
