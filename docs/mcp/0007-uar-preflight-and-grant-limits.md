# ADR-0007: check-uar reads a contract-pinned settings shape; operator grants under `--confirm=cli`; MCP App view off switch

## Status

Accepted, 2026-10-09, by `ipfs-agent-architect`, resolving the contract gaps raised by `mcp-server-engineer` in `mcp-agent-principal-enforcement` (task 1.3 open) and by `mcp-app-views` task 1.2. Records the decisions in `openspec/changes/mcp-agent-principal-enforcement/design.md` and `openspec/changes/mcp-app-views/design.md`.

## Date

2026-10-09

## Context

ADR-0004 makes `ipfs mcp check-uar` read UAR's effective settings and fail closed when it cannot. `mcp/uar/requests/CONFORMANCE.md` item 8 asks UAR for a documented endpoint but names no path or shape. The engineer's implementation (`mcp/server/uarcheck.go`) assumed `GET /api/uar/settings` returning one object with `revision`, `llm.provider_host`, `knowledge_base_embedder.provider_host`, `memory_embedder.provider_host`, `governance.enabled`, `native_tools.terminal_exec_enabled` and `mcp_servers[]`, sent the admin credential as `Authorization: Bearer`, and left the minimum UAR version empty.

What UAR serves at HEAD `a7cb9729` (2026-10-01, read-only inspection of `/Users/gqadonis/Projects/prometheus/universal-agent-runtime`):

- `GET /api/uar/settings` exists (`src/server.rs:1727` nests `src/uar/api/settings.rs`; handler `list_settings`, `settings.rs:616`). It returns a JSON **array** of per-key records (`SettingsWithMetaResponse`, `settings.rs:522`: `key`, `data`, `saved`, `effective`, `revision`, `source`, `is_drift`, ...), not one object. There is no aggregate revision; each record has its own.
- The admin credential is the `x-uar-admin-key` header (`require_admin_key`, `settings.rs:364-374`), not a bearer token. When `security.settings_mutation_auth_required` is false the endpoint answers without any credential (`settings.rs:365-367`). A missing or wrong key gives 403, not 401.
- Keys that exist: `llm.model`, `llm.base_url` (`settings/manager.rs:2705`), `memory.embedding_provider` and `memory.embedding_model` (`manager.rs:2481`), `knowledge_bases.default` and `.named` carrying `embedding_provider` (`manager.rs:1797-1846`), `native_tools.terminal_exec_enabled` (`manager.rs:3010`), `mcp.servers` (`manager.rs:3120`), `governance.enabled` (`manager.rs:1216`). Governance also has `GET /api/uar/settings/governance/status` with `effective_enabled` and a numeric `revision` (`settings.rs:63, 302`; `governance/runtime_control.rs:61`).
- Embedders are exposed as provider **names** (`openai`, `cohere`), never as hosts, so the egress check cannot be computed from this API without Kubo mapping provider names to vendor endpoints.
- Whether any of this is in a published UAR release is unknown; the gate rule in CONFORMANCE.md counts published releases only, and `mcp/uar/spike/REPORT.md` does not exist yet.

## Decisions

### 1. Settings shape: a Kubo-normalized contract schema, read only from a pinned path

- `mcp/contract/agentic/uar-effective-settings.schema.json` is the only settings shape `check-uar` accepts. It keeps the engineer's field names (they match what CONFORMANCE item 8 asks for) and adds: `revision` may be a string or a non-negative integer; `provider_host` is a host (optionally with port) or the literal `local` or `disabled`; `native_tools.shell_tools` (optional) lists other enabled shell-capable tools; `mcp_servers[].transport` is one of `http`, `streamable-http`, `sse`, `stdio`. Required: `revision`, `llm`, both embedders (missing means refuse). Tool inventory (`governance`, `native_tools`, `mcp_servers`) is optional; missing means write groups are withdrawn (HG-2). Extra fields are ignored.
- The path is **not** guessed. `mcp/contract/agentic/uar-preflight.json` holds `settingsPath`, currently `null`. With `null`, settings are unreadable and the unauthenticated probe has no target: `check: verify` refuses the principal; `check: assert` records the operator's verification and the version check still applies.
- Kubo does not read UAR's per-key record list. It cannot yield embedder hosts, it has no aggregate revision, its credential header differs from the requested bearer token, and it is not known to be in a published release. A response that does not validate against the schema, including that array, is `settings_unreadable` (fixture `mcp/contract/fixtures/uar/observed-records-a7cb9729.json`).
- When a published UAR release documents an endpoint, the architect pins its path in `uar-preflight.json` together with the version (decision 2). If its documented shape differs from the schema, the pin change adds a translation in `mcp/server` and fixtures recorded from that release; the schema stays the normalized form.
- The admin credential is sent as the `X-UAR-Admin-Key` header, not as a bearer (corrected 2026-10-16; the first implementation sent `Authorization: Bearer`, which UAR's `require_admin_key` does not accept), only to the path under the card URL's base, only over https or to loopback. CONFORMANCE.md item 8 records the observed `x-uar-admin-key` header.

### 2. Minimum UAR version: pinned in the contract, set by the architect at the gate

- The shipped pin lives in `mcp/contract/agentic/uar-preflight.json` (`minVersion`, with `pinnedFrom: {tag, artifact, report}`), read through `contract.LoadUARPreflight()`, not as a Go constant in `mcp/server`. `minVersion` and `settingsPath` are pinned together from one release, and only with `pinnedFrom` evidence (enforced by `contract` and its tests).
- Who sets it: the architect, in `agentic-uar-integration` task 1.2, from the gate result in `mcp/uar/spike/REPORT.md`. `mcp/uar/profile/` (task 1.1) documents the profile and points at the contract pin; it does not hold a second copy.
- Meanwhile `minVersion` is `null`, so a `kind: uar` principal without its own `uar.min_version` is refused with `min_version_not_set`. That is the intended outcome while topology B is not supported.
- `mcp/contract/config/examples/uar-principal.json` does **not** get an explicit `min_version`. UAR's card currently reports the literal `"0.1.0"` for every build (CONFORMANCE item 9), so any value we put there would pass a non-conformant UAR. The example is a schema example; its expected `check-uar` result today is `refuse` (`min_version_not_set`, `auth_probe_inconclusive`, `settings_unreadable`). An operator may set `uar.min_version` to a release they tested; that is their decision and is reported as such.

### 3. No new keys for the recheck interval or the Kubo MCP address

- The recheck interval is a security bound (the longest time a verdict is reused between session initializes), not an operator preference, and checks are request-driven with no background traffic, so there is nothing to switch off beyond removing the `uar` block. It moves from a `mcp/server` constant to `recheckInterval` in `uar-preflight.json` (`30s`; the contract rejects values outside `(0, 60s]`) so the documented value has one source.
- UAR's `ipfs mcp` entry stays identified by the bound HTTP listen address (`isKuboMCPServer`). A mismatch (a container alias, a proxy) makes the entry look foreign and withdraws write groups, which is the safe direction. No `kubo_mcp_url` key until a real deployment needs one.

### 4. The `--auth-token-*` operator identity under `--confirm=cli` stays read-only for writes

- No exemption key. The token from `--auth-token-env`/`--auth-token-file` is typically given to a coding agent, which usually has a shell; that is exactly the client the rule exists for. The CLI resolver has no presence factor (ADR-0003, Round 4 amendment section 1).
- The supported path is a configured principal: `kind: human-mcp`, its own `credential`, the `write_groups` it needs and `shell_capable: false`. That puts the assertion in the config file next to the grant, where it is reviewed, instead of in an implicit flag.
- The refusal and the startup message for the operator identity must name that path.

### 5. MCP App views: a dedicated off switch, linked-tool rule kept, no `confirmation_deny` tool

- **(a) Off switch.** Tool selection is not an off switch for the views: turning views off that way turns the tools off too. New key `tools.app_views` (boolean, default `true`, flag `--no-app-views`, owner change `mcp-app-views`). `false` registers no `ui://` resource and puts no `_meta.ui` on any tool; tools are otherwise unchanged. The default is on because views are static embedded bundles with no network access (deny-all CSP) and call only tools the session already has.
- **(b) Linked-tool rule confirmed.** With `app_views` true, a view's `ui://` resource is registered only while at least one tool linked to it in the view manifest is registered for that server. Per-principal filtering of `resources/list` is not required: the bundle is static and carries no node data, and every data path goes through a tool call that is checked per principal.
- **(c) `confirmation_deny` is out of the contract.** Expiry already is denial, an app-only visibility is enforced by the host rather than by `ipfs mcp`, and the tool would let any caller cancel a pending human decision. Revisit only if `mcp-app-views` task 1.4 (real-host check) shows the deny-by-expiry behavior is a usability problem; it would then be a catalog entry with its own confirmation-class analysis.

## Alternatives considered

| Alternative | Rejected because |
|---|---|
| Read UAR's per-key `GET /api/uar/settings` records now | No embedder hosts; mapping provider names to vendor endpoints in Kubo hard-codes third-party endpoints; no aggregate revision; different credential header; not known to be released. |
| Operator-configured `uar.settings_url` | Lets the admin credential go to a URL other than the UAR instance; the path is a property of the UAR release, so it belongs with the version pin. |
| Explicit `min_version` in the example | Passes a non-conformant UAR whose card always says `0.1.0`. |
| `uar.recheck_interval` config key | Invites weakening a security bound; no background traffic to switch off. |
| `operator_shell_capable: false` flag or key | Applies to a token whose holder is usually shell-capable; a configured principal already expresses the same thing with review. |
| Rely on tool selection to remove views | Couples the view off switch to losing tools (AGENTS.md: every feature has its own off switch). |

## Consequences

- Topology B cannot be used with `check: verify` until a published UAR release documents an effective-settings endpoint and the architect pins it. With `check: assert`, an operator can run it on their own verification; the version floor and the egress and shell rules on whatever is readable still apply.
- `check-uar` does not depend on undocumented UAR internals, so a UAR refactor of its settings store cannot silently change Kubo's verdict.
- The uncomfortable part: until UAR ships item 8, the only way to use topology B is `check: assert`, which is the operator vouching for exactly the properties (remote model, shell tools) that make topology B risky. That is the operator's call, recorded in their config; Kubo prints it at startup.
