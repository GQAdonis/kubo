# console-session-auth 6.7: after OG-26 (cutover know-me-cluster #48)

Date: 2026-10-19. Operator: "PR #48 is merged" (merge `efeaf57`). Context `know-me`, read-only checks by the lead. No token or cookie value is recorded here.

## Sync and policy

- Argo `ipfs` Synced to `efeaf57` at 21:11:17Z, Healthy.
- SecurityPolicy `ipfs/ipfs-webui-auth`: spec keys `authorization, csrf, jwt, oidc, targetRefs` (no `basicAuth`); ancestor `ipfs-gateway-gw` `Accepted=True/Accepted`.
- HTTPRoute `ipfs-webui`: parent `https-console` `Accepted=True`, `ResolvedRefs=True`. A stale status entry for parent `https` carries `RouteRulesOverlap=True` naming `ipfs/ipfs-webui-legacy`, a route that no longer exists; the spec attaches only to `https-console`. Informational, no traffic effect.
- Unauthenticated probes: `/`, `/webui/`, `/api/v0/version`, `/auth/session` all 302 to `https://oauth.know-me.tools/oauth2/auth?client_id=ipfs-console&code_challenge=…` (S256); `/oauth2/callback` 401. No route answered 500.

## Scripted checks (unauthenticated)

| Check | Result |
|---|---|
| `console-session.sh unauth` (`console-session-unauth.txt`) | RESULT: PASS (sign-in redirects with S256 and the right callback; cookies Secure, HttpOnly, SameSite=Lax, host-only; fetch-style 401 without redirect; no Basic challenge; CSRF 403 `Invalid origin` for a foreign and a missing Origin; console Origin passes CSRF then 401; gateway `/mcp` 401 without bearer) |
| `origin-isolation.sh unauth` | RESULT: PASS (encoded `/%61pi/v0/version` POST refused at the gateway, 403 `Invalid origin`) |
| Manual: `GET /api/v0/version` with `Sec-Fetch-Mode: cors`, `Authorization: Bearer x`, no cookies | 401, 18-byte gateway body, not kubo's |
| `verify.sh ipfs` (`console.prometheusags.ai=/webui/:302`, `ipfs.prometheusags.ai=/ipfs/bafkqaaa:200`) | PASS, 0 failing |
| `session-allowlist.sh`, `webui-auth-targeting.sh`, `networkpolicy-check.sh` on main `efeaf57` | PASS |

## Still to do

1. Planned restart of `ipfs-0` (operator-approved) so `ipfs mcp` loads the new `ipfs-mcp-config` (`auth.oidc`, routes `session`); then the sidecar log line and the JWKS fetch over the 10443 egress rule.
2. Operator browser steps a, a2, b (incl. `/auth/session` 403), b2 (MEDIUM-4 `?access_token=`), c, d, d2, e, and the signed-in frame-deny header check.
