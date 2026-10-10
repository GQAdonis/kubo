# Tasks: mcp-llm-proxy-loop

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-llm-proxy-loop`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-agent-principal-enforcement`, `mcp-pinning-with-confirmation`. Scheduled in Round 6.

## 1. LLM proxy and loop

- [x] 1.1 Implement backend configuration (`--llm-*` flags and config keys, no defaults, key from env or file) and the `noagent` build-tag stub. Verify: unit tests show nothing mounted and no dialer call when unset, a literal key rejected, and `make build GOTAGS=noagent` builds without agent code
- [x] 1.2 Implement the OpenAI-compatible chat-completions client with `tools`, SSE streaming and tool-call assembly over the standard library. Verify: unit tests against recorded fixtures including split tool-call deltas; `go list -m all` shows no new module
- [x] 1.3 Implement in-process egress enforcement for `contentEgress` results with the structured refusal. Verify: unit tests with a fake model server show refused content never reaches the request body (G10-4)
- [x] 1.4 Implement the bounded loop: limits, principal-scoped contract tools only, untrusted-content framing, in-memory sessions, no content logging. Verify: unit tests with a scripted model cover each limit and the offered tool set
- [x] 1.5 Map write and destructive calls to pending confirmations with a transport-neutral interrupt and resume; denial and expiry return denied results. Verify: unit tests show a denied call never reaches the RPC fake
- [x] 1.6 Add test/cli falsifier tests G10-1 to G10-4 on dynamically chosen free ports: tool-call round trip with Ollama (and the pinned published UAR release when available), zero outbound connections when unconfigured, egress refusal. Verify: `make build && go test ./test/cli/... -run TestMCPAgentLLM -v` passes with a recorded Ollama run
