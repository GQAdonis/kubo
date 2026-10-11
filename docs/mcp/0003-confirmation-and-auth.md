# ADR-0003: Confirmation of writes and destructive calls, and a replaceable authentication verifier

## Status

Accepted, 2026-10-09, per the operator's binding answers in `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`:

- answer 4: static bearer token for Streamable HTTP now, HTTP can do everything stdio can, and authentication designed to move to Ory Kratos (identity) and Ory Hydra (OAuth2) later without reworking tool handlers;
- answer 5: writes and destructive tools over every transport, each requiring confirmation;
- "Answers to the G9-G12 addendum", human gate (Q4): a native OS prompt for agents that cannot elicit, CLI confirm only for headless servers behind an explicit opt-in;
- Round 0 answer R3: fail closed where no presence-factor prompt exists.

Recorded in `versions.toml` as `mcp-http-auth`, `mcp-http-scope` and `mcp-confirmation`.

## Date

2026-10-09

## Context

- `AGENTS.md` (User Agency): the operator decides what the node stores, serves and spends; nothing may happen silently; every feature has an off switch.
- `PRODUCT.md`: "Deny is final." Every write and destructive action passes a confirmation gate before it runs; a denied or unconfirmed action does not happen.
- MCP tool annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) are advisory to the host. Hosts may or may not prompt on them, so they cannot be the gate.
- go-sdk v1.8.0 (read from `$GOMODCACHE/github.com/modelcontextprotocol/go-sdk@v1.8.0` on 2026-10-09):
  - `ServerSession.Elicit` (`mcp/server.go:1729`) lets the server ask the client's human for input, when the client advertises the elicitation capability at initialize. It refuses on protocol 2026-07-28, so it is not used (see [Amendment (2026-10-09)](#amendment-2026-10-09), item 1).
  - `auth.TokenVerifier` is `func(ctx context.Context, token string, req *http.Request) (*TokenInfo, error)` (`auth/auth.go:41`). `auth.TokenInfo` carries `Scopes`, `Expiration`, `UserID` and `Extra` (`auth/auth.go:21-30`). `UserID`, when set, lets the transport bind a session to one user and refuse session hijacking.
  - `auth.RequireBearerToken(verifier, opts)` (`auth/auth.go:97`) returns 401 with a `WWW-Authenticate` header carrying `resource_metadata` and required scopes. `RequireBearerTokenOptions` has `ResourceMetadataURL`, `Scopes`, `AllowMissingExpiration` and `ClockSkew`. By default a `TokenInfo` with a zero `Expiration` is rejected.
  - `auth.ProtectedResourceMetadataHandler` (`auth/auth.go:188`) serves RFC 9728 OAuth 2.0 Protected Resource Metadata.
- Kubo RPC: `API.Authorizations` protects only the cmds handler (`core/corehttp/commands.go:144-147`), and `AllowedPaths` uses `strings.HasPrefix` (`core/corehttp/commands.go:186-191`), so it is not segment-aware.
- The adversarial review of the G9-G12 addendum (round 2, WARNING, upheld) showed that a 0600 control socket or a loopback confirmation page is reachable by every same-user process, including an agent runtime with a shell tool (`assessment-addendum.md`, "Unresolved review findings" and "Orchestrator adjudication" item 2).

## Decision

### 1. Confirmation classes are server-enforced

Each tool in `mcp/contract/catalog.json` has a confirmation class: `none`, `write` or `destructive`. The class is enforced by `ipfs mcp`; the MCP annotations are derived from it and stay advisory. The contract validator rejects a `destructive` tool with `readOnlyHint: true`, and a `none` tool without `readOnlyHint: true` (`mcp-tool-contract` spec).

- `none` tools never prompt.
- No side effect of a `write` or `destructive` tool happens until that exact call is confirmed.
- A decline, a cancel, a denial and an expiry each end the call with no side effect, and the result says so ("Denied. Unpinning requires confirmation."). Deny is final: the agent must start a new call, which needs a new confirmation.

### 2. Destructive tools are opt-in

The destructive tools are `pin_rm`, `mfs_rm`, `key_rm` and `repo_gc` (and any later tool whose contract class is `destructive`). (Superseded in part: `pin_update` is also destructive, and the class is defined by effect, including option defaults; see [Amendment (2026-10-09)](#amendment-2026-10-09), item 3.)

- **Default.** On every transport, the default tool set excludes the `destructive` class. A write-capable session sees writes, not destructive tools.
- **stdio.** The operator enables them by naming the destructive group in `--tools` (or `tools.groups` in the `ipfs mcp` config file named by `--mcp-config`).
- **Streamable HTTP, AG-UI and A2A.** Each configured principal is read-only by default. Write and destructive groups are granted to a principal by name (ADR-0004, principals).
- **Once enabled,** every destructive call still requires confirmation. For a principal that confirms through an AG-UI chat interrupt, destructive calls also raise the native prompt (ADR-0004).
- **WebMCP** exposes no destructive tools (`assessment.md`, G8 decision).

This keeps operator answer 5 (destructive tools are available on every transport) while giving them the off-by-default switch `AGENTS.md` asks for. It supersedes the assessment draft's wording only in naming the mechanism; the draft already had destructive tools off unless explicitly enabled.

### 3. How a call is confirmed

The gate picks the first mechanism that applies to the calling session.

1. **MCP elicitation.** If the client advertised elicitation at initialize, ~~the server calls `ServerSession.Elicit` with~~ the server asks the client's human with a summary naming the tool, the CIDs, paths or names, and the effect. Accept runs the call once. Decline or cancel aborts it. (Superseded in part: the call to `ServerSession.Elicit` is replaced by a multi round trip; see [Amendment (2026-10-09)](#amendment-2026-10-09), item 1.)
2. **External resolution with a human-presence factor.** For every session or principal that cannot elicit, the call becomes a pending confirmation in an in-process broker with an ID, a summary and an expiry. It is resolved only by a surface a same-user background process cannot drive: the paired IPFS Desktop confirmation window, a macOS LocalAuthentication prompt (Touch ID or password), or a Linux polkit authentication-agent prompt. An AG-UI interrupt answered by a user gesture in the chat island resolves calls for the `ui` principal. The resolvers, their platform limits and the headless `--confirm=cli` opt-in are decided in ADR-0004 and built in `native-confirmation-prompt`.
3. **No resolver available.** Where neither elicitation nor a presence-factor resolver exists (Windows without paired Desktop, Linux without the operator-installed polkit action), writes and destructive calls are refused with `confirmation_unavailable`. They never fall through to silent execution (Round 0 answer R3).

### 4. The two-step confirm token: binding handle, not consent

The token from `mcp-pinning-with-confirmation` stays, with a narrower role than the one it was designed for.

- **What it is.** HMAC-SHA256 with a per-process random key over the session ID, the principal, the tool name, the canonical JSON of the arguments without the token, and the expiry. Single-use (in-memory spent set), 120-second TTL. A process restart invalidates every outstanding token. (Superseded in part: the signed claims also carry a `use` claim, and the token is verified for that use only; see [Amendment (2026-10-09)](#amendment-2026-10-09), item 2.)
- **Flow.** A first call to a `write` or `destructive` tool from a non-eliciting session has no side effect. It returns a structured `confirmation_required` result with the pending ID, the token and a summary naming the operation and its CIDs or paths. The agent repeats the call with the token.
- **What it proves.** The token proves that the same session asked twice for exactly the same arguments within the TTL. It does not prove that a human agreed. This is limitation T2 (`plan.md` revision 1 trade-off T2; `mcp-pinning-with-confirmation` design).
- **How T2 is closed.** A token is accepted only when its pending confirmation has been approved by mechanism 1 or 2 above. A token presented while the confirmation is pending, denied or expired is rejected with no side effect. The server never accepts a token as consent by itself from any principal that cannot elicit, and an externally approved call runs exactly once (`mcp-pinning-with-confirmation` task 1.5; `plan.md` D3, "T2 superseded").
- **What the token still guarantees.** The executed call is the confirmed call: replay, changed arguments, another session and another principal are all rejected.

### 5. Authentication: one `AuthVerifier`, mapped onto go-sdk `auth.TokenVerifier`

HTTP authentication is an interface so that the verifier can change without touching tool handlers.

```go
// Package mcp/server/auth (shape, not final code).
type Principal struct {
	ID     string   // stable identifier; becomes auth.TokenInfo.UserID
	Kind   string   // "human-mcp", "ui", "a2a" or "uar" (ADR-0004)
	Scopes []string // e.g. "ipfs:read", "ipfs:write", "ipfs:admin"
}

type AuthVerifier interface {
	Verify(ctx context.Context, token string, req *http.Request) (Principal, time.Time, error)
	// ProtectedResourceMetadata returns RFC 9728 metadata, or nil if the
	// verifier has none (the static verifier has none).
	ProtectedResourceMetadata() *oauthex.ProtectedResourceMetadata
}
```

The mapping onto go-sdk is one adapter:

- `AuthVerifier.Verify` is wrapped as an `auth.TokenVerifier`. It returns `&auth.TokenInfo{Scopes: p.Scopes, Expiration: exp, UserID: p.ID, Extra: {"principal": p}}`. A verification failure unwraps to `auth.ErrInvalidToken`.
- The wrapped verifier is installed with `auth.RequireBearerToken` in front of every route on the `--http` listener: `/mcp`, `/agui`, `/a2a`. The A2A agent card at `/.well-known/agent-card.json` and the RFC 9728 metadata document are the only unauthenticated paths, and both carry public metadata only.
- Tool handlers read the principal from `auth.TokenInfoFromContext`. They never see a token, so a verifier change does not touch them.
- `UserID` binds each Streamable HTTP session to one principal, which go-sdk uses to refuse session hijacking.
- The static verifier has no natural expiry. It sets `Expiration` to the current time plus a short window on each verification, rather than turning on `AllowMissingExpiration` for every verifier.

**Scopes and authorization.**

- Scopes map to tool groups and confirmation classes: `ipfs:read` for class `none`, `ipfs:write` for `write`, `ipfs:admin` for `destructive`. The principal's configured groups narrow this further.
- Authorization is path-scoped: a principal authorized for one route is not authorized on another route for tools outside its groups (falsifier G9-4 in ADR-0004).
- A token that maps to no configured principal gets nothing.

**The static bearer verifier (this phase).**

- The token is read from a file or an environment variable named by a flag or the `ipfs mcp` config file (`--mcp-config`). A literal token on the command line or in the config file fails validation.
- The comparison is constant-time.
- A non-loopback `--http` bind without a configured verifier makes `ipfs mcp` exit with an error.
- TLS termination is left to a reverse proxy and documented. Remote exposure stays the operator's explicit choice.

**The path to Ory Hydra (later, no handler change).**

- A second `AuthVerifier` implementation acts as an OAuth 2.0 resource server: it validates Hydra-issued access tokens by RFC 7662 token introspection against an operator-configured Hydra admin URL (no default), or by JWT validation against Hydra's JWKS, and maps the token's scopes and subject to a `Principal`.
- Kratos is the identity provider behind Hydra's login and consent flow; `ipfs mcp` never talks to Kratos directly.
- That verifier returns RFC 9728 Protected Resource Metadata (resource identifier, `authorization_servers` naming the operator's Hydra issuer, `scopes_supported`). `ipfs mcp` then serves it through `auth.ProtectedResourceMetadataHandler` at `/.well-known/oauth-protected-resource` and sets `RequireBearerTokenOptions.ResourceMetadataURL`, so MCP clients discover the authorization server from the 401 response.
- The RFC 9728 endpoint is mounted only when the verifier supplies metadata. The static verifier supplies none, so nothing is served in this phase.
- Like every endpoint, the Hydra and JWKS URLs are configurable with no default, and the verifier is off unless configured.

### 6. Authentication toward Kubo

`ipfs mcp` authenticates to the daemon as any RPC client does, with `--api-auth`. Docs and tests cover a dedicated `API.Authorizations` secret whose `AllowedPaths` list only the RPC commands of the enabled tool groups. `ipfs mcp --print-rpc-allowed-paths` prints that list (`mcp-http-transport-auth`). Because `AllowedPaths` is prefix-matched, the helper prints full command paths and the docs warn that a shorter prefix grants more than it appears to.

## Consequences

### Positive

- No write or destructive call runs without a human decision on every transport where a human-presence mechanism exists, and none runs where it does not.
- Destructive tools have an off-by-default switch on every transport.
- The token can no longer be used by an agent to confirm itself, and it still prevents replay and argument substitution.
- Moving from a static token to Hydra is a new verifier and configuration, not a handler change, and it brings standard OAuth discovery through RFC 9728.

### Negative

- Hosts without elicitation get an extra step per write: a native prompt outside the agent host. This invites approval fatigue.
- Users on Windows without a paired IPFS Desktop, and on Linux without the polkit action, cannot perform writes from non-eliciting clients at all.
- The registered `mcp-pinning-with-confirmation` tasks 1.1-1.4 were written when the token was the fallback gate. Under this ADR its real-daemon test of "a confirmed `pin_rm` removes the pin" needs an approved external resolution (or elicitation), not only a repeated call with the token.
- The static bearer token has no rotation or per-request expiry of its own; rotation means restarting `ipfs mcp` with a new token file.
- Scopes are coarse (three) until an OAuth verifier allows finer ones.

## Rejected alternatives

| Alternative | Rejected because (evidence) |
|---|---|
| The two-step token as the human gate for non-eliciting clients | It proves only that the agent asked twice (T2). Any agent can repeat its own call. Superseded in `plan.md` revision 2 (T2) and D3. |
| A 0600 local socket or a loopback confirmation web page as the gate | Reachable by every same-user process, including an agent runtime with a shell tool such as UAR's `terminal_exec` (`uar:src/uar/tools/terminal_exec.rs:22-27`); addendum review WARNING, upheld. Cut in `plan.md` section 3. |
| `ipfs mcp confirm <id>` as the default gate | Same-user reachable. Kept only behind `--confirm=cli` for headless servers, with a TTY check and a printed warning, and only for principals the operator asserts are not shell-capable (ADR-0004). |
| Relying on host approval prompts driven by `destructiveHint` | Annotations are advisory; hosts differ and some auto-approve. The server must enforce the gate. |
| No confirmation over stdio ("the user is at the keyboard") | Contradicts operator answer 5 and `PRODUCT.md`. A stdio host can be driven by prompt-injected content. |
| Destructive tools on by default, gated only by confirmation | Leaves no off switch by default; one approval mistake can unpin or GC data. Opt-in costs one flag. |
| OAuth 2.0 or Ory now | Operator answer 4: static token now, Ory later. The interface keeps that path open. |
| Mounting MCP on the RPC port to reuse `API.Authorizations` | Bypasses `API.Authorizations`, which wraps only the cmds handler (`core/corehttp/commands.go:144-147`) (ADR-0001, C2). |
| MCP sampling to ask the host's model to confirm | A model is not a human. |
| mTLS client certificates | Not requested; poor fit for agent hosts and browser surfaces this phase. Possible later behind the same interface. |

## Where this is weak

- **Elicitation support across hosts is unverified.** If the main hosts (Claude Code, Codex, OpenCode) do not elicit, most writes go through native prompts, and the experience depends on platform mechanisms that are not built yet.
- **The macOS prompt without cgo is unverified.** Kubo has no cgo. If LocalAuthentication cannot be reached without it, macOS uses the Desktop window only or fails closed (`native-confirmation-prompt` design, risk 1).
- **Presence factors have stated limits.** A process with macOS Accessibility permission can click a dialog but cannot supply Touch ID or the password; Wayland synthetic-input testing is unavailable. These limits are tested by HG-1 and documented, not eliminated.
- **The broker is in memory.** A restart of `ipfs mcp` drops pending confirmations; they count as denied.
- **Canonical JSON binding** depends on one canonicalization used everywhere. A mismatch either rejects valid confirmations or, worse, accepts a token for semantically different arguments. The replay, tampering and argument-change unit tests must cover number and key-order edge cases.
- **`AllowedPaths` prefix matching** in Kubo itself is not segment-aware. `ipfs mcp` can only print correct lists; it cannot fix Kubo's matcher.

## Falsifiers

| Assumption | Observable falsifier | Consequence |
|---|---|---|
| No side effect before confirmation | A `test/cli` run shows a pin removed, a file written or a key deleted before approval | Block |
| A token alone never confirms a non-eliciting call | A unit test repeats a write with its own token while the confirmation is pending, and it executes | Block |
| Same-user processes cannot approve | HG-1 (ADR-0004): an automated or synthetic-input attempt approves a pending write | Block that mechanism; fail closed on that platform |
| Tokens are bound | A token issued for one CID, session or principal is accepted with another | Block |
| Handlers are independent of the verifier | Adding a second `AuthVerifier` requires a change in any tool handler | Fix the boundary before the Hydra verifier is written |
| Destructive tools are opt-in | `tools/list` from a default stdio session or a default HTTP principal lists a `destructive` tool | Block |

## Amendment (2026-10-09)

This amendment records decisions made during implementation of `mcp-pinning-with-confirmation` on 2026-10-09. The text above is kept as accepted; sentences it replaces are marked "Superseded in part" inline with a pointer here. Where the two disagree, this amendment governs.

### 1. Mechanism 1 is a multi round trip, not `ServerSession.Elicit`

- **Finding.** go-sdk v1.8.0 refuses `ServerSession.Elicit` on protocol 2026-07-28, so the server cannot open a server-to-client elicitation request in the middle of a `tools/call` on that protocol.
- **Decision.** Mechanism 1 uses go-sdk v1.8.0 multi round trip input requests (SEP-2322). The first `tools/call` has no side effect and returns an input-required result carrying the input requests (the summary naming the tool, the CIDs, paths or names, and the effect) plus a server-signed `requestState`. The client asks its human and retries the same call with the answer in `inputResponses` and the `requestState` echoed back. Accept runs the call once; decline or cancel ends it with no side effect, and deny stays final.
- **Contract.** `InputRequiredResult`, `InputRetry`, `RequestStateClaims` and `RequestStateAcceptance` in `mcp/contract/confirmation/confirmation.schema.json`.
- **Coverage so far.** Only the 2026-07-28 path is exercised. Clients that negotiate an older protocol version, where `ServerSession.Elicit` is still available, are untested; how they are confirmed (elicitation through `ServerSession.Elicit`, or mechanism 2 or 3) is a review item at the `ipfs-mcp-server` phase boundary. Until that review, no claim is made that older-protocol clients are confirmed by mechanism 1.

### 2. The request state and the confirm token are distinct handles

- **Finding (2026-10-09).** A self-approval bypass: the request state and the confirm token were signed with the same per-process key and had no domain separation, so a confirm token could be replayed as a `requestState` together with a forged accept in `inputResponses`, and the call was approved without a human. This defeats the rule in section 4 that the token is never consent.
- **Decision.** Both handles carry a signed `use` claim, `request_state` or `confirm_token`, and each is verified only for its own use. A handle presented for the other use is rejected with no side effect.
- **Acceptance of a request state** (`RequestStateAcceptance`). A `requestState` is accepted only when all of these hold: the HMAC verifies and `use` is `request_state`; session, principal, tool and canonical arguments match; the session advertised elicitation at initialize; the confirmation exists and was opened by mechanism 1 (elicitation); the request state has not expired; and the confirmation is still pending. Only then does the user's answer count. A request state never carries an approval; it only lets the answer resolve the confirmation it names.
- **Acceptance of a confirm token** is unchanged (`TokenAcceptance`): the confirmation must already be approved by mechanism 1 or 2.
- **Falsifier.** A unit test presents a confirm token as a `requestState` with a forged accept, or a request state from a session that did not advertise elicitation, or for a confirmation opened by mechanism 2, or after the confirmation left `pending`; if any of these executes the call, block.

### 3. `pin_update` is destructive, and the class follows the effect

- **Decision.** `pin_update` is class `destructive`: its default `unpin: true` removes the old pin, which is the effect of `pin_rm`. It is off by default and every call is confirmed, as section 2 requires.
- **General rule.** Any tool that can remove a pin, an MFS entry, a key or repo blocks is `destructive`, including when the removal happens only through an option's default value. The class is decided by the worst effect the tool can have with any accepted arguments, not by its name or its primary purpose. Passing `unpin: false` does not downgrade a `pin_update` call to `write`; the tool's class is fixed in the catalog.

### 4. `ConfirmationRequired.requires_native_prompt`

- **Decision.** When `requires_native_prompt` is true in `ConfirmationRequired` (and `PendingConfirmation`), no in-chat approval exists for that call. The system prompt (paired IPFS Desktop window, macOS LocalAuthentication, Linux polkit, or the headless `--confirm=cli` opt-in; Superseded in part, see Round 4 amendment section 1: `cli` no longer qualifies) is the only resolver that can approve it, and `agui-interrupt` is not a resolver: an in-chat surface must not offer an approve control and may only offer deny. Absent means false.
- **Scope.** This makes section 2's "destructive calls also raise the native prompt" for the `ui` principal machine-checkable: a destructive call from the `ui` principal sets it to true. (Extended to the `a2a` principal by the [Round 7 amendment](#amendment-round-7-2026-10-09-destructive-calls-from-a2a-peers-need-a-presence-resolver).)

### Consequences of the amendment

- The self-approval path through the request state is closed, and section 4's guarantee (a token is never consent) holds across both handles.
- Mechanism 1 works on protocol 2026-07-28 without `ServerSession.Elicit`, at the cost of an untested older-protocol path until the phase-boundary review.
- One more tool, `pin_update`, is off by default; moving a pin needs destructive tools enabled. Without them, an agent can still add the new pin with `pin_add`; removing the old one stays a destructive call.

## Amendment (Round 4, 2026-10-09)

Decisions from the Round 4 review of the `ipfs-mcp-server` phase. Where this disagrees with the text above, this amendment governs. The contract encodes each one (`mcp/contract/`, recorded in `openspec/changes/mcp-tool-contract/design.md`, note of 2026-10-09).

### 1. `requires_native_prompt` is satisfied only by presence resolvers

- **Decision.** A confirmation with `requires_native_prompt: true` can be approved only by `macos-localauth`, `linux-polkit` or `desktop` (`IsPresenceResolver`). `cli` (`ipfs mcp confirm`) has no presence factor beyond the terminal and does not qualify; `agui-interrupt` never did. With no presence resolver available, the call is refused with `confirmation_unavailable`.
- **Falsifier.** A destructive call from the `ui` principal is approved through `ipfs mcp confirm` or an AG-UI interrupt: block.

### 2. The human gate is on by default and has one off switch

- **Decision.** The out-of-band gate for clients that cannot elicit runs by default with `native,desktop`. It is selected by `--confirm`, then `IPFS_MCP_CONFIRM`, then `confirmation.modes` in the config file, then the default. `cli` is enabled only by naming it. `IPFS_MCP_CONFIRM=none` (or `--confirm=none`, or `confirmation.modes: []`) turns the gate off.
- **What "off" means.** Per-call confirmation of write and destructive tools is never turned off. With the gate off, elicitation still confirms calls from clients that support it; every other write or destructive call is refused with `confirmation_unavailable`. Turning the gate off can only make the server refuse more.

### 3. The anonymous loopback principal is read-only

- **Decision.** An HTTP listener on loopback with no credential configured serves every request as `anonymous-loopback`: it reads every group and has no write or destructive grant. Write and destructive tools over HTTP need a configured principal. A non-loopback listener without a verifier is refused (unchanged).

## Amendment (Round 7, 2026-10-09): destructive calls from A2A peers need a presence resolver

Raised by `mcp-server-engineer` while implementing `mcp-a2a-endpoint`; accepted by the architect. Where this disagrees with the text above, this amendment governs.

- **Decision.** `requires_native_prompt` is true for a destructive call from a principal of kind `a2a`, as it already was for kind `ui`. Such a call is approved only by a presence resolver (`desktop`, `macos-localauth`, `linux-polkit`). `cli` alone does not qualify: with only `--confirm=cli` available, a destructive call from an A2A peer is refused with `confirmation_unavailable` and nothing runs (`gate.openExternal`, `mcp/server/gate.go`). Write-class calls from A2A peers are unchanged: they wait for any out-of-band resolver that fits the principal, including the `cli` opt-in.
- **Why.** ADR-0004 says an A2A caller is an agent, so its writes are resolved by the native prompt. Section 4's original scope ("for the `ui` principal") predates the A2A endpoint. The `cli` resolver has no presence factor beyond a terminal, and the remote peer is exactly the party that may share a host with an automated terminal; for the class that removes data, the presence factor is the gate.
- **Contract.** `ConfirmationRequired.requires_native_prompt` and `PendingConfirmation.requires_native_prompt` in `mcp/contract/confirmation/confirmation.schema.json` now say "ui and a2a"; the stale "or cli" in the `ConfirmationRequired` description is removed (Round 4 section 1 had already superseded it). The A2A task metadata `ipfs-mcp` carries `requires_native_prompt: true` for such a call.
- **Cost.** A headless server that confirms only through `--confirm=cli` cannot accept destructive work from A2A peers at all. That is intended; the operator can still run the destructive tool over a human MCP session.
- **Falsifier.** A destructive call from an `a2a` principal is approved through `ipfs mcp confirm`, an A2A message, or an AG-UI interrupt: block.
- **Not decided here.** Kind `uar` (topology B, deferred by ADR-0008) is not covered by this amendment; it is decided when topology B is resumed.

## Amendment (R11, 2026-10-11): presence, elicitation scope, pairing, helper trust, prompt caps, resource reads

Recorded from the R11 phase-boundary review (`.agent-team/kubo-ipfs-agentic/findings/security/r11-findings.md` H1-H4, M1, M3; `.agent-team/kubo-ipfs-agentic/findings/review/r11-findings.md`) and the batched fix pass; accepted by the architect. Where this disagrees with the text above, including the Round 4 and Round 7 amendments, this amendment governs.

### 1. `desktop` is not a presence resolver yet

- **Decision.** Only `macos-localauth` and `linux-polkit` satisfy `requires_native_prompt` (`server.IsPresenceResolver`, `mcp/server/broker.go`). `desktop` is removed from the presence list in Round 4 section 1 and in the Round 7 amendment. It counts again only once an IPFS Desktop confirmation window exists that consumes the pairing key and adds its own presence factor.
- **Why.** No Desktop window consumes the key today. The key alone would be the whole factor, and any same-user process that ran `ipfs mcp pair` holds it.
- **Unchanged.** A paired `desktop` channel still resolves write-class confirmations that do not require a native prompt.
- **Pins.** `TestIsPresenceResolver`, `TestDesktopIsNotAPresenceResolver`, `TestUIDestructiveNeedsAPresenceResolverNotCLI`, `TestA2ADestructiveNeedsThePresenceResolver`.
- **Falsifier.** A destructive call from a `ui` or `a2a` principal is approved over the paired Desktop channel: block.

### 2. Elicitation is only for `human-mcp` principals and stdio

- **Decision.** The gate confirms through elicitation (mechanism 1) only when the principal's kind is `human-mcp` and the session advertised elicitation (`mayElicit`, `mcp/server/gate.go`). The stdio principal is `human-mcp`. `elicitAnswer` checks the kind again on the retry.
- **Why.** A `ui`, `a2a` or `uar` principal can declare elicitation and answer its own prompt. Such principals go to an out-of-band resolver, and a destructive `ui` or `a2a` call goes to the native prompt.
- **Pins.** `TestElicitationIsOnlyForHumanMCPPrincipals`, `TestHumanMCPPrincipalStillConfirmsThroughElicitation`, `TestUIDestructiveWithoutNativePromptIsRefusedEvenWithElicitation`, `TestElicitAnswerRechecksThePrincipalKind`.
- **Falsifier.** A principal of any kind other than `human-mcp` approves a call through `inputResponses`: block.

### 3. `ipfs mcp pair`

- **Decision.** `ipfs mcp pair` requires a terminal on stdin. It refuses while any server accepts connections on a confirmation socket in the socket directory; a stale socket file that nothing listens on does not block it.
- **Replacement.** A new pairing replaces the old one, and the old key stops working at once on running servers too. The channel reads the stored verifier again for every request; it is not cached at start.
- **Pins.** `TestRunPairNeedsATerminal`, `TestRunPairRefusesWhileAServerHoldsTheSocket`, `TestRunPairIgnoresAStaleSocket`, `TestReplacedPairingRevokesTheOldKeyOnARunningChannel`.
- **Limit.** A TTY is not a presence factor. This narrows the self-pairing path for automated callers; it does not make `desktop` a presence resolver (item 1).

### 4. Presence helpers run from trusted paths with a minimal environment

- **Polkit.** `pkcheck` and `pkaction` run only from `/usr/bin/pkcheck` and `/usr/bin/pkaction`, never from `$PATH`. Each binary and its directory must be root-owned and not writable by group or others. This is checked at probe time and again before every `pkcheck` call; otherwise the resolver is unavailable.
- **Environment.** Every helper (`/usr/bin/swift` on macOS, `pkcheck`, `pkaction`) is started with an environment built from an allowlist (`mcp/server/confirm/exec_env.go`). `PATH` is fixed to `/usr/bin:/bin:/usr/sbin:/sbin`. Only `HOME`, `TMPDIR`, `USER`, `LOGNAME`, `LANG`, `LC_ALL`, `LC_CTYPE` and `__CF_USER_TEXT_ENCODING` are kept, and `DEVELOPER_DIR`, `TOOLCHAINS`, `SDKROOT`, `DYLD_*` and `LD_*` are always stripped.
- **macOS probe.** The macOS availability probe is bounded at 30 seconds, independent of the request's context.
- **Why.** An MCP host's env block is written by whoever wrote the MCP config, not necessarily the person at the keyboard. Before this change, a fake `pkcheck` earlier on `$PATH`, or a `DEVELOPER_DIR` pointing at a fake toolchain, could approve destructive calls.
- **Pins.** `TestPolkitUsesAbsoluteSystemPathsEvenWithAPoisonedPATH`, `TestPolkitRefusesAnUntrustedHelper`, `TestTrustedBinaryRequiresARootOwnedFile`, `TestTrustedBinaryRefusesAWritableHelper`, `TestSafeEnvKeepsOnlyAllowlistedVariables`, `TestMacProbeIsBoundedAndIndependentOfTheRequest`.

### 5. At most two open out-of-band confirmations

- **Decision.** At most 2 out-of-band confirmations may be pending at once per principal, and 2 per session (`maxOpenExternal`, `mcp/server/broker.go`). A call beyond the cap does not run. It gets `confirmation_pending` (retryable) with `details.reason: "too_many_open_confirmations"`, and no prompt is raised.
- **Not covered.** Elicitation confirmations are not counted.
- **Why.** Each confirmation can raise a system prompt. Without a cap, a prompt-injected loop could flood the person with prompts until one is approved by fatigue.
- **Pins.** `TestBrokerCapsOpenExternalConfirmationsPerPrincipalAndSession`, `TestBrokerCapDoesNotApplyToElicitationOrUnlimitedCreate`, `TestGateRefusesExtraOpenConfirmations`.

### 6. `resources/read` on `ipfs://` and `ipns://` follows the `ipfs_cat` policy

- **Decision.** A content resource returns the same bytes as `ipfs_cat`, so it follows the same policy. An authenticated principal needs a read grant on the `content` group, and the `contentEgress` check (`egress_ack`) applies as it does to `ipfs_cat` (`resourceAccess`, `mcp/server/tools_content.go`).
- **Why.** Before this change, a principal with only `groups: ["node"]` and no `egress_ack` could read up to 16 MiB of content through `resources/read`.
- **Pins.** `TestContentResourcesFollowTheIPFSCatPolicy`, `TestContentResourceIsReadableWithTheGrantAndAck`.

### Consequences of the R11 amendment

- On Windows, and on any host without the macOS or polkit factor, destructive calls from `ui` and `a2a` principals are refused with `confirmation_unavailable` until the Desktop window ships, even when Desktop is paired. Destructive tools stay usable from a `human-mcp` session that elicits.
- The `requires_native_prompt` descriptions in `mcp/contract/confirmation/confirmation.schema.json` still list `desktop`, and the generated `mcp/ui/catalog/ipfs-mcp.catalog.json` copies that text. Both need a coordinated regeneration with the `ui-ux-engineer` role. Until then, this amendment and `IsPresenceResolver` govern.

## References

- `.kbd-orchestrator/phases/ipfs-mcp-server/operator-answers.md`: answers 4 and 5, consequences, G9-G12 answers (Q4), Round 0 R3
- `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`: D3, D4, section 3 (T2), section 4a
- `openspec/changes/mcp-pinning-with-confirmation/`, `openspec/changes/mcp-http-transport-auth/`, `openspec/changes/native-confirmation-prompt/`
- RFC 9728, OAuth 2.0 Protected Resource Metadata; RFC 7662, OAuth 2.0 Token Introspection
- ADR-0001 (placement), ADR-0004 (principals, human gate, agentic endpoints)
