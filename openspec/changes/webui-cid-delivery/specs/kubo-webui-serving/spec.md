# Spec Delta: kubo-webui-serving

## Purpose

How this fork's daemon serves the IPFS Web UI at `/webui` on the RPC listener after delivery choice D2 (operator, 2026-10-12): which Web UI build `/webui` points to, which earlier builds stay reachable, and what happens when the build cannot be loaded.

## ADDED Requirements

### Requirement: /webui points to the fork's WebMCP-enabled Web UI build
`WebUIPath` in `core/corehttp/webui.go` SHALL name the CID of the GQAdonis/ipfs-webui build that contains the WebMCP integration (`webui-webmcp-integration`). A request to `/webui/` on the RPC listener SHALL answer `302 Found` with `Location` set to that path, as it does today for the upstream build. The bump SHALL be made only after that build's CID is final and retrievable on the network.

#### Scenario: Redirect to the fork build
- **WHEN** a client requests `GET /webui/` on the RPC listener of a daemon with default `Gateway` settings
- **THEN** the response is `302 Found` and `Location` is the new `WebUIPath`

### Requirement: Earlier Web UI builds stay reachable
The previous `WebUIPath` SHALL be kept in `WebUIPaths`, and no existing entry SHALL be removed, so bookmarks and caches that name an earlier build still load on the RPC listener.

#### Scenario: Bookmark of the previous build
- **WHEN** a client requests the previous `WebUIPath` (the upstream v4.13.0 CID) on the RPC listener
- **THEN** the daemon serves it as before the bump, not a 404 or a redirect to the new build

### Requirement: Unavailable and incompatible states are unchanged
The existing `/webui` error behavior SHALL apply to the new CID unchanged: with `Gateway.NoFetch=true` and the new CID not in the local blockstore, `/webui/` answers `503` with the "IPFS WebUI Not Available" text naming `Gateway.NoFetch=true`; with `Gateway.DeserializedResponses=false` it answers `503` with the "IPFS WebUI Incompatible" text. The off switch for the fork build is the existing one: keep the node from fetching it (`Gateway.NoFetch=true` without pinning it) or load an earlier build by its path.

#### Scenario: NoFetch node without the new build
- **WHEN** `Gateway.NoFetch=true` and the new Web UI CID is not in the local blockstore, and a client requests `GET /webui/`
- **THEN** the response is `503` and the body contains "IPFS WebUI Not Available" and "Gateway.NoFetch=true"

#### Scenario: NoFetch node that pinned the new build
- **WHEN** `Gateway.NoFetch=true` and the operator ran `ipfs pin add` on the new Web UI CID, and a client requests `GET /webui/`
- **THEN** the response is `302 Found` to the new `WebUIPath`

### Requirement: No change outside /webui
The bump SHALL NOT change any `/api/v0` command, the HTTP gateway on `Addresses.Gateway`, the default CID recipe or wire behavior, and SHALL add no Kubo config key.

#### Scenario: Gateway conformance
- **WHEN** the gateway conformance suite and `TestGateway` run against the bumped build
- **THEN** they pass unchanged
