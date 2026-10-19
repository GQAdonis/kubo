# console-session-auth 4.5: Hydra read-only verification after OG-23 and OG-24

Date: 2026-10-19. Context `know-me`, run by the lead. OG-23: PRs #40 and #42 merged and synced 2026-10-18. OG-24: run 2026-10-19 around 08:40Z by `og24-hydra-bootstrap.sh` on operator direction ("I need to you to find a way to automate this"; then "go"). No secret value was printed in any step.

## OG-24 result

- Role `hydra` (LOGIN) and database `hydra` owned by it; PUBLIC revoked on `hydra`. The password was set as a SCRAM-SHA-256 verifier computed locally. Role `hydra` keeps PUBLIC `CONNECT` on the other databases (#40 LOW-2, accepted; fix is 6.0).
- Secret `flint-core/flint-hydra` with keys `dsn`, `secretsCookie`, `secretsSystem`.
- `hydra-automigrate`: "Successfully applied 224 migrations." `deploy/hydra` 1/1, pod `hydra-bb7b74658-psblr`. The pod from before the Secret existed was in `CreateContainerConfigError` and was not Ready after 120s, so the script deleted it once; the replacement became Ready.
- Client `ipfs-console`: `grant_types` `authorization_code`, `refresh_token`; `response_types` `code`; `scope` `openid offline_access email`; `redirect_uris` `https://console.prometheusags.ai/oauth2/callback`; `skip_consent` true; `client_secret_basic`.
- Secret `ipfs/ipfs-console-oidc` with key `client-secret`.

Post-OG-24 preflight (flint-core and argocd) posted on #40: https://github.com/Prometheus-AGS/know-me-cluster/pull/40#issuecomment-6263750710. It closed 4.1-4.3. Its two FAIL lines are unrelated: flint-core Argo health Degraded from the pre-existing `flint-gate` rollout `7d8485bd58` (CrashLoopBackOff, "migration 6 was previously applied but has been modified"; the old ReplicaSet serves 2/2; waived as in 5.2), and `no Application with path namespaces/argocd` (the gateway is in app `argocd-config`).

## Checks

| Check | Observed | Result |
|---|---|---|
| `https://oauth.know-me.tools/.well-known/openid-configuration` `issuer` | `https://oauth.know-me.tools` | PASS |
| `id_token_signing_alg_values_supported` (unverified point 5) | `["RS256"]` | recorded |
| JWKS `https://oauth.know-me.tools/.well-known/jwks.json` has at least one key | 2 keys, both `RSA` `RS256` `use=sig` | PASS |
| `hydra-admin` has no route on any Gateway | no HTTPRoute, GRPCRoute, TCPRoute or TLSRoute in any namespace references `hydra-admin`; public `/admin/clients` returns 404 | PASS |
| Probe pod in another namespace cannot reach `hydra-admin.flint-core.svc:4445` | BLOCKED | PASS |
| `hydra-public` 4444 answers | through its designed path, `https://oauth.know-me.tools/health/ready` from the probe pod: `{"status":"ok"}` | PASS (see deviation) |
| Allowed path: `kratos-ui` reaches 4445 | `kratos-ui -> 4445 200 {"status":"ok"}` | PASS |

## Probe pod (09:14:46Z, deleted 09:14:55Z)

`busybox:1.36` in `default`, label `purpose=csa-probe-4-5` and no other labels, run under the operator's "go" to "run the 4.5 check". Deleted afterwards; no pod is left with the label.

```
BLOCKED hydra-admin.flint-core.svc:4445
BLOCKED hydra-public.flint-core.svc:4444
REACHED ipfs-swarm.ipfs.svc:4001
CONTROL oauth.know-me.tools:443 REACHED {"status":"ok"}
```

The `ip=` field the probe printed read `unresolved` on every line, the same awk defect as in 5.4, so it carries no information and is omitted above. The positive control is the 4001 line: the same pod resolved and reached a Service in another namespace on a port its policy allows from anywhere, so the 4445 and 4444 failures come from `hydra-ingress`, not DNS or routing.

## Deviation from the task text

4.5 asks that `hydra-public` 4444 answer while 4445 does not. `hydra-ingress` (5.2) admits 4444 only from the `argocd-gateway` Envoy pods, so a probe pod in another namespace is blocked on 4444 too, by design. The check was made through the public URL instead, which is how every in-cluster consumer reaches Hydra (the policy comment says so). The gateway reaching 4444 is shown by that public 200.

## Result

PASS. Remaining in group 4: 4.6, the operator's browser check.

# 4.6: operator browser check (2026-10-19)

Run by the operator in a browser. The operator had no Kratos account; registration is enabled (`selfservice.flows.registration.enabled: true`, password method), so the operator signed up at `auth.know-me.tools` from the Hydra login page.

| Step | Operator result | Logs |
|---|---|---|
| Throwaway authorization request for `ipfs-console` (state `237982d5…`) | Kratos sign-up and login completed; no consent screen; landed on `console.prometheusags.ai/oauth2/callback?code=ory_ac_…&scope=openid+email&state=237982d5…` (404, expected: nothing serves the callback before 6.1) | Hydra: `requests/login/accept` 200, `requests/consent/accept` 200 (kratos-ui auto-accepts for `skip_consent`) |
| Logout `https://oauth.know-me.tools/oauth2/sessions/logout` | "Your log out request however succeeded" on Hydra's fallback page | Hydra `requests/logout/accept`, `fallbacks/logout/callback`; kratos-ui `GET`/`POST /ui/logout`; Kratos `whoami` 401 at 09:32:32Z (Kratos session ended) |
| Login again (state `9b2e4c6a…`) | Asked for the password again; landed on the callback with a new `code` | Kratos login form 09:37:47Z, `POST /self-service/login` 09:38:18Z 303; Hydra login and consent accept 200 |

#39 LOW-2 check: across login, consent skip and logout, no kratos-ui log line mentions 4434, `kratos-admin` or `oryapis`; `KRATOS_ADMIN_URL` is unset; a request from kratos-ui to `kratos-admin:4434` times out (`kratos-ingress`), and the flow still completed, so kratos-ui does not need Kratos admin. Limit: kratos-ui logs only inbound requests and no egress policy selects it, so an outbound call to `oryapis.com` would not leave a log line; the result there is "no evidence of a call", not proof. No change to `KRATOS_ADMIN_URL` or `kratos-ingress` is needed. The v1.3.1 image switch in 4.3 is not needed (the UI is on the pinned v26.2.0 digest since #42).

Side note: the lead first misread the logout logs (no `/self-service/logout` in a 20-minute window) as the Kratos session surviving logout; the operator's re-login asked for the password and Kratos `whoami` returned 401 after logout, so that reading was wrong and is withdrawn.

## Follow-ups for group 6 (operator, 2026-10-19)

- After the OIDC login the operator must land on `https://console.prometheusags.ai/webui/`, not `/` (which does not serve GET). Operator, verbatim: "this should have taken me to the `/webui` URL and not the root".
- Hydra `urls.post_logout_redirect` is unset, so logout ends on Hydra's fallback page ("Logout succeeds but currently has nowhere to go"). Set it, and point the `ipfs-console` client's post-logout URI at `https://console.prometheusags.ai/webui/` (currently `/`).

## Result

4.6 PASS.
