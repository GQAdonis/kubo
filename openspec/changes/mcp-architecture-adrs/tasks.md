# Tasks: mcp-architecture-adrs

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS (plan revision 2), phase path `ipfs-mcp-server`, change ID `mcp-architecture-adrs`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `team-ownership-grant`. Scheduled in Round 1.

## 1. Architecture decision records

- [x] 1.1 Write `docs/mcp/0001-mcp-server-placement.md` from the assessment ADR DRAFT, with the status taken from the Q1 answer. Verify: the ADR cites the recorded Q1 answer, and every rejected alternative has evidence
- [x] 1.2 Write `docs/mcp/0002-webmcp-web-ui.md`, with the status set from the spike report (Accepted per GO host, or Deferred). Verify: the status line cites the spike report path
- [x] 1.3 Write `docs/mcp/0003-confirmation-and-auth.md` covering elicitation, the token fallback and its limits, `AuthVerifier`, scopes and RFC 9728. Verify: the ADR names the go-sdk `auth.TokenVerifier` mapping and the T2 limitation
- [x] 1.4 Write `docs/mcp/0004-agentic-endpoints-and-llm.md` from the addendum revision-3 ADR-0004 draft: topology A (AG-UI and A2A in `ipfs mcp --http` on `127.0.0.1:5050`), the L3 bounded loop and OpenAI-compatible proxy (UAR or other, no default), in-process egress, the human gate (elicitation, else the native prompt with a presence factor, CLI headless opt-in only), topology B gated on a published UAR release, `noagent`, and the cost of two topologies. Verify: the ADR records the goal-fit re-scoring that excluded UAR-hosted-only and cites `operator-answers.md`
- [x] 1.5 Write `docs/mcp/0005-agent-chat-ui.md` from the addendum ADR-0005 draft: the React 19 shadow-root island in the Web UI fork, Promethity, `ipfs mcp` `/agui` only, the Desktop fork changes, and the admin-origin risk with the U3 flip. Verify: rejected alternatives U1, U3 and U4 have evidence
- [x] 1.6 Write `docs/mcp/0006-embeddings-surrealdb.md` from the addendum ADR-0006 draft, completed with plan revision 2's re-score: `ipfs mcp` embeds and stores in both topologies; embedded = managed local `surreal start surrealkv://` from an operator-supplied binary; remote = operator URL; cgo excluded. Verify: the ADR cites the SurrealDB Go embedding docs fetched 2026-10-09
