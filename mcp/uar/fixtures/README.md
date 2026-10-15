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

The agent, skill and binding replies that the contract does not show are built by the test double in `mcp/server/uar_fake_test.go`; `agent-backend-uar` 1.5 (`test/cli/mcp_uar_backend_test.go`) is the first check of them against the published image.
