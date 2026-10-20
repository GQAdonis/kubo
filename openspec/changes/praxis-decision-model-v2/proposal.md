# Proposal: praxis-decision-model-v2

## Why

A consumed approval is not a completed effect. Provide eleven truthful shared presentations, exact reviewed-effect binding, separate execution outcomes and success receipts, with existing human approval authority intact.

## What Changes

Existing IDs: `1.1–1.3`, `2.1–2.3`, `3.1–3.5`, `4.1`.

- Architect owns amended **1.1** in `mcp/contract/confirmation/confirmation.schema.json`, generated-type specification and ADR; UI engineer owns generation in `mcp/ui`. Correct the obsolete `mcp/contract/schemas` path and server ownership claim.
- Server engineer owns **1.2–1.3** in `mcp/server/broker.go`, `gate.go`, `elicit.go`, `registry.go`, transport payload mappers and confirmation channel. Bind `effect_digest` to the exact normalized operation; carry reviewed digest through each supported resolver without making absent legacy fields an approval bypass.
- Add **1.4**, server engineer: execution lifecycle and success receipt. Preserve the one-time claim used by `executed`; record actual handler start/outcome separately. Receipt appears only after confirmed success; known failure maps to “Failed after approval”; timeout/partial/transport ambiguity stays unknown. No replay or automatic write retry.
- Add **1.5**, architect/server engineer serial handoff: resolver mechanism versus resolving surface identity and precedence. Surface IDs are attribution, not authorization. Outcome facts survive an “answered elsewhere” presentation; two same-kind chat panes are distinguishable without exposing another identity.
- Amend **2.1–2.3, 3.1, 3.3, 3.5, 4.1** to eleven states. Preserve the original ten labels and add “Failed after approval.” Never infer “nothing changed” from a post-start failure.
- **2.2, 3.2** must define Ask again as a fresh proposal, Check now as a read-only lookup and Review again as a newly reviewed digest. No old token resend, hidden execution retry or fabricated restart success.
- **3.4** uses the same shared model in every MCP App view. Desktop consumption belongs to T below.
- Add **4.2**, architect: defaults/off switches and changelog Highlights/TOC disclosure for new outcome/decision behavior; preserve generated changelog sections.

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Capabilities

### New Capabilities

- `ipfs-decision-model`: revised existing in-flight behavioral delta.

### Modified Capabilities

No main capability is renamed; existing in-flight delta paths remain unchanged.

## Impact

- `openspec/changes/praxis-decision-model-v2/**`
- `mcp/contract/**`
- `docs/mcp/**`
- `mcp/server/**`
- `test/cli/*mcp* (explicit scoped test grant in OpenSpec)`
- `mcp/ui/src/approval/**`
- `mcp/ui/src/components/{approval-card,confirmation-dialog}/**`
- `mcp/ui/src/contract/generated/**`
- `mcp/ui/catalog/**`
- `mcp/ui/tests/**`
- `mcp/app/src/** and tests/**`
- `DESIGN.md (explicit grant)`
- `docs/config.md`
- `docs/changelogs/v0.44.md (TOC and Highlights only)`

## Planning references

Library annotations: cand-001, cand-007. Exact scoped assignments: phase plan Task model assignments, `agentic-ui-implementation/praxis-decision-model-v2/<backend-task-id>`. Numbered task display order is not the execution graph.
