# Product

<!-- impeccable:product-schema 1 -->

Scope: the agent-facing surfaces of the GQAdonis/kubo fork — the `ipfs mcp` MCP App views (`mcp/app`), the shared React 19 / A2UI component library (`mcp/ui`), the WebMCP tools in the GQAdonis/ipfs-webui fork, and the agent skills that drive them — plus the Promethity rebrand of the whole Web UI fork (`../ipfs-webui`, GQAdonis/ipfs-webui) and the visible branding of the IPFS Desktop fork windows (`../ipfs-desktop`, tribe-health/ipfs-desktop). Kubo itself (daemon, CLI, RPC, gateway) is the substrate, not this product's surface.

Decisions recorded for the wider scope (plan `agent-backend-settings`, change `promethity-design-scope`): D6 (product name) and D8 (attribution and non-affiliation) in Brand, D11 (Desktop display branding) under Constraints.

## Platform

web

The surfaces render inside agent hosts: MCP App views in sandboxed `ui://` iframes (Claude, Codex desktop, Kimi desktop, other MCP Apps hosts), A2UI catalog components rendered by a host's A2UI renderer, and the IPFS Web UI served by Kubo at `/webui` or bundled in IPFS Desktop (Electron).

## Users

All three audiences, designed developer-first:

1. **Developer with their own node (primary).** Works in Claude Code, Codex or Kimi and lets an agent add, pin, publish and fetch content on their local Kubo node. They look at the UI to see what the agent did and to approve or deny each write before it runs.
2. **Remote node operator (secondary).** Runs a node on a server reached over Streamable HTTP and supervises agents managing storage, pins and IPNS names from a desktop agent app.
3. **IPFS Desktop / Web UI user (secondary).** Asks an assistant in plain language to share files or publish a site; the Web UI is their main view of what happens.

## Product Purpose

Let agents operate a user's own IPFS node through MCP and WebMCP while the human stays in control. The UI is the trust surface between agent and node: it shows what an agent intends, asks for confirmation before any write or destructive action, and shows verifiable results (CIDs, pins, IPNS records). Success: an agent completes real IPFS work, and the user can see and approve every change to their node without leaving the agent host.

## Positioning

Control first, backed by own-node verifiability. Every write and destructive action is visible and confirmed by a human before it runs; nothing happens silently. The agent acts on the user's own Kubo node, not a third-party pinning service: content is content-addressed and verifiable, and nothing leaves the machine unless the operator exposes the node.

## Brand

Promethity identity v1 (October 2026), applied in full: fonts, styles, tokens, icons and components. Source of truth: `/Users/gqadonis/Projects/promethity/branding` (`design-system/README.md` holds the rules; `design-system/tokens.json`/`tokens.css`, component CSS, font files, `logo/`). Tagline: "Fire you can hold."

**D6 (product name).** The user-facing product carries the name "Promethity": in Web UI titles and copy, in the Desktop visible display name, and in host tool lists and plugin marketplaces. The name lives in one constant (`src/promethity/brand.js` in the Web UI fork); it is never hard-coded per site.

**D8 (attribution and non-affiliation).** The Web UI fork's About page and footer carry the line "Built on the IPFS Web UI (MIT, Protocol Labs)" plus a one-line statement that Promethity is not affiliated with or endorsed by Protocol Labs or the IPFS project. The upstream `LICENSE` (MIT) is kept verbatim, with the fork notice beside it.

Brand commitments this product must honor, mapped to its mechanisms:

| Brand line | Mechanism in this product |
|---|---|
| Deny is final. | Every write and destructive tool passes a confirmation gate before it runs; a denied or unconfirmed action does not happen. |
| Data stays where it lives. | Tools act on the user's own node over its RPC API; remote HTTP access is off unless the operator enables it. |
| Every decision leaves a record. | Confirmations and their outcomes are shown to the user; any persistent action log is local-only and operator-owned (see Constraints). |

## Voice

From the Promethity content fundamentals: name the mechanism, never claim a virtue; statement, mechanism, stakes. Short declarative sentences; present tense for system state ("Denied. Unpinning requires confirmation."). Sentence case for headings, buttons and labels. Address the reader as "you". Real numbers and exact names (full CIDs available, peer IDs, byte sizes). No exclamation marks, no emoji, none of the brand's banned words.

## Constraints

- **Kubo user-agency rules (AGENTS.md) outrank the brand.** No tracking or telemetry; no data about users, content, requests or peers sent anywhere. "Every decision leaves a record" is satisfied only by records shown to or stored for the operator on their own machine, with an off switch — never by reporting out.
- **No network fetches from the UI.** MCP App views run under a deny-by-default CSP and call server tools rather than gateways or the RPC API directly. Brand fonts ship as the bundled woff2 files from the branding directory; no Google Fonts, CDNs or remote assets.
- **Confirmation is a product requirement, not decoration.** Writes and destructive actions require confirmation on every transport (operator decision, 2026-10-09). In the Web UI fork, the in-page WebMCP approval is an intent and accident check that a page-driving host agent can answer, not a security boundary; approval with an OS presence factor through `ipfs mcp` is a later opt-in (operator decision, 2026-10-12).
- **Accessibility:** WCAG 2.2 AA (the phase goal) in both light and dark themes; the brand's own standard is WCAG AA with a focus ring that holds at least 3.7:1.
- **One component set, two adapters.** The same React 19 components serve MCP App views and the A2UI v0.9 catalog, built from the shared tool contract in `mcp/contract`.
- **The ipfs-webui fork is a separate repository (React 16).** WebMCP work there follows this product context. The fork also carries the full Promethity rebrand (goal 5); its structural edits stay minimal and are enumerated in `../ipfs-webui/docs/PROMETHITY-FORK.md` so upstream merges stay possible.
- **D11 (Desktop display branding).** The Desktop fork changes icons, window titles and the visible display name only. `productName` stays "IPFS Desktop" and the bundle id stays `io.ipfs.desktop`, so userData paths, Keychain entries and update channels are untouched.

## Open decisions

- Whether a persistent local action log exists, where it is stored, and its default (on or off). Must satisfy the no-tracking constraint above.
- ~~How far the Promethity identity is applied inside the ipfs-webui fork beyond the WebMCP surfaces.~~ Answered (2026-10-13, change `promethity-design-scope`): in full. The identity — tokens, fonts, icons, components and voice — applies to every route of the Web UI fork and to the visible surfaces of the Desktop windows, with three recorded seams: the `/explore` route gets token-level theming only because it renders the external `ipld-explorer-components` package (D9, DESIGN.md); non-Latin scripts fall back to system font stacks because the bundled faces are Latin-only (R8, DESIGN.md); and Desktop branding stops at icons, window titles and the display name (D11 above).
- ~~Whether the user-facing product carries the Promethity name or an IPFS-family name in host tool lists and plugin marketplaces.~~ Answered by D6 (Brand): the product carries the Promethity name.
