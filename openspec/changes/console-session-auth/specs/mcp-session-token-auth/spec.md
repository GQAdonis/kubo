# Spec Delta: mcp-session-token-auth

## Purpose

How `ipfs mcp` authenticates an OpenID Connect ID token forwarded from the console session (ADR-0013), maps it to configured principals, enforces admin step-up and the request Origin, and reports the session, without changing static bearer authentication.

## ADDED Requirements

### Requirement: Off by default, static bearers unchanged
JWT authentication SHALL be active only when `auth.oidc.enabled` is true. When it is false or absent, `ipfs mcp` SHALL accept no JWT, SHALL fetch no JWKS, SHALL NOT mount `/auth/session`, and SHALL authenticate static bearer tokens exactly as before. Enabling it SHALL NOT change how a static bearer token is matched.

#### Scenario: Disabled
- **WHEN** `auth.oidc` is absent and a request presents a validly signed ID token
- **THEN** the response is 401 and no outbound request to any JWKS URL was made

#### Scenario: Static token with OIDC enabled
- **WHEN** `auth.oidc.enabled` is true and a request presents a configured static bearer token
- **THEN** it authenticates as that principal, as it did with OIDC disabled

### Requirement: Required configuration, no built-in endpoint
When `auth.oidc.enabled` is true, `issuer`, a non-empty `audiences` and `jwks_url` SHALL be required. The config SHALL be rejected at load when any is missing. `jwks_url` SHALL be `https`, or `http` only for a loopback host. `algorithms` SHALL NOT accept `none` or any `HS*` value. `ipfs mcp` SHALL contain no default issuer, JWKS URL or audience.

#### Scenario: Missing JWKS URL
- **WHEN** a config sets `auth.oidc.enabled: true` without `jwks_url`
- **THEN** config loading fails and the error names `auth.oidc.jwks_url`

#### Scenario: Symmetric algorithm
- **WHEN** a config lists `HS256` in `auth.oidc.algorithms`
- **THEN** config loading fails

### Requirement: Token validation
A JWT SHALL authenticate only if all of the following hold:
- its signature verifies against a key from the configured JWKS whose `kid` matches;
- its `alg` is in `algorithms`;
- `iss` equals `issuer` exactly;
- `aud` contains one of `audiences`;
- `exp` is in the future and `nbf`, when present, is in the past, both within `clock_skew`;
- `iat` is not in the future beyond `clock_skew`.

Any failure SHALL return 401 without saying which check failed. The verdict SHALL expire no later than the token's `exp`.

#### Scenario: Wrong audience
- **WHEN** a validly signed token from the configured issuer carries an `aud` not in `audiences`
- **THEN** the response is 401

#### Scenario: Expired token
- **WHEN** a token's `exp` is earlier than now minus `clock_skew`
- **THEN** the response is 401

#### Scenario: Unknown key
- **WHEN** a token's `kid` is not in the cached JWKS
- **THEN** the JWKS is refetched at most once per 30 seconds, and the token is rejected if the key is still absent

### Requirement: Subject mapping, never email
A token SHALL resolve only through `principals[].subjects`, matched exactly against `sub`. The `email` claim SHALL NOT grant or select any principal. For each request, the verifier SHALL select the principal whose kind the route accepts. Two principals of the same kind listing the same subject SHALL be a config error. A valid token whose `sub` maps to no principal accepted on the route SHALL get 403.

#### Scenario: Email matches, subject does not
- **WHEN** a valid token's `email` equals an operator's email but its `sub` is in no principal's `subjects`
- **THEN** the response is 403 and no principal is resolved

#### Scenario: One person, two capabilities
- **WHEN** a subject is listed in a `ui` principal and an `admin` principal, and its token calls `/agui` and then the settings route
- **THEN** `/agui` runs as the `ui` principal, and the settings route evaluates the `admin` principal

### Requirement: Admin step-up
For an `admin` principal reached by JWT, `amr` SHALL contain at least one method of the principal's `require_amr`. The default is any of `totp`, `webauthn`, `passkey` and `lookup_secret`. `now - auth_time` SHALL NOT exceed its `max_auth_age` (default 1h). A token missing `auth_time` SHALL fail this check. A failing request SHALL get 403 with code `step_up_required`. An explicit `require_amr: []` SHALL accept any `amr` and SHALL be documented as a weakening.

#### Scenario: Password-only session
- **WHEN** a subject mapped to `admin` presents a token with `amr: ["password"]` under the default `require_amr`
- **THEN** a settings read returns 403 `step_up_required`

#### Scenario: Stale admin session
- **WHEN** the token's `amr` includes `totp` and `auth_time` is 2 hours ago
- **THEN** a settings write returns 403 `step_up_required` and nothing is changed

### Requirement: Route scope and Origin for session identities
A JWT SHALL authenticate only on routes listed in `auth.oidc.routes` (default `agui`, `settings`, `index`, `session`). With `auth.oidc.require_origin` true (the default), a JWT-authenticated request with a method other than GET, HEAD or OPTIONS SHALL carry an `Origin` listed in `agent.origin_allowlist`, and SHALL get 403 otherwise. This includes a request with no `Origin`. Static bearer requests keep their existing Origin behaviour.

#### Scenario: JWT on /mcp by default
- **WHEN** a valid, mapped token is presented on `/mcp` with the default `routes`
- **THEN** the response is 401

#### Scenario: Missing Origin on a session write
- **WHEN** a valid `ui` token posts to `/agui` without an `Origin` header
- **THEN** the response is 403 and no run starts

### Requirement: Session report route
When `auth.oidc.enabled` is true, `GET /auth/session` SHALL return, for a JWT identity:
- the principal names and kinds it can resolve to;
- the `email` claim, if any;
- the token's `exp`;
- for admin, one of `ok`, `step_up_required` or `not_eligible`.

It SHALL return 401 when there is no valid identity. It SHALL NOT return the token, `sub` or any other claim.

#### Scenario: Signed-in ui user
- **WHEN** a mapped `ui` subject without an admin mapping calls `GET /auth/session`
- **THEN** the response lists the `ui` principal and reports admin `not_eligible`, and contains no token and no `sub`

### Requirement: Failures never stop startup or leak tokens
An unreachable or invalid JWKS SHALL NOT stop `ipfs mcp` from starting, and SHALL NOT affect static bearers. JWT requests SHALL get 401 until a key set loads. No log, audit record or response SHALL contain a token. Audit records for JWT identities SHALL name the principal and the `sub`.

#### Scenario: JWKS down at startup
- **WHEN** `auth.oidc.enabled` is true and the JWKS URL refuses connections
- **THEN** `ipfs mcp` starts, a static bearer request succeeds, a JWT request gets 401, and the log names the URL and no token
