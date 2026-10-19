# Confirmation read: know-me-cluster #48 at `b4f2cf4` (rebased on `acc6e73`)

This is a confirmation of my earlier read (`pr50-pr48-security-read.md`), not a fresh review. It was done with no Write tool and no shell, so the lead saves it verbatim.

## Verdict

**#48: PASS. No security gate remains before OG-26.** There is no CRITICAL, HIGH or open MEDIUM.

| Gate | Decision | Evidence |
|---|---|---|
| MEDIUM-3 | **CLOSED** (removed, not just accepted) | 6.0 README:114-116 and `decisions.md:130-132`. All five off-cluster names were deleted. Every remaining A, CNAME and AAAA record points at 23.239.29.33. A backup of the zone exists. |
| MEDIUM-4 | **CLOSED** in config; confirmed live in 6.7 step b2 | `pr48-v3.diff:1177-1180` sets `extractFrom` to the Authorization header with prefix `Bearer `, and the server dry run keeps it (`dryrun-out.yaml:209-212`). Envoy adds the default `?access_token=` location only when headers, params and cookies are all empty (`up/envoy-jwt-extractor.cc:222-228`). Envoy Gateway copies `ExtractFrom` as written (`up/eg-jwt.go:261-265`). The pass-through matchers that read `extractFrom` are built only when `passThroughAuthHeader` is true (`up/eg-oidc.go:223-225`), and it is pinned false. |
| LOW-13 | **CLOSED** | Image README:69-81 shows the tested image ID `a50f357c…` as the manifest's `config.digest`, with RepoDigest `3467dea8…`. The image is deployed on that digest with commit `1140ff1` (6.0 README:120). |
| LOW-14 | **CLOSED** | The parser splits each Set-Cookie on `;`, trims, lower-cases and compares attribute names exactly (`pr48-v3.diff:178-199`). The self-test passes 12 parser cases and 2 end-to-end cases (`check-logs/console-st.txt`). Operator step a2 covers the five session cookies. |
| LOW-18 | **CLOSED** | `1.3-f-exact-config.txt` ran the same digest with the manifest's exact sidecar args. Startup logs `routes session` only. `/auth/session` answers 401 with no token, a garbage token or a forged JWT. A JWT gets 401 on `/mcp` and the existing bearer still gets 200. `cfg-exact-48.json` matches the ConfigMap text in the rebased tree (`kmc-cutover/.../manifests.yaml:917-967`). I compared content; I did not check the sha. |
| Rebase gate 6 | **MET** (on the reported outputs) | The tree pins `3467dea8…` (`manifests.yaml:62,136,200`). These checks pass on the rebased tree: `webui-auth-targeting.sh`, `session-allowlist.sh` (25 checks), its self-test (24 of 24 mutations), `networkpolicy-check.sh`, kubeconform (0 invalid), the dry run, and the preflight (16:32Z, after the #50 sync, 0 failing). |

Other items the builder claimed:

- **CSRF before OAuth2: verified.** Envoy Gateway orders CSRF at 4, OAuth2 at 9, JWT at 10 and RBAC at 301 (`up/eg-httpfilters.go:109-142`). `csrf: {}` is fully enforced, with no shadow mode (`up/eg-csrf.go:146-171`). The CSRF filter answers 403 `Invalid origin` when Origin is missing or wrong (`up/envoy-csrf_filter.cc:105-125`). So checks 5, 6 and the new 6b are sound.
- **LOW-15: closed.** Six carrier pins were added, each with a mutation. `webui-auth-targeting.sh` now also covers console-host routes to `ipfs-mcp`.
- **LOW-16a: closed in config.** Five backend rules set the frame-deny headers (`dryrun-out.yaml:34-123`). Live confirmation is in the 6.7 list.
- **Break-glass fallback: done.** See the body's "Break glass" section.
- **INFO-K: done.** ADR-0013 line 133. Nit: line 136 still says "accepted or fixed by operator's choice", but LOW-17 is now accepted (`decisions.md:133`).

## `agent.origin_allowlist` observation: not a finding (INFO-P)

- **What happens:** with no allowlist, `ipfs mcp` refuses any request that carries an `Origin` with 403 before auth (probes 9 and 10). It does not allow it. That is the correct fail-closed default.
- **Why it changes nothing today:** a browser sends no `Origin` on a same-origin GET. Only GET `/auth/session` is routed, so nothing depends on the allowlist.
- **Availability dependency:** 6.7 step a ("Signed in as") is the live proof that the Web UI's request reaches `ipfs mcp` without an Origin. If step a fails with a 403 from `ipfs mcp`, this is the cause. It is not a security regression.
- **Prerequisite for `lke-agent-settings`:** before any write route (`/agui`, settings) goes on the console, set `origin_allowlist` to exactly `https://console.prometheusags.ai`, never `*`. Add a test that `Origin: https://ipfs.prometheusags.ai` gets 403.

## Before the operator merges #48 (OG-26)

1. Mark the PR ready (it is still DRAFT) and merge exactly `b4f2cf4`. Any later push means re-running `session-allowlist.sh` with `--self-test`, `webui-auth-targeting.sh` and the dry run.
2. Nothing else. The post-logout URIs are set (6.0 README:85-93), #50 is synced, and MEDIUM-3 and LOW-17 are recorded in `decisions.md`.

## Post-sync acceptance (6.7)

- `ipfs-webui-auth` is `Accepted` and no rule answers 500. If it is not Accepted, run the 6.4 revert.
- Do the planned restart of `ipfs-0`. The pod's `ipfs-mcp` log must show `auth.oidc is on … routes session`, and the JWKS fetch over the 10443 egress rule must work. LOW-18 was container-level only.
- `verify.sh ipfs console.prometheusags.ai=/webui/:302` PASS.
- `console-session.sh unauth` PASS on all checks, including 5, 6, 6b and 8.
- `origin-isolation.sh unauth` PASS.
- Operator steps a, a2, b (including 403 on `/auth/session`), b2 (MEDIUM-4, live), c, d, d2 (record what happened) and e.
- Manual probe, not scripted: `GET /api/v0/version` with `Sec-Fetch-Mode: cors` and `Authorization: Bearer x` and no cookies answers 401 from the gateway, not kubo's body.
- Signed-in devtools check: the `/webui/` and `/api/v0` responses carry `X-Frame-Options: DENY` and `Content-Security-Policy: frame-ancestors 'none'`. Envoy's own replies (the 302s and 401s) do not carry them, so an unauthenticated curl cannot show this.
- On any FAIL, run the revert and record it.

## Recommended, not gating

- INFO-L: run the live `clientCIDRs` grep.
- Follow-up 5: check whether the access log keeps the query string on the console logout URL, which carries `id_token_hint`.
- Delete the two `_lovable` TXT records.
- Add a DNS guard so that a new know-me.tools record pointing off-cluster fails a check. Such a record would reopen MEDIUM-3.
- Carried: INFO-O (task 8.1 verifier review, before `lke-agent-settings`), MCP-03 per-identity rate limit, LOW-16b, LOW-17 Web UI fix.

## Not verified here

- That the tree I read is commit `b4f2cf4` (no git).
- Any run of the checks. I read logs and reported outputs only. The negative `/agui` run of `webui-auth-targeting.sh` appears only in the PR body.
- The DNS zone, which I accept as the lead's observation.
- Any live cluster or browser state.

## Lead's note (appended by the lead, not part of the review)

- INFO-L live grep was run earlier on 2026-10-19: `kubectl --context know-me get securitypolicy -A -o yaml | grep -n clientCIDRs` returned nothing.
