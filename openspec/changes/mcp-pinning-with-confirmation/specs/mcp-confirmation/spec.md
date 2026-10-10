# Spec Delta: mcp-confirmation

## Purpose

Guarantees that every write and destructive MCP tool call is confirmed before it takes effect, on every transport, and is never executed silently.

## ADDED Requirements

### Requirement: Confirmation before effect
The server SHALL NOT perform any side effect of a tool whose confirmation class is `write` or `destructive` until that call is confirmed.

#### Scenario: Unconfirmed destructive call
- **WHEN** a client without elicitation support calls `pin_rm` without a confirm token
- **THEN** the pin remains, and the result is `confirmation_required` with a token and a summary naming the CID

### Requirement: Elicitation when supported
When the client advertises elicitation support, the server SHALL request confirmation through elicitation. A decline or cancel SHALL abort the call with no side effect.

#### Scenario: User declines
- **WHEN** the elicitation response is decline
- **THEN** the tool returns a declined result, and no side effect occurs

### Requirement: Token integrity
A confirm token MUST be single-use, expire, and be bound to the session, principal, tool and canonical arguments.

#### Scenario: Replayed token
- **WHEN** a confirm token that was already used is presented again
- **THEN** the call is rejected with no side effect

#### Scenario: Changed arguments
- **WHEN** a token issued for one CID is presented with a different CID
- **THEN** the call is rejected with no side effect

### Requirement: No self-confirmation
The server SHALL NOT accept a two-step confirm token from a principal marked as unable to elicit; such calls SHALL wait for an out-of-band resolution.

#### Scenario: Agent presents its own token
- **WHEN** a principal of kind `uar` repeats a write call with the token it received
- **THEN** the call is rejected and the pending confirmation waits for the out-of-band resolver
