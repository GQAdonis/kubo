# Tasks: praxis-decision-model-v2

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-decision-model-v2`. Decision D5 in `analysis.md`.
Change-level prerequisites (child plan r1): none. Scheduled in Round 1.

## 1. Contract

- [ ] 1.1 Add optional `effect_digest` (`sha256:` + 64 hex), `destination` (`kind`: `node` | `pinning_service` | `ipns_name`, `name`) and `resolved_by` (Resolver enum) to the pending-confirmation schema in `mcp/contract/schemas`, and regenerate `mcp/ui/src/contract/generated/confirmation.ts`. Verify: contract tests pass, and a fixture without the new fields still validates
- [ ] 1.2 Server computes the effect digest over the canonical JSON of tool name and arguments, emits it with `destination`, and rejects a confirm whose digest no longer matches with a distinct digest-mismatch error. Verify: a Go test changes one argument between prompt and confirm and gets the digest-mismatch error; the call does not run
- [ ] 1.3 Server sets `resolved_by` on every state change it reports. Verify: a test resolving through `desktop` sees `resolved_by: "desktop"` on the AG-UI and MCP state update

## 2. Model

- [ ] 2.1 Extend `APPROVAL_PHASES`, `PHASE_WORD` and `phaseSentence` in `mcp/ui/src/approval/model.ts` to the ten states with the badge words from the spec table; add a `muted` tone for `resolved_elsewhere`. Verify: a table-driven test asserts each state's word, tone and control set against one fixture table
- [ ] 2.2 Map contract states to model states (`executed` → `committed`; `pending` + `requires_native_prompt` → `waiting_system_prompt`) and derive `invalidated`, `resolved_elsewhere`, `uncertain` (configurable outcome timeout, default 30s) and `needs_resubmit`. Verify: unit tests cover each derivation, including a fake clock for the timeout
- [ ] 2.3 Add the card fields to `ApprovalModelSchema`: object, destination, scope, class in words, named authority, egress, short digest formatter. Verify: the formatter returns `sha256:7f3a…c1` for a known digest, and a model without a digest renders the "no digest supplied" row

## 3. Components and catalog

- [ ] 3.1 Update `ApprovalCard`, `ApprovalPanel` and `ConfirmationDialog` to render the ten states, the card fields and the controls (Deny focused, Approve after 1s arm, Ask again, Review again, Check now, View receipt). Verify: component tests render all ten fixtures, and Approve does nothing for the first second after render or content change
- [ ] 3.2 "Ask again" requests a new confirmation and never re-sends the old confirm token. Verify: a test spies on the outbound call and finds a new request without the old token
- [ ] 3.3 Regenerate `mcp/ui/catalog/ipfs-mcp.catalog.json` with `pnpm gen:catalog` and extend catalog validation to require all ten states on `ApprovalCard` and `ConfirmationDialog`. Verify: validation passes, and fails when a state is removed from the catalog
- [ ] 3.4 Update the MCP App views in `mcp/app/src/**` that render a confirmation to use the new model. Verify: the views test renders a `pin_rm` confirmation in `pending`, `approved` and `committed`
- [ ] 3.5 Run WCAG 2.2 AA (axe, keyboard, reduced motion) on all ten states in light and dark, and the no-remote-assets scan. Verify: no AA violation and no external URL; each state's word is present without relying on colour

## 4. Docs

- [ ] 4.1 Replace the five-row "Confirmation states (trust surface)" table in `DESIGN.md` with the ten-row table and the card field list, and change the `approved` copy rule to "Approved, not yet run". Verify: the table matches `PHASE_WORD` (a docs test or a reviewer diff), and no banned word appears
