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
`ipfs mcp confirm` SHALL resolve confirmations only when `--confirm=cli` is set and stdin is a TTY.

#### Scenario: Agent self-confirm attempt
- **WHEN** a non-TTY process runs `ipfs mcp confirm <id>` without `--confirm=cli`
- **THEN** the command fails and the confirmation stays pending

### Requirement: Presence factor resists same-user automation
An out-of-band approval SHALL require a presence factor (macOS LocalAuthentication, Linux polkit authentication, or a user gesture in the paired Desktop window). Synthetic input or IPC messages from a same-user background process SHALL NOT approve a pending confirmation.

#### Scenario: Scripted approval attempt
- **WHEN** a same-user background process without a TTY sends synthetic clicks or keystrokes to the prompt, or sends approve over the confirmation channel without a valid pairing key
- **THEN** the write is not performed, the confirmation stays pending until expiry and is then denied
