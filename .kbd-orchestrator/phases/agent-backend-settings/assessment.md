# Assessment: agent-backend-settings

Phase goals: `goals.md` (5 goals). Assessed 2026-10-13 by the kubo-ipfs-agentic team in two parallel, independent passes:

- `assessment-backend.md`, by `ipfs-agent-architect`: goals 1-4 on the server, Desktop process, UAR and LKE side, sections 1-7 with file:line evidence.
- `assessment-ui.md`, by `ui-ux-engineer` using the prometheus-ui-ux, impeccable, ui-ux-pro-max, vercel-react-best-practices and vercel-composition-patterns skills: goal 1 (settings UI) and goal 5 (Promethity rebrand), sections 1-6.

This file is the synthesis and the authority where the evidence files disagree (see "Corrections to the evidence files"). Revision 2 addresses the adversarial review (`artifact-critic`, round 1: BLOCK, 2 CRITICAL, 9 WARNING); see "Review disposition".

## Prior context

Recalled lessons that shape this assessment (`prior-context.md`):

- "The operator approves merges, tags and machine-level changes explicitly. A single up-front approval question at the phase boundary keeps the run unblocked." Every operator decision is batched below, each with a recommendation.
- "Tests must discover their environment ... or the phase-boundary gate on a fresh worktree will disagree with the worker's run." Applies to the Desktop sidecar (port discovery, see risk R4) and LKE route tests.

Knowledge gaps (`prior-context.md`): none recorded. Unverified facts are listed at the end; they are evidence gaps, not knowledge-base gaps.

## Starting state

- Nothing about the agent backend can change after `ipfs mcp` starts: config read once (`mcp/server/config.go:138`), agent built once (`agent.go:207`), `/agui` and A2A handlers bind that agent (`agui.go:67`, `a2a.go:49`), API key read once (`llm_client.go:126-131`). With no LLM configured, `/agui` is not mounted (`agui.go:37`), so a settings screen could never bring the chat up.
- The existing `ipfs mcp --agui` loop already is the OpenAI-compatible proxy (streaming, tool calls, started as `ipfs mcp`). Goal 3 extends it. Missing: A2UI/CUSTOM/STATE events over AG-UI, a configurable system prompt (constant at `agent.go:175`), a live liter-llm test (compatibility unverified).
- Desktop never starts `ipfs mcp`; its Web UI bridge is read-only (`bridge.js:25-26`); the window's chat settings, CSP and agent origin are computed once at window creation (`src/webui/index.js:216-221`), so a port or origin change needs the window recreated.
- The Web UI fork has only a chat toggle, a `/agui` URL and a token (`src/agent-chat/AgentChatSettings.js`); no backend choice, model list, test connection or apply feedback.
- Everything ships off, contradicting AGENTS.md `5f019f0df`: Desktop `agentChat.enabled` false and `desktopConfirm.enabled` off; Web UI chat and WebMCP defaults `{enabled:false}` (`src/agent-chat/settings-store.js:15`, `src/webmcp/settings-store.js:20`); `ipfs mcp --http`/`--agui` off.
- UAR (goal 2) stays blocked at UAR HEAD `b4244aea`: of the ADR-0008 NO-GO reasons only E1 changed (issue #379 filed, open, no comments). At HEAD, an agent cannot carry its own MCP server URL or token, skills cannot be uploaded, the MCP client lacks elicitation (`registry.rs`, `().serve`) so kubo writes would fail, the AG-UI path does not end in `/agui` (rejected by `mcp/chat` `endpoint.ts:36`), and approval interrupts differ. Uncommitted UAR work covers several items; items 2, 3, 7 are unverified.
- Web UI stack is React 16 / CRA / Tachyons: no Tailwind 4 or shadcn in the shell. The chat island (React 19, shadow root) already uses Promethity tokens. `DESIGN.md` carries Promethity v1 tokens scoped to `mcp/ui` and `mcp/app`.
- Rebrand footprint: 139 `src` files mention IPFS, about 45-60 need real edits; 5 source titles with `| IPFS` (Settings, Analytics, Status, Diagnostics, Peers), 15 locale files carry the suffix (all `welcome.json`), 5 e2e title assertions; 36 files hard-code hex colours, 40 use border classes, 11 use shadows (the brand forbids borders and shadows). No dark mode or theme control. 31 locales, Transifex-managed (edit `en` only).
- Brand gaps: Hanken 700 missing from the branding folder (present in `mcp/ui/fonts`); brand fonts are Latin-only versus 31 locales; no OFL licence texts bundled.
- `/explore` renders `ipld-explorer-components`, an external package the override layer cannot fully restyle (UI §1.2, §5.8). Goal 5 says every route; see decision D9.
- LKE: one hostname `ipfs.prometheusags.ai` serves the path gateway (`/ipfs`, `/ipns`, `/`), the Web UI and `/api/v0` (basic auth), and `/mcp` (bearer). `Gateway.PublicGateways` sets that host with `UseSubdomains:false` (know-me-cluster `origin/main` `namespaces/ipfs/manifests.yaml` ~284; routes at 533, 580, 710). The sidecar runs without `--agui`, with `confirmation.modes: []` (line 654), no `ui` principal, no `origin_allowlist`. Envoy basic auth and the bearer both use `Authorization`. `/webui` returning 401 without credentials was observed live on 2026-10-13; the SecurityPolicy object itself is not in git.
- Any Web UI rebuild changes its CID: `WebUIPath` bump in kubo (`core/corehttp/webui.go`) and a re-pin on LKE (CAR import works). Do it once, at the end.

## Cloud threat model (new in revision 2)

Because the path gateway serves arbitrary content on `ipfs.prometheusags.ai`, any `/ipfs/<cid>` HTML page runs in the same browser origin as the Web UI. The fork already names this for WebMCP (`ipfs-webui src/webmcp/settings-store.js:8-11`, R13 L3). Consequences for this phase:

1. Allowing that origin in `agent.origin_allowlist` lets any opened CID page call `/agui` and the settings API.
2. Anything the Web UI stores in that origin's `localStorage` (today the chat token, `src/agent-chat/settings-store.js:2-4`) is readable by such a page.
3. LKE has no native confirmation, so a settings write that repoints the backend or extends `egress_ack` would have no human prompt.

**Existing exposure, live today, independent of this phase.** The LKE init script sets `API.HTTPHeaders.Access-Control-Allow-Origin` to include `https://ipfs.prometheusags.ai` (plus `http://localhost:3000`, `http://127.0.0.1:5001`, `https://webui.ipfs.io`) with `Access-Control-Allow-Credentials: true` (manifest ~275-278, verified 2026-10-13), and `/api/v0` sits behind basic auth on that same hostname (580-610) next to the unrestricted gateway (533-560). Any HTML at `https://ipfs.prometheusags.ai/ipfs/<cid>` opened in a browser that holds the operator's basic-auth credentials can drive the whole RPC API (config replace, key export, pin rm, shutdown). This is an operator incident to address now, before any change in this phase.

Therefore the agent surfaces (Web UI, settings API, `/agui`) must not share an origin with the path gateway. Recommended fix (decision D1): serve the Web UI, `/api/v0`, `/agui` and the settings API on a dedicated hostname (for example `console.prometheusags.ai` or `node.prometheusags.ai`) that serves no gateway content, keep `ipfs.prometheusags.ai` as the gateway only, set `origin_allowlist` to the dedicated origin only, keep the operator writer token out of `localStorage` (memory or session only, re-entered per session), and require a typed re-confirmation in the settings page for backend changes and egress extensions on hosts with no native confirmation. Subdomain gateway mode (`UseSubdomains:true`) is the alternative isolation; it needs wildcard DNS and a wildcard certificate.

## Gaps by goal

| Goal | Gap | Size |
|---|---|---|
| 1 Settings | Hot-reloadable backend in `ipfs mcp` (safe swap with runs in flight), `/agui` served with a "not configured" state, token-protected settings read/write API that never returns secrets, a writable settings store separate from the read-only LKE ConfigMap, test-connection and model list, change events | Large |
| 1 Settings UI | `#/settings/agent` page shared by cloud and Desktop through a `SettingsClient` (HTTP or Desktop bridge); Desktop write bridge with safeStorage for the key; cloud auth on the dedicated origin per the threat model | Medium |
| 2 UAR | UAR-side release (per-agent MCP server and token, skills upload, elicitation, AG-UI path); kubo-side proxy of a UAR agent behind `ipfs mcp /agui` | Large, gated on UAR |
| 3 OpenAI proxy | A2UI registry over AG-UI, configurable system prompt, liter-llm live test | Medium |
| 4 Defaults on | Desktop starts `ipfs mcp --http --agui` on loopback with port discovery; flip chat, WebMCP (reads; writes stay per-page-load and off), Desktop confirmation; documentation, changelog, disabled-state tests; LKE dedicated host, `--agui`, routes, principal, origin allowlist | Medium |
| 5 Rebrand | `src/promethity/` override layer (tokens light/dark, fonts with OFL, ipfs-css colour remap, `brand.js`), assets, shell, titles and copy, data-viz reskin, route polish, `/explore` seam; `docs/PROMETHITY-FORK.md` to keep the fork mergeable | Large |

## Ownership (corrected in revision 2)

`team.json` today: `ui-ux-engineer` owns `ipfs-webui` `src/App.js`, `src/settings/**`, `src/agent-chat/**`, `src/webmcp/**`, the `agentChat` locale, and `ipfs-desktop` `src/agent/**` (main-process bridge, secret store, confirm), `src/webui/**`, `test/**`, `package.json`; `mcp-server-engineer` owns `mcp/server/**` and `cmd/ipfs/kubo/mcp*.go`; the architect may edit `core/corehttp/webui.go` only for an operator-approved Web UI CID bump.

Unowned paths this phase must touch: in `ipfs-webui` `src/navigation/**`, `src/status/**`, `src/files/**`, `src/peers/**`, the other route folders, `src/bundles/routes.js`, `public/index.html`, `public/manifest.json`, `public/locales/en/*.json` beyond `agentChat`; in `ipfs-desktop` `src/tray.js`, `src/daemon/**` and the Desktop branding assets. Per AGENTS.md these route through `ipfs-agent-architect`, which records the ownership extension in an OpenSpec change before any role edits them. Proposed (decision D10): an architect change `team-ownership-agent-backend` granting `ui-ux-engineer` the Web UI fork (excluding `src/webmcp` write paths, which keep security review) and Desktop branding and tray, `mcp-server-engineer` the Desktop sidecar lifecycle (`src/daemon/**` additions), and keeping the `WebUIPath` bump with the architect.

## Recommendations

- Settings UI lives in the bundled Web UI as one panel for cloud and Desktop; native Desktop windows only for Touch ID confirmation and pairing.
- Rebrand through an override layer plus about 12 structural edits; no React 19, Vite or Tailwind migration of the shell.
- Extend `ipfs mcp --agui` for goal 3; for goal 2, `ipfs mcp /agui` creates and proxies a UAR agent (ADR-0010); file the UAR-side request first.
- Keep stock kubo's `--http`/`--agui` flag defaults off; Desktop and the LKE manifest pass them, so the fork's products have the features on by default.
- "Applies without restart" means: no manual restart by the operator. In-process hot reload on `ipfs mcp` (LKE and Desktop alike); runs in flight finish on the previous backend. Desktop restarts its child only when the listen port must change, and then recreates the window (CSP).

## Merged change order (input to Plan)

One dependency graph from backend §7 and UI §6.4:

1. `team-ownership-agent-backend` (architect; D10) and `agent-settings-contract` (architect, ADR-0009; absorbs UI-01; includes the cloud threat model, auth and origin design, and the documentation-location decision D12). `uar-conformance-release` (request to the UAR team) starts at the same time; it depends on nothing.
2. In parallel after 1: `mcp-agent-hot-reload`, `mcp-agui-a2ui-registry` (mcp-server-engineer, with ui-ux-engineer for A2UI); UI-00 design scope, UI-10 Promethity foundation, UI-11 assets. Then `mcp-agent-settings-api` (depends on hot reload; includes its own off switch and disabled-state tests).
3. UI-20 shell, UI-21 data-viz, UI-30 primitives, UI-31 titles and copy.
4. `desktop-mcp-sidecar` (with port discovery), UI-40 settings page (against the contract, then the real API from `mcp-agent-settings-api`), UI-41 island, UI-42 Desktop bridge and tray.
5. `agent-features-default-on` (includes recording the Desktop child's idle RSS, since an always-on child is a resource default).
5a. `lke-agent-routes` (console host per D1, `--agui`, routes, principal, CORS list, allowlist; operator merges know-me-cluster) after `agent-features-default-on` and after an independent `security-reviewer` pass on the settings API and the cloud design; it must land before any Web UI build with chat on reaches LKE.
6. UI-50 route polish and the `/explore` seam per D9.
7. `agent-backend-uar` (ADR-0010) when the UAR release exists (D13).
8. UI-60 single verification round with independent `reviewer` and `security-reviewer`, then UI-70 delivery (new CID, `WebUIPath` bump by the architect, LKE re-pin after `lke-agent-routes`, Desktop rebuild into `/Applications`).

## Carry-over from ipfs-mcp-server

Fold in: `agentic-uar-integration` and `uar-integration-spike` 1.2-1.5 (superseded by `agent-backend-uar`), A2 (`desktop-agent-chat` 1.6, 1.8), A3 (`webui-webmcp-integration` 1.4), A4 (`webui-agent-chat` 1.3). Leave deferred: A1 (HG-1), A5 (`mcp-app-views`). `phase-boundary-review` 1.8 closes in the old phase. Count note: the runtime's open set for the old phase lists `phase-boundary-review` and not the two deferred UAR changes, so "6 open" is the runtime's count, not the same six.

## Operator decisions (batched, each with a recommendation)

- D0 Immediate mitigation of the existing exposure (before Plan). Recommend: until D1 lands, do not open gateway content in a browser that holds the basic-auth credentials, and approve a small know-me-cluster PR now that removes `https://webui.ipfs.io` and `http://localhost:3000` from the API CORS list. Removing `https://ipfs.prometheusags.ai` itself would break the current Web UI, so the full fix is D1.
- D1 Cloud origin isolation. Recommend a dedicated console hostname (you pick it and add DNS; cert-manager issues the certificate) serving the Web UI, `/api/v0`, `/agui`, the settings API and `/mcp`; the console host routes `/ipfs` to the API port 5001 (which serves only `WebUIPaths`, `cmd/ipfs/kubo/daemon.go:870-878`), never to the 8080 gateway; the API CORS list is reduced to the console origin only, with the gateway origin removed; `/agui`, settings and `/mcp` stay outside basic auth (bearer only). `ipfs.prometheusags.ai` becomes gateway-only, with redirects from its old `/webui` and `/api/v0` paths and a note for registered `/mcp` clients. Alternative: subdomain gateway mode.
- D2 Who may change settings. The server can only check a bearer, so the cloud gate is bearer-only; a typed re-confirmation in the page is a UI safeguard, not a server-enforced confirmation. Recommend a new settings-scoped principal kind (`admin`: settings read/write only, no tool access) instead of the full-access operator token, kept in memory or session storage, never `localStorage`; on Desktop a backend change also needs Touch ID.
- D3 API keys over HTTP. Recommend accepted through the authenticated settings API and stored write-only (Keychain via Desktop; a small PVC-backed settings file on LKE, with the ConfigMap and Secret still the base).
- D4 Egress. Recommend a settings write may extend `egress_ack` only for the newly chosen model host, only with the confirmation in D2, and the UI states that node content will go to that host.
- D5 kubo flag defaults. Recommend stock `ipfs mcp` keeps `--http`/`--agui` off; Desktop and LKE pass them.
- D6 Product name. Recommend "Promethity".
- D7 Form-field edge. Recommend a 1px `text-muted` edge on form controls only (borderless fill measures 1.30:1 dark, 1.31:1 light, failing WCAG 1.4.11).
- D8 Attribution. Recommend "Built on the IPFS Web UI (MIT, Protocol Labs)" in About and the footer plus a one-line non-affiliation statement.
- D9 `/explore`. Recommend token-level theming only (colours and fonts that leak into the package), accepting that its internal layout stays upstream; the alternative is forking `ipld-explorer-components`.
- D10 Ownership extension as described in "Ownership".
- D11 Desktop rebrand. Desktop's userData directory (`~/Library/Application Support/IPFS Desktop`) is derived from `productName` (`package.json:5`), and `safeStorage` Keychain items are likely keyed to the app name (Electron behavior, untested), so renaming `productName` would orphan the stored chat token and confirmation pairing. Recommend changing icons, window titles and the visible display name only, keeping `productName` and the bundle id `io.ipfs.desktop`; a full rename needs a planned migration step.
- D17 `WebUIPath` bump. The architect may edit `core/corehttp/webui.go` only with operator approval (`team.json`). Recommend approving now the single end-of-phase bump to the rebranded Web UI CID, plus its LKE re-pin.
- D18 Default-on migration of stored settings. Recommend filling only absent keys, so a stored `enabled:false` (which may be an explicit opt-out) is kept; testers who saved an endpoint while the default was off turn the feature on themselves.
- D12 Documentation location. Recommend MCP settings documented in `docs/mcp/config-reference.md` (existing practice: `docs/config.md` has no MCP entries) plus a pointer from `docs/config.md`, recorded in ADR-0009 as the reading of the AGENTS.md off-switch rule for `ipfs mcp` settings.
- D13 Goal 2 completion. Recommend this phase may close goals 1, 3, 4 and 5 while `agent-backend-uar` waits for the UAR release, with goal 2 carried as an explicit open change, not deferred silently. Alternative: hold the phase open until UAR ships.
- D14 UAR design. Recommend kubo creates and proxies the UAR agent and holds a UAR credential; kubo tests run against a published UAR release.
- D15 A2UI scope. Recommend catalog discovery plus server-built surfaces this phase; model-composed surfaces later.
- D16 Theme default. Recommend follow the system, dark when unspecified, with a toggle.

## Risks

- R1 Cloud shared origin (threat model above): the largest risk; D1 and D2 address it.
- R2 Rebrand size versus staying mergeable with upstream ipfs-webui; override layer and `docs/PROMETHITY-FORK.md`.
- R3 Hot reload with runs in flight: a swap mid-run must not drop confirmations or leak a key.
- R4 Desktop sidecar port: `127.0.0.1:5050` is also `DefaultHTTPListen` (`mcp/server/listen.go:19`); an operator already running `ipfs mcp --http` collides. The sidecar needs port discovery and the window must be created with the chosen port (CSP is set once).
- R5 Ordering: a Web UI build with chat on must not reach LKE before `lke-agent-routes`, or same-origin `/agui` falls through the `/` route to the gateway.
- R6 UAR lead time (D13).
- R7 liter-llm compatibility untested.
- R8 Latin-only brand fonts versus 31 locales: non-Latin locales fall back to system fonts.
- R9 Deployment: the last rollout caused an outage; LKE changes ship with a tested rollback and no blocking lifecycle hooks (`.prometheus/gotchas.md` 2026-10-13).

## Corrections to the evidence files

- `assessment-ui.md`: 31 locales (not 33); `agentChat.json` exists only in `en`; 5 source `| IPFS` titles, 15 locale files (`welcome.json`), 5 e2e title assertions; `agent-chat/settings-store.js` DEFAULTS at :15 and keys at :9-10, `webmcp/settings-store.js` DEFAULTS at :20; UAR items 2, 3, 7 are unverified, not "missing". UI §5.5 (basic auth for settings) and UI risk 5 are superseded by the cloud threat model and D1/D2.
- `assessment-backend.md` Q7: `ui-ux-engineer` already owns `ipfs-desktop` `src/agent/**`; the real gap is the unowned paths in "Ownership".

## Review disposition (artifact-critic round 1)

CRITICAL 1 (shared origin, unconfirmed egress): fixed by the cloud threat model, D1, D2, D4, R1. CRITICAL 2 (ownership): fixed by "Ownership" and D10. WARNING 3 (auth contradiction): resolved by D1/D2 and the corrections. WARNING 4 (unverified basic auth): live 401 recorded; the policy object is not in git (listed below). WARNING 5 (goal 2 split): D13. WARNING 6 (`/explore`): D9. WARNING 7 (restart meaning): defined in Recommendations. WARNING 8 (port): R4. WARNING 9 (ordering): merged change order and R5. WARNING 10 (`docs/config.md`): D12. WARNING 11 (counts): corrections. SUGGESTION 12: merged change order.

## Unresolved review findings (artifact-critic round 2, BLOCK; two-round limit reached)

Round 2 returned 1 CRITICAL, 6 WARNING, 5 SUGGESTION. The text above was revised for each, but per the two-round limit the revision was not re-vetted; Plan must treat these as open and confirm each:

- C1 existing RPC takeover via the shared origin and API CORS list: added to the threat model, D0 (now) and D1 (CORS reduction, console `/ipfs` to 5001). Not re-vetted; browser behavior for cached basic credentials on cross-origin requests is untested.
- W1 security review before LKE exposure: step 5a gated on `security-reviewer`.
- W2 graph errors: settings API after hot reload; LKE routes after defaults-on.
- W3 Desktop rename orphans userData and Keychain items: D11 revised (display name only).
- W4 typed confirmation is UI-only, operator token too broad: D2 revised to a settings-scoped `admin` principal; the cloud gate is stated as bearer-only.
- W5 missing `WebUIPath` approval: D17.
- W6 migration semantics: D18.
- S1 settings API off switch and Desktop child RSS: steps 2 and 5. S2 console host details: D1. S3 ownership list also omits kubo `PRODUCT.md` and `docs/config.md`: add both to `team-ownership-agent-backend`. S4 the PVC settings overlay sits outside GitOps (an Argo rollback does not revert backend or egress changes) and a key in a PVC file is weaker than a Kubernetes Secret: open for Plan; D3 should prefer writing the key to a Secret if feasible. S5 the reading of the AGENTS.md off-switch rule for `ipfs mcp` settings belongs in the AGENTS.md process, not ADR-0009: D12 is to be recorded as an operator-approved AGENTS.md clarification if accepted. Also: `src/files/FilesPage.js:327` adds a sixth `IPFS` title part.

## Unverified

liter-llm compatibility; UAR items 2, 3, 7; the `ipfs-webui-auth` SecurityPolicy object (behavior observed, object not in git); `shared_transports_enabled` behavior (backend §3). Nothing was built, run or deployed during Assess.
