# Design: webui-cid-delivery

## Context

See proposal.md and ADR-0002 delivery table (D1-D4).

## Goals / Non-Goals

**Goals:**
- Deliver the WebMCP build without widening RPC CORS to a shared origin.

**Non-Goals:**
- `--unrestricted-api`.
- The D4 path-gateway origin.

## Decisions

- **The choice is deferred to the operator after the spike (Q3).** The cost of D2 is that every fork user's `/webui` changes.

## Risks / Trade-offs

- [D2: the first visit fetches the new CID from the network] → Document `ipfs pin add` for `Gateway.NoFetch` nodes. A test covers the 503 path (`webui.go:127-145`).
