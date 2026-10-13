# SettingsClient contract (client side)

Phase `agent-backend-settings`, change `agent-settings-contract` task 1.5. This directory is the single source of truth for the client side of the agent settings surface: the `SettingsClient` interface the Web UI settings page consumes, its two transports (HTTP to the settings route, and the IPFS Desktop bridge over IPC), and one JSON Schema (2020-12) file per IPC message. The server side of the same surface is specified by the sibling files in `mcp/contract/settings/` (task 1.2); these client shapes mirror them and say so per field.

Consumers: `webui-agent-settings-page` (the `#/settings/agent` page, which injects one implementation through its provider, assessment-ui 5.6) and `desktop-agent-settings-bridge` (the main-process end of the IPC channels).

## Interface

One method per IPC channel; the channel name is the method name prefixed with `ipfsDesktop.agentSettings.`.

```ts
interface SettingsClient {
  get(): Promise<GetResponse>                                  // get.schema.json
  set(req: SetRequest): Promise<SetResponse>                   // set.schema.json
  testConnection(req: TestConnectionRequest): Promise<TestConnectionResult> // test-connection.schema.json
  setApiKey(req: SetApiKeyRequest): Promise<SetApiKeyResponse> // set-api-key.schema.json
  clearApiKey(req: ClearApiKeyRequest): Promise<ClearApiKeyResponse>        // clear-api-key.schema.json
  status(): Promise<StatusResponse>                            // status.schema.json
  onChange(cb: (event: SettingsChangedEvent) => void): () => void           // on-change.schema.json
}
```

Rules that hold on both transports:

- **Revision-based optimistic concurrency.** `get()` and every mutation return the current `revision`. Every mutation requires the caller to present the revision it last saw; a stale one is rejected with `revision_conflict`. On HTTP the revision is the `ETag`/`If-Match` header pair (ADR-0009 section 1); IPC has no headers, so it travels as a body field.
- **No secret material in any read, response, event or error.** Keys are write-only through `setApiKey`/`clearApiKey`, the single secret path; `set()` carries no key field at all. A configured key is reported only as `api_key: {set, source}`.
- **Synchronous, restart-free apply.** A mutation's response is the applied state. There is no job id and no `restartRequired` (see the reconciliation table).
- **Errors** reject with the settings error shape (`../settings-error.schema.json`): the closed code set `backend_not_configured`, `revision_conflict`, `egress_confirmation_required`, `secrets_not_accepted`. On HTTP, 401 means "Sign in again" (assessment-ui 5.5) and 404 means the route is absent (no admin principal or `agent.settings.enabled: false`); on the bridge those states surface through `status()` instead.

## Transport bindings

| Method | HTTP implementation | Desktop bridge (IPC) |
|---|---|---|
| `get` | GET on the settings route, admin bearer | `ipfsDesktop.agentSettings.get`; main process reads through the sidecar |
| `set` | PUT with `If-Match: <revision>` | `ipfsDesktop.agentSettings.set`; Touch ID when the change extends egress (desktop-agent-settings-bridge 1.4) |
| `testConnection` | POST on the test-connection endpoint; on success the client also calls the model-list endpoint to fill `models` | `ipfsDesktop.agentSettings.testConnection`; same two calls through the sidecar |
| `setApiKey` | PUT with `backend.api_key` (write-only) | `ipfsDesktop.agentSettings.setApiKey`; key into `safeStorage` (Keychain), applied to the sidecar by environment, never returned |
| `clearApiKey` | PUT with `backend.api_key: null` | `ipfsDesktop.agentSettings.clearApiKey`; key removed from `safeStorage` |
| `status` | answered locally: `transport: "http"`, `managed_by_desktop: false`, plus the client-tracked `applied` record | `ipfsDesktop.agentSettings.status`; adds `endpoint` and `sidecar {running, port}` |
| `onChange` | subscribes to the settings SSE stream (`../settings-event.schema.json`) | `ipfsDesktop.agentSettings.onChange` pushed by the main process to the trusted frame only (`isTrusted`, desktop-agent-settings-bridge 1.1) |

Transport-specific rules:

- **HTTP**: the client is constructed with the console origin and an admin-token provider; the token lives in memory or `sessionStorage`, never `localStorage` (D2). Secrets never appear in URLs (assessment-ui 5.5). The route path itself is pinned by `mcp-agent-settings-api`, not here; the client takes the base URL as constructor input.
- **Desktop bridge**: channels are answered only for the trusted main frame, and setters run in the main process (desktop-agent-settings-bridge 1.1). When the sidecar port changes the main process recreates the window (1.2), so the renderer never re-reads a stale endpoint.

## Reconciliation with assessment-ui 5.2

Every field of the 5.2 common contract and the two per-backend field tables is listed below, either mapped one-to-one or dropped/deferred with a reason.

### Common contract (5.2)

| UI 5.2 field | Disposition |
|---|---|
| `GET settings` `backend: null\|"uar"\|"openai"` | One-to-one: `backend` in `get.schema.json` (`null`, or `kind: "openai"\|"uar"`) |
| `features.chat` | Dropped from this contract: the switch writes `enabled` in the Web UI store `ipfs-webui:agent-chat` (webui-agent-settings-page 1.4, agent-features-default-on 1.1). It is a Web UI preference, not an `ipfs mcp` setting, and the page composes it beside the `SettingsClient`, not through it |
| `features.webmcp` | Dropped, same reason: the WebMCP store's `enabled`/`dnslink`; `writes` is per-load and never persisted (webui-agent-settings-page 1.4) |
| `features.confirm` | Dropped from `agentSettings.*`: the Desktop confirmation switch is the Desktop store key `desktopConfirm.enabled` (agent-features-default-on 1.2) written through Desktop's own config channel, and the switch is absent without the bridge (webui-agent-settings-page 1.4) |
| `uar.url` | One-to-one: `backend.base_url` with `kind: "uar"` |
| `uar.agentId`, `uar.agentStatus` | Deferred, not dropped: they belong to the UAR provisioning endpoints (`POST`/`GET settings/uar/agent`), which arrive with `agent-backend-uar` (ADR-0010), gated on the published UAR release (OG-4). Until then the UAR form ships honest unavailable states (webui-agent-settings-page 1.3, plan W4). No IPC channel carries them in this contract |
| `openai.baseUrl` | One-to-one: `backend.base_url` |
| `openai.model` | One-to-one: `backend.model` |
| `openai.keySet` | One-to-one: `backend.api_key.set` |
| `applied: {at, ok, error?}` | Mapped, client-sourced: `status().applied` (`status.schema.json` `AppliedRecord`). The server applies synchronously and deliberately keeps no apply history; "last applied by this browser/Desktop" is the only history the UI shows (5.3), so each client implementation records the outcome of its own mutations |
| `restartRequired: false` | Dropped: hot apply is restart-free by ADR-0009 (mcp-agent-hot-reload), so the constant carried no information. The UI asserts the invariant ("Never 'restart required'", 5.3) instead of reading it |
| PUT write-only `openai.apiKey` / `uar.apiKey` | Mapped to the dedicated channels `setApiKey`/`clearApiKey` rather than a field on `set`: exactly one secret write path, into `safeStorage` on Desktop |
| `POST settings/test` `{backend, url, apiKey?}` | `backend` → `kind` and `url` → `base_url`, one-to-one. `apiKey` **dropped**: a probe authenticates with the currently stored key only, never with a key sent in the request (`../test-connection.schema.json`; credential-hygiene and SSRF boundary, mcp-agent-settings-api 1.4) |
| Test result `ok` | One-to-one |
| Test result `latencyMs` | Mapped: `latency_ms`, measured by the client implementation; not a server field |
| Test result `models?` | One-to-one: `models` in the `testConnection` result, filled from the model-list endpoint after a successful probe (there is no separate `listModels` IPC channel) |
| Test result `error?: {code, message}` | Mapped: `detail` carries the message on a failed probe; the stable machine-readable codes ride on settings errors (`../settings-error.schema.json`), which is where the UI switches on codes |
| `GET settings/models` | Server side already specified (`../model-list.schema.json`, task 1.2); reached through `testConnection`'s `models` |
| `POST settings/uar/agent`, `GET settings/uar/agent` | Deferred with `agent-backend-uar` (see `uar.agentId` above) |
| Change event channel (SSE or poll) | One-to-one: `onChange`, payload mirrors `../settings-event.schema.json`; HTTP subscribes to SSE, the bridge pushes from the main process |
| "Never returns a secret" | Kept as an invariant: no request, response or event schema in this directory has a field that can carry key material |

### UAR field table (5.2)

| UI 5.2 field | Disposition |
|---|---|
| Instance URL | One-to-one: `backend.base_url` with `kind: "uar"` |
| Access credential (write-only, Replace/Remove) | One-to-one: `setApiKey` / `clearApiKey`; stored state shown from `api_key: {set, source}` |
| Test connection | One-to-one: `testConnection` |
| IPFS agent (create / step list / Ready / Failed) | Deferred with `agent-backend-uar` (OG-4); the form shows honest unavailable states until then |
| Agent name / model | `model` maps one-to-one to `backend.model`; agent name is deferred with UAR provisioning |

### OpenAI-compatible field table (5.2)

| UI 5.2 field | Disposition |
|---|---|
| Base URL | One-to-one: `backend.base_url` |
| API key (write-only, optional) | One-to-one: `setApiKey` / `clearApiKey`; `api_key.set` shows stored/not stored |
| Test connection | One-to-one: `testConnection` |
| Model combobox | One-to-one: `backend.model`, options from `testConnection`'s `models`; free text stays possible when the backend hides its list (`models` absent) |
| Advanced (temperature / max tokens) | Dropped: 5.2 itself says "omit if the architect does not expose them (do not invent)". The bounded agent loop (G10, ADR-0004) keeps sampling parameters server-side; no settings field exposes them |

## Uncomfortable thing

The reconciliation drops `features.*` from this contract, which means the settings page's feature switches and its backend settings now live in two different stores with two different failure modes: a Web UI store reset (or a fresh browser profile) silently returns the switches to their defaults while the backend configuration survives on the server. The page must therefore never present the feature switches as part of the applied backend state, and its status line must read from the store that actually owns each value.
