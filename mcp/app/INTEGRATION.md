# Integration note for mcp/server (mcp-app-views task 1.2)

For the mcp-server-engineer. The views, the committed bundle and a Go embed package are done; registration of the `ui://` resources and the `_meta.ui` links on tools is yours (`mcp/server`). Nothing in `mcp/server` was edited.

## What is shipped

- `mcp/app/dist/<view>.html` — five single-file HTML documents (script, style and brand fonts inlined), plus `dist/manifest.json`. Committed. Rebuilt with `pnpm build` in `mcp/app`.
- `mcp/app/embed.go` — package `github.com/ipfs/kubo/mcp/app`. `go:embed` cannot reach a sibling directory from `mcp/server`, so the embed lives next to `dist/`. API: `app.MIMEType`, `app.LoadManifest() (*Manifest, error)`, `app.HTML(View) ([]byte, error)`. `embed_test.go` checks every embedded document against the manifest (size, sha256, CSP, no external references). Import it from `mcp/server`; do not add a JS step to `make build`.

## Resources: register one per view

MIME type for every resource and every `resources/read` content item: `text/html;profile=mcp-app` (SEP-1865, `RESOURCE_MIME_TYPE` in ext-apps 2.0.3). go-sdk v1.8.0 has no helper, set it by hand.

| View | URI | Name (title) | Linked tools |
|---|---|---|---|
| content | `ui://ipfs-mcp/content` | IPFS content | ipfs_add, ipfs_cat, ipfs_get, ipfs_ls, ipfs_index_search, ipfs_index_cid |
| pins | `ui://ipfs-mcp/pins` | IPFS pins | pin_ls, pin_add, pin_verify, pin_update, pin_rm |
| mfs | `ui://ipfs-mcp/mfs` | IPFS files | mfs_ls, mfs_stat, mfs_read, mfs_write, mfs_mkdir, mfs_cp, mfs_mv, mfs_rm |
| dag | `ui://ipfs-mcp/dag` | IPFS DAG | dag_get, dag_stat, dag_put, car_import, car_export |
| node-status | `ui://ipfs-mcp/node-status` | IPFS node status | ipfs_node_info, ipfs_node_status, routing_findprovs, routing_findpeer, routing_provide, swarm_peers, stats_bw, repo_stat, repo_gc, ipfs_index_status, ipfs_index_reconcile |

The URIs, names, descriptions and tool lists are in `dist/manifest.json` (generated from the contract's `ui.appView`); iterate it instead of hard-coding. The IPNS and key tools (`ipns_*`, `key_*`) have no `appView` in the contract and get no link.

Resource fields: `uri`, `name`, `description`, `mimeType`. Content item of `resources/read`: `{uri, mimeType, text: <html>}` and `_meta` from `manifest.resourceMeta`, i.e. `{"ui": {"prefersBorder": false}}`.

Do NOT set `_meta.ui.csp`, `_meta.ui.permissions` or `_meta.ui.domain`. Omitted means the host applies its deny-by-default CSP (no network, no origins), which is what the views are built and tested for. The document also carries its own CSP meta tag (`manifest.csp`, `default-src 'none'; ... connect-src 'none'`), which can only narrow the host's.

## Tools: link each one

On every tool in the table, set both forms (older hosts read the flat key):

```json
"_meta": {
  "ui": { "resourceUri": "ui://ipfs-mcp/pins" },
  "ui/resourceUri": "ui://ipfs-mcp/pins"
}
```

Leave `visibility` unset (default `["model","app"]`): the views call the read-only tools (`ipfs_ls`, `mfs_ls`, `mfs_stat`, `pin_ls`, `swarm_peers`, and a repeat of whichever read tool produced the screen) through the host, and need the app to be allowed to call them. The views never call a tool whose confirmation class is not `none` (enforced in `src/core/result.ts` `isReadOnlyTool`, tested).

## Tool results: what the views read

- `structuredContent` is the contract output; `content[0].text` stays the text fallback (hosts without MCP Apps render only that, which is the "tools work without UI" requirement).
- Errors: `isError: true` and `structuredContent = {"error": {...}}` (already so in `registry.go`). A `confirmation_required` / `confirmation_pending` error with `error.confirmation` draws the confirmation panel.
- Recommended, optional: set `_meta["ai.ipfs/tool"] = "<tool name>"` on each result (next to the existing `ai.ipfs/untrusted-content`). The view identifies the tool from that marker first, then `hostContext.toolInfo.tool.name`, then by validating the output against the schemas of the tools linked to the view (tested to be unambiguous for every current tool). The marker removes any dependence on the host supplying `toolInfo`.

## Confirmations in a view (ADR-0003 and Amendment)

- The view never shows Approve. Approval happens in the host's prompt, the paired Desktop window or the OS prompt; the panel says where and that nothing has run. `requires_native_prompt: true` shows "Waiting for system prompt".
- Deny is offered when the host accepts `ui/update-model-context`. It marks the panel Denied and tells the model not to repeat the call. It cannot cancel the server-side pending confirmation (there is no server tool for that); the confirmation expires, and expiry is denial. If you add an app-only tool such as `confirmation_deny {confirmation_id}` (`_meta.ui.visibility: ["app"]`), the view can call it instead; tell the ui-ux-engineer and it is a one-function change in `src/ui/ViewSession.tsx` (`deny`).
- "Request unpin" in the pins view sends a `ui/message` asking the agent to remove the pin. The destructive `pin_rm` call is made by the agent and confirmed server-side as usual. Shown only when the host declares `message` support.

## Verification you can run

- Unit test 1.2: iterate `app.LoadManifest().Views`, register, then `resources/read` each `View.URI` with networking disabled and compare to `app.HTML(view)`.
- `cd mcp/app && pnpm check` (typecheck, 185 tests, build, bundle scan) and `pnpm check:dist` (rebuild-and-diff; exits 1 when `dist/` is stale).
- CI order: `pnpm --dir mcp/ui install --frozen-lockfile && pnpm --dir mcp/ui build`, then `pnpm --dir mcp/app install --frozen-lockfile && pnpm --dir mcp/app check:dist`. The app builds against the built `mcp/ui/dist` (`link:../ui`), so build the library first.

## Real-host check (task 1.4)

Not done here (needs an interactive host). To record it: start `ipfs mcp` with the views registered in Claude Code, run `pin_ls`, and capture the screenshot and transcript; then disable MCP Apps rendering (or use a host without it) and capture that `pin_ls` still returns the structured pin list. `mcp/app/tests/visual/host.ts` plays the host side of the protocol for the browser specs if a quicker loop is needed.
