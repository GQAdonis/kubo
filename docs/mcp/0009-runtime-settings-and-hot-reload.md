# ADR-0009: Runtime settings with hot reload, the `admin` principal, and the cloud console threat model

## Status

**Proposed**, 2026-10-13, by `ipfs-agent-architect`. This ADR records the assumed defaults D0-D5 and D12 and the plan-made assumptions of `.kbd-orchestrator/phases/agent-backend-settings/plan.md` revision 3 (section 1). It stays `Proposed` until the operator answers OG-1 (the batched D1-D18 confirmation). If OG-1 overrides a decision, the affected section is amended before the implementing changes close; the plan's section 1 table names what each override changes.

## Date

2026-10-13

## Context

- Phase `agent-backend-settings`, change `agent-settings-contract` task 1.1. The phase adds a runtime settings API to `ipfs mcp` (read/write the LLM backend, model, system prompt and feature switches without a manual restart), a settings page in the Web UI fork, a Desktop bridge, and the corresponding LKE console routes in know-me-cluster.
- Verified live fact (2026-10-13): the LKE node runs this fork's binary, `ipfs version` 0.44.0-dev-0f15c24. New flags and principals reach LKE only through an image roll (OG-12).
- The cloud node's exposure was verified live on 2026-10-13 (`assessment.md`): one hostname `ipfs.prometheusags.ai` serves the path gateway (`/ipfs`, `/ipns`, `/`), the Web UI and `/api/v0` behind HTTP basic auth, and `/mcp` behind a bearer; `Gateway.PublicGateways` sets that host with `UseSubdomains: false`; the init script sets `API.HTTPHeaders.Access-Control-Allow-Origin` to include `https://ipfs.prometheusags.ai`, `http://localhost:3000`, `http://127.0.0.1:5001` and `https://webui.ipfs.io`, with `Access-Control-Allow-Credentials: true`. `/webui` answering 401 without credentials was observed live; the SecurityPolicy enforcing basic auth is applied out of band and is not in git.
- Prior decisions this ADR builds on and does not reopen: ADR-0001 (F4: `ipfs mcp` is an RPC-only client of the daemon, never opens the repo), ADR-0003 (confirmation classes, the human gate, the AuthVerifier; R14 amendment: default confirmation modes are `native` only, `desktop` opt-in and never a presence resolver), ADR-0004 (principals `human-mcp`/`ui`/`a2a`/`uar`, egress `egress_ack`, the bounded loop), ADR-0008 (topology B deferred). The fork rule of `5f019f0df` (AGENTS.md, User Agency): the agent features that are the fork's purpose ship enabled by default, each with a documented off switch.
- `AGENTS.md` Stability and User Agency rules outrank every decision below: no `/api/v0`, gateway, CID or wire change; every new feature carries an off switch; no hardcoded third-party endpoint without an override and an off switch; no silent opt-out removal.

## Decision

### 1. Runtime settings and hot-reload semantics

- `ipfs mcp` gains a settings resource holding the agent-loop backend (`backend.kind: openai | uar`, base URL, model, key reference), the system prompt, and feature switches. The full wire shapes, error codes and revision scheme are contract schemas (`agent-settings-contract` task 1.2), not prose.
- **Hot reload.** A settings write takes effect without a manual restart of `ipfs mcp`. "Applies without restart" means no manual restart by the operator; it does not promise zero downtime of an in-flight run.
- The runtime holds the effective settings as one immutable snapshot (`LLMClient`, backend host, limits, system prompt) behind an `atomic.Pointer`, read once per run (`mcp-agent-hot-reload` 1.1). Consequences:
  - A run in flight completes on the snapshot it started with; the next run uses the new one.
  - A resumed interrupt reuses its run's original snapshot, or fails with a stable error code; it never silently switches backends mid-run. Since Round 7 a swap that changes the backend endpoint or key revokes the open interrupts: resuming one fails with `failed_precondition` and `details.reason = "backend_changed"`, and the key is dropped at once. A resume already in flight keeps its snapshot until it ends.
  - The previous API key is dropped once no run references the old snapshot.
  - A pending confirmation is never lost across a settings swap (R3).
  - `sessions` and `buckets` survive a swap.
- Validation is one function shared by startup resolution and the runtime apply path (`ApplyAgentSettings`), so a value that would be rejected at startup is rejected by a settings write with the same error.
- **Absent secrets are never fatal (CRITICAL-2).** An absent Secret file or token environment variable disables only the dependent feature (the principal that would authenticate with it, or the backend that would use the key), never startup. The server starts, logs a warning, and the dependent route reports its disabled state.

### 2. The cloud threat model (D0, D1)

**The shared origin.** On the LKE node, `https://ipfs.prometheusags.ai/ipfs/<cid>` (unrestricted gateway content) and `https://ipfs.prometheusags.ai/api/v0` (the full RPC API behind HTTP basic auth) are the *same origin*. A browser that holds the operator's basic-auth credentials sends them on same-origin requests regardless of CORS.

**The live exposure (verified 2026-10-13, pre-existing, independent of this phase).** Any HTML page served as gateway content on that host and opened in such a browser can drive the entire RPC API: config replace, key export, pin removal, shutdown. The API CORS list additionally names `https://webui.ipfs.io` and `http://localhost:3000` with credentials allowed, so content on those two origins can also call the API cross-origin with the browser's credentials.

**Why CORS alone does not close it.** CORS is a check on cross-origin requests. A fetch from `https://ipfs.prometheusags.ai/ipfs/<cid>` to `/api/v0` on the same host is same-origin: no `Origin` check applies, and the credentials go along by default. Removing `https://webui.ipfs.io` and `http://localhost:3000` from the CORS list closes only those two cross-origin callers. Removing the gateway origin itself would break the currently served Web UI (which is same-origin with the API today). The fix is origin isolation, not CORS tuning. (The exact go-ipfs-cmds behavior for a credentialed GET versus POST carrying a disallowed `Origin` is a reviewer's claim, not re-verified by the planner; OG-6 verifies it live.)

- **D0 (interim, in force now).** Until the OG-6 checks pass, the operator does not open gateway content in a browser that holds the Web UI basic-auth credentials. The optional CORS-list PR (removing the two stray origins) is mitigation only and takes effect at pod start; it is not the fix.
- **D1 (the fix).** A dedicated console hostname (operator picks the name, adds DNS; cert-manager issues the certificate) serves the Web UI, `/api/v0` (basic auth) and, later, `/agui`, the settings route and `/mcp` (bearer only). Console `/ipfs` routes to the API port 5001, which serves only `WebUIPaths` (`cmd/ipfs/kubo/daemon.go:870-878`), never to the 8080 gateway. The API CORS list is reduced to the console origin only, keeping `Access-Control-Allow-Credentials`. `ipfs.prometheusags.ai` becomes gateway-only, with redirects from `/webui` and `/api/v0` (which drop `Authorization` and turn POST into GET; clients must move to the console host).
- The CORS-list reduction takes effect only at pod start, because kubo reads `API.HTTPHeaders` at startup; OG-6 includes the planned single-replica restart and immediately verifies: console `/api/v0` without credentials returns 401; a credentialed request carrying `Origin: https://ipfs.prometheusags.ai` gets 403 from kubo; a credentialed POST carrying the console origin succeeds. OG-0 hygiene is lifted only when all three pass.

### 3. The `admin` principal (D2)

- A new principal kind `admin` is added beside `human-mcp`, `ui`, `a2a` and `uar` (ADR-0004). Its scope is the settings API only: settings read and write. It has **no tool access**: an `admin` token is refused on `/mcp`, `/agui` and `/a2a`, and a tool principal's token is refused on the settings route (path-scoped authorization, the G9-4 pattern).
- The settings route is mounted only when an `admin` principal is configured and `agent.settings.enabled` is not `false` (plan-made assumption, confirmed at OG-1). Without an `admin` principal the route answers 404 and nothing is written or dialed.
- A configured `admin` principal whose token environment variable or file is absent is disabled with a warning, not fatal (CRITICAL-2, section 1); the server starts and the principal authenticates nothing.
- On LKE the cloud gate is bearer-only (section 4). Typed re-confirmation of backend and egress changes in the settings page is a UI safeguard only, not a security boundary; on Desktop a backend change additionally requires Touch ID.

### 4. Bearer-only routes outside basic auth; `origin_allowlist`; token storage

- **The Authorization conflict.** Envoy basic auth and the `ipfs mcp` bearer both use the `Authorization` header. If the basic-auth SecurityPolicy targets the new routes, the bearer cannot be sent. Therefore the console routes for `/agui`, the settings route and `/mcp` are *outside* the basic-auth SecurityPolicy and authenticate by bearer only. An offline check per PR asserts no SecurityPolicy targets those rules and that `ipfs-webui-auth` still targets every `/api/v0` and `/webui` rule (`lke-origin-isolation` 1.2, `lke-agent-settings` 1.2).
- **`origin_allowlist` is exactly the console origin.** The strict CORS policy on the `ipfs mcp` listener accepts only the mount's own `origin_allowlist`, not the listener's origin (`mcp/server/http.go:43-60`), so it must name the console origin and nothing else. The bearer routes are not callable from the gateway origin or any other. Since Round 7 the settings route has its own strict policy over `agent.origin_allowlist` (it previously got the listener-wide one): methods `GET`, `PUT`, `POST`, `OPTIONS`; request headers `authorization`, `content-type`, `accept`, `if-match`; response headers `ETag` and `Retry-After` exposed; no credentials header. An operator who relied on `http.allowed_origins` must add the console origin to `agent.origin_allowlist`.
- **The `admin` token never touches `localStorage`.** The settings page holds it in memory or `sessionStorage` only; a 401 from the settings API means "Sign in again". (The chat `ui` token in `localStorage` on the console origin is a separate, pre-existing operator decision of 2026-10-10, recorded in ADR-0004's R11 amendment; `settings-security-review` rules on it under W7. This ADR does not extend that acceptance to the `admin` token.)

### 5. Secrets (D3 with S4)

- API keys are accepted only through the authenticated settings API and stored write-only. On Desktop the key lives in the Keychain via `safeStorage`; the read shape never returns key material, only `api_key: {set, source}`.
- **On LKE (S4): HTTP key writes are off.** `agent.settings.accept_secrets: false`; a key field in a write is refused with `secrets_not_accepted`, and the page renders the field as "Set on the server (Kubernetes Secret)".
- The cloud LLM key lives in an **out-of-band Kubernetes Secret, not in git** (this cluster has no SealedSecrets or SOPS; the existing `ipfs-mcp-token` was created by hand). The operator creates it at OG-14; it is mounted as a file referenced by `llm.api_key` (S1), and every new Secret reference carries `optional: true` so an absent Secret disables the dependent feature instead of failing the pod (CRITICAL-2). Only non-secret settings go to the standalone settings PVC.
- Consequence accepted with S4: the new Secrets exist outside GitOps; a cluster rebuilt from git alone lacks them, and with `optional: true` the agent features stay off rather than the pod failing.

### 6. Egress (D4)

- The settings API is a remote write surface that can change where node content is sent. A settings write may therefore extend the calling principal's `egress_ack` only for the newly chosen model host, only with the D2 typed re-confirmation of that exact host, and the UI states that node content will go to that host. No other host can be added through the settings write; any other change is refused with `egress_confirmation_required` (plan: `mcp-agent-settings-api` 1.6).
- On Desktop the bridge additionally requires Touch ID for an egress extension.
- The ADR-0004 limitation stands: Kubo cannot see what an acknowledged endpoint does with data it receives. The settings API does not widen that; it moves who can set the acknowledgement, which is exactly why the `admin` scope and the per-host confirmation exist.

### 7. Persistence precedence

- Effective settings resolve **flags > overlay > config**: command-line flags win over the overlay file, which wins over the `ipfs mcp` config file (`--mcp-config`).
- The overlay lives in `--settings-dir` (LKE: `/mcp-settings`, a standalone RWO PersistentVolumeClaim; Desktop: `<userData>/mcp`), written with mode 0600. A write-only key stored through the API becomes a 0600 file referenced as `SecretRef{File}`.
- The overlay is applied atomically (write-then-rename); a crashed write never leaves a half-written settings file.
- Every write is optimistic-concurrency checked: the read shape carries a `revision` exposed as an HTTP `ETag`, and a write must present it as `If-Match`; a stale revision is refused with `revision_conflict`.

### 8. Defaults placement (D5)

- Stock `ipfs mcp` keeps `--http` and `--agui` off. Nothing in this phase flips a kubo default. Desktop's sidecar and the LKE manifest pass the flags explicitly; they are the distribution points that turn the features on, not the binary's defaults.
- Rationale: flipping kubo's own defaults is a feature launch for every existing `ipfs mcp --http` user under AGENTS.md ("flipping an existing config default ... is a feature launch"). It would need a changelog launch entry and new disabled-state tests for all of them. Passing flags at the Desktop and LKE integration points reaches the same users without changing the stock binary's behavior for anyone else.
- The fork rule of `5f019f0df` is honored at the distribution points: Desktop starts the sidecar with `--http` and `--agui`; LKE mounts them in the manifest. Each keeps a documented off switch (`mcpSidecar.enabled`; removing the flags from the manifest; `agent.settings.enabled: false`; `agent.a2ui: false`).

### 9. Documentation location (D12)

- `ipfs mcp` settings are documented in `docs/mcp/config-reference.md` (generated by the existing `docs/mcp/config_reference.py` generator from the contract config schema) plus **one pointer paragraph** in `docs/config.md`. `docs/config.md` gains no `ipfs mcp` key entries.
- The reading of the AGENTS.md off-switch rule for `ipfs mcp` settings (whether the `docs/config.md` requirement applies to a surface documented in `docs/mcp/`) goes to the operator as a standalone AGENTS.md PR (round-2 S5, gate OG-5). That PR changes no Stability or User Agency sentence and touches nothing else (`agent-settings-contract` 1.6 drafts it).

### 10. LKE confirmation behavior under `confirmation.modes: []`

The LKE sidecar runs with `confirmation.modes: []` (the out-of-band gate off; ADR-0003 Round 4 amendment: turning the gate off can only make the server refuse more). Documented behavior (`lke-agent-settings` 1.3):

- **MCP clients on `/mcp`** confirm writes and destructive calls through MCP elicitation only. A client that cannot elicit gets `confirmation_unavailable` for every write and destructive call.
- **Chat writes** (`ui` principal on `/agui`) are confirmed by the chat-island interrupt answered with a user gesture, as ADR-0004's human gate provides for that principal.
- **Destructive tools from chat** are refused with `confirmation_unavailable`: they set `requires_native_prompt: true` (ADR-0003), and the LKE pod has no presence resolver (`macos-localauth`, `linux-polkit`; `desktop` never counts, R14 amendment). Fail-closed is the intended state on a headless cluster node.

## The uncomfortable thing

This ADR's own design carries two exposures it does not close, and naming them is part of the decision:

1. **A cloud `admin` bearer holder can repoint egress.** Whoever holds the `admin` token can change the LLM backend host through the settings API; the per-host typed re-confirmation is a UI safeguard in a page that same holder's browser is running, not a server-enforced second factor. The bearer *is* the authorization. `egress_ack` extension is bounded to the newly chosen host, but the holder chooses the host, so the bound is a speed bump, not a wall. The mitigations are scope (the token reaches only the settings route), storage (never `localStorage`), and origin isolation (D1) — none of which helps once the token itself leaks.
2. **An Argo rollback does not revert overlay settings.** The settings overlay lives on the `ipfs-mcp-settings` PVC, outside git. A `git revert` plus Argo sync restores the manifest — flags, routes, Secrets references — but the overlay file keeps whatever backend, model and switches were last written through the API. Rolling back the deployment can therefore leave the node running a settings state that exists nowhere in GitOps. The written rollback procedure (`lke-agent-settings` 1.5) must include the explicit settings reset (delete the overlay file on the PVC); without that step, a rollback is partial and the operator may not know it.

## Amendment (2026-10-16, Round 7: security review corrections and operator rulings)

Source: `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/settings-security-review.md` (PASS WITH FINDINGS, no CRITICAL or HIGH). The dispositions are in that file; this section records the rulings and the behaviour changes that touch this ADR.

### Operator ruling on M1 and M2: accepted with the conditions

Operator, verbatim: "Accept with the conditions (Recommended)". Two MEDIUM residual risks are accepted as interim, until Kratos plus `flint-gate` session auth lands:

- **M1: the console's interim inline Basic credentials in `localStorage`.** The interim API address `https://<user>:<password>@console.prometheusags.ai` is stored verbatim as `ipfsApi`, turned into `Authorization: Basic` on every call and shown in a plain text field. Any same-origin script can read it, it persists in the browser profile, and it grants the full `/api/v0` surface (config replace, key export, pin removal, shutdown) from anywhere. It is contained by the console `/ipfs` pin to `WebUIPath`, kubo's Origin check and the GET 405. The residual is XSS or supply-chain compromise of that bundle, or access to the profile.
- **M2: the chat `ui` token in `localStorage` on the console origin** (ADR-0004 R11 amendment, ADR-0005 amendment). It has no expiry or endpoint binding and is a bearer usable from anywhere on `/agui` and `/mcp`. The same script that reads M1 gets it. A stolen token means unattended reads plus the console `ui` principal's write groups; destructive calls stay refused. The 2026-10-10 acceptance holds only while M1 is in force.

Conditions, as recorded in `.prometheus/decisions.md`:

1. a dedicated htpasswd user for the console browser;
2. rotate it and the `ui` token when session auth lands, and clear the stored API address in every browser profile;
3. no screenshots or evidence of console Settings or Status meanwhile;
4. the console `ui` principal gets the smallest `write_groups` that works;
5. check read-only whether Envoy Gateway can send a fixed basic-auth realm (unverified; it would remove the workaround).

When session auth replaces Basic auth the `ui` token also leaves `localStorage` (a session cookie from `flint-gate`, or memory plus `sessionStorage`). The reviewer also suggested an explicit `/mcp` kind list on LKE that excludes `ui`; it is not decided.

### Carried to the session-auth ADR

Design notes from the review for the Kratos and `flint-gate` ADR. They are not findings and nothing here is implemented:

- **Cookie sessions bring back CSRF.** Use `SameSite=Strict`, `HttpOnly` and `Secure`, an Origin check at the gateway, and no ambient credentials reaching the bearer-designed `ipfs mcp` routes.
- **Trusted identity headers.** Accept identity headers only from the proxy, strip them from client input, and stop direct pod access with a NetworkPolicy. `lke-origin-isolation` MEDIUM-4 is still open and becomes load-bearing.
- **NetworkPolicy** as above, enforced and verified live (unverified today).
- **`admin` as a distinct identity.** Map the admin capability to a distinct Kratos identity or role with step-up, not to any logged-in session.

### Settings fixes now in code

- **L1: plain-http backends.** A settings write or probe accepts a plain `http` backend URL only for a loopback host or a host listed in `embeddings.insecure_http_hosts` (before, a write accepted `http` to any host when no key went with it). Round 8 (N1) widened the scheme rule: any `https` to `http` downgrade needs typed confirmation (`egress_confirmation`) whatever the port, and a same-host scheme change in either direction also needs confirmation; an `http` to `https` move to a new, acknowledged host does not. (Round 7 text: "a scheme change on the same host is treated as a host change".)
- **L2: audit log.** Settings writes log at `Info`, or `Warn` for a host move or an egress extension, with the principal ID, the field names, the old and new host and whether a key was not carried; never a key, prompt, model or path.
- **L3: strict CORS** on the settings route, as in section 4.
- **L4: limits.** Route 60 requests a minute with a burst of 60 per principal; probes 10 a minute with a burst of 10 and 2 in flight; 16 open change streams. A refusal is `429` with `Retry-After`.
- **L5: open interrupts after a backend change** fail with `failed_precondition` and `details.reason = "backend_changed"` (AG-UI `409`; A2A `BACKEND_CHANGED`), as in section 1.
- **I5: `*.localhost` is no longer loopback.** Only `localhost` and literal loopback IPs are. Go's resolver sends other `*.localhost` names to DNS, so a key could leave the machine in cleartext.
- **I2: overlay `egress_ack` persistence.** The overlay's per-principal `egress_ack` is stored per principal ID, is not pruned, and a principal re-created under the same ID inherits the hosts. Moving back to a previously confirmed host asks for no new confirmation. Deleting the overlay clears it (the rollback step in "The uncomfortable thing", item 2).
- **I3: stale preflight.** The `/mcp` preflight for `ui` and `a2a` reads `BackendHosts` from the configuration file, not the overlay, so after a settings move it can check a stale host. Nothing leaks (the content check uses the snapshot host) but a refusal can be wrong, and the D4 copy that says content will go to the new host overstates when the `ui` principal's `egress_ack` does not cover it: the chat fails closed. `chat-setup.md` now says so.

## Consequences

### Positive

- Backend, model, prompt and feature switches change without a manual restart, on Desktop and on LKE, with in-flight runs and pending confirmations undisturbed.
- The cloud origin-isolation fix (D1) removes the live same-origin RPC exposure instead of decorating it, and OG-6 verifies the rejection live before OG-0 hygiene is lifted.
- The `admin` scope bounds what the most powerful new token can do: settings, never tools.
- Secrets never cross the browser on LKE; the cluster's lack of SealedSecrets/SOPS is answered with an out-of-band Secret rather than a PVC file or a git commit.
- Stock kubo defaults are untouched; no existing `ipfs mcp` user sees a behavior change.

### Negative

- Three planned single-replica outages (OG-6, OG-12/OG-13, OG-9) on a one-replica node.
- The overlay is state outside GitOps (the uncomfortable thing, item 2), and the new Secrets are created by hand (item: S4 consequence in section 5).
- Typed re-confirmation invites the same approval-fatigue reading as any repeated prompt; it is recorded as UI-only so nobody mistakes it for a boundary.
- The settings API is one more remote write surface to review; `settings-security-review` (Round 7) is a hard prerequisite of the LKE rollout.

## Falsifiers

| Assumption | Observable falsifier | Consequence |
|---|---|---|
| Settings apply without restart | A backend change requires a process restart to take effect in `test/cli` | Block `mcp-agent-settings-api` |
| Runs and confirmations survive a swap | `TestAgentSwapResumeUsesOriginalBackend`, `TestAgentSwapMidStream` or `TestAgentSwapKeepsPendingConfirmation` fails | Block `mcp-agent-hot-reload` |
| Absent secrets are never fatal | `ipfs mcp` exits or the settings route 500s with a principal token file or LLM key file absent | Block; violates CRITICAL-2 |
| `admin` is settings-scoped | An `admin` token is accepted on `/mcp`, `/agui` or `/a2a`, or a tool token on the settings route | Block (G9-4 pattern) |
| CORS is not the fix | OG-6 shows a credentialed request with the gateway origin succeeding against `/api/v0` after isolation | Origin isolation failed; revert per OG-6 and reassess D1 |
| Precedence holds | A test shows overlay beating a flag, or config beating the overlay | Block |
| LKE fail-closed confirmation | A destructive call from chat on LKE executes without a presence resolver | Block; the refusal is the design |

## References

- `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (revision 3): section 1 (D0-D18 and plan-made assumptions), section 2 (OG-0, OG-1, OG-6, OG-12, OG-13, OG-14), section 4 (`agent-settings-contract`, `mcp-agent-hot-reload`, `mcp-agent-settings-api`, `lke-origin-isolation`, `lke-agent-settings`), section 9 (CRITICAL-1, CRITICAL-2, W4, W7, W8, S1, S2, S4, S5)
- `.kbd-orchestrator/phases/agent-backend-settings/assessment.md`: the verified shared-origin exposure and live CORS list (2026-10-13)
- ADR-0001 (placement, F4), ADR-0003 (confirmation gate; R14 amendment), ADR-0004 (principals, egress, bounded loop; R11 amendment), ADR-0005 (chat UI), ADR-0008 (topology-B gate)
- `openspec/changes/agent-settings-contract/`
- `AGENTS.md`: Stability and User Agency rules; the `5f019f0df` default-on rule
