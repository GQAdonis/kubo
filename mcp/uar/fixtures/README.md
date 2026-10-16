# UAR v1.1.0 fixtures

Server-sent-event streams of `POST /api/uar/ag-ui/agents/{agent_id}`, one file per scenario, used by `mcp/server/uar_proxy_test.go` and `test/cli/mcp_uar_backend_fake_test.go`.

They are derived from the pinned contract `mcp/uar/v1.1.0-kubo.md` ("Complete AG-UI interrupt contract"), not recorded from a running instance: a tool approval interrupt carries `id`, `reason: "tool_approval"`, `message`, `toolCallId`, `expiresAt` and `responseSchema` (an object requiring `approved`); an elicitation interrupt carries `reason: "mcp_elicitation"`, `responseSchema` with `action` and `content`, and `metadata.uar` with `serverName`, `request` (form mode with `requestedSchema`, or URL mode with `url` and `elicitationId`) and `resolvable`. Kubo's own confirmation is a form with one required boolean, `confirm`.

| File | Scenario |
|---|---|
| `run-text.sse` | a run that finishes with a text reply |
| `run-tool-approval-interrupt.sse` | a `pin_add` call waiting for `tool_approval` |
| `run-elicitation-interrupt.sse` | a form elicitation (kubo's yes/no confirmation) |
| `run-both-interrupts.sse` | both reasons in one segment |
| `run-url-elicitation-interrupt.sse`, `run-form-elicitation-interrupt.sse`, `run-unresolvable-elicitation-interrupt.sse` | interrupts the chat cannot show |
| `run-resumed-executed.sse`, `run-resumed-denied.sse` | the segment after an approving and a denying resume |
| `run-error.sse` | `RUN_ERROR` |
| `run-hostile-events.sse` | event types and fields the proxy must not relay (`CUSTOM` with an A2UI surface, `STATE_*`, `MESSAGES_SNAPSHOT`, `RAW`, `STEP_*`, UAR thread and run ids on non-`RUN_*` events) |
| `run-leaks-secrets.sse`, `run-leaks-secrets-interrupt.sse` | the UAR key and the run grant in text, tool names and results, `RUN_ERROR` and an approval message; the two secrets are the constants of `uar_proxy_test.go` |

The agent, skill and binding replies are built by the test double in `mcp/server/uar_fake_test.go` from the shapes UAR v1.1.0 accepted in its own release acceptance and from its route handlers (`mcp/uar/v1.1.0-image-config.md`; every shape in `mcp/server/uar_provision.go` cites the UAR file and line at the tag). They are still a double: `agent-backend-uar` 1.5 (`test/cli/mcp_uar_backend_test.go`, opt-in with the UAR and liter-llm environment) is the first run against the published image, and it has not run.
