# Design: praxis-context-inspector

## Context

See `proposal.md`. The chat island (`mcp/chat`) renders AG-UI events from `/agui`. The server knows which tools it offers (`Server.ToolNames()`, `Server.UnavailableTools()`) and builds the system prompt (`agent_frame.go`), but none of this reaches the person. The Web UI knows which page, CID or path is open, but does not pass it.

## Goals / Non-Goals

**Goals:**
- The person sees, before sending, what goes with the message, and can remove items they do not want sent.
- Every displayed fact has a stated source.
- Each turn names its phase.

**Non-Goals:**
- Budget arithmetic, truncation or ranking (CX-02..05, agent backend).
- Memory. Memory scope reads "None" because the IPFS agent has no memory store.
- Event persistence or replay (WS-02 follow-up).

## Decisions

- **Provenance labels.** Each strip item carries one of: "From this page", "Added by you", "From settings", "Reported by the server". Server facts come only from the run-start payload; the island never infers a tool count from its own list.
- **Removal semantics.** Container and attached files are removable; removal drops them from the next request. Tools offered and model placement are not removable here because they are settings; the item links to `#/settings/agent`.
- **Run-start facts.** The AG-UI run-start payload gains `offered_tools` (names) and `catalog_size` (total tools in the contract), and `system_prompt_digest` (`sha256:` short form). The strip renders "N of M tools offered" from these. The system prompt text itself is not sent to the client.
- **Inspector sections.** Overview, Context, Prompt plan, Events. Events is the current run only, read-only, with event type, time and a one-line summary; tool arguments are shown as they appear in the tool-call card, not raw.
- **Lifecycle strip from events.** `RUN_STARTED` → Planning; `TOOL_CALL_START` → Calling `<tool>`; interrupt with a confirmation → Waiting for approval; tool result streaming → Running; `RUN_FINISHED` → Done; `RUN_ERROR` → Failed; cancellation → Cancelled. A phase with no matching event is not shown.
- **Container from the Web UI.** The bridge sends `{route, cid?, path?}`; the island renders it as the container item. Nothing is sent when the person removes it, and it is re-added only when the route changes.

## Risks / Trade-offs

- [Container leaks a path the person did not mean to share] → shown before send, removable, and sent only to the configured backend under the existing egress rules; the egress badge on the strip names the host.
- [Event list grows long] → current run only, virtualized above 200 rows.
- [Payload change on run-start] → additive; older islands ignore it, newer islands show "Not reported by this server" when absent.
