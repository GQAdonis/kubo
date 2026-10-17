# Spec Delta: console-session-gateway

## Purpose

How the ingress of `https://console.prometheusags.ai` signs a person in through Ory Hydra and Kratos, authorizes them by identity, guards against cross-site requests, keeps the IPFS pod reachable only through the gateway, and signs them out (ADR-0013).

## ADDED Requirements

### Requirement: Sign-in through Hydra and Kratos
A request without a valid console session SHALL be handled in one of two ways:
- a top-level navigation to any console path under the session policy SHALL be redirected to the configured Hydra issuer's authorization endpoint, and after a Kratos login SHALL return to the original URL with a session;
- a request carrying `Sec-Fetch-Mode: cors` SHALL get 401 instead of a redirect.

#### Scenario: First visit
- **WHEN** a browser with no console cookies opens `https://console.prometheusags.ai/webui/`
- **THEN** it is redirected to the Hydra issuer, then to the Kratos login UI, and after a successful login it lands on `/webui/` signed in

#### Scenario: Expired session on fetch
- **WHEN** the Web UI issues a `fetch` to `/api/v0/id` with an expired session
- **THEN** the response is 401, not a redirect

### Requirement: Session cookie scope
The console session cookies SHALL be set only for the host `console.prometheusags.ai` (no `Domain` attribute), with `Secure`, `HttpOnly` and `SameSite=Lax`, and SHALL hold the tokens encrypted. They SHALL NOT be sent to `ipfs.prometheusags.ai`.

#### Scenario: Gateway host
- **WHEN** a signed-in browser requests `https://ipfs.prometheusags.ai/ipfs/<cid>`
- **THEN** the request carries no console session cookie

### Requirement: Authorization by identity, default deny
Every console route under the session policy SHALL forward only requests whose token passes all of the following, and SHALL reject the rest with 403:
- a valid signature against the issuer's JWKS;
- the expected `iss`;
- `aud` containing the console client ID;
- an `exp` that is not past;
- a `sub` in the operator's allowlist.

The default action SHALL be Deny. Holding a Kratos account SHALL NOT be enough.

#### Scenario: Registered but not allowlisted
- **WHEN** a person registers a new Kratos account and signs in to the console
- **THEN** every console route answers 403, including `/api/v0` and `/webui`

### Requirement: No trusted identity headers
The ingress SHALL carry identity to backends only as the signed ID token in `Authorization: Bearer`. It SHALL overwrite any client-supplied `Authorization` on session routes. It SHALL NOT add claim-derived headers.

#### Scenario: Client sends its own bearer
- **WHEN** a signed-in browser sends `Authorization: Bearer <old ui token>` to `/agui`
- **THEN** the backend receives the session's ID token, not the client's value

### Requirement: Cross-site request protection
A mutating request (POST, PUT, PATCH, DELETE) on a console session route whose `Origin` is missing or is not `https://console.prometheusags.ai` SHALL be rejected with 403 at the gateway, before any backend sees it.

#### Scenario: Gateway-hosted page posts to the console
- **WHEN** a page at `https://ipfs.prometheusags.ai/ipfs/<cid>` sends a POST to `https://console.prometheusags.ai/api/v0/config/replace`
- **THEN** the response is 403 and kubo receives no request

### Requirement: Bearer-designed routes stay cookie-free
`/mcp` SHALL NOT be under the session policy. A console session cookie SHALL have no effect on `/mcp` authentication.

#### Scenario: Cookie on /mcp
- **WHEN** a signed-in browser sends a request to `/mcp` with no `Authorization`
- **THEN** the response is 401

### Requirement: No path around the gateway
Inside the cluster, the IPFS pod's ports 5001, 5050 and 8080 SHALL accept connections only from the console gateway's Envoy proxies. The swarm port SHALL stay open. Hydra's admin port SHALL accept connections only from the Kratos and Kratos UI pods.

#### Scenario: Pod in another namespace
- **WHEN** a pod in another namespace connects to `ipfs-api.ipfs.svc:5001`
- **THEN** the connection does not succeed

#### Scenario: Peering
- **WHEN** a remote peer dials the node's swarm port
- **THEN** the connection is accepted as before

### Requirement: Logout and lifetime
`/oauth2/logout` SHALL clear the console session cookies, SHALL end the Hydra login session, and SHALL return the browser to the console. A console session SHALL NOT outlive the refresh-token lifetime (12h) without a new sign-in. The documentation SHALL state that the Kratos session on `know-me.tools` survives a console logout.

#### Scenario: Sign out
- **WHEN** a signed-in user opens `/oauth2/logout`
- **THEN** the console cookies are cleared, and the next console navigation goes to the issuer again

### Requirement: One-commit rollback
Reverting the cutover commit SHALL restore basic auth on the console in one Argo sync. The htpasswd Secret SHALL remain until the operator ends the soak period.

#### Scenario: Revert
- **WHEN** the cutover merge is reverted and synced
- **THEN** the console `/api/v0/version` answers 401 with a Basic challenge, as before the cutover
