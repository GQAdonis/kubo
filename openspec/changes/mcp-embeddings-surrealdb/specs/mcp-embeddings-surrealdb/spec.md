# Spec Delta: mcp-embeddings-surrealdb

## Purpose

N7: G12 SurrealDB store in embedded and remote modes, and the embedder client, inside `ipfs mcp`.

## ADDED Requirements

### Requirement: Disabled by default
Without an `embeddings` section, `ipfs mcp` SHALL spawn no `surreal` process, open no store or embedder connection, and list no index tools.

#### Scenario: Default start
- **WHEN** `ipfs mcp` starts without an `embeddings` section
- **THEN** no child process exists, a network capture shows no store or embedder connection, and no `ipfs_index_*` tool is listed

### Requirement: Embedded mode is supervised and loopback-only
In embedded mode the SurrealDB process SHALL be the configured absolute binary path, SHALL bind 127.0.0.1 on an ephemeral port, and SHALL NOT outlive `ipfs mcp`.

#### Scenario: Crash cleanup
- **WHEN** `ipfs mcp` is killed with SIGKILL while embedded mode runs
- **THEN** no orphaned `surreal` process remains after the next start or the supervisor's cleanup

### Requirement: Remote mode has no default
Remote mode SHALL require an explicit URL and credentials from env or file.

#### Scenario: Missing URL
- **WHEN** remote mode is configured without a URL
- **THEN** the store is not started and the error names the field

### Requirement: Embedder egress
Content SHALL NOT be sent to an embedder host absent from `egress_ack`.

#### Scenario: Unacknowledged embedder
- **WHEN** the embedder host is not in `egress_ack`
- **THEN** indexing is refused with a warning naming the host
