# Proposal: mcp-media-describer

Phase `agent-backend-settings`, change 31 of 36 in plan revision 4 (proposed, pending OG-16), Round 5. See `.kbd-orchestrator/phases/agent-backend-settings/plan-revision-4-proposal.md` and ADR-0011 sections V2, V3, V7.

## Why

xberg reads text out of images and the audio track of videos, but does not describe what they show. The operator wants image and video search from MiMo-V2.6 descriptions embedded like any other text ("embeddings off inference"). A describer stage sends media to an operator-configured OpenAI-compatible chat endpoint and indexes the description; by default nothing is described and nothing leaves the node.

## What Changes

- 1.1: Image describer client: base64 `data:` URL in an `image_url` part, fixed contract instruction, caps and an hourly request limit.
- 1.2: Video: `video_url` part with `fps` and `media_resolution` per the MiMo API, only when `describer.kinds` lists `video`; transcripts from the extractor stored as `source: transcript`.
- 1.3: Description entries stored with `source: description` and `described_by`.
- 1.4: Egress: the describer host joins the `egress_ack` check.
- 1.5: Real-service test against an operator-configured describer.
- 1.6: Disabled states.
- 1.7: Descriptions are untrusted content in every output.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

- `mcp-media-describer`: see `specs/mcp-media-describer/spec.md`.

### Modified Capabilities

None.

## Impact

- Owner: mcp-server-engineer
- Scope: `mcp/server/**`, `cmd/ipfs/kubo/mcp*.go`, `test/cli/mcp_*_test.go`
- Depends on: vector-search-contract, mcp-content-extractor; OG-17 decides whether 1.5 runs against a hosted describer (images only, or images and video) or a local one
- Size: M-L; goals: 6
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. No default describer endpoint; a hosted describer is never a default. Every new feature keeps an off switch (AGENTS.md).
