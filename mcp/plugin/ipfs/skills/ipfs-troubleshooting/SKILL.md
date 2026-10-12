---
name: ipfs-troubleshooting
description: Use when something about IPFS is not working - an IPFS tool is missing from the tool list, a tool returns an error code such as confirmation_unavailable or egress_not_acknowledged, the daemon is unreachable, a CID hangs, times out or has no providers, or announcements do nothing. Invoke this skill first, before guessing - it gives the diagnosis order starting with ipfs_node_status, the meaning of each error code, and the content, storage and stale-name checks.
license: Apache-2.0 OR MIT
compatibility: Requires the Kubo "ipfs mcp" server (stdio, or Streamable HTTP on 127.0.0.1:5050 by default) and a Kubo daemon. Tool names follow mcp/contract/catalog.json. Server flags and config belong to the operator.
metadata:
  area: troubleshooting
  tools: ipfs_node_status ipfs_node_info routing_findprovs routing_findpeer routing_provide swarm_peers stats_bw repo_stat
  contract: mcp/contract/catalog.json
---

# Troubleshooting IPFS tools and content

Start with facts from the server, not guesses. Diagnosis tools are read-only and never prompt. Fixes that need configuration, flags or restarts belong to the operator: describe the fix and let the person apply it; do not edit configuration, restart processes or change flags yourself.

## Step 1: ipfs_node_status

Call `ipfs_node_status` first. Read `daemon_reachable`, `daemon_version`, `version_supported`, `transport`, `read_only`, `enabled_groups` and `unavailable_tools`. Each unavailable tool has a `reason`; when several apply the first in this order is reported.

| reason | Meaning | What the operator does |
|---|---|---|
| `read_only` | Server runs with `--read-only`; only tools that change nothing are offered. | Restart `ipfs mcp` without `--read-only`. |
| `group_disabled` | The tool's group is not selected. | Add the group to `--tools` or to `tools.groups` in the `--mcp-config` file. |
| `destructive_disabled` | The group is on, the tool is destructive (`pin_update`, `pin_rm`, `mfs_rm`, `key_rm`, `repo_gc`) and the destructive selector is not named. Off by default. | Start with `ipfs mcp --tools destructive`, or add `destructive` to `tools.groups` in the `--mcp-config` file (not the root `--config`). Every destructive call is still confirmed. |
| `principal_not_granted` | HTTP principal lacks the group or destructive grant. | Grant it in the principal's configuration. |
| `not_configured` | Needs configuration that is missing (for example the search index without embeddings). | Configure it, or do without. See `ipfs-content-search`. |
| `daemon_too_old` | The daemon predates the tool. | Upgrade Kubo. |
| `missing_rpc_command` | The daemon lacks a required RPC command (`missing_rpc` names them). | Upgrade or fix API allowed paths (`ipfs mcp --print-rpc-allowed-paths` prints the needed list). |

Report the reason and the operator-side fix. Do not substitute a different tool to get a destructive effect, and do not fall back to the `ipfs` CLI to bypass `destructive_disabled`, `read_only` or a declined confirmation.

If the server itself is unreachable over HTTP: the default listener is `127.0.0.1:5050`; a non-loopback bind needs a configured bearer token (read from a file or environment variable, never typed into a config literal). A loopback listener with no credential serves a read-only anonymous principal, so writes through it will be refused.

## Step 2: tool error codes

Results with `isError` carry `error.code`, `message` and `retryable`.

| code | Do |
|---|---|
| `daemon_unreachable` | The Kubo daemon is not running or the API address is wrong. Ask the person to start `ipfs daemon` (or fix `--api`). There is no offline mode. |
| `daemon_version_unsupported` | Upgrade Kubo. |
| `tool_unavailable` | See Step 1. |
| `confirmation_required`, `confirmation_pending`, `confirmation_declined`, `confirmation_expired`, `confirmation_invalid` | Follow [references/confirmation.md](references/confirmation.md). |
| `confirmation_unavailable` | No human-presence mechanism exists on this machine (Windows without paired IPFS Desktop; Linux without the polkit action; or macOS prompt unavailable). Writes and destructive calls cannot run; tell the person the operator must set up a resolver or use a host that supports confirmation. Never work around it. |
| `permission_denied`, `unauthenticated` | The principal or token lacks rights. Operator action. |
| `egress_not_acknowledged` | The operator has not allowed content from this tool to flow to the model host. Report; do not retry. |
| `size_limit_exceeded` | Use `offset` and `length`, the `resource` link, or `car_export`. |
| `timeout` | Often unreachable content; go to Step 3. Retry once at most. |
| `not_found`, `already_exists`, `invalid_argument` | Fix the argument; check paths and CIDs. |
| `index_unavailable` | See `ipfs-content-search`. |
| `rpc_error` | Quote `details` to the person; the daemon rejected the call. |

## Step 3: content will not load

1. `ipfs_node_info` and `swarm_peers`: does the node have any connected peers? Zero peers means a connectivity problem on this node, not a missing CID. `stats_bw` shows whether traffic flows.
2. `routing_findprovs` with the CID (default `num_providers: 20`): it returns `providers` with peer IDs and addresses. An empty list means the routing system found nobody announcing it. Report that plainly: the content may be offline, never announced, or announced by a node that is down.
3. `routing_findpeer` with a provider's `peer_id` to see whether it is reachable.
4. If the person owns the content, ask them to make sure the source node has it pinned and is online, and that it announced it (`routing_provide`; `announced: false` with `providing_disabled: true` means the node has providing turned off, which only its operator can change).
5. Do not loop on retries. Do not suggest hosted pinning services or specific third-party endpoints; suggest the person re-add the content on a node they run or obtain it from a source they trust.

## Step 4: resource and storage

`repo_stat` gives `repo_size`, `storage_max` and `num_objects`; near-full repos cause failures when adding or pinning. Clearing space is `ipfs-pin-management` (review before any garbage collection).

## Slow or stale names

For IPNS problems check lifetime, TTL and `nocache` in `ipfs-ipns-names`; a name whose record expired will not resolve until its owner republishes.

## CLI fallback (diagnosis only)

With a running daemon: `ipfs id`, `ipfs swarm peers`, `ipfs routing findprovs <cid>`, `ipfs stats bw`, `ipfs repo stat`. These read state. Do not run repo-opening commands while the daemon is running, and do not use the CLI to circumvent a refused or declined action.
