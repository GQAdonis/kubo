# Design: praxis-mcp-app-a2ui-seam

## Context

Share one contract-derived result catalog across first-party A2UI and compatible external MCP App hosts, with actual SDK transport checks and inert fallback. See proposal.md and the phase assessment/research for observed implementation gaps.

## Goals / Non-Goals

The operator confirmed UI-01 through UI-16 in `.kbd-orchestrator/phases/agentic-ui-implementation/decision-log.md`. The reviewed phase `plan.md`, authored `tasks.json` dependency graph and `adopted-contract.json` scenario oracles govern. Historical `docs/design` copies remain visual references; canonical amendments replace conflicting architecture and acceptance assumptions.

This is a plan artifact, not production implementation. F/1.1 creates implementation architecture/ownership grants; architect-owned schema/ADR tasks execute only after readiness. No RPC/gateway/default CID/wire change, direct repository opening, silent content injection, unconfigured endpoint or new approval authority. F4/G10/E6 and ADR-0010/0013 remain authoritative.

No deployment, publication, push/merge, WebUI CID bump, durable history, profile editor, trusted-peer settings, new pairing protocol, uploads or multimodal ingestion. All production/test/fixture authoring precedes one final verification boundary; prerequisite compilation is allowed, per-task QA loops are not. Baseline source identities are in phase `source-baselines.json`; capture historical screenshots only in isolated fixtures at that final boundary.

## Decisions

### A/1.3 catalog tuple generation

D/3.3 source inspection found that the catalog producer still emits an incumbent protocol-version literal and omits `x-ipfs-mcp.catalogVersion`. The shared UI owner must derive both catalog-version and protocol-version metadata from the canonical derivations descriptor when authoring A/1.3's shared view fixtures, regenerate the catalog, and add mismatch/missing-metadata fixtures. Backend advertisement and client validation must agree with that generated tuple. No second version authority or direct generated-file edit is permitted.

The A/1.3 owner may extend `generate-contract.mjs`/`genMeta` to emit browser-safe `A2UI_CATALOG_VERSION` and `A2UI_PROTOCOL_VERSION` from that descriptor, add a targeted `--tool-meta` generation entry point, and regenerate only `src/contract/generated/tool-meta.ts` before catalog generation. Catalog runtime, manifest producer and validator consume these derived constants. This slice is reserved to the A2UI worker; other UI owners must coordinate before touching these producers or outputs.

### A/2.1 resource metadata projection

The historical checkbox's `exception` and correlation wording is fulfilled by the operative canonical `externalApps` declaration, whose closed schema permits `reason: external_mcp_app_host` and `transport: standard_ext_apps` at `ai.ipfs/iframe-exception`. Do not add fields forbidden by that schema. A/2.1 projects this declaration into the built manifest while preserving standard UI metadata and view/tool mappings; the server already projects it into listed and read resources. Correlation remains standard JSON-RPC transport behavior, implemented by A/2.2, rather than an invented metadata or private handshake field.

### A/2.2 transport and lifecycle slice

Pinned-source inspection found `App.connect(Transport)` supports the wrapper, while the SDK React hook cannot inject it. The hook declaration's cleanup prose differs from the installed JavaScript, which does close on unmount; implementation decisions follow the actual source. The canonical negotiation prose now defines exact-parent, correlated initialize-result origin bootstrap, fresh standard request-ID namespaces and teardown. The A2UI owner may add the local host lifecycle hook and make the narrow `McpAppRoot.tsx` import/hook integration change needed to consume it. D/3.4 retains all decision/session/view ownership; this transport slice must preserve its existing HostPort interface and does not grant resolver authority.

### A/3.1 generated fallback schema slice

The A2UI owner may add `generateUiNegotiation()` through the existing schema-to-Zod producer, deriving `InertResult`, its label, reason enum and text bound from the canonical negotiation schema. A targeted `--ui-negotiation` command writes only `src/contract/generated/ui-negotiation.ts`; ordinary generation includes the same producer. The shared fallback consumes this browser-safe projection. It is a host fallback, not a new model-addressable catalog component, so no catalog version or canonical schema field changes are authorized.

### A/1.4 canonical negotiation and mapping

[ADR-0016](../../../docs/mcp/0016-catalog-negotiation-and-app-mapping.md) and [normative negotiation semantics](../../../mcp/contract/agentic/ui-negotiation.md) define the exact bounded catalog tuple and inert fallback. `derivations/a2ui-components.json` now owns catalogVersion/protocolVersion, five viewBindings and external-host metadata; `catalog.json` retains per-tool component/output authority. The mfs allowlist deliberately includes ContentCard for existing mfs_read as well as MfsTree. Generated Go/TypeScript/resource projections must preserve that distinction.

First-party offers at forwardedProps.ipfs.a2ui.catalogs select only server-built A2UI with an exact supported ID/version tuple. Missing/invalid/unmatched offers disable A2UI alone; ordinary tool/text output and standard ui:// MCP Apps remain. The iframe exception declaration requires no private SDK handshake field. Fixtures author UI-106 expectations; no generated artifacts, server/UI code, verification builds, tests or independent reviews run in this tranche.

Existing IDs: **1.1–1.3, 2.1–2.2, 3.1–3.3**. Add **1.4**, architect, for catalog negotiation/view-component mapping contract and ADR. Execute 1.4 before 1.1 and 1.2 despite its display order.

- Architect adds contract negotiation and view/component derivation rules under existing contract authority; UI engineer owns `mcp/app/src/view-specs.ts`, app host adapter and `mcp/ui` renderer; server engineer owns `agui_a2ui.go` and validated AG-UI input.
- **1.1** derives server/client mapping from one contract source. A second independently maintained TS map must not become a second authority.
- **1.2** explicitly accepts a bounded advertised catalog identifier/version, validates it, selects server delivery and keeps older MCP hosts working. A flag may not select arbitrary HTML.
- **2.2** must target actual SDK message transport (`mcp-port.ts` / main SDK setup), not `port.ts` interface. Validate source window and trusted origin/bootstrap plus standard request correlation using pinned SDK capabilities. Do not add a private field external hosts cannot echo. Record any precise spec amendment and evidence before implementation.
- **3.1** fallback is inert text for unknown component, unsupported catalog version and schema-invalid result. No executable markup, action, navigation or remote fetch.
- **3.2** starts after S host placement is available; **1.1–2.2** can precede shell completion when file ownership is disjoint.

## Reuse evidence

### cand-001: React 19.3.1 — adopt

Explicit operator choice and source-level fit; reuse existing behavior and preserve authority boundaries.

- Source: https://react.dev/blog/2024/04/25/react-19-upgrade-guide. Evidence: Removed DOM/context/defaults APIs require source migration; existing shared packages already use React19.

Maintenance: React19.3.1 registry release verified; active upstream. Installed compatibility still untested.

Risk: Installed-runtime compatibility is unverified until final integration; see scoped research notes.

### cand-006: modelcontextprotocol/ext-apps 2.0.3 — adapt

Explicit operator choice and source-level fit; reuse existing behavior and preserve authority boundaries.

- Source: https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx. Evidence: Standard JSON-RPC over postMessage and sandbox proxy need compatible adapter checks; inspect exact pinned source before choosing injection API.

Maintenance: Existing2.0.3 pinned SDK and current primary protocol docs inspected. Latest release date/support window not established within research budget; preserve pin and inspect resolved source instead of claiming maintenance certification.

Risk: Installed-runtime compatibility is unverified until final integration; see scoped research notes. Verify actual package license/notice files during dependency import; no license audit claimed here.

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
