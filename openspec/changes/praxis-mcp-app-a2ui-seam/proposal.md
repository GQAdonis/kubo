# Proposal: praxis-mcp-app-a2ui-seam

## Why

Share one contract-derived result catalog across first-party A2UI and compatible external MCP App hosts, with actual SDK transport checks and inert fallback.

## What Changes

Existing IDs: **1.1–1.3, 2.1–2.2, 3.1–3.3**. Add **1.4**, architect, for catalog negotiation/view-component mapping contract and ADR. Execute 1.4 before 1.1 and 1.2 despite its display order.

- Architect adds contract negotiation and view/component derivation rules under existing contract authority; UI engineer owns `mcp/app/src/view-specs.ts`, app host adapter and `mcp/ui` renderer; server engineer owns `agui_a2ui.go` and validated AG-UI input.
- **1.1** derives server/client mapping from one contract source. A second independently maintained TS map must not become a second authority.
- **1.2** explicitly accepts a bounded advertised catalog identifier/version, validates it, selects server delivery and keeps older MCP hosts working. A flag may not select arbitrary HTML.
- **2.2** must target actual SDK message transport (`mcp-port.ts` / main SDK setup), not `port.ts` interface. Validate source window and trusted origin/bootstrap plus standard request correlation using pinned SDK capabilities. Do not add a private field external hosts cannot echo. Record any precise spec amendment and evidence before implementation.
- **3.1** fallback is inert text for unknown component, unsupported catalog version and schema-invalid result. No executable markup, action, navigation or remote fetch.
- **3.2** starts after S host placement is available; **1.1–2.2** can precede shell completion when file ownership is disjoint.

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Capabilities

### New Capabilities

- `ipfs-mcp-app-a2ui-seam`: revised existing in-flight behavioral delta.

### Modified Capabilities

No main capability is renamed; existing in-flight delta paths remain unchanged.

## Impact

- `mcp/app/src/** and tests/**`
- `mcp/ui/src/a2ui/** and tests/**`
- `../ipfs-webui/src/{shell,agent-chat}/**`
- `mcp/server/**`
- `test/cli/*mcp* (explicit scoped test grant in OpenSpec)`
- `openspec/changes/praxis-mcp-app-a2ui-seam/**`
- `mcp/contract/**`
- `docs/mcp/**`

## Planning references

Library annotations: cand-001, cand-006. Exact scoped assignments: phase plan Task model assignments, `agentic-ui-implementation/praxis-mcp-app-a2ui-seam/<backend-task-id>`. Numbered task display order is not the execution graph.
