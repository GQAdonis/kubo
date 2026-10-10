# Agentic endpoints: the agent loop, AG-UI and A2A

`ipfs mcp` can run a small, bounded agent loop against a model server you choose, and serve it to chat UIs over [AG-UI](https://docs.ag-ui.com/) (`POST /agui`) and to other agents over [A2A 1.0](https://a2a-protocol.org/) (`POST /a2a` and the agent card). All of it is off unless you configure a model backend, and all of it can be compiled out. The decision record is [ADR-0004](0004-agentic-endpoints-and-llm.md); the chat UI is [ADR-0005](0005-agent-chat-ui.md). Every key named here is in the [configuration reference](config-reference.md); the basics (install, `--http`, principals, confirmation) are in the [user guide](user-guide.md).

## Contents

- [Topologies](#topologies)
- [The model backend](#the-model-backend)
- [Principals and `egress_ack`](#principals-and-egress_ack)
- [AG-UI](#ag-ui)
- [A2A](#a2a)
- [The loop's limits](#the-loops-limits)
- [Confirmation for agents](#confirmation-for-agents)
- [What `egress_ack` cannot see](#what-egress_ack-cannot-see)
- [Off switches](#off-switches)

## Topologies

**Topology A (delivered).** `ipfs mcp --http` hosts everything on one listener, by default `127.0.0.1:5050`:

| Path | Protocol | Who may call it |
|---|---|---|
| `/mcp` | MCP Streamable HTTP | principals of kind `human-mcp`, `ui`, `uar`; the operator token |
| `/agui` | AG-UI over SSE | principals of kind `ui` only |
| `/a2a` | A2A 1.0 JSON-RPC | principals of kind `a2a` only |
| `/.well-known/agent-card.json` | A2A agent card | anyone who can reach the listener (no token) |

The loop runs inside `ipfs mcp` and calls the model server you configure. A token for one path's principal kind does not open another path.

**Topology B (deferred).** UAR would host AG-UI and A2A and call `ipfs mcp --http` as a configured principal of kind `uar`. It is deferred because no published UAR release met the conformance gate ([ADR-0008](0008-topology-b-gate-no-go.md)). `ipfs mcp check-uar` and the `uar` principal block exist, but a `uar` principal with `"check": "verify"` is refused while no UAR settings endpoint is pinned in the contract. UAR can still be used in topology A as an ordinary OpenAI-compatible model backend (next section).

## The model backend

The loop talks to any server with the OpenAI-compatible chat-completions API and tool calling (`POST <base_url>/chat/completions` with `tools`): a local server such as Ollama or llama.cpp's server, UAR's `/v1/chat/completions`, or a hosted service. **There is no default URL and no default model.** Naming only one of the two is an error; naming neither leaves the agent off, mounts nothing and makes no model call.

On the command line:

```bash
ipfs mcp --http --agui \
  --llm-base-url http://127.0.0.1:11434/v1 \
  --llm-model qwen3:8b \
  --mcp-config ./ipfs-mcp.json
```

Or in the config file (flags win over the file):

```json
{
  "llm": {
    "base_url": "https://models.internal.example/v1",
    "model": "my-model",
    "api_key": { "env": "LLM_API_KEY" },
    "timeout": "60s"
  }
}
```

- `llm.api_key` (or `--llm-api-key-env NAME` / `--llm-api-key-file /abs/path`) is a reference; a literal key is rejected and never echoed. The key is never sent over plain `http` to a non-loopback host.
- `llm.base_url` must not carry credentials, a query or a fragment. Redirects are not followed.
- `llm.timeout` bounds each model request (default `60s`).
- The loop has been run against Ollama with `qwen3:8b` (tool calling worked in 5 of 5 runs) and `llama3.2:1b` (3 of 6). Small models call tools unreliably; prefer a model with good tool-calling support. UAR as the backend was not tested.
- On start, `ipfs mcp` prints `agent loop enabled: ...` to stderr with the backend it will use.

## Principals and `egress_ack`

AG-UI and A2A callers are always configured principals (`principals` in the config file); the operator token and the anonymous loopback listener cannot use `/agui` or `/a2a`.

```json
{
  "http": { "enabled": true, "listen": "127.0.0.1:5050" },
  "principals": [
    {
      "name": "webui-chat",
      "kind": "ui",
      "credential": { "env": "IPFS_MCP_UI_TOKEN" },
      "write_groups": ["pin", "content"],
      "egress_ack": ["local"]
    },
    {
      "name": "peer-agent",
      "kind": "a2a",
      "credential": { "file": "/etc/ipfs-mcp/peer-agent.token" },
      "groups": ["node", "pin"],
      "egress_ack": ["local"]
    }
  ],
  "llm": { "base_url": "http://127.0.0.1:11434/v1", "model": "qwen3:8b" },
  "agent": { "agui": true, "a2a": true, "origin_allowlist": ["http://127.0.0.1:5001"] }
}
```

- Every principal is read-only until a group is named in `write_groups`; destructive tools need `destructive_groups`.
- `egress_ack` lists the hosts the principal's node content may be sent to. A tool whose result carries node content (`contentEgress` in the catalog: `ipfs_cat`, `ipfs_get`, `dag_get`, `pin_ls`, `mfs_read`, `ipfs_index_search` and others) reaches the model only when the model host is covered. For `ui` and `a2a` principals the list must also cover the `llm` host and, when configured, `embeddings.embedder` and `embeddings.remote` hosts; otherwise the call is refused and the model receives `Content not sent: <host> is not in egress_ack.`
- `"local"` covers loopback hosts (`localhost`, `127.0.0.1`, `::1`). A host name covers any port unless the entry names one (`models.internal.example`, `10.0.0.5:8000`).
- **Decided 2026-10-10:** `swarm_peers`, `routing_*` and `key_list` are not `contentEgress`. A principal with read grants on the `diag`, `routing` or `names` groups can therefore send the node's peer list, routing answers and key names to the model backend, or receive them as an A2A peer, without `egress_ack`. Leave those groups out of `groups` if that is not acceptable ([ADR-0004](0004-agentic-endpoints-and-llm.md), 2026-10-10 amendment).

## AG-UI

`--agui` (or `agent.agui`) serves `POST /agui` on the `--http` listener. It is mounted only when `--http` and a model backend are both configured; otherwise `/agui` answers 404 and `ipfs mcp` prints that the flag is ignored.

- **Auth.** A bearer token of a principal of kind `ui`. Other kinds get 403; a missing or unknown token 401.
- **Browser origins.** A browser page may call `/agui` only from an origin in `agent.origin_allowlist` (empty by default). Any other `Origin`, including the listener's own and `http.allowed_origins`, gets 403. Requests without an `Origin` header need only the token.
- **Threads.** The `threadId` is the server-side session; the server keeps the history (30 minutes idle, 64 sessions, 256 messages) and reads only the last user message, so a client cannot forge tool results.
- **Writes.** A write ends the run with one interrupt; a user gesture in the chat approves or denies it. A destructive call is never approved by the chat: it waits for the system prompt or the paired Desktop window.

The chat UIs that use it are described in [chat setup](chat-setup.md).

## A2A

`--a2a` (or `agent.a2a`) serves the A2A 1.0 JSON-RPC endpoint `POST /a2a` and the agent card at `/.well-known/agent-card.json` on the `--http` listener, under the same conditions as `/agui` (both answer 404 otherwise).

```bash
curl -s http://127.0.0.1:5050/.well-known/agent-card.json
```

- **Auth.** A bearer token of a principal of kind `a2a`. With no such principal, `ipfs mcp` warns at start that `/a2a` will refuse every caller.
- **Methods.** `SendMessage`, `SendStreamingMessage`, `GetTask` and `CancelTask`. The card's skills come from the contract and list only what the configured peers may use.
- **Writes.** An A2A peer can never approve a change. A write or destructive call leaves the task in the input-required state until a person approves it on this machine (the system prompt or the paired Desktop window); a deny, an expiry or a `CancelTask` ends it as rejected. `ipfs mcp confirm` cannot approve a destructive A2A call.
- **Errors.** JSON-RPC errors carry the shared `ToolError` in `error.data`, selected by `@type` `type.googleapis.com/ipfs.mcp.ToolError` (ADR-0004, Round 7).

## The loop's limits

| Key | Default | Effect |
|---|---|---|
| `agent.loop.max_steps` | `8` | Model turns per run. |
| `agent.loop.max_tokens` | `32768` | Token budget per run. |
| `agent.loop.wall_clock` | `120s` | Time per run. |

The loop's only tools are the contract tools the calling principal may use; there is no shell, filesystem or network tool. Results that can carry third-party text are framed as untrusted data. Sessions live in memory only, and logs carry outcomes and counts, never conversation content. A limit ends the run with an error that names it.

## Confirmation for agents

Writes by agents are confirmed like any other ([user guide](user-guide.md#confirmation-and-its-limits)). What differs:

| Caller | Write | Destructive |
|---|---|---|
| `ui` (chat) | the chat's approve gesture, or the system prompt / Desktop | system prompt or Desktop only |
| `a2a` | system prompt or Desktop only | system prompt or Desktop only |
| `human-mcp` client that elicits | elicitation | elicitation |
| `human-mcp` client that cannot elicit | system prompt or Desktop (or `ipfs mcp confirm` under `--confirm=cli`) | same |

The native prompt per OS:

- **macOS:** LocalAuthentication (Touch ID, Apple Watch or the login password) through `/usr/bin/swift`, which needs the Xcode command line tools (`xcode-select --install`). On by default.
- **Linux:** polkit, after root installs the action from `mcp/server/confirm/polkit/` (`INSTALL.md`) and with an authentication agent in the session. Not yet run on a Linux machine.
- **Windows:** no native prompt; only a paired IPFS Desktop window, which is not delivered yet. Non-eliciting writes are refused there.
- **Headless:** `--confirm=cli` on the server and `ipfs mcp confirm [<id>]` in a terminal, which reads the answer from the TTY only. It has no presence factor beyond the terminal, never approves destructive calls from `ui` or `a2a` principals, and requires `"shell_capable": false` on every principal with write grants.

The real-hardware check that a same-user process cannot approve the native prompt (HG-1) is still pending.

## What `egress_ack` cannot see

`egress_ack` controls what `ipfs mcp` itself sends. It cannot see what happens afterwards:

- a loopback model server that forwards requests upstream;
- a UAR backend whose upstream provider `ipfs mcp` cannot read (its per-session check is deferred with topology B);
- **stdio MCP hosts** (Claude Code, Codex, OpenCode) that send tool results to their own models: over stdio `ipfs mcp` cannot identify the client and applies no `egress_ack`. A stdio client whose `clientInfo` says it is UAR is refused content tools, but that name is self-reported, so this is a misconfiguration guard, not a boundary;
- WebMCP tools running in a browser (not delivered).

An acknowledged but misconfigured endpoint leaks by the operator's choice. No part of this feature claims that no content leaves the machine.

## Off switches

| To turn off | Do this |
|---|---|
| The agent loop | Leave `llm`, `--llm-base-url` and `--llm-model` unset (the default). |
| AG-UI | Leave `--agui` and `agent.agui` unset (default off). |
| A2A and the agent card | Leave `--a2a` and `agent.a2a` unset (default off). |
| Browser access to `/agui` | Keep `agent.origin_allowlist` empty (default). |
| All agent code | `make build GOTAGS=noagent`; `--llm-*`, `--agui` and `--a2a` then do not exist and an `llm` section is refused. |
| A principal | Remove its entry or its credential. |
