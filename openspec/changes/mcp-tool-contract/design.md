# Design: mcp-tool-contract

## Context

See proposal.md. Four consumers read this contract: the Go server (`mcp/server`), the TypeScript WebMCP package (`mcp/webmcp`), the A2UI catalog (`mcp/ui`) and the skills.

## Goals / Non-Goals

**Goals:**
- One emitter, one validator.
- Every consumer derives its metadata from the contract.

**Non-Goals:**
- Code generation for the Go server. Handlers are written by hand and checked against the contract at startup.

## Decisions

- **Plain JSON files with JSON Schema 2020-12.** They are language-neutral. A Go-struct source of truth was rejected because the TypeScript consumers could not read it without a generator.
- **The confirmation class is separate from the MCP annotations.** `destructiveHint` is advisory, while the confirmation class is enforced by the server.
- **WebMCP annotations are derived, not authored.**
  - `consequentialHint` is true for every tool that is not read-only.
  - `untrustedContentHint` is set from `returnsUntrustedContent`.
  - `idempotentHint` and `openWorldHint` are dropped for WebMCP, because the WebMCP draft of 8 Oct 2026 does not define them.
- **`minKuboVersion` and required command paths are recorded per tool.** They let the server degrade per tool against stock daemons.

## Risks / Trade-offs

- [Untyped `files/*` output drifts across Kubo versions] → The contract output schemas are checked by real-daemon tests in each tool change.
- [Schema churn during the tool changes] → Each tool change updates its own entries, and the validator test fails on inconsistency.

## Follow-up (2026-10-09)

Corrections from the server and UI streams before Round 4 consumes the contract. Destructive tools stay off by default and always confirmed (ADR-0003, operator answer 2026-10-09).

- **`destructive_disabled` reason.** `ipfs_node_status` distinguishes a destructive tool left out because the `destructive` selector was not named (`destructive_disabled`) from a tool whose group is not selected (`group_disabled`). Precedence: `read_only`, `group_disabled`, `destructive_disabled`, then the rest. Golden: `testdata/node-status-default.json`.
- **`pin_update` is class `destructive`.** Per MCP annotation semantics `destructiveHint: true` was already right: the default `unpin: true` removes the old pin. As class `write` it was a bypass of the destructive opt-in (pin_update from any recursive pin to any other CID unpins it). The class rule is now explicit: a tool that can remove a pin, MFS entry, keystore key or repo blocks, including through an option default, is destructive; `TestRemovingRPCIsDestructive` checks it from the `rpc` list. Write tools keep `destructiveHint: true` where they replace state at the named target (`mfs_write`, `ipns_publish`, `ipfs_index_reconcile`). WebMCP no longer exposes `pin_update`; its scope becomes `ipfs:admin`.
- **Elicitation round trip.** go-sdk v1.8.0 refuses `ServerSession.Elicit` on protocol 2026-07-28, so mechanism 1 is an SEP-2322 multi round trip (`InputRequiredResult`, `InputRetry`). The `requestState` is an opaque, server-signed binding handle, not consent: `RequestStateClaims` adds `use: request_state` for domain separation from the confirm token (same key), and `RequestStateAcceptance` accepts it only from an eliciting session, for a confirmation opened by elicitation and still pending, with an `accept` answer carrying `confirm: true`. This is the inverse of `TokenAcceptance`, which requires `approved`. ADR-0003 mechanism 1 still names `ServerSession.Elicit` and needs an amendment note.
- **Native prompt on every surface.** `ConfirmationRequired` carries `requires_native_prompt`; when true the summary class is destructive and `agui-interrupt` is not a resolver. The AG-UI fixture no longer invites approval in chat.
- **A2UI catalog ID is a URI.** `catalogId` is `https://github.com/GQAdonis/kubo/blob/master/mcp/ui/catalog/ipfs-mcp.catalog.json` (identifier, never fetched; advertised in `supportedCatalogIds`); `ipfs-mcp` is kept in `catalogAliases`.
- **Routing tools render in `NodeStatusPanel`** (`appView` `node-status`): provider lists and peer addresses are peer data like `swarm_peers`.

## Follow-up after Round 4 (2026-10-09)

Contract changes that record the Round 4 decisions and align the `rpc` lists with what the server calls. ADR-0003 has a Round 4 amendment; ADR-0004 and ADR-0001 are updated in place.

- **`rpcOptional` (new catalog field).** `rpc` stays "required": the probe hides a tool when the daemon lacks any of them. A command used only for an optional input goes in `rpcOptional`, which does not hide the tool and is disjoint from `rpc`. `Tool.AllowedRPC()` returns both and is what `--print-rpc-allowed-paths` must print; `TestRPCPathsExist` and `TestRemovingRPCIsDestructive` now check both lists.
  - `ipfs_add`: `rpcOptional: ["provide/once"]`, called only when `provide: true`. Not in `rpc`, because `provide/once` exists only from Kubo 0.42 and `ipfs_add` has `minKuboVersion` 0.29.0: listing it as required would hide `ipfs_add` on 0.29 to 0.41. On those daemons, or when the secret forbids it, the add succeeds with a warning and `provided: false` (current server behavior).
  - `dag_get`: `rpc: ["dag/get", "dag/resolve"]`. `dag/resolve` is needed for sub-path resolution; without it only bare `/ipfs/<cid>` paths work. It exists on every supported daemon, so requiring it hides nothing.
  - `routing_provide`: `config` is deliberately not listed. Kubo matches `AllowedPaths` by string prefix, so `/api/v0/config` would also grant config writes (`config <key> <value>`), `config/replace` and `config/profile/apply`, i.e. any write-class tool secret could rewrite the node's config, including `API.Authorizations`. The server already treats a forbidden `config` read as "unknown" and relies on `routing/provide`, which refuses with `Provide.Enabled is set to 'false'` by itself; the result is the same `providing_disabled: true`. A new `Check` rule forbids listing `config`, `key`, `repo`, `files`, `pin`, `name`, `dag`, `routing` or `provide` as a bare path in either list.
- **`--mcp-config` / `IPFS_MCP_CONFIG`.** The config file is named with a dedicated flag and environment variable, not the root `--config` (`--config-file`), which in Kubo names the repo config file. The config schema title, description and root `flag`/`env` annotations say so; README and ADR-0001/0003/0004/0006 references are updated.
- **Anonymous loopback principal is read-only.** A loopback HTTP listener with no credential configured serves `anonymous-loopback`, which reads every group and has no write or destructive grant. Recorded in the `principals` key's `offSwitch` and ADR-0003 Round 4 amendment section 3.
- **MFS tools reject content namespaces.** `MfsPath` now rejects `/ipfs/`, `/ipns/` and `/ipld/` paths (RE2- and ECMA-compatible pattern without lookahead, so the UI Zod generator keeps a plain regex). The `mfs_cp` source uses the new `MfsCopySource`, which also accepts `/ipfs/<cid>[/path]` but not `/ipns/` or `/ipld/`, matching the server. Pinned by `TestMfsPathsRejectContentNamespaces`.
- **Untrusted-content marker.** Every successful result of a `returnsUntrustedContent` tool carries `_meta["ai.ipfs/untrusted-content"] = true`; the key is `contract.UntrustedContentMetaKey` and is described in `catalog.schema.json`.
- **`car_import` never announces immediately.** The server sends `fast-provide-root=false` and `fast-provide-dag=false`; the catalog description says so and points to `routing_provide`. Blocks still enter the node's regular provide queue according to the operator's `Provide.Strategy`, as for any local import; that is Kubo behavior the operator controls, not an MCP announcement.
- **Human gate on by default, one off switch.** New config key `confirmation.modes` (`native`, `desktop`, `cli`; default `["native", "desktop"]`, changed to `["native"]` on 2026-10-12 (ADR-0003, R14 amendment); `flag` `--confirm`, `env` `IPFS_MCP_CONFIRM`; precedence flag, env, file, default). Off switch: `IPFS_MCP_CONFIRM=none`, `--confirm=none` or `modes: []`. Per-call confirmation is not turned off by it: with the gate off, only eliciting clients can confirm writes and other writes get `confirmation_unavailable`. `confirmation.resolvers` keeps narrowing the presence resolvers. Example `config/examples/confirm-gate-off.json`; must-fail `config/invalid/unknown-confirm-mode.json` (`agui-interrupt` is not a mode).
- **`requires_native_prompt` is satisfied only by presence resolvers** (`macos-localauth`, `linux-polkit`, `desktop`), never `cli` or `agui-interrupt`. This supersedes the ADR-0003 amendment text that listed the `--confirm=cli` opt-in; recorded in the `confirmation.resolvers` description and the README rules.
- **AG-UI gaps from `agent-chat-island`** (`mcp/chat/EVIDENCE.md`; ADR-0005 amendment of 2026-10-09). Implemented later by `mcp-agui-endpoint`.
  - Deny shape confirmed: `resolved` + `{approved: false}`. It is answered on the original `toolCallId` with `confirmation_declined` (`write-resume-denied.json`). `cancelled` is used only for an expired interrupt inside a batch; no payload is allowed with it.
  - Native-prompt return path: such an interrupt's `responseSchema` is `{approved: false}` or `{wait: true}`. `{wait: true}` holds the run open until the broker resolves or the confirmation expires. The outcome is reported as the tool result, `confirmation_declined` or `confirmation_expired` on the original `toolCallId` (`destructive-native-prompt-wait-{approved,denied,expired}.json`). `{approved: true}` gets HTTP 422.
  - Parent message rule: a `parentMessageId` is introduced (`TEXT_MESSAGE_START` or `MESSAGES_SNAPSHOT`) before the turn's first `TOOL_CALL_RESULT` or the end of the run. `read-pin-ls.json` and `egress-refused.json` now carry the snapshot. The server keeps no thread history.
  - Expiry is denial: no resume is sent, and a new user message is accepted once every open interrupt has expired. A late resume gets `confirmation_expired`.
  - No default client endpoint. The `agent-chat-island` proposal is amended to match its `design.md`.
  - CORS on `/agui`: the existing key `agent.origin_allowlist` (default empty; off switch is the empty list) now states the response headers. A `cors` annotation lists allowed methods `POST, OPTIONS`, allowed headers `authorization, content-type`, no `Allow-Credentials`, and `Vary: Origin`.
  - `TestAGUIFixtureRules` enforces the parent rule, no re-emission on resume, result content (tool output schema or `ToolError`), and coverage of approve, deny, wait, declined and expired.
- **Consumer follow-ups (not in this change's ownership).** `mcp/server/rpcpaths.go` must append `t.AllowedRPC()` instead of `t.RPC`, or the printed `AllowedPaths` omit `provide/once`; the server's `config.go` must read `--mcp-config`/`IPFS_MCP_CONFIG` and `confirmation.modes` (it currently only validates the `confirmation` section), and `mcp/server/README.md` and the `ipfs mcp` help text still say `--config`; `mcp/ui` must regenerate `src/contract/generated/common.ts` (`pnpm gen:contract`) for `MfsPath` and `MfsCopySource`; `openspec/changes/mcp-user-docs`, `mcp-agent-principal-enforcement` and `mcp-http-transport-auth` tasks still say `--config`.

## Open questions (operator, Round 7)

- **OQ-EGRESS-ENUM: do `swarm_peers`, `routing_*` and `key_list` become `contentEgress`?** Decided 2026-10-10 by the operator: no. All three stay `contentEgress: false`; no catalog, schema, fixture or test change follows. Consequence, accepted by the operator: a principal holding read grants on the swarm, routing or key groups can send the node's peer list, routing answers and key names to the model backend (`/agui`, the agent loop) or to an `a2a` peer without that host being in its `egress_ack`. Operators who do not want that withhold those groups from `ui` and `a2a` principals. The text below is the question as it stood before the decision. ADR-0004 (Round 6 amendment) defines content egress as stored bytes, names taken from stored data, or an enumeration of what the node stores, and left peer and key enumerations out. Arguments for yes: the peer list and routing answers say whom the node talks to and what it provides, and key names are operator-chosen labels, all of which then reach a model host or an `a2a` peer without `egress_ack`. Arguments for no: none of it is stored content, and flagging it refuses the node-status view (`NodeStatusPanel`) to every principal without `egress_ack`. A yes is a catalog change only (`mcp/contract/catalog.json` flags, the `contentEgress` rule in `catalog.schema.json`, a pinning test like `TestPinsetEnumerationIsContentEgress`, and the `node-status` fixtures); `/agui`, `/a2a` and the loop already enforce the flag per result. Referenced from `mcp-agui-endpoint` and `mcp-a2a-endpoint` design.md.

## Error codes (Round 7)

The shared `ErrorCode` list (`schemas/common.json`) gains `session_busy`, `failed_precondition` and `resource_exhausted` for the agent endpoints. The list is additive; the ErrorCode description now states the client rule that makes additions safe: an unknown code is an error, and `retryable` alone says whether to retry. The mapping is in ADR-0004 (Round 7 amendment), `mcp-agui-endpoint` design.md and `mcp-a2a-endpoint` design.md. AG-UI `RUN_ERROR.code` has its own vocabulary (`RunErrorCode` in `fixtures/agui.schema.json`).

## Agent endpoints as implemented (Round 7, after `mcp-a2a-endpoint` and AG-UI 2.1/2.2)

- **`requires_native_prompt` covers `a2a`.** The `ConfirmationRequired` and `PendingConfirmation` descriptions in `confirmation/confirmation.schema.json` and the `confirmation.resolvers` description in the config schema now say "ui and a2a"; the stale "or cli" in the `ConfirmationRequired` description is removed. No shape changed. ADR-0003 Round 7 amendment.
- **A2A streams.** `fixtures/a2a/stream-read.json` follows A2A 1.0.0 section 3.1.2 (Task first, then updates, terminal status update last), and `fixtures/a2a.schema.json` enforces the order for every stream fixture (`TestA2AStreamOrder`).
- **A2A `error.data`** is the A2A 9.5 array with a `google.rpc.ErrorInfo` and an `ipfs.mcp.ToolError` detail holding the shared ToolError. No contract schema models JSON-RPC error bodies yet; the shape is recorded in `mcp-a2a-endpoint` design.md ("Errors").
- **CORS.** `agent.origin_allowlist` documents `Access-Control-Expose-Headers: Retry-After`, and its `cors` annotation gains `exposeHeaders: ["Retry-After"]`.
