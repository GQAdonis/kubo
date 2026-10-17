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
- [Search the node](#search-the-node)
- [Session tokens from the console (OpenID Connect)](#session-tokens-from-the-console-openid-connect)
- [Off switches](#off-switches)
- [Troubleshooting](#troubleshooting)

## What is not delivered yet

These parts are designed and partly built, but do not work end to end in this build. Do not rely on them:

- **The chat in the Web UI that Kubo serves.** The Web UI that `ipfs daemon` serves at `/webui` does not include the agent chat yet. The chat exists in the Web UI fork's source tree only (see [chat setup](chat-setup.md)).
- **The native confirmation prompt, verified on real hardware.** The macOS Touch ID or password prompt is on by default and works on the development machine, but the check that a same-user process cannot approve it (HG-1) has not been run on real hardware, and the Linux polkit prompt has not been run on a Linux machine at all.
- **IPFS Desktop pairing and its confirmation window.** `ipfs mcp pair` creates a pairing key, and the IPFS Desktop fork's confirmation window is built but not released or run by the operator yet. `desktop` is off unless you name it in `--confirm`. On Windows there is no out-of-band prompt this phase (Desktop's channel client is inert there): only clients that support MCP elicitation can confirm writes.
- **Desktop chat pairing.** The Desktop chat has no way yet to obtain its token from `ipfs mcp`.
- **WebMCP in the Web UI.** Not available.
- **A Kimi Code plugin.** Kimi Code gets the skills only; there is no plugin package for it.
- **UAR as an agent host (topology B).** Deferred ([ADR-0008](0008-topology-b-gate-no-go.md)). A principal of kind `uar` with `"check": "verify"` is refused until a UAR release documents a settings endpoint.

## Install

### From the release archives

The release archives are built locally from this repository; nothing is published or downloaded by these steps.

```bash
mcp/release/build.sh     # builds through `make build` for each target
mcp/release/verify.sh    # checks the signature and the archives against SHA256SUMS
```

`build.sh` writes `mcp/release/dist/kubo-mcp_<version>_<goos>-<goarch>.tar.gz` (`.zip` on Windows) for darwin-arm64, darwin-amd64, linux-amd64, linux-arm64 and windows-amd64, plus `SHA256SUMS` and its SSH signature `SHA256SUMS.sig`. If you received an archive from someone else, verify it as described in [Verifying a release](#verifying-a-release) before you run anything, then unpack it:

```bash
tar -xzf kubo-mcp_<version>_darwin-arm64.tar.gz
```

#### Verifying a release

Release archives come with `SHA256SUMS` and `SHA256SUMS.sig`, an SSH signature over the checksum file made with a dedicated release key (OpenSSH 8.1 or later, namespace `kubo-mcp-release`). The signature proves the checksums came from the key holder; the checksums prove the archives match them.

1. Get the release public key independently of the download, for example from [`mcp/release/allowed_signers`](../../mcp/release/allowed_signers) in a checkout you trust, not from the same place as the archives. Save it as `allowed_signers`. Each line has the form `<identity> namespaces="kubo-mcp-release" <keytype> <base64>`.
2. Check the signature, then the checksums, in the directory that holds the archives. Both must succeed; if either fails, do not run the binary.

```bash
ssh-keygen -Y verify -f allowed_signers -I <identity> -n kubo-mcp-release -s SHA256SUMS.sig < SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
```

From a source checkout, `mcp/release/verify.sh [--allowed-signers FILE] [DIST_DIR]` does both steps; `DIST_DIR` defaults to `mcp/release/dist`.

A release built with `KUBO_MCP_UNSIGNED=1` has no `SHA256SUMS.sig` and is unsigned. Only its checksums can be checked (`verify.sh --allow-unsigned`), which proves nothing about who built it.

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

**A daemon with `API.Authorizations`.** Pass the secret from an environment variable or a file: `ipfs mcp --api-auth-env IPFS_RPC_SECRET` (the variable holds, for example, `bearer:<secret>`) or `ipfs mcp --api-auth-file /absolute/path/to/secret`. The global `--api-auth bearer:<secret>` also works, but it puts the secret in the process list, where other local users can read it, and `ipfs mcp` prints a warning. Use only one of the three. See [Restrict what `ipfs mcp` can do on the daemon](#restrict-what-ipfs-mcp-can-do-on-the-daemon).

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
ipfs mcp --http=false                 # stdio, even when the config file sets http.enabled: true
```

The listener refuses the ports of other local services (4000, 4001, 5001, 8080, 1906-1908, 50051, 8181, 8081, 11434, 23001, 28000). Every request must carry a loopback `Host` header on a loopback listener (DNS rebinding protection) and no foreign `Origin`; browser origins are refused unless listed in `http.allowed_origins` in the config file.

**Without a credential**, a loopback listener serves only the read-only tools, to any local process, and prints a warning saying so. It refuses (403) any request that carries a `Forwarded`, `X-Forwarded-For` or `X-Real-IP` header, because such a request was relayed from somewhere else. A non-loopback address is refused without a credential.

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
- put TLS in front with a reverse proxy; `ipfs mcp` serves plain HTTP only. Behind a proxy a credential is required: configure the operator token or principals. Without one, every proxied request is refused, since proxies add forwarding headers;
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

It honors `--tools`, `--read-only` and `tools.disabled`, and exits without contacting the daemon. Replace the secret, add the entry under a name of your choice in the daemon's [`API.Authorizations`](../config.md#apiauthorizations), restart the daemon, and start `ipfs mcp` with `--api-auth-env NAME` (a variable holding `bearer:<the secret>`) or `--api-auth-file /absolute/path`. Avoid `--api-auth bearer:<the secret>`: the secret then shows in the process list.

Two cautions:

- Once `API.Authorizations` has any entry, the daemon refuses every RPC request without a matching secret. Other clients of the same daemon (the Web UI, IPFS Desktop, scripts) then need entries of their own.
- Kubo matches `AllowedPaths` by string prefix, so a shorter path grants more than it appears to. Use the printed list as is.

## Confirmation and its limits

Every tool that changes something is confirmed per call, on every transport. This cannot be turned off; it can only become stricter.

1. **Elicitation.** If the client supports MCP elicitation, the server asks the client's user, and only an explicit approval runs the call. A decline, a cancel or no answer leaves the node unchanged.
2. **Out-of-band prompt.** If the client cannot elicit, the call waits for a person on this machine: the system prompt (`native`: Touch ID or the login password on macOS, polkit on Linux) or, if you opted in, a paired IPFS Desktop window (`desktop`, write-class calls only). The server answers `confirmation_required` with a confirmation ID; the client repeats the identical call after approval, and it runs exactly once.
3. **Nothing available.** The call is refused with `confirmation_unavailable` and nothing changes.

`--confirm` (or `IPFS_MCP_CONFIRM`, or `confirmation.modes`) chooses the out-of-band mechanisms; the default is `native` only. `desktop`, the paired IPFS Desktop window, is opt-in: name it, for example `--confirm=native,desktop`. It approves write-class calls only, after Touch ID in the Desktop window, and never a destructive call from a `ui` or `a2a` principal. A mechanism the host cannot offer is skipped. `--confirm=none` turns them off, after which only eliciting clients can confirm. `--confirm=cli` adds `ipfs mcp confirm`, which approves from a terminal on headless servers; it has no presence factor beyond that terminal, it needs a TTY on stdin and the same `--confirm=cli` on both sides, and under it every HTTP principal with write grants must declare `"shell_capable": false`.

Limits you should know:

- **A confirm token is not consent.** The token in `confirmation_required` binds the repeat call to the same session, principal, tool and arguments; it is accepted only after a person approved. An agent repeating its own call never approves it.
- **Approval is per call, not per intent.** The prompt shows the tool and its summarized arguments. Read it: approving a `pin_rm` approves removing those pins.
- **The macOS prompt needs `/usr/bin/swift`** (the Xcode command line tools, `xcode-select --install`). Without it the native mechanism is skipped. Each prompt takes a few seconds to appear. A process with Accessibility permission can click the dialog but cannot supply Touch ID or the password. A session with no GUI (ssh) cannot show it.
- **Linux needs the polkit action** installed by root from `mcp/server/confirm/polkit/` (see `INSTALL.md` there) and a polkit authentication agent in the session; without them `native` is skipped.
- **Pending confirmations live in memory** and expire after 120 seconds. Expiry and a restart of `ipfs mcp` count as denial.
- **Destructive calls from the chat (`ui`) and from A2A peers** need the system prompt or the Desktop window; the chat itself can only deny them, and `ipfs mcp confirm` never approves them.
- See also [What is not delivered yet](#what-is-not-delivered-yet): HG-1 and the Desktop window are pending.

## Search the node

With an `embeddings` section in the config file, `ipfs mcp` can build a semantic search index over your content and answer searches from agents, MCP clients and a Web UI search box:

- `ipfs_index_search` searches; `ipfs_index_status` reports the index; `ipfs_index_cid` indexes one file; `ipfs_index_reconcile` removes entries for content you no longer pin.
- `ipfs_index_bulk_start`, `ipfs_index_bulk_status` and `ipfs_index_bulk_cancel` run one operator-started, capped, resumable indexing pass over your pins and MFS. Every read is offline, and a run adds, pins and announces nothing.
- Documents, code and structured data can go through an xberg extractor, and images and videos through a vision-model describer, so they are searchable in the same index. Both are off until you configure them, and a hosted describer receives the media bytes as base64 uploads — it is never a default.
- On the HTTP listener, `POST /index/search` and `GET /index/status` serve the Web UI with the same principal and `egress_ack` rules as the tools; `embeddings.search_route.enabled: false` unmounts them.

Everything is off without an `embeddings` section, indexing and search need a principal whose `egress_ack` covers the hosts involved, and the operator identity cannot index. The full setup, every key with its default and off switch, and the LKE and Desktop deployments are in the [embeddings guide](embeddings.md) ([ADR-0006](0006-embeddings-surrealdb.md), [ADR-0011](0011-whole-node-vector-search.md)).

## Session tokens from the console (OpenID Connect)

A deployment can put a sign-in in front of the console (the Web UI and its `/agui`, settings and search routes) and have the gateway forward the signed-in person's OpenID Connect ID token to `ipfs mcp` as `Authorization: Bearer <id_token>`. `ipfs mcp` then checks the token itself and maps the person to a principal. The design and its limits are in [ADR-0013](0013-console-session-auth.md). Nothing here applies to IPFS Desktop, the loopback Web UI, stdio or an MCP client using a static bearer; those are unchanged.

It is **off by default**. With `auth.oidc.enabled` false or absent, `ipfs mcp` accepts no token but the static ones, fetches no key set, and `GET /auth/session` answers 404. There is no built-in issuer, audience or key URL; you configure all three.

```json
{
  "auth": {
    "oidc": {
      "enabled": true,
      "issuer": "https://oauth.example.org",
      "audiences": ["ipfs-console"],
      "jwks_url": "https://oauth.example.org/.well-known/jwks.json"
    }
  },
  "agent": {"agui": true, "origin_allowlist": ["https://console.example.org"]},
  "principals": [
    {"name": "console-ui", "kind": "ui", "subjects": ["<identity id>"]},
    {"name": "console-admin", "kind": "admin", "subjects": ["<identity id>"]}
  ]
}
```

How it decides:

- **Token checks.** Signature against the key set at `jwks_url` (asymmetric algorithms only; `none` and `HS*` are refused), the `kid`, exact `iss`, one of `audiences`, and `exp`, `nbf` and `iat` within `clock_skew` (30 s by default, at most 2 minutes). Any failure is a plain 401 that does not say which check failed. An unknown `kid` causes at most one key set refetch every 30 seconds.
- **Who the person is.** Only the token's `sub` (the identity ID), matched exactly against `principals[].subjects`. The `email` claim is shown by `/auth/session` for display and never selects or grants anything. A valid token whose `sub` is in no principal for the route gets 403. A principal needs a `credential`, `subjects`, or both; two principals of the same kind cannot list the same subject, and one subject may appear in a `ui` and an `admin` principal.
- **Where it applies.** `auth.oidc.routes` (default `agui`, `settings`, `index`, `session`). A token on any other route, `/mcp` and `/a2a` included, gets 401. Add `mcp` only if you mean it.
- **Browser safety.** With `auth.oidc.require_origin` (default true), a session request that is not GET, HEAD or OPTIONS must send an `Origin` listed in `agent.origin_allowlist`; a request with no `Origin` gets 403. Static bearers keep their existing behaviour.
- **Admin.** An `admin` session needs one of `totp`, `webauthn`, `passkey` or `lookup_secret` in the token's `amr`, and an `auth_time` within one hour (`require_amr`, `max_auth_age`). Otherwise the settings route answers 403 `step_up_required`. `"require_amr": []` accepts a password-only session; that is a weakening, and the token still proves nothing about who is at the keyboard for a given write.
- **Startup.** An unreachable key set never stops `ipfs mcp`: static bearers work, and session tokens get 401 until the keys load. The log names the key set URL, never a token.
- **Logs.** The settings audit line names the principal and the `sub`. No log line, audit record or response contains a token or the email.

`GET /auth/session` returns the principals the token maps to (name and kind), the email for display, the token's `exp`, and the admin state (`ok`, `step_up_required` or `not_eligible`). It never returns the token or the `sub`. A static bearer alone gets 401 there.

To turn it off, set `"auth": {"oidc": {"enabled": false}}` (or remove the section) and restart `ipfs mcp`; to narrow it, shorten `routes` or empty a principal's `subjects`. Signing out of the console ends the gateway's cookies and the issuer's login session, but **the Kratos session on the sign-in site survives a console logout**, so the next console sign-in is silent while it lives. A full sign-out is the sign-in site's own logout.

## Off switches

| To turn off | Do this |
|---|---|
| The whole command | Build with `make build GOTAGS=nomcp`. |
| The agent loop, AG-UI and A2A | Do not configure a model backend (the default), or build with `GOTAGS=noagent`. |
| A tool group | `--tools` with the groups you want, or `tools.groups`; `tools.disabled` lists single tools. |
| Every write | `--read-only` (or `tools.read_only`). |
| Destructive tools | Off by default; they need `destructive` in `--tools` or a principal's `destructive_groups`. |
| The HTTP listener | Off by default; do not pass `--http` or set `http.enabled`. `--http=false` overrides `http.enabled: true`. |
| A principal | Remove its entry or the variable or file its credential names. |
| MCP App views | `--no-app-views` (or `tools.app_views: false`). |
| Out-of-band confirmation prompts | `--confirm=none` or `IPFS_MCP_CONFIRM=none`. Per-call confirmation itself cannot be turned off. |
| Embeddings and search | Off unless the config file has an `embeddings` section. Per part: remove the `extractor` or `describer` block, set `bulk.enabled: false`, set `search_route.enabled: false` (see the [embeddings guide](embeddings.md)). |
| Session tokens (OpenID Connect) | Off by default. `"auth": {"oidc": {"enabled": false}}`, or remove the section; narrow with `auth.oidc.routes` or a principal's `subjects`. |
| AG-UI, A2A | Off by default; they need `--agui` / `--a2a`, `--http` and a model backend. |

Each option, its default and the test that runs it in the off state are listed in the [configuration reference](config-reference.md).

## Troubleshooting

- **`ipfs daemon API is not reachable`.** The message names the address or `api` file it tried. Start the daemon, or pass `--api` / set `IPFS_PATH`.
- **A tool is missing from `tools/list`.** Its group is not enabled, it is destructive and `destructive` is not selected, the principal lacks the grant, or the daemon lacks its RPC command. `ipfs_node_status` gives the reason for each tool.
- **`confirmation_unavailable`.** The client cannot elicit and no out-of-band mechanism is available (no `/usr/bin/swift` on macOS, no polkit action on Linux, no paired Desktop, or `--confirm=none`).
- **`egress_not_acknowledged`.** The principal has no `egress_ack` covering the host the content would go to. See the [agentic guide](agentic.md#principals-and-egress_ack).
- **HTTP 403 from `/mcp`.** A non-loopback `Host` header on a loopback listener, an `Origin` not in `http.allowed_origins`, or a forwarding header (`Forwarded`, `X-Forwarded-For`, `X-Real-IP`) on a listener with no credential configured.
- **`confirmation_pending` with reason `too_many_open_confirmations`.** Two out-of-band confirmations are already waiting for this principal or session. Nothing ran; answer them or let them expire, then retry.
