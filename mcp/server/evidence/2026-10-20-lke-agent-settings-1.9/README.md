# lke-agent-settings 1.9: post-merge evidence (2026-10-20)

know-me-cluster PR #55 merged at 2026-10-20T17:27:54Z as `b4e3b59`. Argo app `ipfs` synced `b4e3b59` at about 17:30 and recreated `ipfs-0` (RollingUpdate). The init container completed at 17:31:01. Argo `Synced/Healthy`.

## Image check (1.10)

| Container | Ready | Restarts | Image |
|---|---|---|---|
| ipfs | true | 0 | `ghcr.io/gqadonis/kubo@sha256:2e4c906ee3de71435c0d3bdeb629f7bc9e937d10631824e2c7e35e23ba144d17` |
| ipfs-mcp | true | 0 | same |

This digest is kubo `b17bf0dd4`, which contains the mcp-oidc-hardening merge. PVC `ipfs-mcp-settings` is Bound.

`ipfs-mcp` startup log (no token values):
- `AG-UI endpoint enabled at /agui (1 browser origins allowed)`
- `no model backend is configured ..., so /agui answers 503 backend_not_configured until one is`
- `agent settings API enabled at /agent/settings (admin principals only)`
- `warning: principal admin (admin) is disabled: read credential file: ... no such file or directory` (the optional admin Secret was not created)
- `auth.oidc is on: session tokens from https://oauth.know-me.tools are accepted on routes session,agui`

## Unauthenticated checks (agent, read-only)

`agent-settings.sh unauth` from `b4e3b59`: 11 of 11 PASS, exit 0. Full output in `agent-settings-unauth.txt`.

| Check | Result |
|---|---|
| Console `/api/v0/version`, cors, no session: 401 | PASS |
| Console `/webui/`: 302 to oauth.know-me.tools | PASS |
| Console `/agui`, cors, no session: 401 | PASS |
| POST console `/agui` with a foreign Origin: 403 | PASS |
| Console `/agent/settings`: 404 | PASS |
| Console `/agui/`, `/agui/catalog`; `/agent/settings` on `ipfs.` and `rpc.`: 404 | PASS |
| `ipfs.prometheusags.ai/mcp` without a bearer: 401 | PASS |
| Live HTTPRoutes: no rule to ipfs-mcp can match `/agent/settings` | PASS |

Regression: RPC through `rpc.prometheusags.ai` answers `ipfs version` (basic auth), and the gateway serves the Web UI CID with 200 (`rpc-and-gateway.txt`).

## Operator checks (pending)

| Step | Expected | Result |
|---|---|---|
| Signed in at `/webui/`, open chat | `/agui` answers 503 `backend_not_configured` | pending |
| `GET /auth/session` | reports `console-ui` | pending |
| Second, non-allowlisted Kratos account on `/agui` | 403 | pending |
| `/oauth2/logout`, then `/agui` | 401 | pending |
| Port-forward admin | skipped: the admin Secret was not created, so admin is disabled (401 to all) | n/a |

The `azp` check against a live Hydra ID token is proven by the first signed-in `/agui` call: 503 means the token was accepted, and 401 with a valid session means `azp` differs.
