# Spec Delta: mcp-media-describer

## Purpose

Phase `agent-backend-settings`, change `mcp-media-describer` (ADR-0011 V2, V3, V7): `ipfs mcp` asks an operator-configured OpenAI-compatible chat model to describe images and, if enabled, videos, and indexes the description so media are found by text search.

## ADDED Requirements

### Requirement: Description is off unless configured
Without an `embeddings.describer` block, `ipfs mcp` SHALL make no description request, and images and videos SHALL be indexed only through OCR or transcription when those are enabled.

#### Scenario: Image without a describer
- **WHEN** an allowlisted PNG is indexed with no describer and OCR off
- **THEN** the result is `chunks_indexed: 0` with `skipped_reason: describer_not_configured`, and no request leaves `ipfs mcp`

### Requirement: Media are sent as bytes, never as node URLs
The describer request SHALL carry the media as a base64 `data:` URL and SHALL NOT contain an URL that points at the node or any gateway.

#### Scenario: Describing an image
- **WHEN** an image is described
- **THEN** the request body contains a `data:` URL for the image and no `http` or `https` URL referring to its CID

### Requirement: Video is described only when listed
A video SHALL be sent to the describer only when `describer.kinds` includes `video`, and only within `video_max_bytes`.

#### Scenario: Default kinds
- **WHEN** a video is indexed with `describer.kinds` at its default `["image"]`
- **THEN** no video is sent to the describer

### Requirement: Descriptions are labelled and untrusted
Description entries SHALL be stored with `source: description` and `described_by`, and search output SHALL mark them as untrusted content.

#### Scenario: Searching a described image
- **WHEN** a search matches a description chunk
- **THEN** the hit has `source: description`, `described_by` set to the model, and the untrusted-content marking

### Requirement: Describer egress is acknowledged
The describer host SHALL be in the calling principal's `egress_ack` before any request to it is built.

#### Scenario: Hosted describer not acknowledged
- **WHEN** the describer is a non-loopback host absent from the principal's `egress_ack`
- **THEN** the describer receives no request and the file result names the host with `egress_not_acknowledged`
