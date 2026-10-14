# Spec Delta: mcp-whole-node-indexing

## Purpose

Phase `agent-backend-settings`, change `mcp-whole-node-indexing` (ADR-0011 V4, V5): operator-started bulk indexing of the files the node pins or keeps in MFS, and HTTP routes that let the Web UI search the index.

## ADDED Requirements

### Requirement: Bulk runs are operator-started and confirmed
A bulk run SHALL start only through `ipfs_index_bulk_start` with a confirmation on every transport, and SHALL never start or resume on its own.

#### Scenario: Restart during a run
- **WHEN** `ipfs mcp` stops while a run is `running` and starts again
- **THEN** the run is `paused` and indexes nothing until a principal resumes it with a new confirmation

### Requirement: Bulk runs read only local blocks and change nothing
A bulk run SHALL read with `offline: true`, SHALL skip files whose blocks are missing with `not_local`, and SHALL NOT add, pin, write to MFS or announce any CID.

#### Scenario: Run over pins and MFS
- **WHEN** a bulk run completes over a node with pins and MFS files
- **THEN** the pin set, the local block set and the MFS listing are identical before and after the run

#### Scenario: Pin with missing blocks
- **WHEN** a pinned file's blocks are not in the local blockstore
- **THEN** the file is skipped with `not_local` and nothing is fetched from the network

### Requirement: Bulk runs are capped
A run SHALL stop at `max_files_per_run` or `max_bytes_per_run`, SHALL index at most `files_per_minute` files per minute, and only one run SHALL be active at a time.

#### Scenario: Second start
- **WHEN** a run is active and another start is requested
- **THEN** the request is refused and the active run continues

### Requirement: Bulk runs need acknowledged egress
A run SHALL start only when the starting principal's `egress_ack` covers every host the configured stages may contact; the operator identity SHALL be refused.

#### Scenario: Stdio start
- **WHEN** a bulk start is requested over stdio
- **THEN** it is refused with `egress_not_acknowledged` and no request is made

### Requirement: Principal-scoped search routes
`POST /index/search` and `GET /index/status` SHALL be mounted only with `--http`, an `embeddings` section and `search_route.enabled` not `false`, SHALL accept only `ui` and `human-mcp` principals holding the `index` group, and SHALL apply the same egress check as `ipfs_index_search`.

#### Scenario: Admin token
- **WHEN** an `admin` bearer posts to `/index/search`
- **THEN** the response is 403 and no embedder request is made

#### Scenario: Route switched off
- **WHEN** `embeddings.search_route.enabled` is `false`
- **THEN** both routes answer 404
