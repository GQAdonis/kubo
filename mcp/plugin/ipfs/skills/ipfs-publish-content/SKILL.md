---
name: ipfs-publish-content
description: Use when the user wants to publish, share, upload or put text, notes, JSON or a small file on IPFS, get a CID or ipfs:// or gateway link for it, or make content available to other peers. Invoke this skill first, before calling ipfs_add - the tool schema alone does not say whether to pin or announce, how to hand back the CID and link, how to verify the result, or how the required human confirmation works.
license: Apache-2.0 OR MIT
compatibility: Requires the Kubo "ipfs mcp" server (stdio or Streamable HTTP, default 127.0.0.1:5050) connected to a running Kubo daemon. Tool names follow mcp/contract/catalog.json. The ipfs CLI is an optional fallback when the MCP tools are not available.
metadata:
  area: push
  tools: ipfs_add ipfs_node_status routing_provide ipfs_get ipfs_cat pin_add
  contract: mcp/contract/catalog.json
---

# Publish content to IPFS

Adding content is a write. The node stores new blocks, may pin them, and may announce them to the network, so every `ipfs_add` call is held for a human decision. Plan the call, state what it will do, then let the confirmation flow run.

## Before you add

1. Call `ipfs_node_status`. Continue only when `daemon_reachable` is true and `ipfs_add` is not in `unavailable_tools`. If it is, read `reason` and follow the `ipfs-troubleshooting` skill.
2. Settle with the person: what bytes go in, whether to keep them (`pin`), whether to announce them (`provide`). Pin and provide are required fields because both affect the operator's node and network footprint; never default them silently.
3. Check the size. `content` is capped at 1,048,576 decoded bytes. Larger data does not belong in a single call; ask the person for another route (for example a CAR import, see `ipfs-dag-car`).

## The add call

`ipfs_add` takes the bytes in the request itself. It never reads host files, and a path-like string is added literally as text, so never pass a local path hoping the server reads it. Read the file's text yourself with the host's own file tools if the person wants it published, then put it in `content`.

| Field | Use |
|---|---|
| `content` | The text, or base64 bytes. |
| `encoding` | `utf8` (default) or `base64` for binary data. |
| `filename` | Optional. When set, the file is wrapped in a directory, so the returned CID is the directory's. |
| `pin` | `true` to keep the data through garbage collection; `false` leaves it collectable. Required. |
| `provide` | `true` to announce the CID to the routing system. Required. Has no effect when the node has providing disabled. |

Leave `cid_version`, `raw_leaves`, `chunker` and `hash` unset. Omitting them keeps the node's default CID recipe, which keeps the same bytes producing the same CID everywhere. Set one only when the person names a specific recipe, and say which option you are changing and why.

## Handle the result

A successful result has `cid`, `size`, `name`, `pinned`, `provided`, `resource` (an `ipfs://` URI) and optional `warnings`.

- Report `cid` and the `ipfs://` link from `resource`.
- Compare `pinned` and `provided` with what was requested. If `provided` is false although you asked for it, the node has providing turned off; say so. Do not change node configuration to force it.
- Surface every entry in `warnings` verbatim.
- For an HTTP gateway URL, use the gateway address the person or operator gave you, written as `<their-gateway>/ipfs/<cid>`. There is no tool that returns one and no built-in default: never invent or hardcode a public gateway.

## Confirmation

Read [references/confirmation.md](references/confirmation.md) before the first write of a session. In short: the first call does nothing and returns what is pending; the human decides through the host or a native OS prompt; only then does the call run, and a `confirm_token` is used once, for the same arguments, never reused and never made up. A decline is final. `confirmation_unavailable` means this machine has no way to confirm; tell the person and stop.

## Announcing later

If you added with `provide: false` and the person now wants it announced, call `routing_provide` with the `cid` (a write, confirmed too). The result's `announced` and `providing_disabled` fields say what happened. Then verify reachability with the `ipfs-troubleshooting` skill.

## Verify after publishing

Read the content back with `ipfs_cat` using the new CID and compare it to what you sent. See `ipfs-retrieve-verify`. To keep it long term, confirm `pinned` is true or follow `ipfs-pin-management`.

## Making content findable by meaning

Content added through `ipfs mcp` can be indexed when the operator configured embeddings; see `ipfs-content-search`. A failed indexing attempt appears as a warning on the add, not as an error.

## CLI fallback (only when no MCP tool is available)

With a running daemon, `ipfs add --pin=true --quiet <file>` adds a file and prints its CID, and `ipfs routing provide <cid>` announces it. These are still writes on the operator's node: ask the person first, run them one at a time, and do not use them to get around a declined confirmation. Do not run commands that open the repository directly (for example offline `ipfs repo` operations) while the daemon is running, because the daemon holds the repo lock.
