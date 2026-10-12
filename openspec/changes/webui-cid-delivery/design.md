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
- **2026-10-12: D2 chosen by the operator** after the spike (Codex GO; `Kimi.app` GO with a caveat; ADR-0002 Accepted). The bump replaces `WebUIPath` with the fork build's CID and prepends it to `WebUIPaths`, keeping every earlier entry (the gateway on the RPC listener serves only `WebUIPaths`, `cmd/ipfs/kubo/daemon.go:875`). It waits for that build. `TestWebUI` gains the redirect case and a NoFetch case with the new CID pinned; the existing 503 cases must keep passing.

## Risks / Trade-offs

- [D2: the first visit fetches the new CID from the network] → Document `ipfs pin add` for `Gateway.NoFetch` nodes. A test covers the 503 path (`webui.go:127-145`).
- [D2: the build CID is not retrievable when the bump lands] → Every node without it gets a hanging or failing `/webui`. The operator makes the CID retrievable (pin and provide it, or publish the CAR) before 1.2 is applied.
- [D2: the 503 "Not Available" text links to upstream `ipfs/ipfs-webui` releases, which do not contain the fork build] → Open question for the operator: keep the upstream text (no change to `webui.go` beyond the CIDs) or point it at the fork's release. Changing it is a user-visible text change and is not part of 1.2 unless the operator asks.
