# Proposal: mcp-agui-endpoint

Phase `ipfs-mcp-server`, plan revision 2 (new change). N12: AG-UI endpoint served by `ipfs mcp --http` (G9, topology A). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G9 asks for AG-UI so agent UIs (the Web UI chat island, Desktop) can drive the node agentically. The operator chose to serve it from `ipfs mcp`, reusing the AuthVerifier, scope map and confirmation gate in-process.

## What Changes

- First, verify the AG-UI protocol from primary sources (RunAgentInput, event types, interrupts and resume) and record them in `mcp/server/agui/SOURCES.md`; validate emitted events against the published `@ag-ui/core` schemas (G9-1).
- `POST /agui` on the `--http` listener (default `127.0.0.1:5050`), behind `--agui` (off by default) and mounted only when an LLM backend is configured (N11): RunAgentInput in, spec events over SSE out, hand-written types over `net/http`.
- A pending confirmation ends the run with `RUN_FINISHED` `outcome: {type: "interrupt", interrupts: [...]}`; the resume input carries `resume: [{interruptId, status, payload}]`. An interrupt answered in the chat island counts as the human gate for the `ui` principal, the same way elicitation does; destructive tools additionally raise the native prompt (N14). Other principals are resolved by the native prompt.
- Origin allowlist: config `agent.origin_allowlist`, **empty by default** (Round 7 correction; the config schema and ADR-0005 always said empty). With the default, every `/agui` request that carries an `Origin` header gets 403 with no CORS headers; requests without `Origin` (non-browser clients) are checked by bearer only. To enable the Web UI chat the operator adds `webui://localhost`, `http://127.0.0.1:5001` or `http://localhost:5001`, and any subdomain-gateway origins they serve the Web UI from. Those values are documented examples, never defaults.
- Client `tools`, `context`, `state` and `forwardedProps` in `RunAgentInput` are accepted and ignored (real clients send `tools: []`).

## Capabilities

### New Capabilities

- `mcp-agui-endpoint`: N12: AG-UI endpoint served by `ipfs mcp --http` (G9, topology A)

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`. No daemon listener and no `/api/v0` change. Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.
