# Proposal: mcp-a2a-endpoint

Phase `ipfs-mcp-server`, plan revision 2 (new change). N13: A2A 1.0 endpoint and agent card served by `ipfs mcp --http` (G9, topology A). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G9 asks for A2A so other agents can delegate IPFS work to the node. A2A callers are agents, so they can never approve their own writes: approval comes from the out-of-band human gate.

## What Changes

- First, verify A2A 1.0 from primary sources (agent card path, JSON-RPC methods, task-state wire values, security schemes) and validate against the `a2aproject/A2A` 1.0 JSON schema (G9-1). Hand-written types by default; `a2a-go/v2` only after a measured dependency review and an operator answer.
- Agent card at `/.well-known/agent-card.json` generated from the contract's A2A skill metadata, HTTP bearer security scheme; JSON-RPC at `/a2a`: `message/send`, `message/stream`, `tasks/get`, `tasks/cancel`.
- A pending confirmation is `TASK_STATE_INPUT_REQUIRED`; a missing or insufficient credential is `TASK_STATE_AUTH_REQUIRED`. No A2A message can approve a write; the native prompt (N14) decides.
- Behind `--a2a` (off by default), mounted only with a configured LLM backend.

## Capabilities

### New Capabilities

- `mcp-a2a-endpoint`: N13: A2A 1.0 endpoint and agent card served by `ipfs mcp --http` (G9, topology A)

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `test/cli/mcp_*_test.go`. Same listener as Streamable HTTP and AG-UI (default `127.0.0.1:5050`). Port rule (operator, 2026-10-09): no component defaults to 4000 (liter-llm), 1906-1908 or 50051 (UAR), 8181/8081 (openai-proxy), 11434 (Ollama), 23001/28000 (SurrealDB), 4545/4546, 5173, 5432, 6379 or 8080. Tests pick free ports dynamically and never hardcode them.
