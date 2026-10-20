# Design: praxis-context-inspector

## Context

Send visible removable bounded route/CID/IPFS-path references and display actual built-in/UAR run facts; never invent model context, tool counts or prompt-budget calculations. See proposal.md and the phase assessment/research for observed implementation gaps.

## Goals / Non-Goals

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Decisions

### C/1.3 reference bootstrap and final integration fixture grant

The server engineer may wire the already specified `agent.context_references` setting through the existing AG-UI initializer in `cmd/ipfs/kubo/mcp_agent.go`, preserving other defaults and listener behavior. The engineer may author isolated `test/cli/` fixtures and a leased test helper for UI-108/UI-111: only the `make build` binary, temporary repositories, dynamic loopback ports, explicit fixture content, and exact child-process cleanup. Built-in and UAR modes require actual explicitly configured test collaborators; missing prerequisites report unavailable, never substitute a fake backend. No production entry point, operator configuration or credentials are read or changed.

The helper may supply public endpoint/CID/process metadata and ephemeral test authentication through a protected process channel or mode-0600 temporary descriptor. It never logs credentials. Browser consumers retain actual assertions, screenshots and command receipts, but must not persist credential-bearing traces or network logs. The lease closes on parent stdin closure or timeout and joins teardown; normal broad CLI runs do not start a leased fixture without its explicit test configuration. This is final-boundary fixture authoring, not permission to execute acceptance during production.

### C/1.2 narrow bootstrap grant

The server engineer may add only `RunFacts: s.cfg.Agent.RunFacts` to the existing `srv.EnableAGUI` settings initializer in `cmd/ipfs/kubo/mcp_agent.go`, wiring the already specified `agent.run_facts` switch into the server's `AgentConfig`/`AGUISettings`. Preserve existing help, defaults, model selection, endpoint and origin configuration. All other implementation remains under `mcp/server/**`; no RPC, gateway, repo or CLI behavior expansion is authorized. Author disabled-state fixtures and include this bootstrap edit in the final `make build` and relevant MCP CLI checks. Context snapshots remain omitted until C/1.3 actually accepts references.

Existing IDs: **1.1–1.2, 2.1–2.5, 3.1–3.2**.

- Architect owns **1.1** schema extension in the actual AG-UI schema/contract definitions; UI engineer regenerates consumers. Effective facts come from principal-filtered offered tools and retained per-run prompt/backend snapshot, not global `Server.ToolNames()`.
- **1.2** supports built-in and UAR: emit known facts only; no synthesized prompt digest/tool count for UAR when unreported.
- Add **1.3**, architect then server engineer: bounded typed route/CID/IPFS-path references accepted by AG-UI decoder and built-in/UAR adapter. No filesystem access, implicit content fetching, uploads or authority from context strings. Resolve content only through existing authorized tools/egress checks. Frame incoming references as untrusted data.
- **2.1–2.2, 3.1–3.2**: visible removable reference chips; removed context absent on every subsequent request until route changes. “Files attached” prototype language becomes accurate reference language; no unavailable upload control.
- **2.3** current-run-only event list, virtualized after 200; no prompt text/secrets/raw sensitive args. **2.4** only event-supported lifecycle phases; cancellation and unknown backend facts are distinct.

## Reuse evidence

### cand-001: React 19.3.1 — adopt

Explicit operator choice and source-level fit; reuse existing behavior and preserve authority boundaries.

- Source: https://react.dev/blog/2024/04/25/react-19-upgrade-guide. Evidence: Removed DOM/context/defaults APIs require source migration; existing shared packages already use React19.

Maintenance: React19.3.1 registry release verified; active upstream. Installed compatibility still untested.

Risk: Installed-runtime compatibility is unverified until final integration; see scoped research notes.

### cand-007: Existing Kubo broker and protected SSE patterns — adapt

Explicit operator choice and source-level fit; reuse existing behavior and preserve authority boundaries.

- Source: mcp/server/broker.go; mcp/server/settings_events.go. Evidence: Reuse one-time claim and authenticated subscriptions; build distinct outcomes, scoped snapshots, bounded history and epoch recovery.

Maintenance: In-repository code maintained by server team;17 existing broker/session baseline tests passed. New outcomes/feed require project-owned lifecycle/security tests.

Risk: Installed-runtime compatibility is unverified until final integration; see scoped research notes.

## Alternatives and trade-offs

- Preserve existing architecture and shared components instead of introducing a new agent runtime, new auth gate or protocol.
- Maintain source-controlled compatibility fixes instead of silent installed-dependency patching.
- Keep history in memory and label unknown facts instead of expanding into durable storage or pretending unavailable state is known.

## Risks / Trade-offs

- [Dependency declarations do not prove runtime compatibility] → inspect resolved sources and preserve all existing operations in final scenarios.
- [Identity or consent confusion] → subject-bound ownership, existing resolver authority and separate actual outcomes.
- [Large shared-tree work] → explicit native writer slices and one generator owner; no stash/reset or gate bypass.

## Migration and verification

Local artifact assembly only; no deployed rollout or CID change. Tests/docs are authored with production, then the final packet evaluates real browser/Electron/backend behavior, gateway/telemetry/provider compatibility, negative controls and off switches. Unknown collaborators/hardware stay unverified. Independent reviewers run after complete implementation. No canonical task is complete merely because this document exists.

## C/1.1 canonical contract amendment (2026-10-20)

The historical checkbox titles remain stable. Operative acceptance is the additive standard `CUSTOM ipfs.run.facts` event, **not custom RUN_STARTED properties**. The latter can be stripped by ordinary AG-UI SDK schemas. `mcp/contract/agentic/run-context.schema.json` and `run-context.md` define exact optional fields, source provenance, typed references, bounds and backend handoff; `mcp/contract/fixtures/agui.schema.json` dispatches the event. See ADR0018.

- **1.1** authors those contracts plus reviewed-effect wire placements in the confirmation schema/README; it does not generate UI or implement server adapters.
- **1.2** constructs actual facts for both backend paths and emits at most once after RUN_STARTED and optional immediate a2ui.catalog, before tool/text. Use Agent.offer(eff) and the retained effective run snapshot, never raw Server.ToolNames() or a reconstructed prompt. UAR facts not actually reported remain absent. Clients bind thread_id/run_id and show Not reported for absent facts.
- **1.3** validates forwardedProps.ipfs.context independently of the A2UI branch: maximum 16 unique references and 16,384 encoded UTF-8 bytes, syntax-only CID/logical IPFS/IPNS/MFS paths, visible 422 invalid_argument before either backend for invalid references, and no implicit read/fetch/name resolution. MFS always means the connected node's logical MFS, never host files. Frame accepted references as untrusted data and preserve existing egress/confirmation/grants.
- **2.1** owns generated consumers and faithful external-reference/conditional-schema conversion. **2.2, 3.1–3.2** omit removed references across sends until a visible route change; a resume retains its accepted turn and rejects new nonempty references. Generic context/tools/state never supply a hidden fallback.
- **2.3** retains only current-run safe event projections, at most 1,000 entries/524,288 encoded bytes, virtualizes after 200, discloses truncation and resets on inspected run switch. **2.4** uses actual lifecycle evidence and unknown status; approval is not success. No token-budget guesses or raw prompt/args.
- `agent.context_references=false` rejects nonempty references while ordinary messaging works; `agent.run_facts=false` suppresses only the fact projection. D/4.2 integrates generated config references, default examples, changelog and disabled-state checks.
