# Assessment — agentic-ui-implementation

Date: 2026-10-20. Status: DRAFT; interview and assessment verification remain open.

## Baseline and ownership

The `kubo-ipfs-agentic` architect, UI engineer and backend engineer independently inspected the design handoff and current code. The three design proposals match. Eight canonical `openspec/changes/praxis-*` changes exist but the proposed workspace is unimplemented. Canonical Kubo changes take precedence over distributed design copies. A separate React 19 migration foundation is now explicitly required by the operator.

The previous `agent-backend-settings` work remains unfinished and is preserved. Existing unrelated Kubo changes include knowledge snapshots, settings tasks and security/build evidence. Web UI and Desktop were clean at initial inspection. No source implementation was dispatched in this assessment.

## Implementation status

| Area | Status | Evidence and gap |
|---|---|---|
| Confirmation backend | PARTIAL | `mcp/server/broker.go`, `gate.go`, `elicit.go` enforce approval binding, expiry and single use. Broker claim marks `executed` before the actual handler runs; this cannot mean Done. Outcome receipts, digest presentation and resolver attribution are missing. |
| Shared decision UI | PARTIAL | `mcp/ui/src/approval/` and components provide five-state confirmation UI. New states, provenance, recovery controls and receipts are missing. Known execution failure has no honest entry in the proposed ten-state table. |
| Workspace shell | MISSING | Web UI has working legacy IPFS routes and branding, but lacks `src/shell/` and `src/activity/`. Existing agent chat uses an isolated React 19 runtime within a React 16 shell. Operator now requires React 19 throughout. |
| Context inspector | MISSING | Existing chat consumes AG-UI but lacks inspector and removable page-context contract. `agui_wire.go` ignores context input and the UAR proxy sends empty context. Tool facts must derive from actual run offers and retained prompt snapshots. |
| Decision inbox/activity | MISSING | Broker and agent sessions are bounded in memory; no identity-scoped decision list/history or idle event subscription exists. AG-UI run streams terminate at interrupts. |
| MCP App/A2UI seam | PARTIAL | Five existing app views, A2UI catalog and tool surfaces are reusable. Design incorrectly targets a port interface as a message implementation; actual messaging uses ext-apps SDK. |
| Desktop attention | PARTIAL | Sidecar, pairing, native confirmations, main-process arming and Touch ID already exist. Proposed attention queues and shared decision metadata are absent. Preserve native authority and the 1.5-second focused arming period. |
| Compatibility and operator control | PARTIAL | Existing protections remain authoritative. New routes, data, outcomes and UI need additive contracts, off switches and disabled-state coverage. |

## Baseline checks

- PASS: 17 focused existing backend tests covering broker expiry and concurrent resolution, single-use approval, native prompt constraints, session quotas, same-principal/different-subject isolation, and rejection of cross-session approval. Package time 2.397 seconds, exit 0; no race suite or integration daemon.
- Backend command: `go test ./mcp/server -run '^(TestBroker(ExpiryIsDenial|ApprovedButUnclaimedExpires|DoubleApprovalRunsOnce|ConcurrentResolversOneWins|NativePromptRefusesChatApproval)|TestTokenFlowRequiresApproval|TestExternalApprovalRunsExactlyOnce|TestDeniedConfirmationIsFinal|TestApprovedButExpiredDoesNotRun|TestUIDestructiveWithoutNativePromptIsRefusedEvenWithElicitation|TestAGUITwoSubjectsOnePrincipal(CannotShareAThread|CannotAnswerEachOthersInterrupt|NeverEvictEachOther)|TestPrincipalSessionQuotaEvictsOnlyItsOwn|TestFullTableRefusesInsteadOfEvictingAnotherPrincipal|TestAGUICannotApproveAConfirmationOfAnMCPSession|TestAGUIAnotherThreadsInterruptCannotBeNamed)$' -count=1 -timeout=120s -v`.
- UNKNOWN: UI component baseline did not run because `mcp/ui/node_modules` is absent and `vitest` is unavailable. Chat baseline stopped at the CSS prerequisite because `postcss` is absent.
- UI attempts: `pnpm --dir mcp/ui test tests/components/confirmation.test.tsx`; `pnpm --dir mcp/chat test tests/unit/session.test.ts tests/unit/session-mode.test.ts tests/unit/interrupt-ui.test.tsx`.
- The UI engineer reverted only an incidental Corepack `packageManager` insertion. Subsequent commands must use `COREPACK_ENABLE_AUTO_PIN=0`.
- UNKNOWN: full builds, lint, visual comparison, accessibility and real-service flows have not run in this phase. Existing passing tests do not prove the new features.
- Model preflight is degraded: judge and critic resolve to the same configured MiniMax model. This is recorded as a review limitation, not an implementation failure.

## Adopted outcome inputs

The operator wants a person to use one agentic workspace across Web UI, MCP App and Desktop: provide visible removable context, run the configured assistant, review accurately described writes, and see truthful decisions and outcomes without losing existing IPFS workflows.

Confirmed constraints and exclusions are recorded in `decision-log.md`. All React runtimes must migrate to React 19. Support both built-in and UAR backends; retain current-identity visibility, existing write-confirmation authority, compatibility and off switches. History is intentionally ephemeral and bounded. Publishing, deployment, additional prototype-only features and local-file ingestion are excluded.

Supported-environment evidence must cover browser desktop/mobile layouts, both themes and Electron integration. Real native-presence hardware verification remains unproven and must be reported honestly. Exact browser/version matrix and migration dependency coverage remain to be established.

Decision owner is the operator. No acceptance of unresolved security or compatibility failures has been inferred.

## Prior lessons applied

`prior-context.md` records: “Nothing was opened in a browser; the only checks were a JavaScript syntax check on the prototype and validate --strict on the OpenSpec changes.” Therefore prototype approval is design authority, not behavioral test evidence.

It also records: “Visual system is my assumption, not your choice.” The operator has now explicitly confirmed Praxis roles with Promethity values (UI-01).

The prior plan lesson includes assumed defaults and unfinished review. It is historical information; current operator answers and current repository evidence supersede those assumptions. Preserve the outstanding parent work rather than declaring it complete.

Knowledge gaps: none recorded by the recall hook. Newly discovered engineering gaps are listed above and are not asserted to be completed research.

## React 19 migration inventory

The UI engineer inspected manifests, lockfiles, source and current primary documentation. All six relevant package roots lack `node_modules`; versions below describe pins, not verified installed runtimes.

| Surface | Baseline | Required migration |
|---|---|---|
| Web UI | React/DOM 16.14.0; React types 17; TypeScript 4.9.5; CRA 5.0.1; Jest 29.7; Enzyme React 16 setup; Storybook 6.5 | React 19 mounting, modern context binding, compatible controls, JSX transform/types, test/build migration |
| MCP UI, App, Chat | React/DOM 19.3.0; React types 19.3.0; TypeScript 7.0.2; Vite 8.3.4; Vitest 5.0.3 | Normalize exact React pins and verify consuming builds |
| Desktop | Electron 44.3.0; bundled Web UI/chat; imperative confirmation, pairing and search renderers | Shared React 19 rendering for these live windows; retain main/preload security enforcement |
| WebMCP/OpenCode package | No React renderer | Do not introduce a React dependency without a rendering need |

Concrete blockers:

- `ipfs-webui/src/index.js` and `src/status/NodeBandwidthChart.js` use removed `ReactDOM.render`.
- `src/files/files-list/FilesList.js` and `src/files/FilesPage.js` use removed `findDOMNode`.
- `redux-bundler-react@1.2.0` uses removed legacy context despite a permissive React peer range. Replace the React binding using modern context/store subscriptions while retaining existing selectors, actions and RPC behavior.
- Function component defaults, React type namespaces/ref initialization and the JSX transform need migration.
- Test setup initializes an Enzyme React 16 adapter; boot tests mock the old root API.
- Editors, charts, forms, virtualized lists, tours and several smaller presentation dependencies need compatible versions or replacements.
- Drag-and-drop code accesses React DnD internals. Preserve folder hierarchy, streaming and progress rather than substituting a generic drop handler.
- Latest inspected `ipld-explorer-components` still includes old React-oriented dependencies. A simple version bump is not sufficient.

The operator's React 19 requirement supersedes the old React 16 and Explore no-change boundaries in `PRODUCT.md` and `DESIGN.md` only as necessary for migration. Record explicit design amendments in the foundation change. Preserve the existing explorer's user operations and the custom-element security/credential boundary; equal React versions do not remove CSP or host isolation needs.

Recommended foundation: Vite/Vitest, compatible shared React pins, modern state binding, and a source-controlled minimal Explore compatibility fork. Q14 and Q15 ask the operator to settle the two material maintenance choices. Registry metadata reported React/DOM 19.3.1 at assessment time; exact pins must be verified and recorded during Analyze, not adopted solely from latest-version availability.

Documentation checked by the UI engineer through Context7 and primary sources:

- [React 19 upgrade guide](https://react.dev/blog/2024/04/25/react-19-upgrade-guide): removed APIs, context, defaults and type/test changes.
- [Create React App retirement](https://react.dev/blog/2025/02/14/sunsetting-create-react-app): toolchain maintenance context.
- [Published redux-bundler-react source](https://unpkg.com/redux-bundler-react@1.2.0/index.m.js): actual legacy context implementation.

## Remaining assessment work

1. Finish the requested interview and record shared understanding.
2. Settle React 19 toolchain and Explore maintenance choices (Q14/Q15); the migration inventory is complete at source level.
3. Finalize adopted outcome/environment requirements.
4. Vet the completed assessment and write the assess handoff before entering Analyze.

Analyze, Plan, Execute and Reflect have not been completed or skipped.
