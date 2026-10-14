# IPFS MCP

`ipfs mcp` serves the connected Kubo node to AI agents over MCP (stdio and Streamable HTTP), AG-UI and A2A. It is a client of a running daemon's `/api/v0` RPC API only: it never opens the repo, and it changes nothing in `/api/v0`, the HTTP gateway, the default CID recipe or wire behavior. Decisions are recorded in [`docs/mcp/`](../docs/mcp/) (start with [ADR-0001](../docs/mcp/0001-mcp-server-placement.md) and [ADR-0003](../docs/mcp/0003-confirmation-and-auth.md)).

This README describes the **tool contract** in [`contract/`](contract/): the single source of truth that every other part of `mcp/` is generated from or validated against.

## Using it

- [User guide](../docs/mcp/user-guide.md): install, client configuration, HTTP and authentication, least privilege, confirmation and its limits, use with stock Kubo and IPFS Desktop, off switches, and what is not delivered yet.
- [Configuration reference](../docs/mcp/config-reference.md): every config key, option, environment variable and build tag with its default, off switch and the tests that run it disabled. Generated from the config schema by [`docs/mcp/config_reference.py`](../docs/mcp/config_reference.py).
- [Agentic guide](../docs/mcp/agentic.md): the agent loop, model backends, AG-UI, A2A, principals and `egress_ack`.
- [Embeddings guide](../docs/mcp/embeddings.md): SurrealDB store modes, the embedder, indexing policy and pin-aware deletion.
- [Chat setup](../docs/mcp/chat-setup.md): the agent chat in the Web UI and IPFS Desktop.
- Plugins for Claude Code, Codex and OpenCode: [`plugin/README.md`](plugin/README.md).

## Contract layout

| Path | What it defines |
|---|---|
| [`contract/catalog.json`](contract/catalog.json) | Every tool: name, group, title, description, MCP annotations, confirmation class (`none`, `write`, `destructive`), `returnsUntrustedContent`, `contentEgress`, required `/api/v0` command paths (`rpc`) and paths used only by an optional input (`rpcOptional`), `minKuboVersion`, pagination, schema paths, A2UI and MCP App bindings, A2A skill and example, confirmation summary template. Also the global minimum daemon version and the default tool selection. The description of each skill entry tool (22 tools) ends with one pointer sentence, "If the ipfs-<skill> skill is available, load it first ...", because live trigger evals showed models call the tool instead of loading the skill; clients without skills can ignore it (`ipfs-agent-skills` 1.4, decisions.md 2026-10-12). |
| [`contract/catalog.schema.json`](contract/catalog.schema.json) | Shape of the catalog. |
| [`contract/schemas/common.json`](contract/schemas/common.json) | Shared value types (CID, paths, cursor pagination `PageInfo`, `ResourceRef`, `ConfirmToken`) and the shared error model `ToolError` with its `ErrorCode` list. |
| [`contract/schemas/tools/`](contract/schemas/tools/) | One JSON Schema 2020-12 input and one output schema per tool. Inputs and outputs use snake_case properties and `additionalProperties: false`. |
| [`contract/confirmation/confirmation.schema.json`](contract/confirmation/confirmation.schema.json) | Confirmation protocol: elicitation request and result, `confirmation_required` payload, confirm-token claims and the token acceptance table, the pending-confirmation broker record, outcomes. Examples in [`contract/confirmation/examples/`](contract/confirmation/examples/). |
| [`contract/resources/`](contract/resources/) | Resource URI conventions for `ipfs://`, `ipns://` and `ui://ipfs-mcp/<view>` ([`uri-conventions.json`](contract/resources/uri-conventions.json), [`resource-uri.schema.json`](contract/resources/resource-uri.schema.json), examples). |
| [`contract/derivations/`](contract/derivations/) | Derived metadata rules: WebMCP annotations ([`webmcp.json`](contract/derivations/webmcp.json)), HTTP scopes ([`scopes.json`](contract/derivations/scopes.json)) and A2UI component binding references with chat rules ([`a2ui-components.json`](contract/derivations/a2ui-components.json)), which also name the catalog route (`GET /agui/catalog`, served from the AG-UI mount under the same protections as `/agui`) and the AG-UI `CUSTOM` event names for server-built surfaces (`a2ui`) and the per-run catalog advertisement (`a2ui.catalog`). |
| [`contract/agentic/a2a-skills.json`](contract/agentic/a2a-skills.json) | A2A skills for the agent card, one per set of tool groups. |
| [`contract/fixtures/`](contract/fixtures/) | AG-UI event sequences and A2A exchanges with their schemas ([`agui.schema.json`](contract/fixtures/agui.schema.json), [`a2a.schema.json`](contract/fixtures/a2a.schema.json)). Authored from the specifications until the endpoint changes replace them with recordings. |
| [`contract/config/ipfs-mcp-config.schema.json`](contract/config/ipfs-mcp-config.schema.json) | The `ipfs mcp` config file, named with `--mcp-config=<path>` or `IPFS_MCP_CONFIG` (the root `--config` flag keeps its Kubo meaning, the repo config file): HTTP listener, tool selection, principals, LLM backend, agent endpoints, confirmation modes and resolvers, embeddings. Every key carries `default`, `offSwitch` and `ownerChange`; the config reference in `docs/mcp/` is generated from them. Valid examples in [`contract/config/examples/`](contract/config/examples/), must-fail cases in [`contract/config/invalid/`](contract/config/invalid/). |
| [`contract/contract.go`](contract/contract.go), [`check.go`](contract/check.go), [`derive.go`](contract/derive.go), [`bundle.go`](contract/bundle.go) | Go package `github.com/ipfs/kubo/mcp/contract`: embeds the files, loads the catalog, enforces cross-field rules, evaluates derivation rules, computes tool selections and bundles a tool schema into a self-contained form for MCP clients. |
| [`contract/testdata/`](contract/testdata/) | Test-only data: the 2020-12 meta-schema, the golden WebMCP derivation table and the golden default `ipfs_node_status`. |

## Rules the contract enforces

- Tool names are snake_case with one of the prefixes `ipfs_`, `pin_`, `ipns_`, `key_`, `mfs_`, `dag_`, `car_`, `routing_`, `swarm_`, `stats_`, `repo_`.
- Confirmation class `none` means `readOnlyHint: true` and `destructiveHint: false`. Classes `write` and `destructive` mean `readOnlyHint: false`, and `destructive` also means `destructiveHint: true`. Class `destructive` is for every tool that can remove an operator-held object (a pin, an MFS entry, a keystore key or repo blocks), including through an option default; a tool that calls `pin/rm`, `pin/update`, `files/rm`, `key/rm` or `repo/gc` must be destructive. A `write` tool may still set `destructiveHint: true` when it replaces state at the target it names (`mfs_write`, `ipns_publish`, `ipfs_index_reconcile`); the hint is advisory, the class is enforced. Write and destructive tools take an optional `confirm_token` input and carry a `confirmationSummary`; read tools have neither.
- A tool with `returnsUntrustedContent` is also `contentEgress`: its result is refused to principals without `egress_ack` and never sent to an unacknowledged model or embedder host.
- **Destructive tools are off by default** (ADR-0003 section 2). The default tool set is every group with classes `none` and `write`. `pin_update` (its default `unpin: true` removes the old pin), `pin_rm`, `mfs_rm`, `key_rm` and `repo_gc` are enabled only by naming the selector `destructive` (`--tools` or `tools.groups`) or, for HTTP, AG-UI and A2A principals, by a `destructive_groups` grant. Configured principals are read-only by default. Every enabled destructive call is still confirmed. `ipfs_node_status` reports a destructive tool left out this way with reason `destructive_disabled`, and a tool whose group is not selected with `group_disabled`.
- **A confirm token is a binding handle, not consent** (ADR-0003 section 4). It is accepted only after its pending confirmation was approved by elicitation or a human-presence resolver, and it is never issued to or accepted from principals of kind `a2a` or `uar`.
- **Elicitation is a multi round trip, and its request state is not consent either.** The server never calls `ServerSession.Elicit` (go-sdk v1.8.0 refuses it on protocol 2026-07-28). The first call returns an `InputRequiredResult` with `inputRequests.confirm` and a server-signed `requestState`; the retry (`InputRetry`) carries the user's answer in `inputResponses.confirm`. Only `accept` with `confirm: true`, while the confirmation is pending, approves. The request state carries `use: request_state`, is accepted only from a session that advertised elicitation and only for a confirmation opened by elicitation, and is never interchangeable with a confirm token (`RequestStateClaims`, `RequestStateAcceptance`).
- A confirmation with `requires_native_prompt: true` (destructive calls from the `ui` and `a2a` principals) is decided by a presence resolver only: `macos-localauth` or `linux-polkit`. `desktop` never counts (ADR-0003, R14 amendment of 2026-10-12), and `cli` and `agui-interrupt` never satisfy it. The code (`server.IsPresenceResolver`) and the `requires_native_prompt` descriptions in `confirmation/confirmation.schema.json` agree. `ConfirmationRequired`, `PendingConfirmation` and the AG-UI interrupt all carry the flag; in-chat surfaces then offer deny only, and `agui-interrupt` is never listed as a resolver.
- WebMCP annotations (`readOnlyHint`, `consequentialHint`, `untrustedContentHint`, `debugging`) are derived from catalog fields, never authored; destructive tools are not exposed through WebMCP.
- Every `rpc` and `rpcOptional` path exists in Kubo's command tree; the server omits a tool when a daemon lacks one of its `rpc` paths and refuses to start below `minKuboVersion`. A missing or forbidden `rpcOptional` path only skips the optional step, with a warning in the result (`ipfs_add` with `provide: true` on a daemon before 0.42, which has no `provide/once`). `--print-rpc-allowed-paths` prints both lists (`Tool.AllowedRPC`).
- No tool lists a command whose `AllowedPaths` prefix grants more privileged commands (`config`, `key`, `repo`, `files`, `pin`, `name`, `dag`, `routing`, `provide`): Kubo matches by string prefix, so `/api/v0/config` would also allow config writes, `config/replace` and `config/profile/apply`. `routing_provide` therefore does not list `config`; when reading `Provide.Enabled` is forbidden it relies on `routing/provide`, which refuses by itself when providing is off.
- MFS path inputs (`MfsPath`) reject `/ipfs/`, `/ipns/` and `/ipld/` paths, which the daemon would resolve from the network. The one exception is the `mfs_cp` source (`MfsCopySource`), which also accepts `/ipfs/<cid>[/path]`.
- Every successful result of a `returnsUntrustedContent` tool carries `_meta["ai.ipfs/untrusted-content"] = true` (`contract.UntrustedContentMetaKey`).
- AG-UI runs (`fixtures/agui.schema.json`) follow these rules:
  - Every interrupt ends in a `TOOL_CALL_RESULT` on its original `toolCallId`: the tool output, `confirmation_declined` or `confirmation_expired`. The call is never re-emitted.
  - A deny is `resolved` + `{approved: false}`.
  - A native-prompt interrupt is observed with `{wait: true}`.
  - Expiry is denial.
  - `parentMessageId` messages are introduced before the turn's first result.
  - There is no default client endpoint.
  - Browser origins need `agent.origin_allowlist` (CORS without credentials).
- An HTTP loopback listener with no credential configured serves the read-only `anonymous-loopback` principal; no write or destructive group can reach it.
- The out-of-band human gate is on by default (`confirmation.modes` `["native"]` since 2026-10-12, `desktop` opt-in; `--confirm`, `IPFS_MCP_CONFIRM`); `IPFS_MCP_CONFIRM=none` turns it off, after which only eliciting clients can confirm writes. Per-call confirmation itself cannot be turned off.
- Config credentials are `{"env": ...}` or `{"file": "/absolute/path"}`, never literals. The default HTTP listen address is `127.0.0.1:5050`; ports of other local services are rejected. No key has a default network endpoint.

## Consumers

| Consumer | Uses |
|---|---|
| `mcp/server` (Go, `ipfs mcp`) | `contract.Load`, `CheckToolNames` at startup, `SelectTools`, `BundleSchema` for `inputSchema` and `outputSchema`, the confirmation protocol, resource URIs, the config schema, A2A skills and the fixtures in its endpoint tests. |
| `mcp/webmcp` (TypeScript) | Tool names, descriptions, input schemas and the WebMCP derivation rules. |
| `mcp/ui` and `mcp/app` (React 19) | Output schemas for component props, the A2UI binding references and chat rules (catalog ID `https://github.com/GQAdonis/kubo/blob/master/mcp/ui/catalog/ipfs-mcp.catalog.json`, an identifier that is never fetched and that servers advertise in `supportedCatalogIds`; `ipfs-mcp` is a short alias only), `ui://` views, the AG-UI fixtures. |
| `mcp/skills` | Tool names; every name a skill mentions must exist in the catalog. |
| `docs/mcp/` | The config reference, generated from the config schema annotations (`python3 docs/mcp/config_reference.py --write`; `--check --ipfs <binary>` fails on any key, option or test that does not match). |

## Validating

```bash
go test ./mcp/contract/...
```

The test validates every schema against the JSON Schema 2020-12 meta-schema (vendored in `testdata/`; nothing is fetched), the catalog and all self-described documents against their schemas, every example and fixture, the cross-field rules, the derived WebMCP annotations against the golden table, the bindings, the RPC paths and the config key annotations. It fails, for example, when a tool is marked both destructive and read-only, when a consumer names a tool the catalog lacks, or when a config key loses its `default` or `offSwitch`.

Changing the contract: edit the JSON, update the golden table in `testdata/` only when a derivation input changed on purpose, and run the test.
