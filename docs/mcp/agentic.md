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

**Topology B (not built).** UAR would host AG-UI and A2A itself and call `ipfs mcp --http` as a configured principal of kind `uar`, with the Web UI and Desktop talking to UAR directly. It stays unbuilt ([ADR-0008](0008-topology-b-gate-no-go.md), [ADR-0010](0010-uar-agent-behind-agui.md)): the island keeps one endpoint, `/agui`, and one token model. `ipfs mcp check-uar` and the `uar` principal block still apply to a UAR agent that calls this server. UAR can be used as an ordinary OpenAI-compatible model backend (next section) or as an agent behind `/agui` ([UAR agent backend](#uar-agent-backend)).

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

- `llm.api_key` (or `--llm-api-key-env NAME` / `--llm-api-key-file /abs/path`) is a reference; a literal key is rejected and never echoed. The key is never sent over plain `http` to a non-loopback host. Any `https` to `http` downgrade needs typed confirmation whatever the port, and a same-host scheme change in either direction also needs confirmation; an `http` to `https` move to a new, acknowledged host does not. The same rule covers the settings API: a settings write or a Test connection probe accepts a plain `http` backend URL only for a loopback host (`localhost` or a literal loopback address; a `*.localhost` name does not count) or a host listed in `embeddings.insecure_http_hosts`, and changing the scheme of the backend URL needs the `egress_confirmation` for that host.
- `llm.base_url` must not carry credentials, a query or a fragment. Redirects are not followed.
- `llm.timeout` bounds each model request (default `60s`).
- The loop has been run against Ollama with `qwen3:8b` (tool calling worked in 5 of 5 runs) and `llama3.2:1b` (3 of 6). Small models call tools unreliably; prefer a model with good tool-calling support. UAR as an OpenAI-compatible backend was not tested; UAR as an agent is the next section.
- On start, `ipfs mcp` prints `agent loop enabled: ...` to stderr with the backend it will use.

## UAR agent backend

`llm.kind: "uar"` makes the backend a Universal Agent Runtime (UAR) instance that you run. `ipfs mcp` creates one agent on it, with kubo's system prompt (the fixed safety prefix included), a binding to this server and the IPFS skills, and proxies each `/agui` run to that agent. The Web UI and Desktop chat are unchanged: same endpoint, same token, same approval card. The design and the release gate are in [ADR-0010](0010-uar-agent-behind-agui.md); the UAR side is the pinned contract `mcp/uar/v1.1.0-kubo.md`.

It is off by default (`llm.kind` is `openai`), and nothing contacts a UAR instance unless you configure one. It is accepted only while `mcp/contract/agentic/uar-preflight.json` pins a published UAR release; this build pins `v1.1.0` (image digest `sha256:064f5004...`, see `mcp/uar/spike/REPORT.md`).

```json
{
  "http": { "enabled": true },
  "principals": [
    { "name": "webui-chat", "kind": "ui", "credential": { "env": "IPFS_MCP_UI_TOKEN" } },
    { "name": "uar-agent", "kind": "uar", "credential": { "env": "IPFS_MCP_UAR_TOKEN" },
      "write_groups": ["pin"], "egress_ack": ["liter.example.com"],
      "uar": { "card_url": "https://uar.example.com/.well-known/agent-card.json",
               "admin_credential": { "env": "UAR_ADMIN_TOKEN" } } }
  ],
  "llm": {
    "kind": "uar",
    "base_url": "https://uar.example.com",
    "model": "qwen3:8b",
    "api_key": { "env": "UAR_TRUSTED_HOST_JWT" },
    "uar": {
      "agent_id": "kubo-ipfs",
      "provider": "liter-llm",
      "mcp_name": "kubo-ipfs",
      "mcp_principal": "uar-agent",
      "mcp_credential_revision": "operator-issued-revision",
      "skills_dir": "/opt/kubo/mcp/skills"
    }
  },
  "agent": { "agui": true, "origin_allowlist": ["http://localhost:3000"] }
}
```

- `llm.base_url` is the UAR instance (no default). `llm.api_key` is the credential `ipfs mcp` presents to UAR, by reference: for provisioning and runs it is the issuer-signed JWT of a UAR trusted host (`uar_credential_kind: "issuer"`, matching an entry in UAR's `security.trusted_host_principals`). It is read at every request, so a rotated token in a mounted file takes effect without a restart. `llm.model` (required in the configuration file; a settings write may leave it empty) and `llm.uar.provider` (optional) set the agent's default model and provider on UAR; the model provider itself (for example liter-llm) is configured on UAR, not here.
- **What kubo creates.** On the first run after start, or after a settings change, `ipfs mcp` checks the instance's agent card (a published release at or above the pinned minimum, else the run fails and nothing is created), uploads each `SKILL.md` under `llm.uar.skills_dir`, creates or updates the agent `llm.uar.agent_id`, and makes its skill bindings exactly those skills. Kubo owns the agent's prompt (a same-id agent is overwritten) and the skills it uploads, and it unbinds only skill ids it bound in this process, so `llm.uar.agent_id` must be unique per node on a shared UAR (the default `kubo-ipfs` collides). A skill already on the instance is not overwritten (UAR's upload never overwrites): it is bound only if its `artifact_digest` equals the digest of kubo's `SKILL.md`; a different or missing digest fails provisioning and nothing is bound, so rename it or ask the UAR administrator. Only `SKILL.md` is uploaded; the contract accepts a single manifest, not supporting files. Leave `skills_dir` unset to upload no skills.
- **What the UAR administrator does** (kubo cannot, it needs an admin credential): register the destination `llm.uar.mcp_name` with `PUT /api/uar/mcp/servers/{name}`, `transport: remote_http`, the `/mcp` URL of this server, and a `grant_policy` with `trusted_hosts`, `required_scopes` (default `["ipfs"]`) and `allowed_agent_ids: ["<agent_id>"]`, so that only this agent can reach it. This is an operator step after the first provisioning: kubo does not do it, because its configuration has no UAR-reachable URL for itself and no trusted host id (follow-up: `llm.uar.mcp_url` and `llm.uar.host_id`). `llm.api_key` must be a trusted-host JWT, not an ordinary API key.
- **The tool credential.** `llm.uar.mcp_principal` names a principal of kind `uar`. Its credential is sent to UAR with each run as the run grant (`forwardedProps.uar.mcp_servers`: `credential_revision` from `llm.uar.mcp_credential_revision`, `scopes`, an advisory expiry `llm.uar.mcp_grant_ttl` ahead, and the `Authorization` header), never stored on UAR. The grant is the principal's static bearer: the expiry is advisory, `ipfs mcp` does not enforce it, and a leaked grant stays usable until the principal's secret is rotated (a short TTL does not limit a leak). The grant, like the key, is refused over plain http to a non-loopback host unless the host is listed in `embeddings.insecure_http_hosts`, and the settings confirmation names it. The principal's own grants, `egress_ack` and `check-uar` preflight apply to every tool call the agent makes, as for any `uar` principal ([Principals and `egress_ack`](#principals-and-egress_ack)). `mcp_principal` is required with `llm.kind: "uar"` (with `mcp_credential_revision`). UAR v1.1.0 resolves an empty tool allowlist to automatic selection (`src/uar/domain/policy.rs`), which would expose the instance's own native tools, so this is a security rule: a `uar` backend without it is refused at configuration time and at settings writes, never created without a kubo binding. Likewise `llm.uar.provider`, or `llm.model` written as `provider/model`, is required.
- **Approvals.** UAR asks for approval as an interrupt (`tool_approval`, or `mcp_elicitation`). `ipfs mcp` shows it in the chat in a confirmation-shaped card labelled "UAR approval, not a node confirmation" (its id starts `cf_uar_`; the island schema requires the `cf_` prefix, so the card still resembles a node confirmation, and the label is what tells them apart), and the answer goes back to UAR; if the person denies, every other interrupt of that run is denied with it and nothing runs. This approval is UAR's, not kubo's. A write the agent then calls on this server is still confirmed by the confirmation gate, and a `uar` principal never answers by elicitation, so it needs the system prompt, the Desktop window or `--confirm=cli` on this machine. Interrupts the chat cannot show are never approved: a URL elicitation is declined, a form that asks for anything but one yes or no and an interrupt whose connection is gone are cancelled.
- **Egress.** Content leaves the node only through tool results to the `uar` principal, under its `egress_ack` and `check-uar`. The UAR instance itself is the backend host for the settings API's typed host confirmation (D4), and its upstream model provider must be covered by the `uar` principal's `egress_ack` (or be local) for `check-uar` to pass; where `check-uar` cannot read it, content tools are refused.
- **Limits.** `/agui` only: `--a2a` is ignored with a UAR backend, and `/a2a` answers 503 to every method while UAR is the backend, reading a task included. A task that was running when a settings write switched to UAR ends FAILED instead of crashing the process. The proxy relays only `TEXT_MESSAGE_*` and `TOOL_CALL_*` events, and only their listed fields; it writes `RUN_*` itself with the island's thread and run ids, and drops `CUSTOM` (so no A2UI surface), `STATE_*`, `MESSAGES_SNAPSHOT`, `RAW`, `STEP_*` and `*_CHUNK`. The UAR key and the grant are replaced in every relayed string (a secret split across two events is not caught). An open interrupt survives a failed resume. A resume of one interrupt among several is held until all are answered, because UAR rejects a partial resume.
- **Settings API.** `PUT /agent/settings` with `{"backend": {"kind": "uar", "base_url": "https://uar.example.com", "model": "qwen3:8b", "api_key": "<jwt>"}}` switches to UAR and `{"backend": {"kind": "openai", ...}}` switches back. A kind change is a move: it needs the typed `egress_confirmation` even for the same host, and no stored key is carried across kinds (ADR-0012); the read shape reports `backend.kind`. `llm.uar.*` is configuration-file only. Test connection with `kind: "uar"` reads the public agent card and sends no credential.
- **Startup order.** The sidecar waits for the daemon API (`daemon.wait`, default `60s` only with `--http`; stdio fails at once as before, and `0s` turns it off); see [chat-setup](chat-setup.md#server-side). The first retry is logged at Warn, later ones at Info.
- **Off.** Set `llm.kind` to `openai` or remove it, remove `llm.uar`, or `make build GOTAGS=noagent`. With a UAR backend and no key that resolves, runs fail with a clear 502 and nothing else changes.
- **Checked against the published image?** Not yet. The fixtures in `mcp/uar/fixtures/v1.1.0/` follow the contract field for field for the interrupts, but the agent artifact, skill record and binding bodies are inferred from the contract's paths, and `test/cli/mcp_uar_backend_test.go` is the first run against the image (see its header for how to run it).

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

`egress_ack` governs where node content may go. It never extends to the model API key: an acknowledged host does not receive the key. The stored key is sent only to the current backend's host and port, by the agent loop, test-connection and model list alike (ADR-0012). Moving the backend to another scheme, host or port clears a key stored through the settings API unless the same write supplies a new `api_key`; a key supplied by reference (environment variable or file) stays as configured, and on a node with `agent.settings.accept_secrets: false` a provider change through the API succeeds and the server-held (referenced) key follows it to the new host. That is an accepted residual of ADR-0012: the `admin` bearer is the root of trust, so keep it scarce or set `agent.settings.enabled: false`.

**Settings route hardening (Round 7).**

- **Audit log.** Every settings write logs one line at `Info` (`Warn` for a host move or an egress extension) with the principal ID, the names of the fields changed, the old and new host and whether a key was not carried. It never contains a key, the prompt, the model name or a path.
- **Browser origins.** The settings route answers CORS only for the origins in `agent.origin_allowlist`: methods `GET`, `PUT`, `POST` and `OPTIONS`; request headers `authorization`, `content-type`, `accept` and `if-match`; response headers `ETag` and `Retry-After` exposed; no credentials header. An operator who relied on `http.allowed_origins` for the console must add the console origin to `agent.origin_allowlist`, or the settings page is refused cross-origin.
- **Limits.** The route allows 60 requests a minute per principal with a burst of 60, test-connection and model-list 10 a minute with a burst of 10 and 2 in flight, and the change stream 16 open subscribers in all. A refusal is `429` with `Retry-After`.
- **Open confirmations after a backend change.** A settings write that changes the backend endpoint or key revokes the interrupts that are waiting for an approval: resuming one fails with `failed_precondition` and `details.reason = "backend_changed"` (`409` on `/agui`, `BACKEND_CHANGED` on `/a2a`), and the old key is dropped at once. A write that changes only the model or the prompt leaves them alone. A resume already in flight when the write lands keeps its snapshot until it ends. The user starts a new message to continue on the new backend: after a revoked resume a new message is accepted at once, and the cancelled approval cannot be reused.

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
| `human-mcp` client that cannot elicit | system prompt, or Desktop if named in `--confirm` (write-class only), or `ipfs mcp confirm` under `--confirm=cli` | same |

The native prompt per OS:

- **macOS:** LocalAuthentication (Touch ID, Apple Watch or the login password) through `/usr/bin/swift`, which needs the Xcode command line tools (`xcode-select --install`). On by default.
- **Linux:** polkit, after root installs the action from `mcp/server/confirm/polkit/` (`INSTALL.md`) and with an authentication agent in the session. Not yet run on a Linux machine.
- **Windows:** no native prompt, and no Desktop window this phase (Desktop's channel client is inert on Windows). Non-eliciting writes are refused there.
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
