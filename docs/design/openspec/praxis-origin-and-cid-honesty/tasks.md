# Tasks: praxis-origin-and-cid-honesty

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md` (child plan r1), section Ordered changes, phase path `agent-backend-settings › praxis-agentic-ux`, change ID `praxis-origin-and-cid-honesty`. Decisions D10, D11 in `analysis.md`.
Change-level prerequisites (child plan r1): every task in this change also requires completion of: `praxis-shell-modes`. Scheduled in Round 3.

## 1. Origin guard (WEB-06)

- [ ] 1.1 Add `../ipfs-webui/src/lib/origin-guard.js` classifying the origin as local, configured node, subdomain gateway or shared path gateway. Verify: unit tests cover `http://127.0.0.1:5001/webui`, `http://localhost:5001/webui`, `https://<cid>.ipfs.dweb.link/`, `https://ipfs.io/ipfs/<cid>/` and `https://example.org/ipns/<name>/`, with only the last two classified as shared path gateway
- [ ] 1.2 Under a shared path gateway: do not mount agent chat, do not register WebMCP write tools, render no approval surface, and drop the "Ask the agent" intent from the composer. Verify: a test with a mocked shared-gateway location finds no island element, no WebMCP write tool registered, and two composer intents
- [ ] 1.3 Show the guard banner with the exact copy "Agent features are off on this address. Pages under a shared gateway path share one origin, so any site there could act as you. Open this Web UI from your node or a subdomain gateway address." and a link to the local address (and subdomain address when known). Verify: a test finds the exact string and a link whose host is `127.0.0.1` or `localhost`
- [ ] 1.4 Read-only IPFS functions keep working under the guard. Verify: Files browse, Explore and Peers render under the mocked shared-gateway location

## 2. CID honesty copy (SEC-06)

- [ ] 2.1 Every unpin and pin-removal confirmation (Files pin toggle, Pins page, remote service unpin) shows exactly "Unpinning stops this node from keeping a copy. Other nodes that have it can still serve it." Verify: a test opens each confirmation and finds the exact string; grep finds one locale key for it, reused
- [ ] 2.2 Update `ipfsUnpinSucceedReason` so success copy does not imply deletion. Verify: the string contains no "deleted" or "removed from IPFS"
- [ ] 2.3 Publish-to-IPNS and share-link modals show "Anyone with this CID can read the content. A CID is an address, not a password." Verify: a test opens each modal and finds the exact string
- [ ] 2.4 MFS remove modal states that removing from Files does not delete content that is pinned or held elsewhere, and adds the unpin sentence when the item is also pinned. Verify: tests open the modal for a pinned and an unpinned item and find the right copy
- [ ] 2.5 Add the unpin sentence to the `pin_rm` `confirmationSummary` in `mcp/contract/catalog.json` (mcp-server-engineer) and regenerate the contract digest and the A2UI catalog. Verify: contract tests pass, and the rendered `pin_rm` approval card contains the exact sentence
- [ ] 2.6 Voice check on all new strings: sentence case, no exclamation marks, no banned words. Verify: a locale lint over the changed keys passes
