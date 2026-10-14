# Spec Delta: mcp-content-extractor

## Purpose

Phase `agent-backend-settings`, change `mcp-content-extractor` (ADR-0011 V1, V2, V7): `ipfs mcp` turns documents, code, data, image text and video audio into Markdown through an operator-configured xberg service and indexes the result through the existing G12 path.

## ADDED Requirements

### Requirement: Extraction runs as a service, never linked
`ipfs mcp` SHALL call xberg only over HTTP, either a supervised operator-supplied binary on loopback (embedded mode) or an operator-configured URL (remote mode). It SHALL NOT link xberg code, use cgo, look the binary up on `PATH`, or download it.

#### Scenario: Relative binary path
- **WHEN** `embeddings.extractor.embedded.xberg_binary` is a relative path
- **THEN** configuration is rejected and no process starts

#### Scenario: Process lifetime
- **WHEN** `ipfs mcp` exits
- **THEN** the xberg process it started is no longer running

### Requirement: Extraction is off unless configured
Without an `embeddings.extractor` block, indexing SHALL behave exactly as before this change: only `text/plain` and `text/markdown` are indexed and no extractor process or request exists.

#### Scenario: PDF without an extractor
- **WHEN** `ipfs_index_cid` is called on a PDF and no extractor is configured
- **THEN** the result is `chunks_indexed: 0` with `skipped_reason: mime_not_allowed`, and no extractor request is made

### Requirement: Extracted text is indexed with its provenance
Extracted Markdown SHALL be chunked by the existing chunker, embedded by the configured embedder, and stored keyed by the file's CID with `source`, `kind`, `mime_type` and the chunk text.

#### Scenario: PDF with an extractor
- **WHEN** a PDF whose MIME type is allowlisted is indexed with an extractor configured
- **THEN** its entries carry `source: extracted` and a search for a phrase in the PDF returns a hit with that CID and the stored text

### Requirement: Egress is acknowledged before extraction
A remote extractor host SHALL be in the calling principal's `egress_ack` (loopback covered by `local`) before any request to it is built.

#### Scenario: Unacknowledged remote extractor
- **WHEN** the extractor is remote and its host is not in the principal's `egress_ack`
- **THEN** no request reaches the extractor and the file result names the host with `egress_not_acknowledged`

### Requirement: Extraction never injects content
Extraction SHALL NOT add, pin, write to MFS or announce any CID, and SHALL discard images the extractor returns.

#### Scenario: Document with an embedded image
- **WHEN** a DOCX containing an image is indexed
- **THEN** the RPC calls made contain no `add`, `block/put`, `dag/put`, `pin/add`, `files/write` or `routing/provide`
