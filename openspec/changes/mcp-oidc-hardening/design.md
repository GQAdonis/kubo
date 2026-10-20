# Design: mcp-oidc-hardening

## Context

See `proposal.md` for the findings this change answers. The verifier is in `mcp/server/jwt.go` (claims), `jwks.go` (key cache), `jwt_session.go` (subject mapping and step-up) and `oidc_config.go` (config). The relevant current behavior, read in this session:

- `jwt.go` validates `aud` with go-jose `jwt.Expected{AnyAudience: ...}` and reads no `azp`.
- `jwksCache.key` serves any held key after a failed fetch, and inside the 30 s refetch window, with no age limit. `fetch` derives its context from the request and sets `lastAttempt` before sending.
- `auth.go` sets `TokenInfo.UserID` to the principal ID. The go-sdk session binding compares UserID. The confirmation broker and gate key on the principal (`broker.go` `dedupeKey`, `gate.go` call construction).
- `DefaultOIDCRoutes()` returns `agui`, `settings`, `index`, `session`.
- The deployed console config sets `auth.oidc.routes: [session]` and maps one `sub` to `console-ui`.

## Goals / Non-Goals

**Goals:**

- Clear the 8.1 LOW gates before `lke-agent-settings` opens `/agui` or settings to session identities.
- Close the 8.2 MAJOR and MINOR items, and both NITs, with one review round.

**Non-Goals:**

- In-process rate limiting of bearer verification (8.1 INFO-1, the carried MCP-03 item).
- Rejecting access-token-shaped ID tokens by `scp`, `client_id` or `typ` (the optional half of LOW-1). See Open Questions.
- Removing the p2p-forge `replace`. That waits for ipshipyard/p2p-forge#96 to be released.
- Any change to the Envoy SecurityPolicy, Hydra or the cluster.

## Decisions

### D1. `azp` rule, OIDC Core 3.1.3.7 steps 4 and 5

A token whose `aud` holds more than one value needs an `azp` that is one of `audiences`. A token with an `azp` needs that `azp` in `audiences`. This is a check on decoded claims after go-jose's own validation, so a failure returns the same generic 401.

- Alternative: require exactly one `aud`. Rejected. It would refuse spec-conformant tokens that name a second audience with the console as `azp`.
- Alternative: do nothing while Hydra access tokens stay opaque. Rejected. The 8.1 scenario only needs an operator to switch Hydra to JWT access tokens, which kubo cannot observe.

### D2. Staleness cap `auth.oidc.jwks_max_stale`, default 24h

The cache records the time of the last successful fetch, `fetchedAt`. Past `fetchedAt + jwks_max_stale`, `lookup` returns no key, stale or fresh, and the cache logs the expiry once until the next successful fetch. The value must be at least `jwks_cache_ttl`, because the TTL already triggers refetches, and a cap below it would drop keys that are still fresh.

- The reviewer offered `max(ttl*4, 1h)` or a fixed 24h. 24h is proposed because it outlasts a typical overnight issuer outage without signing everyone out, and it bounds a revoked key to one day. It is configurable and is an operator ruling (task 1.1).
- No value disables the cap, and `0` is a load error. The off switch of the whole feature remains `auth.oidc.enabled: false`. A "never expire" setting would recreate the finding.
- Alternative: derive the cap from the TTL. Rejected. It is harder to document, and it couples two settings that operators tune for different reasons.

### D3. Key fetch on `context.WithoutCancel`

`fetch` runs on `context.WithoutCancel(ctx)` wrapped in the existing 10 s `jwksFetchTimeout`. A client that disconnects then cannot burn the refetch window. Concurrent waiters still share one fetch through `fetchMu`.

### D4. Per-subject binding for session identities

For an identity with a `Session`, the binding key becomes the principal ID plus `#` plus the `sub`. It is set as `TokenInfo.UserID`, so the go-sdk session check uses it, and the broker and gate use it for confirmation dedupe and answer checks. Static bearer identities keep the principal ID as before. The key is internal: it is never written to a response, and audit records already carry `sub` (ADR-0013 section 9).

- Alternative: enforce "one subject per write-granted principal" at config load. It is smaller, but it rejects configs that are valid today, and it leaves the hazard in place for any principal whose grants change later. It is kept as a fallback only if D4 cannot be done without changing the broker's confirmation contract. The engineer reports that before implementing it, and the architect amends this design.

### D5. Default `auth.oidc.routes` is `[session]`

An absent `routes` now accepts a JWT only on `/auth/session`. Every other route must be listed. When `routes` names a route other than `session` and `agent.origin_allowlist` is empty, startup logs one Warn line and continues, since `require_origin` already fails closed. The schema `default` and the contract example change together.

- This narrows a default; it widens nothing. The changelog says so, with the explicit list an operator sets to keep the old behavior.

### D6. JWKS client and proxy variables: document, do not change

`docs/environment-variables.md` will state that the JWKS fetch honors `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`. The rationale for keeping other `ipfs mcp` clients off the proxy is that they carry content, prompts or keys, and it does not apply here: the JWKS request carries none of them and the response is public keys. The other choice, `Proxy: nil`, could break a deployment whose JWKS egress goes through a proxy. 6.7 recorded JWKS egress as working, and the path it used was not recorded.

## Risks / Trade-offs

- [A 24h cap signs everyone out during an issuer outage longer than a day] → The cap is configurable. The expiry is logged once. Static bearers and loopback access keep working.
- [D4 changes the go-sdk session key for session identities, so a client holding an `Mcp-Session-Id` minted before the upgrade must start a new session] → JWTs are not accepted on `/mcp` in any deployed config (`routes: [session]`), so no live session is affected.
- [Narrowing the routes default breaks a config that relied on the old default] → No deployed config does. The changelog gives the explicit list to restore it.
- [The `azp` rule refuses a Hydra ID token that carries an unexpected `azp`] → Hydra sets `azp` to the client ID, `ipfs-console`, which is the configured audience. This is unverified against a live token. The operator's signed-in browser steps in `lke-agent-settings` task 1.8 confirm that a session still reaches `/auth/session` once the image carrying this change is rolled.

## Migration Plan

- The schema accepts the new key and the old configs unchanged, except an absent `routes`, whose meaning narrows.
- Rollback is a revert of the change's commits. The deployed config sets `routes` explicitly and does not set `jwks_max_stale`, so it loads before and after.

## Open Questions

- Should the verifier also reject tokens that carry `scp` or `client_id`, which are access-token shaped? It depends on the claim names in a real Hydra ID token, which nobody has read. It can be added later without changing these specs' other requirements, and it is not a gate for `lke-agent-settings`.

## Implementation note (2026-10-20): D4 binding key

D4 said the binding key for a session principal is the principal ID plus `#` plus the `sub`. The implementation uses the principal ID plus `#` plus the first 32 hex characters of SHA-256 of the `sub` (`Principal.BindingKey()` in `mcp/server/principal.go`). The confirm token is signed but readable by the client, so it must not carry the raw `sub`. Binding semantics are unchanged: two subjects on one principal never share a session, a confirm token or an elicitation answer. Static principals keep their ID as the key. The startup Warn from D5 is skipped when `require_origin` is false, because the allowlist has no effect then.
