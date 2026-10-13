# Proposal: agent-settings-contract

Phase `agent-backend-settings`, change 2 of 28 in plan revision 3, Round 1. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (ADR-0009; absorbs UI-01).

## Why

Every settings component (server, Web UI page, Desktop bridge, LKE) needs one contract: the settings resource, the `admin` principal, secret handling, egress rules, off switches and the cloud threat model. ADR-0009 records the decisions; the schemas make them checkable.

## What Changes

- 1.1: ADR-0009: runtime settings and hot-reload semantics; the cloud threat model (shared origin, the live exposure, why CORS alone does not close it, D0, D1); the `admin` principal (D2); bearer-only routes outside basic auth, `origin_allowlist` = console origin, admin token never in `localStorage`; secrets (D3 with S4: the cloud key via the GitOps Secret); egress (D4); persistence precedence; defaults placement (D5); documentation location (D12); LKE confirmation behavior under `confirmation.modes: []`.
- 1.2: Settings resource schemas (JSON Schema 2020-12): read shape (no key material; `api_key: {set, source}`), write shape with write-only secret fields, `revision` with `ETag`/`If-Match`, stable error codes (`backend_not_configured`, `revision_conflict`, `egress_confirmation_required`, `secrets_not_accepted`), test-connection and model-list shapes, change events, `backend.kind: openai | uar`, `system_prompt` with a fixed safety prefix.
- 1.3: Config schema keys, each with `default` and `offSwitch`: principal kind `admin`; `agent.settings.enabled`; `agent.settings.accept_secrets`; `--settings-dir`; `agent.a2ui`; `agent.agui` semantics without a backend; updated `llm` `offSwitch` text; an absent Secret file or token environment variable disables only the dependent feature, never startup (CRITICAL-2).
- 1.4: Regenerate `docs/mcp/config-reference.md` with the existing generator and add the `docs/config.md` pointer (D12).
- 1.5: Client-side contract: the `SettingsClient` interface (HTTP and Desktop bridge) and the IPC shapes `ipfsDesktop.agentSettings.{get, set, testConnection, setApiKey, clearApiKey, status, onChange}`, reconciled with assessment-ui 5.2.
- 1.6: Draft the D12 AGENTS.md clarification (round-2 S5) for OG-5.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: architecture decisions, contract schemas and documentation; the behavior they describe is specified in the changes that implement it.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect
- Scope: `docs/mcp/0009-*.md`, `mcp/contract/**`, `docs/mcp/config-reference.md`, `docs/config.md` (pointer)
- Depends on: none; task 1.4 also needs OG-11
- Size: L; goals: 1, 2, 3, 4
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
