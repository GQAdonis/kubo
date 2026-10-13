# Agent chat in the Web UI and IPFS Desktop

The agent chat is a sidebar in the IPFS Web UI that talks to the `/agui` endpoint of `ipfs mcp`. You type a request, the agent loop in `ipfs mcp` calls IPFS tools for you, and every change waits for your approval in the chat (and, for anything destructive, for the system prompt). The design is [ADR-0005](0005-agent-chat-ui.md); the server side is in the [agentic guide](agentic.md#ag-ui).

## What works today, and what does not

- **Works:** the chat in a build of the Web UI fork (`src/agent-chat/` in the fork's source tree), served by the fork's own build, against `ipfs mcp --http --agui`.
- **Not delivered:** the chat in the Web UI that Kubo serves at `/webui`. That Web UI is pinned by CID in Kubo, and the CID has not been moved to a build that contains the chat.
- **Not delivered:** the chat in IPFS Desktop. Desktop's side of the connection (it supplies the endpoint and token to the Web UI) is built, but the pairing that would give Desktop its token is not, so Desktop has no way to obtain a token yet.
- **Not run end to end:** a person using the chat in a browser and in a Desktop build, with a real model backend, performing a read and a confirmed write (`webui-agent-chat` task 1.3) is still pending.

## Server side

The chat needs `ipfs mcp` serving `/agui`, which needs `--http`, a model backend, a principal of kind `ui`, and the Web UI's origin in `agent.origin_allowlist`:

```json
{
  "http": { "enabled": true },
  "principals": [
    {
      "name": "webui-chat",
      "kind": "ui",
      "credential": { "env": "IPFS_MCP_UI_TOKEN" },
      "write_groups": ["pin", "content"],
      "egress_ack": ["local"]
    }
  ],
  "llm": { "base_url": "http://127.0.0.1:11434/v1", "model": "qwen3:8b" },
  "agent": {
    "agui": true,
    "origin_allowlist": ["http://localhost:3000"]
  }
}
```

```bash
export IPFS_MCP_UI_TOKEN="$(openssl rand -hex 32)"
ipfs mcp --mcp-config ./ipfs-mcp.json
```

- `origin_allowlist` must contain the exact origin the Web UI is served from (scheme, host and port). The chat's Settings section shows it under "This Web UI's origin". The example is the fork's development server (`npm start` in the fork, which react-scripts serves on `http://localhost:3000` unless `PORT` is set); IPFS Desktop's origin is `webui://localhost`. The list is empty by default, and an unlisted origin gets 403.
- The `ui` principal is read-only until you name groups in `write_groups`; destructive tools need `destructive_groups` and always the system prompt.
- `egress_ack` must cover the model host, or the chat cannot see file contents (`"local"` for a model on this machine). See [Principals and `egress_ack`](agentic.md#principals-and-egress_ack), including what the peer, routing and key tools may send without it.
- The default listen address is `127.0.0.1:5050`, so the endpoint is `http://127.0.0.1:5050/agui`.

## Web UI side

Open **Settings** and find the **Agent chat** section.

1. **Enable agent chat.** Off by default. While it is off, the chat code is not loaded and no request is made.
2. **Agent endpoint (/agui URL).** For example `http://127.0.0.1:5050/agui`. There is no default. The address must end in `/agui`, use `https` (or `http` on `localhost`, `127.0.0.1` or `[::1]`), and carry no user name, password, query or fragment.
3. **Access token.** Paste the value of `IPFS_MCP_UI_TOKEN` and press **Set token**.

The chat launcher then appears; open it and ask, for example, which pins the node has.

### Where the token is stored

By operator decision (2026-10-10) the token is saved in the browser's `localStorage` for this Web UI origin, under its own key, so it survives a reload. It is never put in the settings object, a URL, an attribute or a log line. The trade-off: **any script that runs in the Web UI's origin can read it**, including script injected through a defect in the Web UI or one of its dependencies, and could then use the chat endpoint as the `ui` principal. What limits the damage:

- **Forget token** removes it at once, and switching the chat off removes it too.
- The token authenticates only the `ui` principal. It is not a Kubo RPC secret and does not open `/api/v0` or `/a2a`; on `/agui` and on `/mcp` (which also accepts `ui` principals) it has exactly that principal's grants.
- That principal can do only what you granted in `groups`, `write_groups` and `egress_ack`, and every write still needs your approval in the chat; destructive calls need the system prompt.

If browser storage is blocked, the token lasts for the open page only. Rotate the token (a new value in `IPFS_MCP_UI_TOKEN` and a restart of `ipfs mcp`) if you think it was exposed.

## IPFS Desktop

When the Web UI runs inside IPFS Desktop with Desktop's chat bridge present, the Settings section says that Desktop manages the connection: the endpoint and token come from Desktop (kept with the operating system's secure storage), never from the Web UI's settings, and nothing is stored in `localStorage`. Desktop's origin `webui://localhost` must be in `agent.origin_allowlist`.

This path is **not usable yet**: Desktop cannot obtain a token until pairing is delivered (see [What works today](#what-works-today-and-what-does-not)). `ipfs mcp pair` exists for the separate confirmation window, which Desktop does not provide yet either.

## Turning it off

| To turn off | Do this |
|---|---|
| The chat in one browser | Switch **Enable agent chat** off. Nothing is loaded, and the stored token is removed. |
| The stored token only | **Forget token**. |
| Browser access for every client | Remove the origin from `agent.origin_allowlist`, or leave the list empty (default). |
| The endpoint | Leave `--agui` and `agent.agui` unset (default), or configure no model backend. |
| All agent code in the server | `make build GOTAGS=noagent`. |
