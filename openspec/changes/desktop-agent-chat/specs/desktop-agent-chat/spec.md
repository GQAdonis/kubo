# Spec Delta: desktop-agent-chat

## Purpose

N6: IPFS Desktop fork: CSP, `webSecurity`, `safeStorage` token bridge and the native confirmation window.

## ADDED Requirements

### Requirement: Content Security Policy
The Desktop Web UI window SHALL be served with a CSP that forbids `unsafe-eval` and remote assets and limits `connect-src` to the RPC and `ipfs mcp` origins, with `webSecurity` enabled.

#### Scenario: Remote fetch blocked
- **WHEN** a script in the Web UI window fetches an external URL
- **THEN** the request is blocked and a CSP violation is reported

### Requirement: Token kept out of page storage
The `ipfs mcp` bearer SHALL be held by the main process through `safeStorage` only.

#### Scenario: Storage audit
- **WHEN** the chat is configured with a token
- **THEN** the token appears in no renderer localStorage, sessionStorage or IndexedDB entry

### Requirement: Paired resolution only
Only a paired Desktop instance SHALL resolve pending confirmations, and denying SHALL prevent the operation.

#### Scenario: Deny in Desktop
- **WHEN** a pending `pin_rm` is denied in the Desktop modal
- **THEN** the pin remains
