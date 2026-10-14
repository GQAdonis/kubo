# Tasks: mcp-agent-settings-api

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `mcp-agent-settings-api`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: agent-settings-contract, mcp-agent-hot-reload. Scheduled in Round 3.

## 1. Tasks

- [x] 1.1 Principal kind `admin`: settings scope only, refused on `/mcp`, `/agui`, `/a2a`; a configured principal whose token environment variable or file is absent is disabled with a warning, not fatal. Acceptance: cross-path token tests (G9-4 pattern) and an absent-token test (server starts; the principal authenticates nothing) exist.
- [x] 1.2 `GET`/`PUT` settings with `ETag`/`If-Match`, shared validation, atomic apply through `ApplyAgentSettings`; never returns key material; values never logged. Acceptance: tests for 412 on a stale revision and for redaction in JSON and slog.
- [x] 1.3 Overlay persistence in `--settings-dir` (0600; flags > overlay > config); write-only key stored as a 0600 file becoming `SecretRef{File}`; `accept_secrets: false` refuses key fields with `secrets_not_accepted`; an absent `llm.api_key` file yields `api_key.set=false` and no startup failure. Acceptance: tests for precedence, file mode, refusal and the absent-file case.
- [x] 1.4 Test-connection (candidate URL only, https or loopback http, no credentials in URL, bounded timeout, nothing persisted) and model list. Acceptance: tests for no persistence and refusal of non-conforming URLs.
- [x] 1.5 Change events (SSE) carrying only the new revision. Acceptance: payload matches the contract schema.
- [x] 1.6 Egress extension (D4): only the newly chosen host, only with the typed host confirmation; on Desktop the bridge adds Touch ID. Acceptance: tests refuse any other host and a missing confirmation.
- [x] 1.7 Off switches and disabled states (W6): `agent.settings.enabled=false` and no admin principal give 404, nothing written, nothing dialed; `GOTAGS=noagent` removes the route. Acceptance: tests exist.
