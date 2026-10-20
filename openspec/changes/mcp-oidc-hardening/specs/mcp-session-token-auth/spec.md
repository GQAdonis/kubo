# Spec Delta: mcp-session-token-auth

## MODIFIED Requirements

### Requirement: Token validation
A JWT SHALL authenticate only if all of the following hold:
- its signature verifies against a key from the configured JWKS whose `kid` matches;
- its `alg` is in `algorithms`;
- `iss` equals `issuer` exactly;
- `aud` contains one of `audiences`;
- when `aud` holds more than one value, `azp` is present and is one of `audiences`;
- when `azp` is present, it is one of `audiences`;
- `exp` is in the future and `nbf`, when present, is in the past, both within `clock_skew`;
- `iat` is not in the future beyond `clock_skew`.

Any failure SHALL return 401 without saying which check failed. The verdict SHALL expire no later than the token's `exp`.

#### Scenario: Wrong audience
- **WHEN** a validly signed token from the configured issuer carries an `aud` not in `audiences`
- **THEN** the response is 401

#### Scenario: Several audiences without an authorized party
- **WHEN** a validly signed token's `aud` lists a configured audience and a second value, and it carries no `azp`
- **THEN** the response is 401

#### Scenario: Several audiences with a foreign authorized party
- **WHEN** a validly signed token's `aud` lists a configured audience and a second value, and its `azp` is not in `audiences`
- **THEN** the response is 401

#### Scenario: Several audiences with the console as authorized party
- **WHEN** a validly signed, mapped token's `aud` lists a configured audience and a second value, and its `azp` is one of `audiences`
- **THEN** the token authenticates

#### Scenario: Single audience with a foreign authorized party
- **WHEN** a validly signed token's single `aud` is configured but its `azp` is not in `audiences`
- **THEN** the response is 401

#### Scenario: Expired token
- **WHEN** a token's `exp` is earlier than now minus `clock_skew`
- **THEN** the response is 401

#### Scenario: Unknown key
- **WHEN** a token's `kid` is not in the cached JWKS
- **THEN** the JWKS is refetched at most once per 30 seconds, and the token is rejected if the key is still absent

### Requirement: Route scope and Origin for session identities
A JWT SHALL authenticate only on routes listed in `auth.oidc.routes`. When `auth.oidc.routes` is absent, the list SHALL be `session` only. With `auth.oidc.require_origin` true (the default), a JWT-authenticated request with a method other than GET, HEAD or OPTIONS SHALL carry an `Origin` listed in `agent.origin_allowlist`, and SHALL get 403 otherwise. This includes a request with no `Origin`. Static bearer requests keep their existing Origin behaviour. When `auth.oidc.enabled` is true, `auth.oidc.routes` names any route other than `session`, and `agent.origin_allowlist` is empty, startup SHALL log one Warn line naming both keys and SHALL continue.

#### Scenario: JWT on /mcp by default
- **WHEN** a valid, mapped token is presented on `/mcp` with `routes` absent
- **THEN** the response is 401

#### Scenario: JWT on /agui by default
- **WHEN** a valid, mapped `ui` token is presented on `/agui` with `routes` absent
- **THEN** the response is 401 and no run starts

#### Scenario: Session report by default
- **WHEN** a valid, mapped token is presented on `/auth/session` with `routes` absent
- **THEN** the session report is returned

#### Scenario: Missing Origin on a session write
- **WHEN** `routes` lists `agui` and a valid `ui` token posts to `/agui` without an `Origin` header
- **THEN** the response is 403 and no run starts

#### Scenario: Write route without an Origin allowlist
- **WHEN** `auth.oidc.enabled` is true, `routes` lists `agui` and `agent.origin_allowlist` is empty
- **THEN** startup succeeds and logs one Warn line naming `auth.oidc.routes` and `agent.origin_allowlist`

## ADDED Requirements

### Requirement: Bounded key staleness
`ipfs mcp` SHALL NOT use a JWKS key once more than `auth.oidc.jwks_max_stale` has passed since the last successful key-set fetch. Past the bound, every JWT SHALL get 401 until a fetch succeeds, and static bearers SHALL be unaffected. `ipfs mcp` SHALL log the expiry once per stale period, without any key material or token.

#### Scenario: Issuer unreachable past the bound
- **WHEN** keys were fetched, the JWKS URL then becomes unreachable, and more than `jwks_max_stale` passes
- **THEN** a token signed with a previously held key gets 401, a static bearer still authenticates, and exactly one log line reports the expiry

#### Scenario: Issuer unreachable inside the bound
- **WHEN** keys were fetched and the JWKS URL becomes unreachable for less than `jwks_max_stale`
- **THEN** a valid token signed with a held key still authenticates

#### Scenario: Recovery
- **WHEN** the bound has passed and a later fetch succeeds
- **THEN** tokens signed with keys in the new set authenticate again

### Requirement: Staleness bound configuration
`auth.oidc.jwks_max_stale` SHALL default to `24h`. It SHALL be a duration no shorter than `auth.oidc.jwks_cache_ttl`, and config loading SHALL fail otherwise. No value SHALL remove the bound. The value SHALL be validated whether or not `auth.oidc.enabled` is true; with `enabled` false nothing SHALL be fetched.

#### Scenario: Bound shorter than the cache TTL
- **WHEN** a config sets `jwks_max_stale` shorter than `jwks_cache_ttl`
- **THEN** config loading fails and the error names `auth.oidc.jwks_max_stale`

#### Scenario: Default
- **WHEN** `auth.oidc` is enabled and `jwks_max_stale` is absent
- **THEN** the bound is 24 hours

### Requirement: Key fetch independent of the triggering request
A key-set fetch SHALL NOT be cancelled when the request that triggered it is cancelled or its client disconnects. The fetch SHALL still be bounded by its own timeout. The key set fetched SHALL be available to the next request inside the refetch window.

#### Scenario: Client drops during a cold fetch
- **WHEN** the cache is empty, a request with a JWS-shaped bearer triggers a fetch, and that request's context is cancelled before the fetch completes
- **THEN** the fetch completes, and a second request with a valid token inside the 30 second refetch window authenticates

### Requirement: Per-subject session and confirmation binding
For an identity authenticated by an ID token, the MCP session binding and every confirmation binding SHALL be keyed on the principal together with the token's `sub`. Two subjects mapped to one principal SHALL NOT be able to continue each other's MCP session or answer each other's pending confirmations. Identities authenticated by a static bearer SHALL keep their existing binding. The `sub` SHALL NOT appear in any response.

#### Scenario: Two people on one principal, MCP session
- **WHEN** subjects A and B are both listed on one principal, the routes include `mcp`, and B presents A's `Mcp-Session-Id` with B's own valid token
- **THEN** the request is refused as a session mismatch

#### Scenario: Two people on one principal, confirmation
- **WHEN** subjects A and B are both listed on one principal and a confirmation is pending for A
- **THEN** B's answer to that confirmation is refused and the pending call is unchanged

#### Scenario: Static bearer unchanged
- **WHEN** a static bearer principal opens an MCP session and continues it
- **THEN** the session binds and continues as it did before this change
