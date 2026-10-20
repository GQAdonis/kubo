# Proposal: mcp-oidc-hardening

Phase `agent-backend-settings`. Follow-up to `console-session-auth` (ADR-0013), from its two phase-boundary reviews:

- `.agent-team/kubo-ipfs-agentic/findings/security/agent-backend-settings/8.1-verifier-security-review.md` (8.1, PASS with LOW-1 to LOW-5);
- `.agent-team/kubo-ipfs-agentic/findings/review/agent-backend-settings/8.2-console-session-auth-compat-review.md` (8.2, PASS with MAJOR-1, MINOR-1 to MINOR-7, NIT-1 and NIT-2).

It runs before `lke-agent-settings`, which depends on it.

## Why

The `ipfs mcp` OIDC verifier is safe in today's deployed configuration (`auth.oidc.routes: [session]`), and both reviews passed. The 8.1 review names five LOW items that must be cleared before `lke-agent-settings` accepts console session tokens on `/agui` or the settings route: the token type is not pinned to the console client, keys are served stale with no age limit, two people mapped to one principal share one session binding, the default route list opens write-capable routes, and three safeguards have no test. The 8.2 review adds a changelog disclosure the operator-approved p2p-forge fork `replace` still owes, plus documentation, test and error-message corrections. Fixing these now keeps them out of the change that first exposes write routes to session identities.

## What Changes

- **Audience and authorized party (8.1 LOW-1).** A token whose `aud` holds more than one value is accepted only if its `azp` is one of `auth.oidc.audiences`. A token that carries `azp` is accepted only if `azp` is one of `auth.oidc.audiences`. This applies OIDC Core 1.0 section 3.1.3.7 steps 4 and 5.
- **Key staleness cap (8.1 LOW-2).** New key `auth.oidc.jwks_max_stale`, proposed default `24h`, subject to the operator's ruling (task 1.1). Past that age since the last successful key-set fetch, held keys are dropped and every JWT gets 401 until a fetch succeeds. The event is logged once. No value removes the cap; the feature's off switch stays `auth.oidc.enabled: false`.
- **Key fetch survives a dropped client (8.2 MINOR-4).** A key-set fetch is no longer cancelled when the request that triggered it is cancelled. The 10 s fetch timeout still bounds it.
- **Per-person session binding (8.1 LOW-3).** For an identity authenticated by an ID token, the MCP session binding and the confirmation binding key on the principal plus the `sub`, not on the principal alone. Static bearer identities are unchanged.
- **Default route list (8.1 LOW-4).** When `auth.oidc.routes` is absent, a JWT is accepted on `session` only, not on `agui`, `settings`, `index` and `session`. Operators list every other route explicitly. A startup Warn line is written when the list names a route other than `session` while `agent.origin_allowlist` is empty. This narrows a default that has shipped only in this fork's unreleased v0.44 work; the deployed LKE config already sets `routes: [session]` explicitly.
- **Tests for existing safeguards (8.1 LOW-5, 8.2 MINOR-5).** Negative tests for redirect refusal, the 1 MiB key-set bound, multi-`aud` handling and a cancelled fetch.
- **Real-binary disabled state (8.2 MINOR-3).** The `test/cli` "off" subtest keeps the session principal configured.
- **Startup message (8.2 MINOR-6).** A config whose principals are all session-only, with `auth.oidc.enabled` false, fails startup with a message that names `auth.oidc.enabled`.
- **dag-jose under go-jose v4.1.6 (8.2 MINOR-7).** A `test/cli` case round-trips a dag-jose object.
- **Documentation (8.2 MAJOR-1, MINOR-1, MINOR-2, NIT-2).**
  - A v0.44 Highlights note discloses the p2p-forge fork `replace`, go-jose v4.1.6, that `go install github.com/ipfs/kubo/cmd/ipfs@...` no longer works, and that the replace is dropped when ipshipyard/p2p-forge#96 ships. The replace itself is operator-approved (ADR-0013:187) and is not changed here.
  - `docs/environment-variables.md` states that the JWKS fetch honors the proxy variables.
  - The `principals[].subjects` off-switch wording matches load-time validation.
  - ADR-0013's Stability wording is corrected, and an ADR-0013 amendment records this change's decisions.
- **NIT-1.** The `/auth/session` encode error is logged at Debug, not discarded.

No `/api/v0`, HTTP Gateway, default CID recipe or wire change. No new dependency, and no new `replace` directive.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `mcp-session-token-auth` (introduced by `console-session-auth`, not yet archived): token validation gains the `azp` rule; the default of `auth.oidc.routes` becomes `session` only; new requirements bound key staleness, keep a key fetch independent of the triggering request, and bind sessions and confirmations per subject. `console-session-auth` must be archived before this change, so that the main spec exists when these deltas are applied.

## Impact

- **Owners.**
  - ipfs-agent-architect: `mcp/contract/config/ipfs-mcp-config.schema.json` and examples, the generated `docs/mcp/config-reference.md`, ADR-0013 amendment and wording, `docs/changelogs/v0.44.md` Highlights, and `docs/environment-variables.md`. The last path is owned by no role; this change records the routing decision that the architect edits it.
  - mcp-server-engineer: `mcp/server/{jwt.go,jwks.go,jwt_session.go,oidc_config.go,auth.go,auth_session.go,broker.go,gate.go}` and their tests. This change also routes `cmd/ipfs/kubo/mcp_http.go` (startup message), `test/cli/mcp_session_token_test.go` and a new `test/cli` dag-jose case to this role. Those paths are owned by no role, and console-session-auth set the precedent.
  - security-reviewer and reviewer: findings files only.
- **Config contract.** New key `auth.oidc.jwks_max_stale`, with a default and off switch. Changed default of `auth.oidc.routes`. Changed off-switch text of `principals[].subjects`.
- **`docs/config.md`.** No change: every key here belongs to the `ipfs mcp` config file documented in `docs/mcp/config-reference.md`, not to the Kubo config.
- **Stability.** No `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new or changed key keeps an off switch. No endpoint is hardcoded.
- **Depends on.** `console-session-auth` (complete).
- **Blocks.** `lke-agent-settings`.
