---
name: ipfs-pin-management
description: Manage pins on the connected IPFS node with the MCP tools pin_ls, pin_add, pin_verify, pin_update, pin_rm and repo_gc - keep content from being garbage collected, audit what is pinned, verify pinned DAGs are complete, move a pin to a new version, remove pins, and run garbage collection safely. Use this skill whenever the user talks about pinning, unpinning, keeping or preserving content, "will this get deleted", garbage collection, freeing disk space, repo size, or checking that pinned data is intact.
license: Apache-2.0 OR MIT
compatibility: Requires the Kubo "ipfs mcp" server connected to a running Kubo daemon. pin_update, pin_rm and repo_gc are destructive and are offered only when the operator started the server with "ipfs mcp --tools destructive" or enabled them in the --mcp-config file. Tool names follow mcp/contract/catalog.json.
metadata:
  area: pinning
  tools: pin_ls pin_add pin_verify pin_update pin_rm repo_stat repo_gc ipfs_node_status ipfs_index_reconcile
  contract: mcp/contract/catalog.json
---

# Pin management and GC safety

A pin tells the node to keep a DAG through garbage collection. Content that is not pinned (and not in MFS) is eligible for removal the next time GC runs. Adding pins is a write; removing pins and running GC are destructive and off by default.

## Tool classes at a glance

| Tool | Class | Effect |
|---|---|---|
| `pin_ls`, `pin_verify`, `repo_stat` | read | Never prompt. |
| `pin_add` | write | Confirmed. Fetches missing blocks from the network. |
| `pin_update` | destructive | Confirmed. With the default `unpin: true` it removes the old pin, so it counts as `pin_rm` on that CID. |
| `pin_rm`, `repo_gc` | destructive | Confirmed. |

Read [references/confirmation.md](references/confirmation.md) before the first write or destructive call.

## Destructive tools may not be there

Destructive tools are off by default. Only the operator can enable them: by starting `ipfs mcp --tools destructive`, or by naming `destructive` in `tools.groups` of the file passed with `--mcp-config` (not the root `--config`, which is Kubo's own repo config). For HTTP principals, a `destructive_groups` grant does it. Call `ipfs_node_status`: a tool listed in `unavailable_tools` with reason `destructive_disabled` is off for that reason. Tell the person what enables it and that it is their call; never try to enable it yourself, edit configuration, or restart the server with other flags. `read_only` and `principal_not_granted` are other reasons you may see; see `ipfs-troubleshooting`.

## Pin something

1. `pin_ls` with `path` set to the CID first: it may be pinned already (`type` direct, recursive or indirect). An `indirect` pin means a recursive pin higher up already keeps it.
2. `pin_add` with `path` (CID or content path), `recursive` (default true: pins the whole DAG; set false only when the person wants just the root block) and an optional `name` to make the pin findable later. The node fetches missing blocks, which can take a while or time out when no provider is reachable; see `ipfs-troubleshooting`.
3. Confirm with `pin_ls` and report the result.

## Audit

`pin_ls` filters by `type` (all, direct, recursive, indirect), `path` and `name`, and pages with `page.next_cursor` passed back as `cursor` (opaque; do not edit it). `pin_verify` checks that pinned DAGs are complete and intact; use `only_failures: true` for a short report. A result with `ok: false` lists `bad_nodes`. The usual repair is to fetch the missing blocks again (`pin_add` on the same CID) while a provider is reachable.

## Replace a pin with a newer version

`pin_update` with `from` (old recursive pin) and `to` (new CID) transfers only the differences. Because `unpin` defaults to true, the old pin is removed after the move. Say this to the person before calling. Pass `unpin: false` if they want both to stay, but the tool remains destructive-class and still needs the destructive setting and a confirmation.

## Remove pins

`pin_rm` makes the content eligible for GC; it does not delete blocks immediately. Before calling:

1. `pin_ls` to show exactly which pin goes and its `name`.
2. Check whether another pin or an MFS path still references the same data (`pin_ls` with `type: indirect`; `mfs_stat`).
3. State the effect in the person's terms ("this unpins X; its blocks can be removed by the next GC") and wait for their go-ahead, then call.

## Garbage collection

`repo_gc` removes every unpinned block from the repo. It is the only tool that frees repo space, and it cannot be undone. A safe sequence:

1. `repo_stat` for `repo_size`, `storage_max` and `num_objects`, so the person sees whether GC is needed at all.
2. `pin_ls` (all pages) so the person sees what is protected. Content you added with `pin: false` or fetched for reading is not.
3. Ask the person whether anything they still want is unpinned. Offer to `pin_add` it first.
4. Only then call `repo_gc`, once, and report `removed_count` and `errors`.
5. If content search is configured, finish with `ipfs_index_reconcile` (a write, confirmed; use `dry_run: true` first) so the index forgets removed CIDs. See `ipfs-content-search`.

Never run GC just because disk is nearly full without that review. Never suggest a third-party pinning service or remote endpoint; if the person wants an off-node copy, offer `car_export` (see `ipfs-dag-car`) and let them choose where it goes.

## CLI fallback (only when no MCP tool is available)

With a running daemon: `ipfs pin ls`, `ipfs pin add <cid>`, `ipfs pin verify`, `ipfs pin rm <cid>`, `ipfs repo gc`. These act on the operator's node: ask first, one at a time, and do not use them to go around a declined confirmation. Run them against the daemon (the default); do not stop the daemon or open the repo directly to garbage collect, because the daemon holds the repo lock.
