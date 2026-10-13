# Spec Delta: mcp-out-of-band-confirmation

## Purpose

N14: native confirmation prompt with a presence factor (human gate).

## ADDED Requirements

### Requirement: Expiry is denial
A pending confirmation that is not approved before its expiry SHALL be treated as denied, and the call SHALL never execute later.

#### Scenario: Expired
- **WHEN** a pending `pin_rm` expires
- **THEN** the pin remains, and a later approval attempt for that ID is rejected

### Requirement: Exactly once
An approved confirmation SHALL execute its call exactly once.

#### Scenario: Double approval
- **WHEN** two resolvers approve the same pending ID
- **THEN** the operation runs once and the second approval is rejected

### Requirement: Fail closed without a resolver
When no out-of-band resolver is available, a write or destructive call from a principal that cannot elicit SHALL be refused with a structured error.

#### Scenario: No resolver
- **WHEN** on a platform with no available resolver, a non-elicitation principal calls `pin_add`
- **THEN** the call returns `confirmation_unavailable` and nothing is pinned

### Requirement: CLI confirm is headless opt-in
`ipfs mcp confirm` SHALL resolve confirmations only when the `cli` mode is enabled and stdin is a TTY. The `cli` mode is enabled only by naming `cli` in `--confirm`, in `IPFS_MCP_CONFIRM` or in `confirmation.modes`, or by `confirmation.cli: true` when neither `--confirm` nor `IPFS_MCP_CONFIRM` is given. The `cli` resolver SHALL NOT approve a confirmation opened by the stdio session (the local principal), because a stdio host cannot assert that it is not shell-capable; when `cli` is enabled, `ipfs mcp` SHALL warn at start that stdio calls are not confirmed through it.

#### Scenario: Agent self-confirm attempt
- **WHEN** a non-TTY process runs `ipfs mcp confirm <id>` while the `cli` mode is not enabled
- **THEN** the command fails and the confirmation stays pending

#### Scenario: Shell-capable stdio host with only the cli resolver
- **WHEN** `ipfs mcp` runs over stdio with `--confirm=cli` (or `confirmation.cli: true` and no other resolver available), and a client that cannot elicit calls `pin_add`
- **THEN** the call returns `confirmation_unavailable`, nothing is pinned, no POST to the confirmation socket with the cli header can approve it, and `ipfs mcp` printed a warning at start

### Requirement: Presence factor resists same-user automation
An out-of-band approval SHALL require a presence factor (macOS LocalAuthentication or Linux polkit authentication). A click or keystroke in the paired IPFS Desktop window is not a presence factor (HG-1 evidence of 2026-10-12, ADR-0003 R14 amendment). The exception is the opt-in `desktop` resolver: it approves write-class confirmations after Touch ID in the Desktop window, which the server cannot verify, and it never approves a confirmation that requires the native prompt. Synthetic input or IPC messages from a same-user background process SHALL NOT approve a pending confirmation.

#### Scenario: Scripted approval attempt
- **WHEN** a same-user background process without a TTY sends synthetic clicks or keystrokes to the prompt, or sends approve over the confirmation channel without a valid pairing key
- **THEN** the write is not performed, the confirmation stays pending until expiry and is then denied

### Requirement: Confirmation config keys can only make the gate stricter
`confirmation.resolvers` SHALL narrow the resolvers enabled by the effective modes after mode resolution, whatever the source of the modes, and SHALL never add a resolver. `confirmation.cli: true` SHALL enable the `cli` mode only when neither `--confirm` nor `IPFS_MCP_CONFIRM` is given, subject to the `principals[].shell_capable` rule and the TTY requirement. `confirmation.ttl` SHALL set the expiry of pending confirmations, confirmation tokens, and the confirm channel and `ipfs mcp confirm`, and SHALL be at least 1 second and at most 10 minutes. With `confirmation.modes` absent, `confirmation.cli: true` SHALL add `cli` to the default modes (`native`, `desktop`). With the three keys absent, behavior SHALL be unchanged (all host-offered resolvers of the effective modes, no `cli`, 120 seconds).

#### Scenario: Resolvers narrow a flag-selected mode
- **WHEN** `ipfs mcp --confirm=native,desktop` runs with `confirmation.resolvers: ["linux-polkit"]` on Linux
- **THEN** only the polkit resolver runs and the paired Desktop channel resolves nothing

#### Scenario: Empty resolver list
- **WHEN** `confirmation.resolvers` is `[]` and a non-eliciting principal calls `pin_add`
- **THEN** the call returns `confirmation_unavailable`, nothing is pinned, and `ipfs mcp` printed a warning at start that no requested resolver is left

#### Scenario: Contradictory cli setting
- **WHEN** the config sets `confirmation.cli: false` and `confirmation.modes: ["native", "cli"]`
- **THEN** `ipfs mcp` refuses to start with a validation error naming `confirmation.cli`

#### Scenario: TTL out of bounds
- **WHEN** the config sets `confirmation.ttl` to `0s`, `500ms` or `11m`
- **THEN** `ipfs mcp` refuses to start with a validation error naming `confirmation.ttl`

#### Scenario: Shorter TTL
- **WHEN** `confirmation.ttl` is `5s` and a pending `pin_rm` is not answered for 5 seconds
- **THEN** the confirmation is denied, its token is rejected, `ipfs mcp confirm` for that ID fails, and the pin remains

### Requirement: The paired Desktop channel is opt-in, write-only and authenticated per request
The default out-of-band modes SHALL be `native` only; the `desktop` resolver SHALL run only when `desktop` is named in `--confirm`, `IPFS_MCP_CONFIRM` or `confirmation.modes`. It SHALL never satisfy `requires_native_prompt`. Every `desktop` request SHALL be authenticated with channel authentication version 2 (`mcp/server/confirm/MECHANISMS.md`): an HMAC over the socket, method, path, timestamp, nonce and body hash, a +/- 30 second window and a nonce replay cache; the server SHALL prove possession of the verifier on every response, and SHALL refuse a bearer key.

#### Scenario: Default modes
- **WHEN** `ipfs mcp` starts with no `--confirm`, no `IPFS_MCP_CONFIRM` and no `confirmation.modes`, and IPFS Desktop is paired
- **THEN** the Desktop channel resolves nothing, and a non-eliciting write is confirmed only by the native prompt or refused with `confirmation_unavailable`

#### Scenario: Harvested bearer key
- **WHEN** a same-user process sends `Authorization: Bearer <pairing key>` to the confirmation socket
- **THEN** the request is refused with `auth_version_unsupported`, and nothing is listed or resolved

#### Scenario: Replayed request
- **WHEN** a correctly signed `POST /v1/resolve` is sent a second time with the same nonce, or more than 30 seconds after its timestamp, or to another server's socket
- **THEN** it is refused with `auth_replayed`, `auth_stale` or `auth_invalid`, and the confirmation is not resolved by it

#### Scenario: Windows
- **WHEN** a client that cannot elicit calls `pin_add` on Windows
- **THEN** the call returns `confirmation_unavailable`, paired Desktop or not
