# Proposal: praxis-context-inspector

## Why

Send visible removable bounded route/CID/IPFS-path references and display actual built-in/UAR run facts; never invent model context, tool counts or prompt-budget calculations.

## What Changes

Existing IDs: **1.1–1.2, 2.1–2.5, 3.1–3.2**.

- Architect owns **1.1** schema extension in the actual AG-UI schema/contract definitions; UI engineer regenerates consumers. Effective facts come from principal-filtered offered tools and retained per-run prompt/backend snapshot, not global `Server.ToolNames()`.
- **1.2** supports built-in and UAR: emit known facts only; no synthesized prompt digest/tool count for UAR when unreported.
- Add **1.3**, architect then server engineer: bounded typed route/CID/IPFS-path references accepted by AG-UI decoder and built-in/UAR adapter. No filesystem access, implicit content fetching, uploads or authority from context strings. Resolve content only through existing authorized tools/egress checks. Frame incoming references as untrusted data.
- **2.1–2.2, 3.1–3.2**: visible removable reference chips; removed context absent on every subsequent request until route changes. “Files attached” prototype language becomes accurate reference language; no unavailable upload control.
- **2.3** current-run-only event list, virtualized after 200; no prompt text/secrets/raw sensitive args. **2.4** only event-supported lifecycle phases; cancellation and unknown backend facts are distinct.

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Capabilities

### New Capabilities

- `ipfs-agent-context`: revised existing in-flight behavioral delta.

### Modified Capabilities

No main capability is renamed; existing in-flight delta paths remain unchanged.

## Impact

- `openspec/changes/praxis-context-inspector/**`
- `mcp/contract/**`
- `docs/mcp/**`
- `mcp/server/**`
- `test/cli/*mcp* (explicit scoped test grant in OpenSpec)`
- `mcp/ui/src/contract/generated/**`
- `mcp/chat/src/{island,agui}/** and tests/**`
- `../ipfs-webui/src/agent-chat/**`

## Planning references

Library annotations: cand-001, cand-007. Exact scoped assignments: phase plan Task model assignments, `agentic-ui-implementation/praxis-context-inspector/<backend-task-id>`. Numbered task display order is not the execution graph.
