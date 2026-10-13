# Proposal: webui-agent-chat

Phase `ipfs-mcp-server`, plan revision 2 (new change). N5: mount the chat island in the GQAdonis/ipfs-webui fork. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

G11 places the chat in the IPFS Web UI, which Desktop also bundles. The fork change stays small: a launcher, a toggle and settings, so the fork stays close to upstream.

## What Changes

- Fork `src/agent-chat/**`: a launcher next to `<Notify/>` that dynamically imports the island, a Settings toggle (off by default), endpoint and token settings, and an `agentChat` i18n namespace.
- Sequenced after `webui-webmcp-integration` because both touch Settings.

## Capabilities

### New Capabilities

- `webui-agent-chat`: N5: mount the chat island in the GQAdonis/ipfs-webui fork

### Modified Capabilities

None.

## Impact

External fork `/Users/gqadonis/Projects/references/ipfs-webui` (`src/agent-chat/**`, a minimal mount in `src/App.js`, Settings, locale files). The operator commits in the fork.

## Dependencies (plan revision 2, review round 1)

Every task here requires `agent-chat-island` (N4) and `mcp-agui-endpoint` (N12) complete, with `/agui` passing its test/cli suite (plan prerequisites PR-14, PR-15). If N12 is deferred (for example G9-1 ships A2A only), this change is deferred as a whole: no launcher, toggle or chat code ships in the fork, and G11 is reported unmet for this phase. The chat never ships enabled against an endpoint that does not exist.
