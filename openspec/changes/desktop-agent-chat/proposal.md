# Proposal: desktop-agent-chat

Phase `ipfs-mcp-server`, plan revision 2 (new change). N6: IPFS Desktop fork: CSP, `webSecurity`, `safeStorage` token bridge and the native confirmation window. See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Operator answer Q7 puts the Desktop fork (`tribe-health/ipfs-desktop`) in scope, owned by ui-ux-engineer and reviewed by security-reviewer. Desktop runs the Web UI with `webSecurity: false` and no CSP (`desktop:src/webui/index.js:41-47`); the chat raises the stakes, and the Desktop window is one of the native confirmation surfaces (operator answer Q4).

## What Changes

- Record what breaks with `webSecurity: true` (RPC CORS for `webui://localhost`) and run a Desktop smoke baseline (files, explore, peers, settings, add, pin).
- A CSP for `webui://localhost`: no `unsafe-eval`, no remote assets, `connect-src` limited to the RPC and `ipfs mcp` origins; `webSecurity` re-enabled. A regression is reported, never hidden by restoring `webSecurity: false` (G11-2).
- A `safeStorage` token bridge: the main process holds the `ipfs mcp` bearer and attaches it through preload; never in page storage (G11-4).
- The Desktop confirmation window: one-time pairing with `ipfs mcp` (code shown on the `ipfs mcp` side), pairing key in `safeStorage`, a native modal requiring a user gesture and unreachable from the Web UI renderer, approve or deny over the N14 channel protocol.

## Capabilities

### New Capabilities

- `desktop-agent-chat`: N6: IPFS Desktop fork: CSP, `webSecurity`, `safeStorage` token bridge and the native confirmation window

### Modified Capabilities

None.

## Impact

External fork `/Users/gqadonis/Projects/references/ipfs-desktop`: `src/webui/**`, `src/agent/**` (new), `package.json` scripts, `test/**`. The operator commits in the fork.

## Dependencies (plan revision 2, review round 1)

Tasks 1.1-1.2 (CSP, `webSecurity`) need nothing beyond the grant. Task 1.3 (token bridge) needs `agent-chat-island` (N4) complete, because it hands the token to the island's property API. Tasks 1.4-1.6 (pairing, confirmation window, e2e, operator run) need `native-confirmation-prompt` (N14) complete. The Desktop chat run itself is `webui-agent-chat` 1.3, which needs N12. If N14 is deferred, tasks 1.4-1.6 defer and Desktop ships hardening only; if N4 is deferred, task 1.3 defers. Disabled state: with chat off and no pairing, Desktop exposes no token bridge call and no confirmation channel (tested in 1.5).
