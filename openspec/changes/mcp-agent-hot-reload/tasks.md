# Tasks: mcp-agent-hot-reload

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `mcp-agent-hot-reload`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: agent-settings-contract. Scheduled in Round 2.

## 1. Tasks

- [x] 1.1 Hold `LLMClient`, `backendHost`, limits and system prompt in one snapshot behind `atomic.Pointer`, read once per run; AG-UI and A2A read the same pointer; `sessions` and `buckets` survive swaps. Acceptance: grep of `agui.go` and `a2a.go` finds no captured `*Agent` backend field.
- [x] 1.2 A resumed interrupt reuses its run's original snapshot or fails with a stable code; the previous key is dropped once no run uses it; no pending confirmation is lost across a swap (R3). Acceptance (W9): tests named `TestAgentSwapResumeUsesOriginalBackend` (a resumed run calls the old fake backend's URL, never the new one), `TestAgentSwapMidStream` (the in-flight stream completes on the old backend and the next run uses the new one) and `TestAgentSwapKeepsPendingConfirmation` (a confirmation created before the swap still resolves exactly once afterwards).
- [x] 1.3 Mount `/agui` whenever enabled; with no backend it answers 503 `backend_not_configured`; update help text (`mcp_agent.go:39-56`) and the schema text. Acceptance: help text changed; `TestCommandDocsWidth` is on the gate list.
- [x] 1.4 Configurable system prompt with a fixed, non-removable safety prefix. Acceptance: a test asserts the prefix survives an empty or hostile override.
- [x] 1.5 Internal apply entry point (`ApplyAgentSettings(snapshot)`) using the same validation as `ResolveAgent`, plus a change-notification hook. Acceptance: grep shows one validation function used by startup and apply.
- [x] 1.6 Unit and `test/cli` tests for 1.1-1.5, plus disabled states (`--agui` off: 404; `GOTAGS=noagent` builds without the route). Acceptance: test files exist; run at the gate.
