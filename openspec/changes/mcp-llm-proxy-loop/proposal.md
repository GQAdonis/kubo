# Proposal: mcp-llm-proxy-loop

Phase `ipfs-mcp-server`, plan revision 2 (new change). N11: G10 L3, an OpenAI-compatible model proxy and a bounded agent loop inside `ipfs mcp`. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Operator answer (Both, option a + L3): `ipfs mcp` serves the agentic endpoints and, when an LLM is needed, proxies model calls to an operator-configured UAR instance or OpenAI-compatible server. A raw OpenAI-compatible server does not call MCP tools, so `ipfs mcp` runs a minimal, bounded loop over contract tools only. UAR serves `/v1/chat/completions` (`uar:src/server.rs:1901`), so one client covers both.

## What Changes

- Configuration: `--llm-base-url`, `--llm-model`, `--llm-api-key-env=<VAR>` or `--llm-api-key-file`, or the same keys in the `ipfs mcp` config file. No default URL or model; a literal key is rejected. Unset means the agent surfaces are not mounted and no model call is ever made.
- An OpenAI-compatible chat-completions client with `tools`, SSE streaming and tool-call assembly, standard library only.
- Egress enforced in-process: a `contentEgress` tool result is included in a model request only if the backend host is in `egress_ack`; otherwise the loop returns a structured refusal ("Content not sent: `<host>` is not in egress_ack."). With a UAR backend, the per-session check of UAR's effective upstream provider is not part of this change: it moved to `agentic-uar-integration` (task 1.5, deferred with topology B by ADR-0008) because it needs the same pinned UAR settings endpoint, which no published UAR release provides. Until then a UAR backend is treated like any other backend: the operator lists its host in `egress_ack`, and the ADR-0004 limitation (Kubo cannot see where the backend forwards content) applies.
- A bounded loop: step limit, token budget and wall-clock limit with conservative defaults; tools are only the contract tools enabled for the calling principal; no shell, filesystem or network tools; write and destructive calls go through the confirmation gate and surface as transport-neutral interrupts; in-memory sessions; no content in logs.
- Everything is compiled out by the `noagent` build tag (and `nomcp`).
- Falsifier tests G10-1 to G10-4: UAR (pinned published release, when available) and one named local server (Ollama) for the tool-call round trip; no model call without configuration (network capture); `egress_ack` enforced for every `contentEgress` result.

## Capabilities

### New Capabilities

- `mcp-llm-proxy-loop`: N11: G10 L3, an OpenAI-compatible model proxy and a bounded agent loop inside `ipfs mcp`

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `cmd/ipfs/kubo/mcp*.go` (flags, `noagent` stub), `test/cli/mcp_agent_*_test.go`. No `/api/v0`, gateway, daemon or `config/` change. No new Go module.
