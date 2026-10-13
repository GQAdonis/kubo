# Spec Delta: mcp-agent-backend-uar

## Purpose

Phase `agent-backend-settings`, change `agent-backend-uar` (ADR-0010, decisions D13, D14): `ipfs mcp /agui` can use a Universal Agent Runtime agent as its backend. Tasks 1.2-1.6 are gated on a published UAR release (OG-4).

## ADDED Requirements

### Requirement: Published release only
A `backend.kind: uar` configuration SHALL be refused unless `uar-preflight.json` pins a published UAR release by tag and digest that meets the minimum version.

#### Scenario: Unpinned release
- **WHEN** `backend.kind` is `uar` and the preflight file has no pinned release
- **THEN** the backend is not activated and the settings read reports the reason

### Requirement: Agent provisioning
With `backend.kind: uar`, `ipfs mcp` SHALL create or update an agent on the configured UAR instance whose system prompt equals the configured prompt including the fixed safety prefix, whose MCP binding names the kubo MCP server, and whose skills are the IPFS agent skills.

#### Scenario: Provisioned agent
- **WHEN** provisioning completes against the released UAR
- **THEN** reading the agent from UAR shows that prompt, that binding and those skills

### Requirement: Island contract unchanged
Runs proxied to UAR SHALL be served on `/agui` with kubo's interrupt shape; UAR `tool_approval` and `mcp_elicitation` interrupts SHALL be translated so that no write executes without the user's approval.

#### Scenario: UAR asks for approval
- **WHEN** the UAR agent requests approval for a write
- **THEN** the island receives a kubo-shaped interrupt and the write runs only after the user approves it
