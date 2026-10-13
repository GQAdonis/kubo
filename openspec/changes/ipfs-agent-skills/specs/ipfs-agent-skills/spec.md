# Spec Delta: ipfs-agent-skills

## Purpose

Packages IPFS task knowledge as agentskills.io-compliant skills aligned with the MCP tool contract.

## ADDED Requirements

### Requirement: Skill inventory
The skill set SHALL consist of these eight skills, one directory each under `mcp/skills/`, whose `name` matches the directory:

- `ipfs-publish-content`: adding and publishing content;
- `ipfs-retrieve-verify`: retrieving content and verifying it against its CID;
- `ipfs-pin-management`: pinning;
- `ipfs-ipns-names`: IPNS names and keys;
- `ipfs-mfs-workspace`: MFS;
- `ipfs-dag-car`: DAG and CAR;
- `ipfs-content-search`: embeddings-backed content search and indexing;
- `ipfs-troubleshooting`: troubleshooting.

#### Scenario: Skill inventory
- **WHEN** `mcp/skills` is listed
- **THEN** exactly these eight skill directories exist, each with a `SKILL.md` whose `name` equals the directory name

### Requirement: Contract alignment and validity
Every tool name a skill mentions MUST exist in `mcp/contract/catalog.json`. Every skill MUST pass `skills-ref validate`.

#### Scenario: Unknown tool name
- **WHEN** a skill references a tool absent from the catalog
- **THEN** the skill check fails
