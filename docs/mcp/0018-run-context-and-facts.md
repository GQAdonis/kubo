# ADR 0018: Bounded references and actual run facts

Status: accepted for production authoring, 2026-10-20. Canonical task: `agentic-ui-implementation/praxis-context-inspector/1.1`. Runtime acceptance remains at the phase's final verification boundary.

## Decision

Use the optional `forwardedProps.ipfs.context` branch for at most 16 visible/removable references and 16 KiB of encoded reference data. Route, CID and logical IPFS/IPNS/MFS references are validated and framed as untrusted data before either backend. They confer no tool, filesystem, resolver or egress authority. In particular, MFS is the connected node's logical filesystem; it is never the host filesystem. Invalid context is a visible 422 payload error. Validate the independent A2UI offer separately so its incompatibility disables only A2UI.

Use a standard AG-UI `CUSTOM ipfs.run.facts` event with explicit thread/run identity and optional, sourced facts. Emit after standard `RUN_STARTED` and optional immediate `a2ui.catalog`, before tool/text events. This supersedes historical task-title wording that adds custom fields directly to run start, which ordinary SDK validators may strip. The builtin tool list comes from the actual effective offer, and the prompt digest from the exact retained selected run snapshot. UAR's unreported prompt, tools, catalog size or memory remain absent. No fabricated budgets or defaults.

The current-run inspector retains only bounded safe projections, discloses truncation and unknown status, and cannot convert approval to success. The new `agent.context_references` and `agent.run_facts` off switches preserve ordinary messaging and all existing gates.

The accompanying D amendment places reviewed-effect metadata only at supported wire locations: pinned Go SDK v1.8.0 `ElicitParams.Meta`, `ElicitResult.Meta` and `CallToolParams.Meta`; AG-UI resume entry outside payload/tool arguments; optional field in the existing authenticated signed native resolve body. The exact schemas and legacy error precedence live in the confirmation contract. No private host nonce handshake or mandatory client migration is introduced.

## Sources and consequences

The canonical definitions and implementation handoff are `mcp/contract/agentic/run-context.schema.json` and `run-context.md`; `mcp/contract/fixtures/agui.schema.json` owns the event dispatcher. The existing `Agent.offer(eff)`, retained `run.snap.systemPrompt`, configured backend snapshot and untrusted framing establish source authority. A global tool registry and newly reconstructed prompt do not.

References can remain in previously accepted conversation text after removal; the UI promises omission from later turns, not retroactive erasure. Native observation-only continuations may report no new facts. Provenance describes the source of a fact, not proof that the browser showed a chip or that an operation succeeded. All server/UI generation and runtime evidence are later task responsibilities.
