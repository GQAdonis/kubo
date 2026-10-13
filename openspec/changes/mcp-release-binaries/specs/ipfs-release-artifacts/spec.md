# Spec Delta: ipfs-release-artifacts

## Purpose

Produces locally built release archives of the fork's `ipfs` binary, with checksums and notes explaining MCP use against existing nodes.

## ADDED Requirements

### Requirement: Reproducible local artifacts
The release script SHALL build every target through `make build`, and SHALL write one archive per target plus a SHA-256 checksum file.

#### Scenario: Checksums match
- **WHEN** the script finishes
- **THEN** `shasum -a 256 -c` on the checksum file succeeds for every archive

### Requirement: No publishing
The release script MUST NOT push tags, create releases or upload artifacts.

#### Scenario: Offline run
- **WHEN** the script runs with no network credentials
- **THEN** it completes and only writes local files
