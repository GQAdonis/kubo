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
An out-of-band approval SHALL require a presence factor (macOS LocalAuthentication, Linux polkit authentication, or a user gesture in the paired Desktop window). Synthetic input or IPC messages from a same-user background process SHALL NOT approve a pending confirmation.

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
