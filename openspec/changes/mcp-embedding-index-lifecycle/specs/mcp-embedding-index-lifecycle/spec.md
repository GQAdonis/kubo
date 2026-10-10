# Spec Delta: mcp-embedding-index-lifecycle

## Purpose

N7b: opt-in indexing trigger, index tools and pin-aware deletion (split from N7 for session size).

## ADDED Requirements

### Requirement: Off by default
Content SHALL NOT be indexed automatically unless indexing is configured on.

#### Scenario: Indexing off
- **WHEN** an agent adds a text document with indexing off
- **THEN** no chunk is written and no embedder request is made

### Requirement: Add never fails because of indexing
An indexing failure SHALL NOT fail the add that triggered it.

#### Scenario: Embedder down
- **WHEN** the embedder is unreachable during `ipfs_add`
- **THEN** the CID is returned with a warning

### Requirement: Remaining pins keep vectors
Vectors for a CID SHALL NOT be deleted while any pin of any type covers it.

#### Scenario: Two pins
- **WHEN** a CID is pinned directly and inside a recursive pin, and the direct pin is removed
- **THEN** its vectors remain

#### Scenario: Pin check fails
- **WHEN** `pin_rm` removes the last pin of an indexed CID and the following `pin ls` check fails
- **THEN** the vectors remain and the `pin_rm` result reports it in its `warnings` field

### Requirement: Lifecycle warnings are part of the structured result
Warnings about index housekeeping after `pin_rm`, `pin_update`, `repo_gc` and `ipfs_index_reconcile` SHALL be returned in the result's optional `warnings` array, and SHALL NOT be carried only in `_meta`.

#### Scenario: Store unreachable after an unpin
- **WHEN** the vector store is unreachable while `pin_rm` checks an unpinned CID
- **THEN** the unpin succeeds and the structured result has a `warnings` entry naming the CID

#### Scenario: Nothing to report
- **WHEN** an unpin needs no index housekeeping or it succeeds
- **THEN** the result has no `warnings` field

### Requirement: Every skip has a reason
`ipfs_index_cid` SHALL return a `skipped_reason` whenever it stores no chunk, using `not_a_file` for a directory or other non-file node and `empty` for an empty or whitespace-only file.

#### Scenario: Directory
- **WHEN** `ipfs_index_cid` is called with the CID of a UnixFS directory
- **THEN** `chunks_indexed` is 0 and `skipped_reason` is `not_a_file`

#### Scenario: Blank file
- **WHEN** `ipfs_index_cid` is called with the CID of a file that holds only whitespace
- **THEN** `chunks_indexed` is 0, `skipped_reason` is `empty`, and no embedder request is made

### Requirement: Store and embedder failures are retryable
A failure of the embedder or the vector store in an `ipfs_index_*` tool SHALL be the tool error `index_unavailable` with `retryable` true, and a missing egress acknowledgement SHALL be `egress_not_acknowledged` naming the hosts.

#### Scenario: Embedder down during search
- **WHEN** `ipfs_index_search` is called by a principal whose `egress_ack` covers the embedder and the embedder is unreachable
- **THEN** the call fails with `index_unavailable` and `retryable` true
