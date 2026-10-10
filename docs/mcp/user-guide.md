# `ipfs mcp` user guide

`ipfs mcp` lets AI agents use a running Kubo node through the [Model Context Protocol](https://modelcontextprotocol.io/specification) (MCP). An MCP client such as Claude Code, Codex or OpenCode starts it, or connects to it over HTTP, and calls tools such as `ipfs_cat`, `pin_add` or `mfs_write`. Every tool that changes something is confirmed by a person before it runs.

The command is **experimental** and exists only in this fork's `ipfs` binary. It is a client of a daemon's RPC API (`/api/v0`): it never opens the repo, never takes `repo.lock`, adds no Kubo repo config key, and changes nothing in `/api/v0`, the HTTP gateway, the default CID recipe or wire behavior. The design decisions are in [ADR-0001](0001-mcp-server-placement.md) (placement) and [ADR-0003](0003-confirmation-and-auth.md) (confirmation and authentication).

Related pages: [configuration reference](config-reference.md) (every option, default and off switch), [agentic guide](agentic.md) (AG-UI, A2A, the agent loop), [embeddings guide](embeddings.md) (semantic search), [chat setup](chat-setup.md) (Web UI and IPFS Desktop chat).

## Contents

- [What is not delivered yet](#what-is-not-delivered-yet)
- [Install](#install)
- [Connect to a daemon](#connect-to-a-daemon)
- [Configure a client over stdio](#configure-a-client-over-stdio)
- [Serve over HTTP](#serve-over-http)
- [Principals and least privilege](#principals-and-least-privilege)
- [Restrict what `ipfs mcp` can do on the daemon](#restrict-what-ipfs-mcp-can-do-on-the-daemon)
- [Confirmation and its limits](#confirmation-and-its-limits)
- [Off switches](#off-switches)
- [Troubleshooting](#troubleshooting)

## What is not delivered yet

These parts are designed and partly built, but do not work end to end in this build. Do not rely on them:

- **The chat in the Web UI that Kubo serves.** The Web UI that `ipfs daemon` serves at `/webui` does not include the agent chat yet. The chat exists in the Web UI fork's source tree only (see [chat setup](chat-setup.md)).
- **The native confirmation prompt, verified on real hardware.** The macOS Touch ID or password prompt is on by default and works on the development machine, but the check that a same-user process cannot approve it (HG-1) has not been run on real hardware, and the Linux polkit prompt has not been run on a Linux machine at all.
- **IPFS Desktop pairing and its confirmation window.** `ipfs mcp pair` creates a pairing key, but IPFS Desktop has no released window to enter it or to approve confirmations. On Windows there is therefore no out-of-band prompt: only clients that support MCP elicitation can confirm writes.
- **Desktop chat pairing.** The Desktop chat has no way yet to obtain its token from `ipfs mcp`.
- **WebMCP in the Web UI.** Not available.
- **A Kimi Code plugin.** Kimi Code gets the skills only; there is no plugin package for it.
- **UAR as an agent host (topology B).** Deferred ([ADR-0008](0008-topology-b-gate-no-go.md)). A principal of kind `uar` with `"check": "verify"` is refused until a UAR release documents a settings endpoint.

## Install

### From the release archives

The release archives are built locally from this repository; nothing is published or downloaded by these steps.

```bash
mcp/release/build.sh     # builds through `make build` for each target
mcp/release/verify.sh    # checks the archives against SHA256SUMS
```

`build.sh` writes `mcp/release/dist/kubo-mcp_<version>_<goos>-<goarch>.tar.gz` (`.zip` on Windows) for darwin-arm64, darwin-amd64, linux-amd64, linux-arm64 and windows-amd64, plus `SHA256SUMS`. If you received an archive and its `SHA256SUMS` from someone else, check it before you run anything:

```bash
shasum -a 256 -c SHA256SUMS --ignore-missing
tar -xzf kubo-mcp_<version>_darwin-arm64.tar.gz
```

Each archive holds one directory with the `ipfs` binary, the licenses and a short README. You do not need to replace the `ipfs` that runs your daemon: keep this binary in its own directory and give your MCP client its absolute path. In the examples below, `ipfs` means that binary.

### As an agent plugin

Plugins for Claude Code, Codex and OpenCode bundle the skills and the MCP server entry. Install steps are in [`mcp/plugin/README.md`](../../mcp/plugin/README.md). Kimi Code gets the skills only.

### Removing it from a build

`make build GOTAGS=nomcp` builds an `ipfs` without `ipfs mcp`; the command then only reports that it is not included. `make build GOTAGS=noagent` keeps MCP but removes the agent loop and its AG-UI and A2A endpoints.

## Connect to a daemon

`ipfs mcp` needs a running daemon, Kubo 0.29.0 or newer; it refuses to start below that and exits with an error naming the address it tried when no daemon answers. It finds the daemon in this order:

1. `--api <multiaddr or URL>`, for example `--api /ip4/127.0.0.1/tcp/5001`;
2. the `api` file in `$IPFS_PATH`, written by `ipfs daemon`;
3. `~/.ipfs/api` when `IPFS_PATH` is not set.

**Stock Kubo.** Any Kubo daemon 0.29.0 or newer works, including one that is not this fork: `ipfs mcp` only calls `/api/v0`. When the daemon lacks a command a tool needs, that tool is left out of `tools/list` and `ipfs_node_status` reports it as unavailable.

**IPFS Desktop.** Desktop runs a Kubo daemon and writes its `api` file into the repo it uses. When that repo is `~/.ipfs` (Desktop's default), `ipfs mcp` finds it with no flags. Otherwise set `IPFS_PATH` to that repo, or pass `--api` with the daemon's RPC address (`Addresses.API` in that repo's config).

**A daemon with `API.Authorizations`.** Pass the secret with the global `--api-auth` option, for example `ipfs mcp --api-auth bearer:<secret>`. See [Restrict what `ipfs mcp` can do on the daemon](#restrict-what-ipfs-mcp-can-do-on-the-daemon).

## Configure a client over stdio

By default `ipfs mcp` speaks MCP on stdin and stdout. The client starts it; nothing listens on the network. Only MCP messages go to stdout; diagnostics go to stderr.

Claude Code:

```bash
claude mcp add ipfs -- /path/to/ipfs mcp
```

Codex:

```bash
codex mcp add ipfs -- /path/to/ipfs mcp
```

Add options after `mcp` as needed, for example `-- /path/to/ipfs mcp --read-only` or `-- /path/to/ipfs mcp --api /ip4/127.0.0.1/tcp/5001`. For OpenCode, and for the plugin form of all three, see [`mcp/plugin/README.md`](../../mcp/plugin/README.md).

Over stdio the client is the operator: it can call every enabled tool, and writes are confirmed as described in [Confirmation and its limits](#confirmation-and-its-limits).

## Serve over HTTP

`--http` serves MCP over Streamable HTTP instead of stdio, at `http://127.0.0.1:5050/mcp` by default. It is off by default.

```bash
ipfs mcp --http                       # 127.0.0.1:5050
ipfs mcp --http=127.0.0.1:5150        # another address (or IPFS_MCP_HTTP_LISTEN)
```

The listener refuses the ports of other local services (4000, 4001, 5001, 8080, 1906-1908, 50051, 8181, 8081, 11434, 23001, 28000). Every request must carry a loopback `Host` header on a loopback listener (DNS rebinding protection) and no foreign `Origin`; browser origins are refused unless listed in `http.allowed_origins` in the config file.

**Without a credential**, a loopback listener serves only the read-only tools, to any local process, and prints a warning saying so. A non-loopback address is refused without a credential.

**With the operator token.** The token is read from an environment variable or a file, never from the command line:

```bash
export IPFS_MCP_TOKEN="$(openssl rand -hex 32)"
ipfs mcp --http --auth-token-env IPFS_MCP_TOKEN
# or: ipfs mcp --http --auth-token-file /absolute/path/to/token
```

The operator token reads and writes every enabled group; destructive tools only when `destructive` is in `--tools`. Point the client at the URL with the token:

```bash
claude mcp add --transport http ipfs http://127.0.0.1:5050/mcp \
  --header "Authorization: Bearer $IPFS_MCP_TOKEN"
codex mcp add ipfs --url http://127.0.0.1:5050/mcp --bearer-token-env-var IPFS_MCP_TOKEN
```

The Claude Code command stores the header value in Claude Code's configuration; Codex reads the variable when it connects.

**Exposing it beyond this machine.** Prefer not to. If you must:

- bind to a non-loopback address only with a credential (`ipfs mcp` refuses otherwise) and give each client its own principal (next section), not the operator token;
- put TLS in front with a reverse proxy; `ipfs mcp` serves plain HTTP only;
- if the proxy forwards to a loopback listener, have it send a loopback `Host` header (the listener address), or the request is refused;
- keep `http.allowed_origins` empty unless a browser page must call `/mcp`.

## Principals and least privilege

A JSON config file, named with `--mcp-config <path>` or `IPFS_MCP_CONFIG`, declares named principals, each with its own credential. It is not the Kubo repo config. Every key is in the [configuration reference](config-reference.md).

```json
{
  "http": { "enabled": true },
  "principals": [
    {
      "name": "laptop-agent",
      "kind": "human-mcp",
      "credential": { "env": "IPFS_MCP_LAPTOP_TOKEN" },
      "groups": ["node", "content", "pin"],
      "write_groups": ["pin"]
    }
  ]
}
```

```bash
export IPFS_MCP_LAPTOP_TOKEN="$(openssl rand -hex 32)"
ipfs mcp --mcp-config ./ipfs-mcp.json
```

- A principal is **read-only** until a group is named in `write_groups`. Destructive tools (`pin_update`, `pin_rm`, `mfs_rm`, `key_rm`, `repo_gc`) are offered only for groups in `destructive_groups`.
- `groups` limits what it can read (default every group). `tools/list` shows only what the principal may call, and a call outside its grants is refused before anything runs.
- `credential` is `{"env": "NAME"}` or `{"file": "/absolute/path"}`. A literal token in the file is rejected and never echoed.
- Tools whose results carry node content (`contentEgress` in the catalog, for example `ipfs_cat`, `pin_ls`, `mfs_read`) are refused to a configured principal without an `egress_ack`. For a principal of kind `human-mcp` the list only has to be non-empty: name the hosts its client may forward content to, or `"local"`. For `ui` and `a2a` principals it must also cover the model and embedder hosts; see the [agentic guide](agentic.md#principals-and-egress_ack). The operator identity (stdio and `--auth-token-*`) and the anonymous loopback principal are not subject to this check when they call tools directly.
- Tool groups: `node`, `content`, `pin`, `mfs`, `names`, `dag`, `routing`, `diag`, `index`. Principal kinds: `human-mcp` (an MCP client a person uses), `ui` (the chat, `/agui`), `a2a` (another agent, `/a2a`), `uar` (deferred).

## Restrict what `ipfs mcp` can do on the daemon

`ipfs mcp` has the RPC access of whatever secret it uses. To give it only the commands its tools need, print an `API.Authorizations` entry for the tools you enabled:

```bash
ipfs mcp --read-only --tools=node,content --print-rpc-allowed-paths
```

```json
{
  "AuthSecret": "bearer:REPLACE-WITH-A-LONG-RANDOM-SECRET",
  "AllowedPaths": [
    "/api/v0/cat",
    "/api/v0/commands",
    "/api/v0/files/stat",
    "/api/v0/id",
    "/api/v0/ls",
    "/api/v0/version"
  ]
}
```

It honors `--tools`, `--read-only` and `tools.disabled`, and exits without contacting the daemon. Replace the secret, add the entry under a name of your choice in the daemon's [`API.Authorizations`](../config.md#apiauthorizations), restart the daemon, and start `ipfs mcp` with `--api-auth bearer:<the secret>`.

Two cautions:

- Once `API.Authorizations` has any entry, the daemon refuses every RPC request without a matching secret. Other clients of the same daemon (the Web UI, IPFS Desktop, scripts) then need entries of their own.
- Kubo matches `AllowedPaths` by string prefix, so a shorter path grants more than it appears to. Use the printed list as is.

## Confirmation and its limits

Every tool that changes something is confirmed per call, on every transport. This cannot be turned off; it can only become stricter.

1. **Elicitation.** If the client supports MCP elicitation, the server asks the client's user, and only an explicit approval runs the call. A decline, a cancel or no answer leaves the node unchanged.
2. **Out-of-band prompt.** If the client cannot elicit, the call waits for a person on this machine: the system prompt (`native`: Touch ID or the login password on macOS, polkit on Linux) or a paired IPFS Desktop window (`desktop`). The server answers `confirmation_required` with a confirmation ID; the client repeats the identical call after approval, and it runs exactly once.
3. **Nothing available.** The call is refused with `confirmation_unavailable` and nothing changes.

`--confirm` (or `IPFS_MCP_CONFIRM`, or `confirmation.modes`) chooses the out-of-band mechanisms; the default is `native,desktop`. A mechanism the host cannot offer is skipped. `--confirm=none` turns them off, after which only eliciting clients can confirm. `--confirm=cli` adds `ipfs mcp confirm`, which approves from a terminal on headless servers; it has no presence factor beyond that terminal, it needs a TTY on stdin and the same `--confirm=cli` on both sides, and under it every HTTP principal with write grants must declare `"shell_capable": false`.

Limits you should know:

- **A confirm token is not consent.** The token in `confirmation_required` binds the repeat call to the same session, principal, tool and arguments; it is accepted only after a person approved. An agent repeating its own call never approves it.
- **Approval is per call, not per intent.** The prompt shows the tool and its summarized arguments. Read it: approving a `pin_rm` approves removing those pins.
- **The macOS prompt needs `/usr/bin/swift`** (the Xcode command line tools, `xcode-select --install`). Without it the native mechanism is skipped. Each prompt takes a few seconds to appear. A process with Accessibility permission can click the dialog but cannot supply Touch ID or the password. A session with no GUI (ssh) cannot show it.
- **Linux needs the polkit action** installed by root from `mcp/server/confirm/polkit/` (see `INSTALL.md` there) and a polkit authentication agent in the session; without them `native` is skipped.
- **Pending confirmations live in memory** and expire after 120 seconds. Expiry and a restart of `ipfs mcp` count as denial.
- **Destructive calls from the chat (`ui`) and from A2A peers** need the system prompt or the Desktop window; the chat itself can only deny them, and `ipfs mcp confirm` never approves them.
- See also [What is not delivered yet](#what-is-not-delivered-yet): HG-1 and the Desktop window are pending.

## Off switches

| To turn off | Do this |
|---|---|
| The whole command | Build with `make build GOTAGS=nomcp`. |
| The agent loop, AG-UI and A2A | Do not configure a model backend (the default), or build with `GOTAGS=noagent`. |
| A tool group | `--tools` with the groups you want, or `tools.groups`; `tools.disabled` lists single tools. |
| Every write | `--read-only` (or `tools.read_only`). |
| Destructive tools | Off by default; they need `destructive` in `--tools` or a principal's `destructive_groups`. |
| The HTTP listener | Off by default; do not pass `--http` or set `http.enabled`. |
| A principal | Remove its entry or the variable or file its credential names. |
| MCP App views | `--no-app-views` (or `tools.app_views: false`). |
| Out-of-band confirmation prompts | `--confirm=none` or `IPFS_MCP_CONFIRM=none`. Per-call confirmation itself cannot be turned off. |
| Embeddings and search | Off unless the config file has an `embeddings` section. |
| AG-UI, A2A | Off by default; they need `--agui` / `--a2a`, `--http` and a model backend. |

Each option, its default and the test that runs it in the off state are listed in the [configuration reference](config-reference.md).

## Troubleshooting

- **`ipfs daemon API is not reachable`.** The message names the address or `api` file it tried. Start the daemon, or pass `--api` / set `IPFS_PATH`.
- **A tool is missing from `tools/list`.** Its group is not enabled, it is destructive and `destructive` is not selected, the principal lacks the grant, or the daemon lacks its RPC command. `ipfs_node_status` gives the reason for each tool.
- **`confirmation_unavailable`.** The client cannot elicit and no out-of-band mechanism is available (no `/usr/bin/swift` on macOS, no polkit action on Linux, no paired Desktop, or `--confirm=none`).
- **`egress_not_acknowledged`.** The principal has no `egress_ack` covering the host the content would go to. See the [agentic guide](agentic.md#principals-and-egress_ack).
- **HTTP 403 from `/mcp`.** A non-loopback `Host` header on a loopback listener, or an `Origin` not in `http.allowed_origins`.
