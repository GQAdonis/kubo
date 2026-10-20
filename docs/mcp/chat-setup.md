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
- `egress_ack` must cover the model host, or the chat cannot see file contents (`"local"` for a model on this machine). The settings page's confirmation of a new model host (D4) extends the `admin` identity's own acknowledgement; it does not extend the `ui` or `a2a` principal's `egress_ack`. Until that principal's `egress_ack` also covers the new host, the chat refuses to send node content there (it fails closed), whatever the confirmation text said, and the `/mcp` preflight reads the hosts from the configuration file rather than the settings overlay, so a refusal can name the previous host. Add the new host to the principal's `egress_ack` in the configuration. See [Principals and `egress_ack`](agentic.md#principals-and-egress_ack), including what the peer, routing and key tools may send without it.
- An acknowledged host does not receive your model key. `egress_ack` governs node content only. The stored key goes only to the current backend's host and port (ADR-0012). If you change the backend host or port in the settings page, a key stored through the page is cleared and you enter it again; a key supplied by environment variable or file follows the move to the new host. On a node with `agent.settings.accept_secrets: false` a provider change through the API therefore succeeds and the server-held key goes with it (accepted residual, ADR-0012).
- The default listen address is `127.0.0.1:5050`, so the endpoint is `http://127.0.0.1:5050/agui`.
- **Waiting for the daemon.** Started as a sidecar (IPFS Desktop, or the LKE pod beside the Kubo container), `ipfs mcp` may come up before the daemon's RPC API does. It waits for the daemon API before giving up: `daemon.wait`, default `60s` with `--http` and none (fail at once) for plain stdio, because an MCP host has its own startup timeout; an explicit value is always honored and `0s` turns the wait off. Without `--api`, the `api` file is read again on each attempt, so a daemon that starts later or restarts on another port is found. A refused credential or a version mismatch is never retried. The first retry is logged at Warn and each later one at Info, so a waiting sidecar is visible at the default log level.

## UAR as the backend

The chat can run its agent on a Universal Agent Runtime (UAR) instance instead of an OpenAI-compatible model server. The chat does not change: it still talks to `/agui` of `ipfs mcp` with the same token and shows UAR's approval requests in the same card. Setup, what kubo creates on UAR, and what the UAR administrator must register are in the [agentic guide](agentic.md#uar-agent-backend).

- **Settings page.** Choose the backend kind *UAR*, enter the UAR address, optionally a model, and the credential `ipfs mcp` presents to UAR. The page sends `PUT /agent/settings` with `{"backend": {"kind": "uar", "base_url": "...", "model": "...", "api_key": "..."}}`; moving to a new host needs the typed host confirmation like any backend. The `llm.uar` options (agent id, tool principal, skills directory) are set in the configuration file.
- **Approvals.** UAR asks for approval of a tool call; you answer in the chat. A change to the node is then still confirmed by `ipfs mcp` itself (system prompt, Desktop window or `--confirm=cli`): approving in the chat does not replace that.
- **Not available with UAR:** A2A, and A2UI surfaces on the stream.
- **Off.** Switch the kind back to *OpenAI-compatible*, or remove `llm.kind` from the configuration. Nothing contacts UAR unless it is configured.
- **State.** Checked against the fixtures of the pinned UAR release; not yet run against the published image.

## Web UI side

Open **Settings** and find the **Agent chat** section.

1. **Enable agent chat.** On by default in the fork's Web UI build (see [Defaults](#defaults-in-this-fork)); a profile that already stored `enabled: false` keeps it off. While it is off, the chat code is not loaded and no request is made. With no endpoint and token set, the chat makes no request either.
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

## The LKE console (cloud node)

On the know-me LKE cluster, `ipfs mcp` runs as the `ipfs-mcp` container of pod `ipfs-0` in namespace `ipfs`, listening on port 5050 (container port name `mcp`). The chat is served on `https://console.prometheusags.ai` under the session policy of [ADR-0013](0013-console-session-auth.md). The deployment is change `lke-agent-settings` in the know-me-cluster repository; until its PR merges (OG-13), the console has no `/agui` rule and this section describes the configuration it ships.

### How the console chat authenticates

The console chat uses the signed-in session, not a stored token:

- You sign in at `/webui/` through Hydra with Kratos login. The gateway (SecurityPolicy `ipfs-webui-auth`: OIDC, JWT, an RBAC allowlist on `sub`, CSRF) holds the session and forwards the ID token to `ipfs mcp` as `Authorization: Bearer`.
- `ipfs mcp` verifies that token itself and maps its `sub` to the `console-ui` principal (kind `ui`, `subjects` only, no `credential`). `auth.oidc.routes` is exactly `["session", "agui"]`, so a session token is accepted on `/auth/session` and `/agui` and nowhere else; `agent.origin_allowlist` is exactly `["https://console.prometheusags.ai"]`, and `auth.oidc.require_origin` stays at its default, `true`.
- The Web UI detects session mode from `GET /auth/session`. It then calls `/agui` same-origin with no `Authorization` header, and it stores no chat token in `localStorage`. The **Access token** step of [Web UI side](#web-ui-side) does not apply on the console, and no `ui` bearer token exists for the console.

### Confirmation on LKE

The pod has no desktop, no system prompt and no paired Desktop window, so the LKE configuration sets `confirmation.modes: []`: no out-of-band resolver runs. Per-call confirmation still applies, and cannot be turned off. With `confirmation.modes: []` there are three cases:

| Who calls | Write tools | Destructive tools |
|---|---|---|
| An MCP client on `/mcp` (`https://ipfs.prometheusags.ai/mcp`, static bearer) | Confirmed only through MCP elicitation. A client that does not support elicitation is refused with `confirmation_unavailable` and nothing runs. | Same as writes: elicitation, else `confirmation_unavailable`. |
| The console chat (`/agui`, signed-in session) | The run stops at an interrupt; the chat island shows the approval card, and the call runs only after you answer it with a click (a user gesture). Deny, or let it expire, and nothing changes. | Refused with `confirmation_unavailable`. A chat answer alone cannot approve a destructive call; it needs a native system prompt, and the pod has none. |

To delete or unpin on the cloud node, use an MCP client that supports elicitation on `/mcp`, or the Kubo RPC API through the console's existing `/api/v0` route.

### Settings on LKE: port-forward only

By operator ruling (2026-10-20, "Port-forward only"; ADR-0013 decision 3, option (c)) the console has no settings route, and no session identity is an admin:

- No rule on HTTPRoute `ipfs-webui` (or any other HTTPRoute on a public host) routes `/agent/settings` to `ipfs-mcp:5050`. On the console that path gets no answer from `ipfs mcp`.
- The `admin` principal has no `subjects`, so no Hydra `sub` maps to it, and `settings` is not in `auth.oidc.routes`, so a session token is refused on the settings route in any case. `GET /auth/session` reports the admin state `not_eligible` for every console user.
- The `admin` principal's only credential is a static bearer read from a file in an out-of-band admin Secret, mounted with `optional: true`. The operator creates that Secret by hand; it is never in git, and agents never create, read or print it. With the Secret absent, the chat and `/auth/session` keep working and no request can pass the settings route's admin check.
- `agent.settings.accept_secrets` is `false`: a settings write that carries a model key is refused with `secrets_not_accepted`. The model key comes only from the out-of-band LLM key Secret through `llm.api_key`.

To read or change settings, the operator runs, from a machine with the `know-me` kube context:

1. Forward a non-default local port (not 5050, 5001, 4001 or 8080, which a local node or `ipfs mcp` may hold) to the pod. `kubectl port-forward` binds `127.0.0.1` only:

   ```bash
   kubectl --context know-me -n ipfs port-forward pod/ipfs-0 15050:5050
   ```

2. In a second terminal, put the admin bearer into a header file only you can read. Use your own copy of the token, the file you created it from (the admin Secret `ipfs-mcp-admin-token`, key `token`, is created from it with `--from-file`). The token is never echoed, typed into a browser or pasted into the console:

   ```bash
   umask 077
   hdr="$(mktemp)"
   printf 'Authorization: Bearer %s\n' "$(tr -d '\r\n' < ~/.ipfs-mcp-admin-token)" > "$hdr"
   ```

   With the admin Secret absent, the `admin` principal is disabled (the sidecar logs a warning) and the route answers 401 to every request.

3. Call the settings route on the forwarded port, passing the header from the file so the token is in no command line:

   ```bash
   curl -sS -H @"$hdr" http://127.0.0.1:15050/agent/settings                # read
   curl -sS -H @"$hdr" -X PUT -H 'Content-Type: application/json' \
     --data @settings.json http://127.0.0.1:15050/agent/settings           # write
   ```

   Without the header, the route answers 401. A write that moves the backend to a new host needs the typed host confirmation, as on any node.

4. Remove the header file and stop the port-forward:

   ```bash
   rm -f "$hdr"
   ```

Writes are persisted in the settings directory (`--settings-dir=/mcp-settings`, on PVC `ipfs-mcp-settings`). To reset them, delete the overlay file `overlay.json` there. To turn the route off, set `agent.settings.enabled: false` (404) or remove the `admin` principal or its Secret.

## Defaults in this fork

This fork ships its agent features enabled, each with an off switch (AGENTS.md, User Agency). Nothing below enables a third-party service: the model backend, embedder, extractor and describer stay unconfigured until you name them. A default-on migration fills only keys that are absent; a stored `false` is kept.

<!-- defaults-table:begin (generated by docs/mcp/config_reference.py from docs/mcp/defaults-table.json) -->

| Feature | Where it is on | Off switch | Disabled-state test |
|---|---|---|---|
| Local `ipfs mcp` agent endpoint on loopback | IPFS Desktop starts `ipfs mcp --http --agui` on `127.0.0.1` (port 5050 preferred) as a supervised child. Stock `ipfs mcp` stays off: it serves no HTTP and no `/agui` without `--http` and `--agui`. | Desktop: the tray toggle, or `mcpSidecar.enabled` = `false` in the Desktop store. Stock: leave `--http` and `--agui` unset. | Desktop `test/unit/mcp-sidecar.spec.js`: `mcpSidecar.enabled=false starts no child and binds no port`, `is on by default`. Kubo: `TestAGUIIsNotMountedUnlessEnabled`, `TestSettingsRouteRequiresAdminPrincipal`; the `test/cli` pin of the stock defaults (`agent-features-default-on` 1.3) is being written (name unconfirmed). |
| Web UI agent chat | The fork's Web UI build (`src/agent-chat/`). On by default; it makes no request until an endpoint and token exist, which Desktop supplies and a browser user enters in Settings. | Web UI Settings, Agent chat, `enabled` off (local storage key `ipfs-webui:agent-chat`). Turning it off loads nothing and removes the stored token. | Web UI `src/agent-chat/agent-chat.test.js`: `removes the element and the launcher when switched off, and forgets the token`, `publishes nothing while the chat is off (disabled-state)`; `src/agent-chat/boot.test.js`: `keeps a stored enabled: false for both features` (survival through app boot). Uncommitted in the builder's tree on 2026-10-15; names to be rechecked when it lands. |
| WebMCP read tools | The fork's Web UI build, in a browser that offers WebMCP. `enabled` defaults to on; DNSLink resolution is a separate switch. | Web UI Settings, WebMCP, `enabled` off (local storage key `ipfs-webui:webmcp`). Off registers nothing and sends nothing, and clears the two dependent switches. | Web UI `src/webmcp/webmcp.test.js`: `registers nothing and sends nothing once the person switches it off, and the choice survives a reload`, `registers nothing on a profile that opted out, even when the browser has WebMCP`; `boot.test.js` as above. |
| WebMCP write tools | Off by default and per page load. This one does not ship enabled: `writes` is never persisted and every write still needs your confirmation. | Already off at every load. Switch `writes` off in Settings, or use "Deny all and turn off agent writes" in the confirmation dialog. | Web UI `src/webmcp/webmcp.test.js`: `adds write tools only under the second switch, and removes them when it goes off`, `the confirmation hook itself declines when writes are off`. |
| Desktop agent chat bridge | IPFS Desktop: supplies the chat endpoint and token (safeStorage) to the Web UI. Store key `agentChat.enabled`, default on; the chat is still inert without a valid endpoint. | `agentChat.enabled` = `false` in the Desktop store. A stored `false` is never overwritten by the default migration (D18). | Desktop `test/unit/agent-defaults.spec.js`: `a stored false survives app start, for each switch`, `disabled state: chat reads off with a stored false even when an endpoint is set`; `test/unit/agent-chat.spec.js`: `disabled state: registers no channel at all`. |
| Desktop confirmation window | IPFS Desktop on macOS and Linux. Never on Windows (`win32`): the feature is inert there and the tray entries are greyed out. Store key `desktopConfirm.enabled`, default on. Writes still need confirmation. | `desktopConfirm.enabled` = `false` in the Desktop store, the tray "Turn off" entry, or "Turn off" in the pairing or request window. | Desktop `test/unit/agent-confirm.spec.js`: `a hand-written off value in the store keeps it off after a restart`, `win32: inert, and the tray entries are greyed out`; `test/unit/agent-defaults.spec.js`: `never switches the confirmation window on for win32`, `disabled state: a stored false keeps the confirmation window off after the defaults ran`. |
| Desktop settings bridge | IPFS Desktop: the `ipfsDesktop.agentSettings.*` bridge to the sidecar's settings API. Store key `agentSettings.enabled`, default on; the bridge needs the sidecar's `admin` principal, which Desktop configures. | `agentSettings.enabled` = `false` in the Desktop store: no channel exists and nothing is pushed. | Desktop `test/unit/agent-settings-bridge.spec.js`: `with the switch off no channel exists and nothing is pushed`, `the store key defaults to on and reads false as off, like the sidecar switch`. |
| Settings API (`agent.settings.enabled`) | `ipfs mcp`, key `agent.settings.enabled` defaults to `true`, but the route is mounted only when an `admin` principal is configured, so a stock `ipfs mcp` serves no settings route. Desktop and the LKE manifest configure one. | `agent.settings.enabled: false`: the route answers 404, nothing is written and nothing is dialed. Or configure no `admin` principal. | `TestSettingsRouteDisabledAnswers404`, `TestSettingsRouteRequiresAdminPrincipal` (`test/cli/mcp_settings_disabled_test.go`). |
| Server-emitted A2UI surfaces (`agent.a2ui`) | `ipfs mcp`, key `agent.a2ui` defaults to `true` wherever `/agui` is on; no effect when `agent.agui` is false. | `agent.a2ui: false`: no A2UI surface message is emitted on `/agui`; the run stream is otherwise unchanged and the client-side tool-call card renders. | `TestNoA2UISurfaceEventsWhenDisabled` (`mcp/server/agui_a2ui_test.go`). |
| Web UI vector search (`search.enabled`) | The fork's Web UI build (`src/search/`). On by default; it is effective only when the node has a configured index (an `embeddings` section with `search_route.enabled` not `false`) and the chat endpoint and token exist; otherwise it shows the not-configured state and makes no request. | Web UI Settings, Agent features, the Search switch off; or on the node, remove the `embeddings` section or set `search_route.enabled` to `false` (`/index/search` and `/index/status` then answer 404). | Web UI `src/settings/agent/SearchSwitch.test.js`, `src/search/SearchPage.test.js` and `test/e2e/search.test.js` (the switch in Settings, Agent features; the disabled state makes no request). |
| Desktop search (`search.enabled`) | Off by default. IPFS Desktop adds search only after you turn it on. | Desktop Search settings, the search switch off (the default). | Desktop `agent-search.spec.js`. |

The default-on rows follow the fork rule in AGENTS.md (User Agency): an off switch is not a default-off requirement, and each feature keeps a documented off switch. No third-party service is enabled without the operator choosing it: the model backend, the embedder, the extractor and the describer all stay unconfigured until you name them.

Resource use: Idle resident memory of the `ipfs mcp` child as Desktop launches it: 42336 KB (about 41.3 MiB), steady across five samples taken 6 s apart, no model backend configured, no tool call or chat run, one run on one macOS arm64 machine ([evidence](../../mcp/server/evidence/2026-10-14-desktop-sidecar-idle-rss/README.md)). The Electron main process cost of supervising the child and the cost under load are not included.

<!-- defaults-table:end -->

The same table is in the [configuration reference](config-reference.md#features-enabled-by-default-in-this-fork).

## Turning it off

| To turn off | Do this |
|---|---|
| The chat in one browser | Switch **Enable agent chat** off. Nothing is loaded, and the stored token is removed. |
| The stored token only | **Forget token**. |
| Browser access for every client | Remove the origin from `agent.origin_allowlist`, or leave the list empty (default). |
| The endpoint | Leave `--agui` and `agent.agui` unset (default), or configure no model backend. |
| All agent code in the server | `make build GOTAGS=noagent`. |
