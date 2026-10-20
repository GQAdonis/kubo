# Design: praxis-decision-model-v2

## C/1.1 reviewed-effect adapter amendment (2026-10-20)

The confirmation schema/README now specify optional reviewed-effect placement on pinned SDK-supported ElicitParams/ElicitResult/CallToolParams `_meta["ipfs-mcp"]`, on AG-UI resume entries outside payload/tool arguments, and in the existing authenticated signed native resolve body. D/1.3 implements these exact placements; old clients omit the echo through the original immutable binding checks. Do not add illegal ElicitResult fields or private MCP App handshakes. Well-formed mismatches fail closed after authorized original binding; malformed explicit echoes are never treated as absent. UAR synthetic approvals without a trusted retained digest refuse an explicit echo after owner/open-ID/payload checks and preserve the continuation unconsumed; the UI cannot synthesize a node digest/receipt from backend text. See `mcp/contract/confirmation/README.md` for exact error precedence and `reviewed-wire-cases.json` for authored fixtures.

## Context

D/1.4 supporting result-delivery amendment: use the optional standard `CallToolResult._meta["ipfs-mcp"]` namespace with `ToolOutcomeMetadata {confirmation_id, effect_digest, execution}` for the authorized current winning node invocation. The receipt remains `execution.receipt`; no raw args/results, BindingKey, tokens or new HTTP credentials. Standard MCP Apps tool-result delivery carries it to external hosts, preserving every existing tool payload/output schema and unrelated metadata. D/3.4 consumes it through the shared model; missing metadata never fabricates Done. Timestamp semantics allow `started_at <= completed_at` and matching receipt completion, including equal observed instants; no fabricated +1ns. Canonical details: confirmation schema/README, ADR0015 amendment and tool-outcome fixture cases.

A consumed approval is not a completed effect. Provide eleven truthful shared presentations, exact reviewed-effect binding, separate execution outcomes and success receipts, with existing human approval authority intact. See proposal.md and the phase assessment/research for observed implementation gaps.

## Goals / Non-Goals

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Decisions

### D/3.3 catalog and renderer parity

The catalog may not advertise rich decision presentations that its A2UI implementation cannot receive. The shared UI owner may update `mcp/ui/src/a2ui/apis.ts` and `implementations.tsx` narrowly to accept the existing rich `model` prop, validate it with `ApprovalModelSchema`, and pass it to the existing components. Preserve the explicit legacy-versus-rich input branches and existing resolver capability rules; callbacks never grant rich-model rights. Describe structural model fields and canonical phase/outcome references in the generated catalog, retaining runtime semantic validation for correlated receipts. Author parity and negative fixtures. This does not add controls or expand host approval authority.

### D/1.1 contract tranche

[ADR-0015](../../../docs/mcp/0015-decision-outcomes-and-observation.md) and [the normative contract semantics](../../../mcp/contract/confirmation/README.md) record the additive amendment in `mcp/contract/confirmation/confirmation.schema.json`: immutable reviewed-effect binding, safe destination, resolver/surface separation, actual execution lifecycle, successful receipts, identity-safe source-distinguished observations and eleven presentations. The legacy required fields, broker state/claim and token/request-state formats remain intact. A bare `executed` is never success; absent optional fields grant no approval bypass.

The schema adds reusable definitions for the following A/I/C tranche, without creating a read endpoint or generic resolver. UI generation is reserved to SU D/2.1, including faithful conversion of new conditional constraints; no generated source/catalog is edited in D/1.1. Positive examples and `mcp/contract/confirmation/fixtures/decision-cases.json` author final-boundary expectations, not passing test evidence. D/4.2 remains the defaults/off-switch assignment.

Existing IDs: `1.1–1.3`, `2.1–2.3`, `3.1–3.5`, `4.1`.

- Architect owns amended **1.1** in `mcp/contract/confirmation/confirmation.schema.json`, generated-type specification and ADR; UI engineer owns generation in `mcp/ui`. Correct the obsolete `mcp/contract/schemas` path and server ownership claim.
- Server engineer owns **1.2–1.3** in `mcp/server/broker.go`, `gate.go`, `elicit.go`, `registry.go`, transport payload mappers and confirmation channel. Bind `effect_digest` to the exact normalized operation; carry reviewed digest through each supported resolver without making absent legacy fields an approval bypass.
- Add **1.4**, server engineer: execution lifecycle and success receipt. Preserve the one-time claim used by `executed`; record actual handler start/outcome separately. Receipt appears only after confirmed success; known failure maps to “Failed after approval”; timeout/partial/transport ambiguity stays unknown. No replay or automatic write retry.
- Add **1.5**, architect/server engineer serial handoff: resolver mechanism versus resolving surface identity and precedence. Surface IDs are attribution, not authorization. Outcome facts survive an “answered elsewhere” presentation; two same-kind chat panes are distinguishable without exposing another identity.
- Amend **2.1–2.3, 3.1, 3.3, 3.5, 4.1** to eleven states. Preserve the original ten labels and add “Failed after approval.” Never infer “nothing changed” from a post-start failure.
- **2.2, 3.2** must define Ask again as a fresh proposal, Check now as a read-only lookup and Review again as a newly reviewed digest. No old token resend, hidden execution retry or fabricated restart success.
- **3.4** uses the same shared model in every MCP App view. Desktop consumption belongs to T below.
- Add **4.2**, architect: defaults/off switches and changelog Highlights/TOC disclosure for new outcome/decision behavior; preserve generated changelog sections.

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

## D/3.5 final fixture integration grant

The shared UI owner also owns WebUI `test/boundary/registrations/decisions.mjs`, `accessibility.mjs` and `test/boundary/accessibility/**` to register shared decision checks and establish the one-registration-per-slot UI-112 contribution interface. Require named decisions, shell, A2UI, inbox and context contributions at the final boundary; missing/empty families block. Later owners add separate contributions. Bind fixture bytes and real observed assertions; do not replace real backend UI-103/104 behavior with component fixtures. Desktop retains its distinct SU slots. No QA runs during this task.

The existing backend lease lacks deterministic after-claim pause/failure control, separately bound identities and direct resolver submissions needed by UI-103/104. Under D/3.5, the server engineer may author a dedicated compiled test lease/driver in server/CLI test files with a documented bounded private control channel. It must exercise the actual production broker, gate, transport and operation handlers against an isolated `make build` daemon and real RPC effects. Test-owned wrappers may delay or fail an operation after the real approval claim; no production bypass flag, fabricated successful receipt, operator credential/node or fake native-presence factor is allowed. UI fixtures use the documented private adapter and preserve exact process, source and effect evidence. Lead serializes this support slice with its native server owner; shared UI remains responsible for browser and accessibility contributions. This task is not authored-complete while required real-gate registrations are placeholders.

The D/3.5 UI owner may narrowly add a unique per-browser-session suffix to WebUI boundary browser evidence and trace filenames, with a focused collision fixture. Multiple named accessibility contributors intentionally use the same theme and viewport, so theme/width alone is not an evidence identity. Preserve credential-bearing capture restrictions and existing receipt semantics; no other runner behavior is in this repair slice.

## D/3.4 shared timeout presentation integration

App integration exposed that `mcp/ui/src/approval/presentation.ts` re-promotes validated running/not_started evidence to approved after the canonical mapper has selected uncertain at its configured outcome deadline. The D/3.4 UI owner may narrowly preserve that uncertain presentation for nonterminal execution and add a focused regression fixture. Keep actual terminal success/failure/unknown precedence and retain execution context; do not discard evidence or create a new timeout authority. No generator or QA execution is needed for this authoring repair.
