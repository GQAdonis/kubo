# Assessment (backend): agent-backend-settings

Stage: ASSESS. Role: `ipfs-agent-architect`. Date: 2026-10-13. Scope: the backend side of goals 1 to 4. The settings UI visuals and the Promethity rebrand (goal 5) are assessed separately by `ui-ux-engineer`.

Method: everything below comes from reading source, git metadata and `gh`. Nothing was built, started or deployed. kubectl was not run, and no Secret value was read. Line numbers are at kubo `38088001d`, ipfs-webui `2be6b78c`, ipfs-desktop `0ea60ce`, universal-agent-runtime HEAD `b4244aea` plus an uncommitted working tree (marked WT), and know-me-cluster `origin/main` (`bb9ab14`). "Unverified" marks claims that were not confirmed.

Policy baseline: AGENTS.md "Every new feature has an off switch" now carries the 5f019f0df sentence. It names four agent features that ship enabled: the local `ipfs mcp` agent endpoint that Desktop starts on loopback, the Web UI agent chat, the WebMCP tools and the Desktop confirmation window. Each keeps an off switch, writes still need confirmation, and no third-party service is enabled without the operator choosing it. A2A is not in that list.

Gotchas that apply (`.prometheus/gotchas.md`):

- 2026-10-09: never `pkill -f "ipfs daemon"`.
- 2026-10-11: no large here-docs; run `kbd`/`kbd-apply` under `timeout` with `</dev/null`.
- 2026-10-13: the LKE postStart detach rule, and never force-delete a pod stuck in PodInitializing.

---

## 1. What already exists

### 1.1 `ipfs mcp` flags and the agent loop (G10 L3)

| Item | Evidence | State |
|---|---|---|
| `--http[=host:port]`, default `127.0.0.1:5050` | `cmd/ipfs/kubo/mcp_http.go:34-35`; help `cmd/ipfs/kubo/mcp.go:78-80`; schema `http.enabled` default `false` | Off by default; stdio otherwise |
| `--llm-base-url`, `--llm-model`, `--llm-api-key-env`, `--llm-api-key-file` | `cmd/ipfs/kubo/mcp_agent.go:19-22, 63-66` | No default. A literal key is rejected (`mcp/server/agent_settings.go:161-175`) |
| `--agui`, `--a2a` | `mcp_agent.go:67-68`, both `.WithDefault(false)`; help says "off by default" (`mcp_agent.go:39-56`) | Off |
| Settings resolution | `Config.ResolveAgent` (`agent_settings.go:107-159`) merges the flags with `llm`/`agent`. It returns nil (agent off) when neither base URL nor model is named, and refuses a half-configured backend, credentials in the URL, or an API key over plain http to a non-loopback host (`:138-140`) | Startup only |
| Agent construction | `Server.EnableAgent` builds one `*Agent` (`mcp/server/agent.go:207-217`). `NewLLMClient` reads the key once (`mcp/server/llm_client.go:111-143`) | Immutable after start |
| Mounting | `EnableAGUI` fails without an agent (`mcp/server/agui.go:36-46`). `mountAGUI` captures the agent pointer in the handler (`agui.go:60-71`), and A2A does the same (`mcp/server/a2a.go:43-49`). `enableMCPAgent` prints "--agui is ignored" when no backend is set (`mcp_agent.go:112-121`) | /agui is 404 without a backend |
| Loop | Bounded steps, tokens and wall clock (`agent_settings.go:19-25`; schema `agent.loop`). OpenAI chat-completions with `stream: true` and `stream_options.include_usage` (`llm_client.go:145-174`), plus a non-stream fallback (`llm_client.go:381-383`). Tool calling uses `ChatTool` built from the contract registry, with `confirm_token` stripped (`agent.go:225-266`) | Built |
| System prompt | A Go `const` (`agent.go:175-178`) | Not configurable |
| Egress | `egressRefusalFor` refuses content-egress tools unless the principal's `egress_ack` covers `backendHost` (`agent.go:601-607`) | Bound to a single backend host |
| Confirmation in chat | Writes end the run with an AG-UI interrupt, `reason: "tool_call"` with `metadata["ipfs-mcp"]` (`mcp/server/agui_wire.go:265-300`). The `aguiResolver` records the user gesture (`agui.go:73-84`) | Built |
| Config file | Read once by `LoadConfig` (`mcp/server/config.go:138-139`). There is no SIGHUP or reload path: grepping `mcp/server` for `Reload`, `SIGHUP` or `/settings` finds nothing | Startup only |
| Build-time removal | `GOTAGS=noagent` (`mcp_agent.go:36-37`, `mcp/server/agent_settings.go:1`) and `GOTAGS=nomcp` | Built |

### 1.2 Contract config schema, `llm` and `agent` (`mcp/contract/config/ipfs-mcp-config.schema.json`)

- `llm`: `{base_url, model, api_key (SecretRef), timeout "60s"}`. Defaults are `null`, `base_url` and `model` are required, and the off switch is "Remove the llm section".
- `agent.agui` and `agent.a2a` default to `false`. `agent.origin_allowlist` defaults to `[]`, which gives 403 to any request that carries an Origin. `agent.loop` holds the limits.
- `confirmation.modes` defaults to `["native"]`. `desktop` has been opt-in since 2026-10-12.
- Nothing in the schema describes a runtime settings resource. Every key is read at startup.

### 1.3 A2A

The A2A 1.0 JSON-RPC endpoint `/a2a` and the card at `/.well-known/agent-card.json` are mounted only with `--a2a`, `--http` and a backend (`mcp_agent.go:141-149`). They are read-only unless the config grants writes, and peers never approve a write (help text at `mcp_agent.go:48-56`). The 5f019f0df policy does not cover A2A, so A2A stays default-off.

### 1.4 A2UI registry versus the AG-UI stream

- The registry exists on the client: `mcp/ui/catalog/ipfs-mcp.catalog.json` has 11 components (ApprovalCard, ConfirmationDialog, ContentCard, DagViewer, EgressBadge, IpnsRecordCard, MessageStream, MfsTree, NodeStatusPanel, PinList, ToolCallCard), A2UI `protocolVersion v0.9`, and is generated from `mcp/ui`. The chat rules are in `mcp/contract/derivations/a2ui-components.json`.
- **The server emits no A2UI.** `agui_stream.go`/`agui_wire.go` emit only:
  - `RUN_STARTED`
  - `TEXT_MESSAGE_START`, `TEXT_MESSAGE_CONTENT`, `TEXT_MESSAGE_END`
  - `TOOL_CALL_START`, `TOOL_CALL_ARGS`, `TOOL_CALL_END`, `TOOL_CALL_RESULT`
  - `MESSAGES_SNAPSHOT`
  - `RUN_FINISHED` (outcome or interrupt)
  - `RUN_ERROR`

  (`mcp/server/agui_stream.go:108-171`, `agui_wire.go:242-252`.) It sends no `CUSTOM` event, no state event and no A2UI surface message, and nothing in `mcp/server/*.go` (non-test) references A2UI or the catalog.
- The chat island builds A2UI locally. For each tool call it runs `createSurface` with the catalog id and a ToolCallCard on a local `MessageProcessor` (`mcp/chat/src/island/ToolCallSurface.tsx:1-40`).
- Conclusion: today, A2UI is how the client draws tool results. It is not something the AG-UI stream carries, and the model cannot compose a surface.

### 1.5 Web UI agent-chat island contract

- The endpoint has no default. It must be https, or http on a loopback host, with no credentials, query or fragment, and **its path must end in `/agui`** (`mcp/chat/src/agui/endpoint.ts:20-37`). Every request is origin-pinned (`:45-58`).
- The token is set as a write-only property, or through `tokenProvider`. A `token` attribute is removed (`mcp/chat/src/element.ts:12-17, 85-131`).
- In the Web UI fork:
  - `src/agent-chat/settings-store.js:15` has `DEFAULTS = { enabled: false, endpoint: '' }`. The token is kept in localStorage by operator decision of 2026-10-10 (`:1-7`).
  - The Desktop bridge path **is already wired**. `AgentChatLauncher.js:21-39` takes the endpoint and `tokenProvider` from `window.ipfsDesktop.agentChat`. Desktop's `docs/agent/HARDENING.md:202-221` still lists this as a pending change; that note is stale.

### 1.6 Desktop fork (`ipfs-desktop` `d45dd38`, `0ea60ce`)

- **Desktop does not start `ipfs mcp`.** It has no spawn of an mcp process; the only spawns are the Kubo daemon (`src/daemon/daemon.js:140-143`). It only connects to a server the operator started:
  - chat endpoint: `src/agent/settings.js:18-33`
  - confirmation sockets: `src/agent/confirm/channel-client.js:90-96`
  - pairing instructions: `src/agent/confirm/README.md`
- Keys in the electron-store (`src/agent/consts.js:7-19`):

  | Key | Default |
  |---|---|
  | `agentChat.enabled` | false (`settings.js:43`) |
  | `agentChat.endpoint` | `''` |
  | `agentChat.tokenCipher` | absent |
  | `desktopConfirm.enabled` | off (`confirm/index.js:20-32, 146`; never on win32) |
  | `desktopConfirm.pairingCipher` | absent |

- Secrets are Electron `safeStorage` ciphertext: Keychain on macOS. The store refuses `basic_text` and `unknown` on Linux (`src/agent/secret-store.js:27-80`).
- The chat settings are read once, when the Web UI window is created (`src/webui/index.js:216-221`). So changing the endpoint or token needs an app restart. No user path sets the token; HARDENING.md:184-189 says to edit config.json.
- The confirmation toggle applies live from the tray (`confirm/index.js:134-144`; `src/agent/confirm/tray-menu.js:12-47`). Approve needs Touch ID (`confirm/touchid.js:13-27`).
- IPC:
  - The chat bridge is read-only: `ipfs-desktop:agent-chat:get-endpoint` and `get-token`, answered only to the Web UI main frame (`src/agent/bridge.js:22-32`). There are no setter channels.
  - The reusable pattern is `createToggler` plus `CONFIG_UPDATED` (`src/utils/create-toggler.js:7`, `src/tray.js:522`).
- Desktop has no LLM config, no WebMCP toggle and no sidecar lifecycle.

### 1.7 LKE deployment (`know-me-cluster`, `origin/main` `bb9ab14`)

The repo is at `/Users/gqadonis/Projects/travis/personal/know-me-cluster`. Local `main` (`2ccab42`) is behind and has no sidecar, so these line numbers are from `git show origin/main:namespaces/ipfs/manifests.yaml`.

- **Sidecar** `ipfs-mcp` (lines 135-178): `ipfs mcp --http=0.0.0.0:5050 --api=/ip4/127.0.0.1/tcp/5001 --auth-token-env=IPFS_MCP_TOKEN --mcp-config=/mcp-config/config.json`.
  - **No `--agui`, no `--a2a`, no `--llm-*`.**
  - The token comes from Secret `ipfs-mcp-token`, key `token` (lines 145-149). The value was not read.
- **ConfigMap** `ipfs-mcp-config` (around lines 631-669):
  - `http` listens on `0.0.0.0:5050`.
  - `tools.groups` is all 8 groups.
  - `confirmation.modes: []`.
  - It has no `principals`, `llm` or `agent`.
  - It is mounted read-only (lines 174-175, 241-242).
- **Routes** on `ipfs.prometheusags.ai`, through Envoy Gateway `ipfs-gateway-gw`:
  - `/ipfs`, `/ipns`, `/` go to 8080 (526-570).
  - `/webui` and `/api/v0` go to 5001 (573-610).
  - `/mcp` goes to `ipfs-mcp:5050` (703-727).
  - **There is no `/agui` route.**
- **Auth:**
  - Basic auth on the `ipfs-webui` route is described in comments only (342-344, 598-600). The SecurityPolicy `ipfs-webui-auth` is not in the repo (unverified; presumably applied out of band).
  - `/mcp` has no gateway auth; it relies on the server's bearer.
  - `BackendTrafficPolicy` with timeout 0 covers `ipfs` and `ipfs-webui` only (613-626), not the mcp route.

---

## 2. Goal 1: runtime settings (gap)

**State.** No runtime settings API exists, and nothing can change after start:

- config is read once (`config.go:138`)
- the agent is built once (`agent.go:207`)
- handlers capture the agent pointer (`agui.go:67`, `a2a.go:49`)
- the key is read once (`llm_client.go:126-131`)
- `/agui` is not mounted at all without a backend (`agui.go:37`, `mcp_agent.go:115-121`)

So a settings UI cannot bring the chat up on a node that started without a backend.

### What `ipfs mcp` needs (mcp-server-engineer, contract by architect)

1. **Mount `/agui` without a backend.** Mount it whenever it is enabled. With no backend it answers a structured "backend not configured" error (for example 503 with a stable code) instead of 404. The island can then show "configure a backend" and point to Settings. This changes current documented behaviour (help text at `mcp_agent.go:41-42`; schema `offSwitch` text for `llm`) and needs a contract and help-text update.
2. **Swap the backend while running.**
   - Hold the `LLMClient`, `backendHost`, limits and system prompt behind an `atomic.Pointer` (or a mutex-guarded snapshot) read once per run. Runs in flight finish on the snapshot they started with.
   - Keep `sessions` and `buckets` across swaps; they live on `Agent` (`agent.go:194-201`).
   - A2A and AG-UI must read the same pointer.
3. **Settings resource.** A new route on the `--http` listener, mounted through `RouteMounter` like `/agui` (pattern at `agui.go:60-71`), for example `GET` and `PUT /settings/agent`. JSON Schema 2020-12 in `mcp/contract/`:
   - **Read** returns `base_url`, `model`, `timeout`, the loop limits, `system_prompt`, `agui`/`a2a` state, `api_key: {set: bool, source: "env"|"file"|"stored"}`, and the backend kind (`openai` or `uar`). **It never returns key material.** The existing `secret` type already redacts in JSON and slog (`agent_settings.go:35-43`).
   - **Write** validates with the same rules as `ResolveAgent`:
     - no credentials in the URL
     - no plain-http key to a non-loopback host
     - no default endpoint or model
   - It applies atomically. The response is the new read shape.
   - Optimistic concurrency: an `ETag`/`If-Match` on the settings revision, so two tabs cannot overwrite each other silently.
4. **Persistence.** `ipfs mcp` must not open the repo (F4) and the LKE config is a read-only ConfigMap. Settings therefore persist to a separate overlay file, for example `settings.json` beside `--mcp-config` or in a `--settings-dir`, mode 0600. It is read at start and layered over the config file, with a defined precedence: flags win over overlay, overlay wins over the config file. On LKE that file needs a writable volume; an emptyDir loses it on restart, so the choice is a PVC or accepting that loss (operator question Q2).
5. **Secrets on write.** A browser cannot set a server environment variable. If the UI must set an API key, the server has to store it, for example as a 0600 file in the settings dir that becomes a `SecretRef{File}`. The alternative is to accept only references (env name or file path), which works for Desktop and LKE and does not work for a browser-only operator. This is a real decision (Q2).
6. **Who may write.**
   - The settings route changes where node content goes. A new backend host plus a principal's `egress_ack` decides egress (`agent.go:601-607`).
   - So it must not be open to the `ui` principal that drives the chat. Options are the operator credential only (`--auth-token-env`, granted full access at `mcp_http.go:37`), or a new principal kind, for example `admin`. The kind list today is `human-mcp`, `ui`, `a2a`, `uar` (`mcp/server/principal.go:8-14`).
   - Whether a backend change also needs a person's confirmation (the F4 gate) is Q1.
7. **The `egress_ack` coupling.** If an operator moves the backend from a loopback model to a cloud one, content tools start returning `egress_not_acknowledged` until `egress_ack` covers the new host. Either the settings write also edits `egress_ack` for chosen principals (a privilege change), or the UI shows the refusal and links to the config. This is Q4; it must not be widened silently.
8. **Off switches.** The settings route needs its own (a flag or config key, plus removal under `GOTAGS=noagent`). Disabled-state tests: route 404, nothing written, nothing dialed.

### How the cloud Web UI reaches it

- The Web UI is same-origin at `https://ipfs.prometheusags.ai/webui`. `/agui` and the settings route need an HTTPRoute to `ipfs-mcp:5050`.
- The strict CORS policy accepts only the mount's own `origin_allowlist`, not the listener's origin (`mcp/server/http.go:43-60`). So `origin_allowlist` must contain `https://ipfs.prometheusags.ai`.
- **Conflict.** Envoy basic auth and the bearer both use the `Authorization` header. If `ipfs-webui-auth` (basic) is applied to the new routes, the bearer cannot be sent. Keep `/agui` and settings on the bearer-only `ipfs-mcp` route, never under the basic-auth SecurityPolicy (unverified how the out-of-band policy is targeted).
- The browser then holds two bearers: the `ui` chat token, already in localStorage, and the settings-writer token (Q1/Q2).

### How Desktop applies it

- Desktop has no sidecar (1.6). It must own an `ipfs mcp --http=127.0.0.1:5050 --agui --confirm=native,desktop` child process: lifecycle tied to the daemon, restart with backoff, stderr to the log, and its own off switch.
- Secrets stay in `safeStorage` and reach the child as environment variables named by `--auth-token-env` and `--llm-api-key-env`, so no key is written to disk. Desktop generates the `ui` token itself. That also closes "no user path to set the token" (HARDENING.md:184-189).
- Desktop applies a change either by restarting the child (simple; loses open chat sessions; meets "no manual restart") or by calling the settings route. Recommendation: restart for backend and secret changes, and send the endpoint and token to the Web UI by a push event instead of the read-once at window creation (`src/webui/index.js:216-221`). That needs a new IPC channel. Setters must stay in the main process: HARDENING.md explains why the renderer must not be able to widen its own CSP.
- Ownership gap: no team role owns `ipfs-desktop` main-process code (AGENTS.md role table). The architect must assign it in the OpenSpec change (Q7).

---

## 3. Goal 2: UAR backend (gap)

### UAR facts (repo `/Users/gqadonis/Projects/prometheus/universal-agent-runtime`)

- **Version:** `Cargo.toml:31-33` is `1.0.0`. The latest plain tag is `v1.0.0` (`9e967a86`, 2026-07-11); HEAD is `b4244aea`. The nearest tag is a `boss-sidecar-*` payload tag. The working tree carries large uncommitted and untracked work.
- **Agent CRUD (HEAD):** `GET`, `POST /api/agents`; `GET`, `PUT` (If-Match), `PATCH`, `DELETE /api/agents/{id}` (`src/server.rs:2004-2014`, `src/uar/api/discovery.rs:95-396`).
  - The body is an `AgentArtifact` (`src/uar/domain/artifact.rs:37-51`). It carries `prompt.system` and `instructions` (`:285-289`), `policy.provider.default{provider, model}` (`:219-230`), and tool and skill policy.
  - Agents persist through SurrealKV or Postgres; without persistence, create returns 503.
- **MCP servers cannot be attached to an agent by URL.** `extensions.mcp_servers` can only narrow servers already bound (`policy_intersection.rs:516-540`).
  - An external server is registered globally: `PUT /api/uar/mcp/servers/{name}` (`src/uar/api/mcp_admin.rs:36-55, 103-107`). `Authorization` must be a `secret_ref` to an environment variable (`src/mcp/config.rs:71-83, 152-178`), so the kubo bearer must be in UAR's process environment. This is refused in sidecar mode.
  - Per-run `mcp_servers` with a direct URL are loopback-http only (`turn/host/mcp.rs:209-225`).
- **Skills:**
  - CRUD at `/api/uar/skills` (name, prompt_overlay, preferred_tools...), and bindings at `/api/agents/{id}/skills` (`skills.rs:51-67, 122-125`).
  - agentskills.io `SKILL.md` is parsed, but `POST /skills/import` only previews a **server-local path** and stores nothing (`skills.rs:441-524`).
  - There is no upload. Pushing `mcp/skills/**` to a remote UAR over the API is not possible today.
- **AG-UI:**
  - HEAD has no `RunAgentInput` endpoint. It has `POST /api/uar/runs` plus `GET /api/uar/runs/{id}/stream?stream_mode=agui_spec`, which uses `CUSTOM uar.*` events for approvals.
  - WT (untracked `src/uar/api/agui.rs`) has `POST /api/uar/ag-ui/agents/{agent_id}`, which emits standard events and interrupts with `reason: "tool_approval" | "mcp_elicitation"`. It rejects frontend tools with 400 and emits no CUSTOM or A2UI events.
- **Auth:** JWT (HS256 or JWKS) or `x-api-key` (`src/uar/security/middleware.rs:161-283`). The AG-UI path rejects anonymous callers. CORS is `Any` at HEAD; the WT has a deny-by-default allowlist.
- **Elicitation:** HEAD's MCP client advertises no elicitation (`registry.rs:698, 726, 834`). The WT implements it (`src/mcp/interaction.rs:178-213`) and surfaces it as an AG-UI interrupt. Inferred: under HEAD, every kubo write tool fails, because kubo confirms writes by elicitation, else the native prompt.
- **LLM:** liter-llm is vendored (`Cargo.toml:581-588`). Per-agent model comes from the artifact policy. UAR also serves `/v1/chat/completions`.

### Issue #379

- OPEN, label `enhancement`, filed 2026-10-12, no comments.
- It lists 9 conformance items and says items 1, 8 and 9 are the minimum for `check-uar`.
- It requires a published release: a tag plus an OCI digest or checksum.

### Do the ADR-0008 NO-GO reasons still hold?

Mostly yes, as of `b4244aea`:

| ADR-0008 evidence | Now |
|---|---|
| E1, request not filed | **No longer holds**: #379 was filed 2026-10-12 |
| E2, no pinned release (`uar-preflight.json` nulls) | Holds |
| E3/E6, no `/.well-known/agent-card.json` | Holds at HEAD; present only in the WT (`a2a/mod.rs:79`, uncommitted) |
| E7, no effective-settings endpoint | Holds at HEAD; WT `effective_settings.rs` is untracked |
| E8, card version literal `0.1.0` | Holds at HEAD (`agent_card.rs:22`); the WT uses `CARGO_PKG_VERSION` |

Items 2, 3 and 7 were not checked (unverified).

### Goal 2 versus topology B

Goal 2 ("create an IPFS agent on UAR and chat through it") is topology B: UAR hosts AG-UI and calls `ipfs mcp --http` as a `kind: uar` principal. Kubo's side for that already ships fail-closed: `check-uar` and `uar-preflight.json` refuse a `kind: uar` principal with `min_version_not_set` (ADR-0007, ADR-0008).

### UAR work required (cross-team, via `team-request` to the UAR team)

1. Commit the WT and cut a release: tag `v1.1.0` plus a GHCR digest (the #379 items 1, 4, 5, 6, 8, 9). Verify 2, 3 and 7.
2. A way to bind an external MCP server to an agent with a credential supplied at run time. Today the bearer must be in UAR's own environment, and per-run URLs are loopback only.
3. Remote skill upload (an archive or SKILL.md body), or a documented server-side install path.
4. Confirmation that globally registered servers get the elicitation channel. `shared_transports_enabled` was not checked (unverified).

### Kubo and Web UI work for goal 2

- **The island cannot talk to UAR as it stands.**
  - `checkEndpoint` demands a path ending in `/agui` (`endpoint.ts:36`); UAR's path is `/api/uar/ag-ui/agents/{id}`.
  - The interrupt shapes differ: kubo sends `reason: "tool_call"` with `metadata["ipfs-mcp"]` (`agui_wire.go:287-299`); UAR sends `tool_approval` and `mcp_elicitation` with an `ElicitResult` resume payload.
  - Bearer semantics also differ (a UAR JWT or API key).
- Options:
  - (a) `ipfs mcp /agui` stays the only browser endpoint and gains a "uar" backend kind. It provisions the agent through `/api/agents`, proxies the AG-UI run, and translates interrupts. The island contract is untouched.
  - (b) Teach the island a second endpoint profile.
- Recommendation: (a). It keeps one origin, one token model and one confirmation UI. It needs an ADR, because kubo would then hold a UAR credential (Q5).
- Stopgap that works today with no UAR change: topology A with UAR as the OpenAI-compatible backend (`/v1/chat/completions`). That gives no UAR-side agent, prompt or skills, so it does not meet goal 2.

---

## 4. Goal 3: OpenAI-compatible proxy (gap)

**Is the `--agui` loop already that proxy?** Largely yes. `ipfs mcp --http --agui --llm-base-url ... --llm-model ...` is a Go process in this codebase. It serves AG-UI, runs a bounded tool loop over the kubo MCP tools, and forwards to any OpenAI-compatible endpoint with streaming and tool calling (`llm_client.go:145-174`, `agent.go:416-460`). It is "started like `ipfs mcp`" because it *is* `ipfs mcp`. Recommendation: **extend it; do not add a new command or binary.**

Missing pieces:

| Gap | Evidence | Needed |
|---|---|---|
| A2UI registry exposure over AG-UI | No A2UI, CUSTOM or STATE events (1.4) | Decide what "serve the A2UI control registry" means (Q6). (i) Serve the catalog JSON at a route and advertise it in `RUN_STARTED` or a `CUSTOM` event, so any AG-UI client can discover it; and/or (ii) emit A2UI v0.9 surface messages (`createSurface`, `updateComponents`, `updateDataModel`) as `CUSTOM` (or `ACTIVITY`) events built server-side from tool results, using the existing chat rules in `derivations/a2ui-components.json`; and/or (iii) let the model compose surfaces via a tool, validated against the catalog. (i) plus (ii) are deterministic and stay within F4; (iii) widens what the model can put on screen |
| liter-llm compatibility | No test against liter-llm. The client sends `stream: true` and `stream_options.include_usage` (`llm_client.go:164`) and handles a non-stream reply | A live check against a liter-llm gateway configured by the operator (not port 4000 by default; `mcp/server/listen.go:17`). Probably compatible; **unverified** |
| Streaming | Built (text deltas, then `TEXT_MESSAGE_CONTENT`) | None |
| Tool calling | Built (OpenAI `tools`, assembled call deltas, `llm_client.go:232-330`) | Optional: `tool_choice` and parallel tool-call flags are not sent; confirm liter-llm defaults |
| Configurable system prompt | `const systemPrompt` (`agent.go:175`) | Make it a setting, keeping the safety sentences (untrusted content, confirmation) as a non-removable prefix |
| Runtime reconfiguration | See section 2 | Hot swap plus settings route |
| Backend kind | One kind (OpenAI chat-completions) | A `backend.kind: openai | uar` discriminator in the contract (goal 2 option a) |

---

## 5. Goal 4: defaults (gap)

### Where each feature is off today

| Feature | Where | Current default | Policy target |
|---|---|---|---|
| `ipfs mcp` runs at all | Nothing starts it; changelog `docs/changelogs/v0.44.md:25` says "do not run `ipfs mcp`; nothing starts by itself" | Not running | Desktop starts it on loopback |
| `--http` | `mcp_http.go:34-35`; schema `http.enabled: false` | Off | On *as passed by Desktop*; kubo's own default can stay |
| `--agui` / `agent.agui` | `mcp_agent.go:68`; schema default `false`; help `mcp_agent.go:39-42` | Off | On in the Desktop-started endpoint |
| `/agui` without a backend | `agui.go:37` (refuses), `mcp_agent.go:115-121` | 404 | Mounted, "not configured" (section 2) |
| `llm` | schema default `null` | No backend | **Must stay null.** No third-party backend is chosen for the operator |
| `--a2a` | `mcp_agent.go:67` | Off | Not in the policy; keep off |
| `agent.origin_allowlist` | schema default `[]` | Empty | Desktop passes `webui://localhost`; LKE passes its own origin |
| `confirmation.modes` desktop | schema default `["native"]` | Desktop mode off | Desktop passes `--confirm=native,desktop` |
| Desktop `agentChat.enabled` | `ipfs-desktop src/agent/settings.js:43` | false | true once the sidecar exists |
| Desktop `desktopConfirm.enabled` | `src/agent/confirm/index.js:20-32,146` | off | on (not on win32) |
| Web UI chat | `ipfs-webui src/agent-chat/settings-store.js:15` | `{enabled:false, endpoint:''}` | On under Desktop (the bridge already makes it follow Desktop, `AgentChatLauncher.js:21-39`). On the cloud node, the endpoint default could be same-origin `/agui`; there is no remote default |
| Web UI WebMCP | `ipfs-webui src/webmcp/settings-store.js:20` | `{enabled:false, dnslink:false, writes:false}` | `enabled: true` (read tools). `writes` is session-only by design and stays so; writes still confirm |

### What each flip requires under AGENTS.md

AGENTS.md "Every new feature has an off switch" and "flipping a default ... is a feature launch". Each flip needs:

- documentation:
  - `ipfs mcp` has no repo config key, so its keys go in `docs/mcp/config-reference.md` and the contract schema (`offSwitch`, `default`).
  - Desktop and Web UI defaults go in their fork docs.
  - A `docs/config.md` entry is needed only if a repo config key is added; none is proposed.
- a Highlights entry in `docs/changelogs/v0.44.md` with the exact disable command for each feature (architect owns TOC and Highlights)
- disabled-state tests:
  - `test/cli/mcp_agui_test.go`, `mcp_http_test.go`, and new settings-route tests
  - Desktop unit tests for the sidecar off switch
  - Web UI tests for the chat and WebMCP off switches
- updated help text (`mcp_agent.go:39-56`, then `TestCommandDocsWidth`)

### Conflicts with the Stability rules

- No `/api/v0` change is needed. Every route is on the `ipfs mcp` listener. Desktop's daemon migration rev 9 (CORS `webui://localhost`, `src/daemon/config.js:218, 355-368`) already exists.
- **No hardcoded endpoints:** the loopback `127.0.0.1:5050` is local, and no model URL default may be added. UAR must be operator-named.
- **Resource use:** an always-on loopback child is a local process for the operator's own benefit and makes no network call without a backend. It is permitted by 5f019f0df but is still a resource default; record the idle RSS.
- **Telemetry:** none is involved. The settings route must not log values (use the existing `secret` redaction).
- **Risk to watch:** flipping kubo's own `agent.agui` default to true would change every `ipfs mcp --http` user, LKE included, without Desktop. The recommendation is that Desktop passes the flags and kubo's defaults stay as they are (Q3).

### LKE changes needed (operator-applied, know-me-cluster)

1. Add `--agui` (or `agent.agui: true`) to the sidecar.
2. Add a `kind: ui` principal to `ipfs-mcp-config` with its credential from a new Secret (key name only). Add `agent.origin_allowlist: ["https://ipfs.prometheusags.ai"]`.
3. Choose the backend: an `llm` section, or the settings overlay on a writable volume (PVC). Today the ConfigMap is read-only (lines 174-175).
4. Add HTTPRoute rules `/agui` (and the settings route) to `ipfs-mcp:5050`, **outside** basic auth. Add a long or streaming request timeout like `ipfs-long-download-timeout`.
5. Confirmation: in-cluster `confirmation.modes: []` means chat writes rely on the AG-UI interrupt resolver. Destructive tools need a native prompt that a container cannot offer (`gate.go:284`), so they stay unavailable from chat on LKE. That is correct and should be stated in the docs.
6. Fast-forward local `main`; it is behind `origin/main`. Apply the postStart detach fix branch `fix/ipfs-poststart-detach` (gotcha 2026-10-13).

---

## 6. Carry-over from `ipfs-mcp-server`

The runtime (`prometheus kbd status --json`) lists 6 pending changes in `ipfs-mcp-server`:

- `desktop-agent-chat`
- `mcp-app-views`
- `native-confirmation-prompt`
- `phase-boundary-review`
- `webui-agent-chat`
- `webui-webmcp-integration`

`agentic-uar-integration` (0/5) and `uar-integration-spike` (1/5) are deferred by ADR-0008. The runtime does not list them as pending, so the operator's count of six differs from the runtime's six (unverified why).

| Change | Open item | Recommendation |
|---|---|---|
| `agentic-uar-integration` (all 5 deferred) | profile, principal, e2e, README, upstream check | **Fold into goal 2**, re-scoped by a new change that supersedes it (the topology now includes kubo-side provisioning, option a). Keep its gate rule: a published UAR release |
| `uar-integration-spike` 1.2-1.5 | operator measurements | Fold with goal 2; run after a UAR release |
| `webui-agent-chat` 1.3 (A4) | live chat read plus confirmed write | **Fold** as goal 1/3 acceptance (cloud plus Desktop) |
| `desktop-agent-chat` 1.6, 1.8 (A2) | paired Desktop run, Touch ID | **Fold** into the goal 4 Desktop sidecar acceptance (1.8 appears built in `0ea60ce`, `confirm/touchid.js`; verify the checkbox) |
| `webui-webmcp-integration` 1.4 (A3) | WebMCP in Codex/Kimi | **Fold** as acceptance for the WebMCP default flip |
| `native-confirmation-prompt` 1.5 (A1, HG-1) | hands-off rerun | Keep as a deferred operator check; not a goal of this phase (progress-report 2026-10-13) |
| `mcp-app-views` 1.4 (A5) | real-host render | Leave in the operator checklist; unrelated to these goals |
| `phase-boundary-review` 1.8 | operator commits | Close in the previous phase; do not fold |

---

## 7. Risks, operator questions, proposed changes

### Risks

- **New remote write surface.** A settings route on the cloud node that can repoint the model and therefore content egress. A bug there is a data-exfiltration path. It needs `security-reviewer` before LKE.
- **Hot-swap races.** A run in flight and the confirmation interrupts that span a swap. Pin each run to a snapshot; a resumed interrupt after a swap must reuse the run's original backend or fail cleanly.
- **The Authorization header conflict** between Envoy basic auth and the bearer on LKE.
- **Goal 2 depends on another team's release.** Without UAR `v1.1.0` plus a digest, goal 2 cannot complete under the gate rule. The uncomfortable version: if the gate holds, this phase repeats ADR-0008, half-delivered again.
- **Default-on WebMCP** exposes read tools to any in-browser agent of that origin. Writes still confirm, but read access to node content is a disclosure surface the policy accepts.
- **No owner for Desktop main-process code.** Work could stall or land without review.

### Questions for the operator (decisions only)

1. **Q1, who may write settings.** Operator credential only, or a new `admin` principal kind? Does a backend change also need a person's confirmation (OS prompt or Desktop window)?
2. **Q2, secrets and persistence.** May `ipfs mcp` store an API key it receives over HTTP (0600 file in a settings dir), or only accept env and file references? On LKE: a PVC for the settings overlay, or keep backend config in the ConfigMap and Secret, so the cloud UI is read-only for secrets?
3. **Q3, where the default lives.** Keep kubo's `--agui` and `--http` defaults off and let Desktop pass them (recommended), or flip kubo's own defaults?
4. **Q4, egress_ack.** May a settings write extend a principal's `egress_ack` to the new backend host, or must that stay a config-file edit?
5. **Q5, UAR topology and gate.** Accept option (a), `ipfs mcp /agui` provisioning and proxying a UAR agent with kubo holding a UAR credential? Keep the "published release" gate for our own UAR, or allow a tagged pre-release?
6. **Q6, A2UI over AG-UI.** Catalog discovery plus server-built surfaces (deterministic), or also model-composed surfaces?
7. **Q7, Desktop ownership.** Which role owns `ipfs-desktop` main-process code (sidecar supervisor, settings IPC)? `ui-ux-engineer` owns only the visuals.

### Proposed changes (for the Plan stage), in dependency order

| # | Id | Owner | Depends on | Content |
|---|---|---|---|---|
| 1 | `agent-settings-contract` | ipfs-agent-architect | Q1-Q4, Q6 | ADR-0009 (runtime settings, defaults placement, egress rule). Contract: settings resource schemas (read with no secrets, write-only secret fields, revision/ETag), `backend.kind`, settings-route off switch, A2UI catalog route/event schema, updated `offSwitch` text |
| 2 | `mcp-agent-hot-reload` | mcp-server-engineer | 1 | `/agui` mounted without a backend (not-configured error), atomic backend snapshot shared by AG-UI and A2A, configurable system prompt with a fixed safety prefix |
| 3 | `mcp-agent-settings-api` | mcp-server-engineer | 1, 2 | Settings route, auth, overlay persistence, secret storage per Q2, disabled-state `test/cli` tests |
| 4 | `mcp-agui-a2ui-registry` | mcp-server-engineer (+ ui-ux-engineer for catalog/island consumption) | 1 | Catalog route plus server-emitted A2UI events per Q6; liter-llm live compatibility check |
| 5 | `desktop-mcp-sidecar` | per Q7 (+ ui-ux-engineer for UI) | 3 | Desktop supervises `ipfs mcp --http --agui --confirm=native,desktop` on loopback, generates the ui token, keeps secrets in safeStorage and passes them by env, pushes settings to the Web UI; flips `agentChat.enabled` and `desktopConfirm.enabled` defaults with off switches; folds A2 |
| 6 | `agent-features-default-on` | ipfs-agent-architect (docs, changelog Highlights) with the owning roles for tests | 3, 5, Web UI settings work | Defaults table, `docs/mcp/config-reference.md`, `v0.44.md` Highlights with disable commands, WebMCP and chat default flips in the Web UI fork, disabled-state tests across the three repos; folds A3, A4 |
| 7 | `uar-conformance-release` (cross-team `team-request` to UAR; GitHub `team:` issue or #379 follow-up) | ipfs-agent-architect files it; UAR team builds | none | Commit the WT, release `v1.1.0` plus digest, external MCP binding with a run-time credential, skill upload, elicitation on global servers |
| 8 | `agent-backend-uar` (supersedes `agentic-uar-integration` and `uar-integration-spike` 1.2-1.5) | mcp-server-engineer + architect (ADR-0010) | 1, 2, 3, 7 | `backend.kind: uar`: provision the agent (`/api/agents`, skills bindings), proxy AG-UI runs, translate interrupts, pin `uar-preflight.json` |
| 9 | `lke-agent-routes` (operator-applied, outside this repo) | operator, with architect-written steps | 3, 6 | Sidecar `--agui`, ui principal plus Secret, origin allowlist, `/agui` and settings HTTPRoute outside basic auth, timeout policy, settings volume per Q2 |

Changes 2-4 and 7 can start in parallel once 1 is accepted; 7 should be filed first because it has the longest lead time.
