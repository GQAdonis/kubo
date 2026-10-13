# R12 security review: confirmation.resolvers / cli / ttl and .github/workflows/mcp.yml

Date: 2026-10-11. Reviewer: security-reviewer (independent context, read-only tools). Recorded by the lead from the reviewer's hand-back, because the reviewer session had no write tool.

Counts: CRITICAL 0, HIGH 0, MEDIUM 1, LOW 4, INFO 5.

## MEDIUM

### M1. Under cli mode a shell-capable stdio agent can approve its own write and destructive calls

- Where: `mcp/server/access.go:125-131,136,144-146` (no-identity sessions skip policy); `mcp/server/principal.go:28` (stdio runs as `LocalPrincipal`, `human-mcp`); `mcp/server/policy.go:113-119` (`shell_capable` checked only for configured identities); `mcp/server/confirm/channel.go:171-175` (cli resolver authenticated by request header; the TTY check is client-side only, `confirm/cli.go:55-57`); `mcp/server/gate.go:272` (native prompt required only for `ui`/`a2a`).
- Scenario: the operator sets `confirmation.cli: true` (new path, `confirm/config.go:93-107`) or `--confirm=cli`; a non-eliciting stdio host with a Bash tool calls a write or destructive tool and approves it with a POST to the confirm socket carrying the cli header.
- Why: ADR-0003 keeps cli "only for principals the operator asserts are not shell-capable"; stdio cannot make that assertion. The accepted-risk record covers the Desktop verifier, not this.
- Scope: predates this diff; the new key adds a persistent way to enable it.
- Fix: while a cli resolver is active, treat `LocalPrincipal` like a principal with no `shell_capable` assertion, or record it as accepted risk with a startup warning.

## LOW

- **L1.** `NarrowedAwayWarning` (`mcp/server/confirm/manager.go:16-17,46-47,52`) says calls are refused with `confirmation_unavailable` even when the cli resolver remains. Fix: condition on `!wantCLI`, or name cli as the remaining resolver.
- **L2.** The config file is read three times per start (`cmd/ipfs/kubo/mcp_confirm.go:125-126,149-150`, `mcp_http.go:113-114`); modes, resolvers and ttl can come from different file versions. No weakening beyond what the file's writer controls. Fix: parse once and pass the result.
- **L3.** Operator and agent messages name only `--confirm=cli` (`confirm/channel.go:173`, `policy.go:125,133`, `cmd/ipfs/kubo/mcp.go:108-111`). Fix: "cli confirmation (--confirm=cli or confirmation.cli)".
- **L4.** `actions/checkout` persists `GITHUB_TOKEN` in `.git/config` (`.github/workflows/mcp.yml:68,108,126,203`), readable by later PR-controlled steps. Fix: `persist-credentials: false` on every checkout.

## INFO

- **I1.** The test-provider ttl override is unbounded and wins over config (`server.go:171-177`, `resolver_registry.go:53-55`, `confirm/testresolver_on.go:32-34`); only in `mcptestresolver` builds, never in `make build` or CI. Defence in depth: clamp in `withProviders`.
- **I2.** A ttl under 1s validates (`config.go:76`), while token `Exp` is whole seconds (`gate.go:334,438`), so tokens can expire on issue. Fails closed. Consider a 1s minimum.
- **I3.** Action SHA pins were not verifiable without network (`mcp.yml:68,70,128,132,241`).
- **I4.** pnpm 11.25.0 and Node 24.11.1 are fetched by version, not hash; electron's install script downloads a binary.
- **I5.** `pull_request` from forks runs untrusted code on macOS runners with a read-only token and no secrets: acceptable.

## Verified safe

- Narrowing only filters (`confirm/config.go:113-115`, `manager.go:44-45`); the schema enum excludes `cli`.
- Presence requirement for `ui`/`a2a` destructive calls is unaffected (`gate.go:272-279`, `broker.go:46-52,372`); desktop and cli never count as presence.
- `shell_capable` follows the resolvers actually built (`policy.go:86-88`).
- cli precedence and the contradiction error hold on every load path (`mcp_confirm.go:115-124`, `config.go:90-92,168`).
- ttl bounded in `ParseConfig` and `New`; expiry is denial; expiry check precedes nonce check.
- No new secrets in logs; schema errors redacted.
- Workflow: no `pull_request_target`, `contents: read`, no secrets, no `github.event.*` in `run:`, repository guard on every job.
