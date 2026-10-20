# Proposal: praxis-origin-and-cid-honesty

Phase `agent-backend-settings › praxis-agentic-ux`, change 7 of 8, child plan r1, Round 3. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

Two KnowMe Praxis ed. 1.2 clauses are marked **P** in the assessment:

- WEB-06 privileged integration not on a shared path-gateway origin. `lke-origin-isolation` covers the console, and the local `/webui` served from the API port is fine, but there is no UI guard when the Web UI is opened through a path gateway (`https://<public-gateway>/ipfs/<cid>/`). On that origin every site under `/ipfs/` shares storage and can script the page, so agent chat, WebMCP writes and approvals must not run there.
- SEC-06 a public CID is neither access control nor a deletion guarantee. Pin removal copy says "Remove pin" (and `unpinLabel` reads "Unpin {name}"); nothing states that unpinning does not delete content other nodes already have, and publish flows do not say the CID is readable by anyone who has it.

Decisions D10 and D11 (decision-log PX-12, PX-13).

## What Changes

- **Origin guard (D10).** On load, the Web UI classifies its origin. When it is served from a shared path gateway (path starts with `/ipfs/` or `/ipns/` on a host that is not the configured Kubo API or gateway, not `localhost`/`127.0.0.1`, and not a subdomain gateway origin `<cid>.ipfs.<host>`), it disables agent chat, WebMCP write tools and every approval surface. In their place it shows: "Agent features are off on this address. Pages under a shared gateway path share one origin, so any site there could act as you. Open this Web UI from your node or a subdomain gateway address." with a link to the node's local address and, when known, the subdomain address. Read-only IPFS functions (Files browse, Explore, Peers) keep working.
- **Unpin copy (D11).** Every unpin and pin-removal confirmation (Files pin toggle, Pins page, remote pinning service unpin, the agent `pin_rm` card description) carries exactly: "Unpinning stops this node from keeping a copy. Other nodes that have it can still serve it."
- **Publish and share copy.** The publish-to-IPNS modal and the share-link modal state: "Anyone with this CID can read the content. A CID is an address, not a password."
- **Remove (MFS) copy.** The MFS remove modal states that removing from Files does not unpin or delete content that is pinned or held elsewhere, with the unpin sentence when the item is also pinned.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope:
  - `../ipfs-webui/src/agent-chat/**` (`boot.js`, `endpoint.js`: guard before mount), `src/webmcp/**` (guard before tool registration), `src/shell/**` (banner), new `src/lib/origin-guard.js`
  - `src/files/modals/remove-modal/**`, `src/files/modals/pinning-modal/**`, `src/files/modals/publish-modal/**`, `src/files/modals/share-modal/**`, `src/files/pin-icon/**`, `src/pins/**`
  - `public/locales/en/files.json` (`unpinLabel` context, publish and share notes), `public/locales/en/notify.json` (`ipfsUnpinSucceedReason`), `public/locales/en/app.json` (guard copy)
  - Kubo: the `pin_rm` `confirmationSummary` template in `mcp/contract/catalog.json` gains the unpin sentence (copy only; owner mcp-server-engineer, contract digest regenerates)
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. The guard only removes agent features on shared path-gateway origins; local and subdomain origins are unchanged.
- Size: S-M.

## Dependencies

- Change 3 `praxis-shell-modes` (banner placement in the shell; composer drops the "Ask the agent" intent under the guard).
