# Proposal: console-session-auth

Phase `agent-backend-settings`, plan revision note of 2026-10-16 (`.kbd-orchestrator/phases/agent-backend-settings/plan.md`, "Plan revision note (2026-10-16): console-session-auth"). ADR: [`docs/mcp/0013-console-session-auth.md`](../../../docs/mcp/0013-console-session-auth.md) (Proposed). It runs before the Round 9 cluster changes `lke-agent-settings` and `lke-vector-search`.

## Why

Today the console at `https://console.prometheusags.ai` relies on Envoy basic auth. Two interim residuals came out of that. The browser holds inline Basic credentials and the chat `ui` bearer in `localStorage`, accepted in ADR-0009 (M1) and ADR-0004 (M2) only until session auth lands. Basic auth's per-URL realm defect also forced that workaround. The operator chose Ory Hydra beside the existing Kratos, with Envoy Gateway's built-in OIDC, and kept the console on prometheusags.ai. This change has to land before the console gains `/agui`, settings and search routes in Round 9, because those routes would otherwise ship on the interim scheme.

## What Changes

- **Architect.** ADR-0013, plus contract keys in `mcp/contract/`:
  - the new `auth.oidc` config section and the `principals[].subjects`, `require_amr` and `max_auth_age` keys, each with a default and an off switch;
  - the `/auth/session` route;
  - `go-jose/v4` promoted to a direct dependency at `v4.1.6`;
  - the generated `docs/mcp/config-reference.md`.
- **mcp-server-engineer.** A JWT verifier for `ipfs mcp`. It checks the Hydra JWKS signature, `iss`, `aud`, `exp`, `nbf` and `iat`, and maps `sub` (never email) to principals through a local allowlist. Admin is a distinct mapping with `amr` and `auth_time` step-up. JWT identities on non-safe requests must send a listed `Origin`. The verifier is off by default; static bearers, Desktop and stdio are unchanged. It comes with tests and the `/auth/session` route.
- **Web UI** (`../ipfs-webui`, ui-ux-engineer). In session mode, detected from `/auth/session`, the Web UI:
  - deletes the stored chat `ui` token and any stored API address carrying userinfo;
  - calls same-origin with no `Authorization`;
  - turns a 401 into "Sign in again";
  - offers sign-out.
  
  Loopback and Desktop behaviour is unchanged.
- **Cluster** (know-me-cluster, ipfs-agent-architect through `lke-gitops-change`, with preflight and verify):
  - Hydra `v26.2.0` pinned by digest in `flint-core`, with its own `hydra` database in the existing Postgres, at the new host `oauth.know-me.tools`;
  - Kratos `oauth2_provider` and the Kratos UI Hydra settings;
  - the out-of-band OAuth client `ipfs-console`;
  - SecurityPolicy `ipfs-webui-auth` changed in place from `basicAuth` to `oidc` + `jwt` + `authorization` (`sub` allowlist, default Deny) + `csrf`;
  - NetworkPolicies for the `ipfs` pod and Hydra;
  - retirement of the htpasswd user and the console `ui` token after a soak.
- **security-reviewer.** Reads the ADR and contract, the verifier diff, and each cluster PR before its merge gate.
- **BREAKING (cluster only, not kubo):** non-browser clients of `https://console.prometheusags.ai/api/v0` that used basic auth are redirected to sign in after cutover. Their supported path is `kubectl port-forward`. Kubo's `/api/v0` contract itself is unchanged.

## Capabilities

### New Capabilities

- `mcp-session-token-auth`: how `ipfs mcp` authenticates an OIDC ID token from the console session, maps it to principals, enforces admin step-up and Origin, and reports the session.
- `console-session-gateway`: how the console's ingress signs a person in through Hydra and Kratos, authorizes them, protects against CSRF, keeps pods unreachable except through the gateway, and signs them out.
- `webui-console-session`: how the Web UI behaves on a session-authenticated console: no stored credentials, same-origin calls, re-sign-in and sign-out.

### Modified Capabilities

None. No main specs exist under `openspec/specs/`. The affected behaviour of `mcp-agent-settings-api` (admin principal) and `lke-agent-settings` (console routes) is extended, not changed. The plan revision note records the task-text amendments to `lke-agent-settings` and `lke-vector-search`.

## Impact

- **Owners:** ipfs-agent-architect (ADR, contract, `go.mod`, docs, cluster PRs); mcp-server-engineer (`mcp/server/**`); ui-ux-engineer (`../ipfs-webui`); security-reviewer (findings only).
- **Code:**
  - `mcp/server/auth*.go`, config loading, a new `/auth/session` handler and the HTTP auth chain;
  - `mcp/contract/config/ipfs-mcp-config.schema.json`, `mcp/contract/routes/` and examples;
  - the Web UI agent-chat and API-address code;
  - in know-me-cluster, `namespaces/flint-core/manifests.yaml` and `namespaces/ipfs/manifests.yaml`, plus `lke-checks/`.
- **Dependencies:** `github.com/go-jose/go-jose/v4` goes from indirect `v4.1.4` to direct `v4.1.6`. New images: `oryd/hydra:v26.2.0@sha256:ff67c7fb5f95074fa53374d41151713554960504b340cd3f95b09e65deaea2a9`, and possibly `oryd/kratos-selfservice-ui-node:v26.2.0` if v1.3.1 fails the challenge flow.
- **Stability:** no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature has an off switch (ADR-0013 section 11). No endpoint is hardcoded in kubo.
- **Operator gates** (plan revision note): OG-23 to OG-27.
