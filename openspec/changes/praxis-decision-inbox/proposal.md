# Proposal: praxis-decision-inbox

## Why

Show the current authenticated identity its decisions across sessions using bounded memory and a protected idle event subscription; observation confers no approval authority.

## What Changes

Existing IDs: **1.1–1.5, 2.1–2.3, 3.1–3.5**.

- Architect owns **1.1** and prerequisite ADR: dedicated list/item/feed descriptors, schemas, auth/off switches; do not reuse vector-search-specific `routes/index.json` semantics. Server engineer consumes the contract.
- **1.2–1.3**: at most 200 resolved items or seven days (earliest eviction), in memory; query/filter by current identity BindingKey across its sessions. Test same configured principal with different OIDC subjects. No credentials, argument payloads, native requests from other identities or raw binding keys in list/feed.
- **1.4**: server-assigned Activity identity survives interrupt/continuation and backend-specific resume; no durable-history claim. UAR mapping preserves supported facts and labels missing ones.
- **1.5**: event id/state/revision payload only. Add **1.6**, server engineer: protected idle subscription on existing MCP listener with identity binding, expiry/disconnect handling, bounded subscribers/buffers, reconnect snapshot reconciliation and no durable replay.
- Add **1.7**, server engineer: read-only item/outcome lookup and capability-scoped continuation metadata. No new generic approval endpoint. Owned chat interruptions resume through existing `/agui` ownership/digest/resolver checks. Other items name their authorized host/native surface.
- **2.1–2.3** use all eleven shared presentations. **3.1–3.2** use authenticated fetch streaming, snapshot-first reconciliation and frame-coalesced invalidation; no bearer in URL and no invented EventSource header support.
- **3.3** reuses the existing trusted activation/resume route only where allowed. Re-reading or receiving an event cannot approve. Read-only observer, A2A/UAR and native-required cards get no new approval authority.
- **3.4–3.5** truthful Activity states and feature-off absence; no profile/history extras.

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Capabilities

### New Capabilities

- `ipfs-decision-inbox`: revised existing in-flight behavioral delta.

### Modified Capabilities

No main capability is renamed; existing in-flight delta paths remain unchanged.

## Impact

- `openspec/changes/praxis-decision-inbox/**`
- `mcp/contract/**`
- `docs/mcp/**`
- `mcp/server/**`
- `test/cli/*mcp* (explicit scoped test grant in OpenSpec)`
- `mcp/ui/src/components/** and tests/**`
- `mcp/ui/catalog/**`
- `mcp/chat/src/island/**`
- `../ipfs-webui/src/{activity,webmcp,agent-chat}/**`

## Planning references

Library annotations: cand-001, cand-007. Exact scoped assignments: phase plan Task model assignments, `agentic-ui-implementation/praxis-decision-inbox/<backend-task-id>`. Numbered task display order is not the execution graph.
