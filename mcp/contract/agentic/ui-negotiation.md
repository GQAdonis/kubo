# A2UI negotiation and MCP App mapping

The canonical catalog identity, version and view bindings live in `derivations/a2ui-components.json`; `ui-negotiation.schema.json` defines bounded negotiation/fallback structures. This is A/1.4's source contract. A/1.1–1.3 generate/integrate consumers, A/2.2 implements actual SDK transport checks, and final UI-106 supplies interoperability evidence. No running implementation is claimed here.

## First-party negotiation

The client advertises `RunAgentInput.forwardedProps.ipfs.a2ui` as:

```json
{"catalogs":[{"catalogId":"https://github.com/GQAdonis/kubo/blob/master/mcp/ui/catalog/ipfs-mcp.catalog.json","catalogVersion":"0.1.0","protocolVersion":"v0.9"}]}
```

The exact tuple is authored once in the derivations descriptor. The URI is an identifier, never a fetch destination. `catalogVersion` versions IPFS catalog semantics; `protocolVersion` is the A2UI wire version. They are distinct from package versions and the overall tool `contractVersion`. No alias, semver range, newer patch assumption or partial ID match supplies compatibility. Future incompatible catalog changes increment catalogVersion through a reviewed contract/generation change; do not keep matching an old semantic version with new incompatible shapes.

Decode only this namespace: at most four distinct offers, 512-character ID, 32-character catalog version, 16-character protocol version, and at most 4096 encoded UTF-8 bytes for the namespace. Duplicate/malformed/unknown properties, oversize or no exact tuple yield no selected A2UI catalog. Generic forwardedProps never becomes model tools, prompt text, host credentials or authorization. Validate the namespace independently so ordinary callers that do not support the extension still get their normal run. Retain existing global request bounds; a capability parse failure must not trigger a network lookup or HTML fallback fetch.

Select A2UI only when /agui is mounted, `agent.a2ui` is enabled, a supported exact tuple is present, and the server can produce the validated surface. Emit the existing `a2ui.catalog` CUSTOM event exactly once immediately after RUN_STARTED, adding `catalogVersion`/`protocolVersion` to the existing `catalogId`/`catalogUrl` fields. `catalogUrl` remains the protected same-listener `/agui/catalog` route; identifiers and caller-offered URLs are not fetched. Keep existing auth/Host/Origin/egress protections. Missing/invalid/no-match offers emit no A2UI catalog or surface events. Normal TOOL_CALL_RESULT/text/structured content remains; a client need not implement A2UI to use /agui.

Surface delivery is server-built only under ADR-0010. An external UAR CUSTOM/A2UI message does not become trusted because its client offered this catalog. Existing ToolCallCard wrapping may remain where specified by `chatRules`; inner result component selection must use the canonical tool/view mapping. Do not treat A2UI support as permission to compose arbitrary HTML or execute a tool.

## One mapping source, including MFS content

`viewBindings` is the sole source for supported views, resource URI, file, title, default component and component allowlist. Preserve the five public view IDs/URIs/MIME. Generate the Go and TypeScript view/component/export/resource tables from it, alongside existing `catalog.json` per-tool `ui.appView`, `ui.a2uiComponent` and `outputSchema`. Existing URI conventions, generated manifests and view arrays are derived projections that must agree, not additional hand-maintained authorities.

| View | Default | Allowed result components |
|---|---|---|
| content | ContentCard | ContentCard |
| pins | PinList | PinList |
| mfs | MfsTree | MfsTree, ContentCard |
| dag | DagViewer | DagViewer |
| node-status | NodeStatusPanel | NodeStatusPanel |

`mfs_read` intentionally maps to ContentCard within the mfs view. Never infer that every tool sharing a view has the default component. `catalog.json` remains the per-tool authority; no second override map copies tool names. A tool without appView (including current IPNS/key tools) remains text/structured/A2UI as currently supported, without inventing a new HTML view.

Source/generation invariants: unique view/URI/file; each default is allowed; every allowed component exists as a view component; every tool appView is exposed; every mapped per-tool component belongs to that view's allowlist; output schema exists; public resource URI/MIME and generated manifest agree. A conflict fails generation/registration explicitly rather than silently guessing another component. Generated catalog `x-ipfs-mcp.catalogVersion` and `x-ipfs-mcp.protocolVersion` expose the descriptor's values for exact client comparison; do not hand-edit `mcp/ui/catalog` or committed build output in this task.

Before interactive rendering, identify the producing tool through the authenticated result/host context, confirm that its declared view/component is allowed, and validate its structured output against that tool's actual output schema. A supplied view/component string cannot widen the allowlist. If inference is necessary for an ordinary host lacking tool hints, it must match exactly one tool in the view; ambiguous output falls back. Do not select the first accepting schema. Conflicting producer/view hints fall back. Error/confirmation output uses its appropriate shared schema, never a success component selected only by isError absence.

## Ordinary MCP App host compatibility

First-party AG-UI catalog support is independent of MCP Apps negotiation. Preserve `_meta.ui.resourceUri` and the existing legacy `ui/resourceUri`, `ui://ipfs-mcp/{view}`, `text/html;profile=mcp-app`, single-file bundled resources, pinned ext-apps SDK and deny-by-default resource policy. An ordinary conforming host need not send this first-party capability branch. Existing NoAppViews/nomcp/build/off behavior remains; disabling first-party A2UI does not disable normal tool results or secretly start an iframe. Disabling app resources does not create a replacement approval path.

Add the descriptor's informational `ai.ipfs/iframe-exception` resource metadata with `{reason: external_mcp_app_host, transport: standard_ext_apps}` on the registered resource and returned contents. This names the established external-host exception; it is not a CSP/sandbox permission, authentication token or host-specific required field. Keep standard `_meta.ui` fields as defined by the SDK/spec. Hosts may ignore the namespaced declaration without failing initialization.

Use standard SDK initialize/notifications and JSON-RPC request IDs. Bind messages to the trusted parent/source and the origin/bootstrap semantics actually supported by pinned ext-apps 2.0.3. Do not trust the first arbitrary message origin, require a private nonce/envelope ordinary hosts cannot echo, invent constructor options, or accept any foreign source just because the origin string is `null`. Opaque-origin sandbox hosts can be legitimate; reject forged source/wrong/stale response IDs while allowing a valid standard opaque-origin bootstrap. A/2.2 must inspect the resolved SDK and document the exact supported wrapper/injection point before editing. This contract does not claim that inspection or transport validation has passed.

A/2.2's inspected injection point is `App.connect(Transport)`. Its wrapper fixes the parent WindowProxy for the mounted session and rejects top-level execution. Before initialization, only a schema-valid `ui/initialize` result from that exact parent, correlated to the current fresh standard JSON-RPC request ID, establishes the parent origin. Unsolicited messages do not establish trust. Pin that origin for the session; an opaque `null` origin is allowed only through the same source and correlated initialization checks. Remap outbound standard request IDs into a fresh per-transport namespace, preserve SDK correlation internally, and reject unknown, consumed or stale response IDs. Closing the session removes listeners and pending mappings; a new mount cannot consume an old response.

This bootstrap binds the existing embedding-host boundary; it does not authenticate a parent application's identity or grant a new resolver capability. It requires no extra response field from an ordinary MCP App host. The pinned SDK's React hook cannot inject this transport, so the App may use a locally owned lifecycle hook around the supported `App.connect` API. Final fixtures must exercise standard normal and opaque-origin hosts, foreign sources, wrong origins, malformed initialization, stale/duplicate response IDs and teardown.

For host-delivered operation state, preserve the standard optional `hostContext.toolInfo.id` when supplied. Repeated or partial-to-full input for that same call must not be mistaken for a new operation; a changed call ID retires the prior operation binding. Ordinary context/error rerenders retain the current input and port identity. Notifications do not themselves carry per-notification request IDs, so an ID-omitting host has a documented conservative input-event fallback; do not invent private fields or claim stronger correlation than the standard host supplies.

## Inert result fallback

Unknown catalog/component/version, an invalid tool result, an unmapped view, ambiguous producer or untrusted surface yields `InertResult` with the exact label: “This result can't be shown here. Nothing in it runs.” Name the safe tool label and show bounded existing safe plain text (maximum 64,000 characters); if none exists, disclose that rather than dumping arbitrary invalid structured fields. Preserve untrusted-content framing.

Render text nodes only. Do not parse HTML, Markdown, autolinks, embeds, function calls, nested component definitions or action/navigation objects. Copy text is the only permitted control. Do not partially retain actions/links from a failed schema or use a fallback to fetch the catalog/result URL. Unknown data cannot create approvals, bypass confirmations or retry writes. D/1.1's execution/SuccessfulReceipt/eleven-state definitions remain the authority for decision displays; a surface acknowledgement does not mean Done.

## Fixture and generator handoff

`fixtures/ui-negotiation/cases.json` contains authored structural/selection/mapping/fallback/host expectations, including the discriminating mfs_read case. `fixtures/agui/read-pin-ls-negotiated.json` uses the exact tuple and advertisement; the original no-capability fixture remains valid without A2UI events. Fixtures are synthetic and not interoperability receipts. A/1.1 generates both language consumers; A/2.2 inspects actual SDK boundaries; all fixtures and tests are authored before the single final QA boundary. No per-task tests/builds/reviews or generator runs certify this tranche.
