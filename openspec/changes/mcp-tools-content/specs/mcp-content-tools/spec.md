# Spec Delta: mcp-content-tools

## Purpose

Lets agents add and read IPFS content through MCP without changing Kubo's default CID recipe or reading the server host's filesystem.

## ADDED Requirements

### Requirement: Default CID recipe preserved
`ipfs_add` SHALL produce the same CID as `ipfs add` with default options for the same bytes, unless the caller passes explicit recipe options.

#### Scenario: Same bytes, same CID
- **WHEN** the same bytes are added with `ipfs_add` defaults and with `ipfs add`
- **THEN** both CIDs are equal

### Requirement: No host filesystem input
`ipfs_add` SHALL accept content only as bytes or text in the request. It MUST NOT read paths on the server host.

#### Scenario: Path-like input
- **WHEN** the input text is `/etc/passwd`
- **THEN** the literal string is added as content, and no file is read

### Requirement: Bounded reads
`ipfs_cat` SHALL enforce a maximum returned size. It SHALL support offset and length.

#### Scenario: Large file
- **WHEN** content larger than the cap is requested without a length
- **THEN** the result is truncated at the cap and marked truncated
