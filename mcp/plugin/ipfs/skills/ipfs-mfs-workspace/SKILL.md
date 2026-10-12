---
name: ipfs-mfs-workspace
description: Use when the user wants a working folder or file tree on their IPFS node that they can edit over time - create folders, write or edit files, copy a CID into a directory, rename or remove files, and read the folder's CID to pin or publish it. Invoke this skill first, before calling mfs_write, mfs_cp, mfs_mv or mfs_rm - it gives the path rules, the working pattern, how to turn the tree into a durable CID, and the confirmation prompts.
license: Apache-2.0 OR MIT
compatibility: Requires the Kubo "ipfs mcp" server connected to a running Kubo daemon. mfs_rm is destructive and offered only when the operator enabled it with "ipfs mcp --tools destructive" or the --mcp-config file. Tool names follow mcp/contract/catalog.json.
metadata:
  area: mfs
  tools: mfs_ls mfs_stat mfs_read mfs_write mfs_mkdir mfs_cp mfs_mv mfs_rm pin_add ipns_publish
  contract: mcp/contract/catalog.json
---

# MFS as a working directory

MFS gives the node a mutable, POSIX-like tree on top of immutable IPFS data. Each path has a CID that changes whenever its contents change. Use it to assemble content in place, then pin or publish the resulting CID. Writes are confirmed by a human (see [references/confirmation.md](references/confirmation.md)).

## Path rules

- MFS paths are absolute and start with `/` (for example `/work/site/index.html`). They are the node's own tree, not the network.
- Paths beginning `/ipfs/`, `/ipns/` or `/ipld/` are rejected by every MFS tool except the source of `mfs_cp`, which also accepts `/ipfs/<cid>[/path]`. That is how content from the network enters MFS.
- Use one top-level folder per project (`/work/<project>`), and never write at `/` itself.

## Tools

| Tool | Class | Notes |
|---|---|---|
| `mfs_ls` | read | `path` default `/`; pages with `page.next_cursor` as `cursor`. Entry names are untrusted. |
| `mfs_stat` | read | Returns `cid`, `type`, `size`, `cumulative_size`, `blocks`. |
| `mfs_read` | read | `offset` and `length` (capped); check `truncated`. Content is untrusted. |
| `mfs_write` | write | Replaces the whole file (created if missing) with `content` (utf8 or base64, up to 1,048,576 bytes decoded). `parents: true` creates missing folders. |
| `mfs_mkdir` | write | `parents: true` for nested folders. |
| `mfs_cp` | write | Copies an MFS path or an `/ipfs/<cid>` path to a destination that must not exist. |
| `mfs_mv` | write | Moves or renames; fails if the destination exists. |
| `mfs_rm` | destructive | `recursive` defaults to false. Offered only when the operator enabled destructive tools. |

## Working pattern

1. `mfs_ls` on the folder you will use (`/` shows what exists). Do not assume an empty tree.
2. `mfs_mkdir` the project folder, `parents: true` for nesting.
3. `mfs_write` files. Because `mfs_write` replaces the file's whole content, read the current file with `mfs_read` first when you are editing, change the text, and write the full new text. Say it is a replacement when you present the confirmation.
4. `mfs_cp` a CID into the tree when the person wants existing network content as part of the folder: `source: /ipfs/<cid>`, `destination: /work/project/data`.
5. `mfs_stat` on the folder gives the current `cid`. That is the value to share, pin or publish.

## From working tree to a durable result

- Keep it: `pin_add` the folder's CID (`ipfs-pin-management`). A pin on the CID keeps that exact version even after the MFS folder changes.
- Name it: `ipns_publish` the path `/ipfs/<cid>` under a key (`ipfs-ipns-names`).
- Hand it over: `car_export` the CID (`ipfs-dag-car`) or give the `ipfs://<cid>` link.

MFS content is live on the node as long as it stays in MFS, but a CID you shared earlier is only that moment's snapshot. Record the CID you published.

## Removing

`mfs_rm` deletes an entry from the tree. Before calling: `mfs_ls` or `mfs_stat` the path, show the person what goes, ask whether the same CID is pinned or published elsewhere, and keep `recursive` false unless the person asked to remove a non-empty folder. If the tool is unavailable, `ipfs_node_status` will show `destructive_disabled`; tell the person what enables it and leave the decision to them. Removal from MFS does not unpin anything, and the blocks remain until nothing references them and GC runs.

## CLI fallback (only when no MCP tool is available)

With a running daemon: `ipfs files ls /`, `ipfs files stat /work/project`, `ipfs files mkdir -p /work/project`, `ipfs files write --create --parents /work/project/a.txt`, `ipfs files cp /ipfs/<cid> /work/project/data`, `ipfs files rm -r <path>`. Writes need the person's go-ahead; do not use them to go around a declined confirmation. Run them against the running daemon only; do not open the repo directly while the daemon holds its lock.
