# Request to the UAR team: protocol conformance and safe defaults for use with Kubo `ipfs mcp`

- **From:** the kubo-ipfs-agentic team, GQAdonis/kubo fork (phase `ipfs-mcp-server`, OpenSpec change `uar-conformance-request`)
- **To:** the Universal Agent Runtime (UAR) team
- **Date written:** 2026-10-09
- **UAR source examined:** `/Users/gqadonis/Projects/prometheus/universal-agent-runtime`, HEAD `a7cb9729` (2026-10-01); re-checked at `b4244aea` (2026-10-12) for issue 379, remote `GQAdonis/universal-agent-runtime`, read-only. Kubo has not edited and will not edit UAR code.

Filed: https://github.com/Prometheus-AGS/universal-agent-runtime/issues/379 on 2026-10-12 (by the lead at the operator's direction; status per item re-checked against UAR `main` `b4244aea`, all nine items missing or partial; to be handled later)

(The line above is completed by the operator when the request is filed. It is task 1.2 of `uar-conformance-request` and is an operator action.)

## Why we are asking

Kubo's fork supports two agentic topologies (Kubo `docs/mcp/0004-agentic-endpoints-and-llm.md`):

- **Topology A:** `ipfs mcp` serves AG-UI and A2A itself and can call UAR's `/v1/chat/completions` as one possible model backend.
- **Topology B:** UAR hosts AG-UI and A2A and calls `ipfs mcp --http` as an MCP tool server. Kubo supports topology B only when the items below ship in a published UAR release.

Each item says what we observed in UAR's source, what we ask for, and an acceptance check that anyone can run against a published UAR release without building from source. We do not prescribe implementation. Where an item has several acceptable designs, the check tests the observable behavior only.

## The gate rule (published releases only)

Kubo counts an item as delivered only when all of these hold:

1. The item is contained in a **published, installable UAR release**: a git tag on the UAR repository, plus either an OCI image digest (`sha256:...`) or a native package with a published checksum, built from that tag.
2. Its acceptance check below passes when run against that artifact, installed as published, with no source build and no patches.
3. The release is published **by the end of Round 4** of the Kubo `ipfs-mcp-server` phase. The Kubo orchestrator evaluates the gate at that point and records the result, with the tag and digest or package checksum and the list of items that passed, in `mcp/uar/spike/REPORT.md` (task 1.1 of `uar-integration-spike`).

**Merged-but-unreleased items do not count.** A change merged on `main`, an open pull request, a nightly build or a branch build does not satisfy the gate, because Kubo's integration tests and setup-cost measurement use published artifacts only.

If the gate is not met, Kubo defers topology B and reports it as unmet for this phase. Kubo will not ship a topology-B configuration against a non-conformant UAR release. Topology A is unaffected either way.

When the gate passes, Kubo pins the minimum UAR version and the path of the item-8 settings endpoint from that release in its tool contract (`mcp/contract/agentic/uar-preflight.json`, referenced by the shipped UAR profile in `mcp/uar/profile/`), and `ipfs mcp check-uar` refuses older versions by reading the version from the agent card (item 9).

## How to run the checks

The checks assume a UAR instance started from the published artifact, with its documented configuration for a local, authenticated deployment. Environment variables used below:

```bash
export UAR=http://127.0.0.1:<uar-port>        # the instance under test
export UAR_TOKEN=<a valid user bearer token>   # for authenticated calls
export UAR_ADMIN_TOKEN=<an admin token>        # for the settings API
```

The A2A 1.0 JSON schema comes from `a2aproject/A2A` at tag `v1.0.0`. The AG-UI event schemas come from the published `@ag-ui/core` package. Schema validation can use any JSON Schema validator (for example `npx ajv-cli validate`). Exact A2A 1.0 method names and enum spellings are taken from that schema, not from this document.

## Requested items

### 1. A2A 1.0 agent card at `/.well-known/agent-card.json`

- **Observed:** the agent card is served at `/.well-known/agent.json` (`src/uar/api/a2a/mod.rs:75`, documented at `:6`). A2A 1.0 specifies `/.well-known/agent-card.json`.
- **Requested:** serve the A2A 1.0 agent card at `/.well-known/agent-card.json`. Keeping the old path as an alias is fine.
- **Acceptance check:**

  ```bash
  curl -sf "$UAR/.well-known/agent-card.json" -o card.json && echo HTTP_OK
  npx ajv-cli validate -s a2a-1.0.0.schema.json -r '<AgentCard definition>' -d card.json
  ```

  Pass: HTTP 200 with `Content-Type: application/json`, and the body validates as an A2A 1.0 `AgentCard`. The card's interface URL points at the instance's A2A endpoint.

### 2. A2A 1.0 TaskState wire values, including auth-required

- **Observed:** `TaskState` serializes with `#[serde(rename_all = "lowercase")]` (`src/uar/api/a2a/types.rs:118`), so `InputRequired` goes on the wire as `"inputrequired"`. There is no auth-required state (`types.rs:114-132`).
- **Requested:** emit task states exactly as the A2A 1.0 schema defines them, including the input-required and auth-required states.
- **Acceptance check:**
  1. Drive a task into the input-required state (the procedure in item 3 does this).
  2. Fetch it with the A2A 1.0 get-task method, and validate the response against the A2A 1.0 schema.
  3. Send a request whose credential is valid for the instance but lacks the scope a requested skill needs, and fetch that task.

  Pass: both responses validate against the A2A 1.0 schema; the first task's state is the schema's input-required value and the second's is the schema's auth-required value (or the request is rejected with the 1.0-specified authentication error, if the schema puts it there). A lowercase `"inputrequired"` fails.

### 3. Tool approval surfaced as A2A input-required, never bypassed for A2A tasks

- **Observed:** MCP tools are marked `ApprovalClass::Required` (`src/mcp/registry.rs:166-167`), but tool approval never reaches the A2A wire as input-required (the only mapping arm, `RunStatus::Paused`, is never assigned). A2A-delegated tasks are built with `tool_approval: ToolApprovalPolicy::Auto` (`src/uar/api/a2a/task_execution.rs:568`; whether that line is production code is unverified). Approval is bypassed when the governance gate is not effectively enabled (`src/uar/runtime/manager.rs:647-652, 5469-5480`).
- **Requested:** when a tool call in an A2A-delegated task needs approval, the task enters the A2A input-required state and waits; the tool does not run until approval arrives through UAR's approval mechanism. A2A tasks honor the configured approval policy and are not silently set to automatic approval.
- **Acceptance check:**
  1. Configure the instance with one MCP server exposing a tool marked as requiring approval (a test MCP server that records each invocation to a file is sufficient).
  2. From a test A2A client, send a message whose handling requires that tool.
  3. Poll the task, and read the test server's invocation log.
  4. Approve through UAR's documented approval call, then poll again.
  5. Repeat steps 1-3 with a second message and deny instead.

  Pass: before approval the task is in the A2A 1.0 input-required state and the invocation log is empty; after approval the tool is invoked exactly once and the task proceeds; after denial the log stays empty and the task ends in a terminal state. Repeat the whole check with the governance gate both enabled and disabled in the instance's configuration; it must pass in both.

### 4. AG-UI `RunAgentInput` endpoint with spec interrupts

- **Observed:** there is no endpoint that accepts `RunAgentInput` (a search of `src` for `RunAgentInput` finds nothing). Approval is sent as the `CUSTOM` event `uar.tool.approval_required` (`src/uar/api/adapters.rs:373`), not as a spec interrupt.
- **Requested:** a documented endpoint that accepts an AG-UI `RunAgentInput` and streams AG-UI events over SSE. A pending tool approval ends the run with `RUN_FINISHED` carrying `outcome: {type: "interrupt", interrupts: [...]}`, and a new run whose input carries `resume: [{interruptId, status, payload}]` resolves it (https://docs.ag-ui.com/concepts/interrupts).
- **Acceptance check:**
  1. `POST` a `RunAgentInput` (validated against `@ag-ui/core`) to the documented endpoint with `Accept: text/event-stream`, asking for something that needs an approval-required tool (as in item 3).
  2. Validate every received event against the `@ag-ui/core` event schemas.
  3. Send a resume run with `status` approving the interrupt, then another scenario denying it.

  Pass: every event validates; the first run ends with `RUN_FINISHED` whose `outcome.type` is `"interrupt"` and whose `interrupts` names the pending tool call; no `CUSTOM` event is the only carrier of the approval; the tool runs once after the approving resume and never after the denying one.

### 5. Configurable CORS allowlist, deny by default

- **Observed:** CORS is `allow_origin(Any)` with any method and header (`src/server.rs:2528-2533`) on every non-sidecar instance, and `src/config.rs` has no CORS key.
- **Requested:** a configuration key listing allowed browser origins. With the key unset, no cross-origin request is allowed.
- **Acceptance check:**

  ```bash
  preflight() { curl -si -X OPTIONS "$UAR/<any API path>" \
    -H "Origin: $1" -H 'Access-Control-Request-Method: POST' | grep -i '^access-control-allow-origin'; }
  preflight http://127.0.0.1:5001     # with the key set to ["http://127.0.0.1:5001"]
  preflight https://evil.example      # same configuration
  preflight http://127.0.0.1:5001     # with the key unset
  ```

  Pass: the first call returns `Access-Control-Allow-Origin: http://127.0.0.1:5001`; the second and third return no `Access-Control-Allow-Origin` header (and never `*`).

### 6. MCP elicitation in UAR's MCP client

- **Observed:** UAR's MCP client does not implement elicitation (a search of `src` for `elicit` finds no implementation).
- **Requested:** UAR's MCP client advertises the `elicitation` capability at initialize, and surfaces an `elicitation/create` request from an MCP server to the human through the session's interactive channel (an AG-UI interrupt, or the A2A input-required state), returning the human's accept, decline or cancel to the server.
- **Acceptance check:**
  1. Configure the instance with a test MCP server that logs the client capabilities it receives at initialize, and whose single tool sends `elicitation/create` before returning.
  2. Trigger the tool from an AG-UI run, answer the resulting interrupt with accept; repeat with decline.

  Pass: the logged client capabilities include `elicitation`; the run pauses with an interrupt that carries the elicitation message; the test server receives `accept` with the submitted content in the first case and `decline` in the second.

### 7. Safe defaults: no default model, local memory embedder, loopback bind, authentication on

- **Observed defaults** (`src/config.rs`): `server.host` `0.0.0.0` (`:1412`); `llm.model` `openai/gpt-4o` (`:1476`); `memory.embedding_provider` `openai` and `memory.embedding_model` `text-embedding-3-small` (`:1464-1465`). `security.jwt_required` defaults to `true` (`:1413`), but the shipped `config.embedded.yaml` sets `jwt_required: false` (`config.embedded.yaml:19`).
- **Requested:**
  - (a) no default model: with no model configured, UAR either refuses to start with a clear message or starts and makes no model call until one is configured;
  - (b) the memory and knowledge-base embedders default to a local embedder, or to disabled, never to a remote provider;
  - (c) the server binds `127.0.0.1` by default;
  - (d) every shipped configuration file and image default requires authentication.
- **Acceptance check**, against the published image or package started with no configuration file and no provider environment variables:
  1. Start the instance. Record whether it starts and the message it prints.
  2. List listening sockets for the process (`lsof -nP -iTCP -sTCP:LISTEN -a -p <pid>` on macOS, `ss -ltnp` on Linux).
  3. Capture outbound connections for five minutes while sending one chat request and one memory-capturing request (`tcpdump` or `nettop`/`nethogs` scoped to the process, or a container with egress blocked and its denied-connection log).
  4. Send an unauthenticated request to a protected API path, with each shipped configuration file and with the image default.

  Pass: every listening socket is bound to `127.0.0.1` or `::1`; zero outbound connections to any non-loopback address; the chat request fails with a "no model configured" error (or the instance refused to start with that message); the unauthenticated request returns 401 in every configuration.

### 8. Readable effective settings

- **Observed:** an admin settings API exists (`GET /api/uar/settings`, `GET` and `PUT /api/uar/settings/{key}`, `src/uar/api/administration_capabilities.rs:148-160`). `terminal_exec` is opt-in through `native_tools.terminal_exec_enabled` (`src/config.rs:2525`). Re-read on 2026-10-09 (Kubo ADR-0007):
  - `GET /api/uar/settings` returns an array of per-key records (`SettingsWithMetaResponse`, `src/uar/api/settings.rs:522, 616`), each with its own `revision`; there is no single value that changes when any listed setting changes.
  - The admin credential is the `x-uar-admin-key` header (`settings.rs:364-374`). With `security.settings_mutation_auth_required` false the endpoint answers without a credential (`settings.rs:365-367`); a missing key gives 403, not 401.
  - The model is readable (`llm.model`, `llm.base_url`), but the memory and knowledge-base embedders are exposed as provider names (`memory.embedding_provider`, `knowledge_bases.*.embedding_provider`), not as the host of their endpoint.
  - `governance.enabled`, `native_tools.terminal_exec_enabled` and `mcp.servers` are readable, and `GET /api/uar/settings/governance/status` reports `effective_enabled` (`settings.rs:63, 302`).
- **Requested:** an authenticated, read-only, documented endpoint that returns the **effective** values (after defaults, files, environment and runtime updates are applied), with stable field names documented per release:
  - the chat model and the host of its provider endpoint;
  - the knowledge-base embedder and the memory embedder, each with the host of its provider endpoint (or `local`);
  - whether the governance gate is effectively enabled, and the tool-approval policy;
  - whether `terminal_exec` (and any other shell-capable native tool) is enabled;
  - the list of configured MCP servers, each with its transport and URL or command;
  - a value that changes whenever any of these change (a revision number or a hash).

  The endpoint accepts the admin credential as `Authorization: Bearer <token>` (it may also accept other forms). A response in the shape of Kubo's `mcp/contract/agentic/uar-effective-settings.schema.json` needs no translation on Kubo's side; any other documented, stable shape is also acceptable, and Kubo adds a translation when it pins the release.
- **Acceptance check:**
  1. `curl -sf -H "Authorization: Bearer $UAR_ADMIN_TOKEN" "$UAR/<documented path>"` and check that every listed field is present.
  2. Change the model and enable `terminal_exec` through the settings update API (or configuration plus restart), then read again.
  3. Repeat the read without a token, and with a non-admin token.

  Pass: all fields are present with values matching the configuration; after step 2 the model, `terminal_exec` and the change indicator reflect the new values; step 3 returns 401 and 403.

### 9. UAR release version in the agent card

- **Observed:** the agent card's `version` is the literal `"0.1.0"` (`src/uar/api/a2a/agent_card.rs:22`), not the UAR release version.
- **Requested:** the agent card served by the instance carries the UAR release version it was built from (for example in the card's `version` field, or a documented extension field), so a client can refuse versions older than a known-good release.
- **Acceptance check:**

  ```bash
  curl -sf "$UAR/.well-known/agent-card.json" | jq -r '.version'
  ```

  Pass: the printed value equals the release tag's version (for tag `v1.4.0`, `1.4.0`), and it differs between two published releases.

## What Kubo does on its side

Nothing in this request needs UAR to trust Kubo or to change for Kubo alone; each item is protocol conformance or a safe default. Independently of UAR, Kubo:

- treats UAR as a configured principal of `ipfs mcp --http`, read-only unless the operator grants write groups;
- requires a human-presence prompt for every write UAR requests, regardless of UAR's own approval or governance settings;
- refuses node content to UAR unless the operator acknowledges UAR's model and embedder hosts (`egress_ack`);
- runs `ipfs mcp check-uar` against items 1, 8 and 9 at startup, at every session and on settings change, and fails closed when it cannot read them.

## Contact and status

The operator files this request with the UAR team and records the link and date on the "Filed:" line at the top. Status changes (accepted, declined, released in a tag) are recorded by the operator in this file. The gate evaluation at the end of Round 4 is recorded in `mcp/uar/spike/REPORT.md`.

### Status re-check 2026-10-13

The operator reported that the current UAR code base fixes this issue. A read-only audit of the UAR working tree at `b4244aea` found items 1, 4, 5, 6 and 8 done, items 3 and 7 mostly done, item 2 partial, and item 9 done in code but not released. All of it is uncommitted; `Cargo.toml` is `1.0.0` and no tag above `v1.0.0` exists. Under the gate rule above no item counts as delivered yet. Per-item evidence and the remaining gaps are in [`2026-10-13-v1.1.md`](2026-10-13-v1.1.md), the follow-up request (OpenSpec change `uar-conformance-release`, phase `agent-backend-settings`).
