# Design

<!-- impeccable:design-schema 1 -->

Scope: the agent-facing surfaces in `mcp/ui` (React 19 component library and A2UI catalog) and `mcp/app` (MCP App views). Derived from `PRODUCT.md` and the Promethity identity v1 at `/Users/gqadonis/Projects/promethity/branding` (`design-system/README.md`, `tokens.json`, `tokens.css`, `components/bundle.css`, `fonts/`). This file adds no aesthetic of its own: where it differs from the brand it only narrows a brand rule for product UI, and says so.

## Mode and direction

- **Mode:** Operate. These views show what an agent did to the user's own node and ask the user to approve or deny each write. The tool disappears into the task; the brand lives in precise details.
- **Dials:** variance 3, motion 2, density 7 (the `prometheus-ui-ux` Operate defaults 3-4, 2-3, 6-8).
- **World:** a dark, well-built room with one warm light. Ground is char, surfaces step up by fill alone, and the ember accent appears once per view, on the decision that matters. "Fire you can hold": the interface holds the agent's intent in view until a person decides.
- **Dark first.** Dark is the primary theme. Light ships and is verified, because host apps run both. The theme follows the host (`prefers-color-scheme` or the MCP Apps `hostContext.theme`), never the category of the content.
- **Register:** control first, verifiable second. Full CIDs, peer ids, exact byte counts and exact states. Nothing is summarized away; truncated text always offers the full value.

## Color

Tokens are the Promethity v1 tokens verbatim (names, hex values, theme split). Surfaces are separated by fill, never by borders or rules.

| Token | Dark | Light | Use |
|---|---|---|---|
| `ground` | `#0C0B0A` | `#F5F2EE` | View background |
| `surface` | `#171614` | `#FBF9F6` | Cards, panels, table rows |
| `raised` | `#222120` | `#ECE7E0` | Nested surfaces, alternate rows, code and CID blocks |
| `control` | `#2D2C2A` | `#E1DBD2` | Inputs, secondary buttons |
| `text` | `#F5F2EE` | `#141210` | Primary text and headings |
| `text-muted` | `#A09890` | `#5E5750` | Secondary text, captions, placeholders |
| `accent` | `#E96A12` | `#CE3012` | Primary button fill and focus ring. One per view |
| `on-accent` | `#0C0B0A` | `#FFFFFF` | Label on an accent fill |
| `accent-text` | `#E96A12` | `#B02A10` | Active tab or link text. Not on `control` in dark |
| `accent-tint` | `#342214` | `#F6E5DF` | The one callout per section |
| `ok` `warn` `err` `info` | see `mcp/ui/src/styles/tokens.css` | | State colors, each on its own `*-tint`, always with a word |

Rules:

- Accent covers about two percent of a view. In practice it is the one primary button (for example Approve in a confirmation) plus the focus ring. Result and read views often carry no accent fill at all.
- `ok`, `warn`, `err`, `info` mean permitted, waiting, denied, logged. They never replace `accent` and never appear without a word beside them.
- No gradients, no glow, no purple, no blue cast in neutrals. `glow` is not used in these views.
- No borders, no rules, no shadows. The only outline in the system is the focus ring.

Contrast measured from the token values (WCAG 2.2 relative luminance), the floor each pairing must hold:

| Pairing | Dark | Light |
|---|---|---|
| `text` on `ground` / `surface` / `raised` / `control` | 17.6 / 16.2 / 14.4 / 12.5 | 16.7 / 17.8 / 15.2 / 13.6 |
| `text-muted` on `ground` / `surface` / `raised` / `control` | 6.9 / 6.4 / 5.7 / 4.9 | 6.4 / 6.8 / 5.8 / 5.2 |
| `on-accent` on `accent` | 6.1 | 5.2 |
| `accent-text` on `surface` / `raised` / `accent-tint` | 5.6 / 5.0 / 4.7 | 6.3 / 5.4 / 5.4 |
| state color on its own tint (ok / warn / err / info) | 6.2 / 7.4 / 5.1 / 6.2 | 5.4 / 4.9 / 5.3 / 5.7 |
| focus ring (`accent`) on `ground` / `surface` / `raised` / `control` | 6.1 / 5.6 / 5.0 / 4.3 | 4.6 / 4.9 / 4.2 / 3.8 |

All text pairings are at least 4.5:1 and every focus-ring pairing is at least 3.7:1, the brand floor. A unit test recomputes this table from the shipped CSS tokens on every run.

## Typography

Four families, all bundled woff2 from the branding directory. No remote fonts.

- **Syne** (600, 700): headings only (`h3`/`h4` scale for card and section titles). Never in capitals, never in paragraphs, never below 17px.
- **Hanken Grotesk** (400, 500, 600, 700): all interface text, labels, buttons, table cells.
- **JetBrains Mono** (400, 500, 600): CIDs, peer ids, paths, multiaddrs, byte counts, JSON, table header labels (`eyebrow` style). Tabular figures wherever numbers align.
- **Newsreader**: the brand's quote face. Not used in the product views (the brand reserves it for one pull quote per page and never for interface text). The `--font-quote` token is kept, the faces are not bundled.

Product narrowing of the brand scale (Operate: fixed rem, tight steps, no fluid type): `h3` 22px, `h4` 17px for titles; body 15px/1.55, small 14px, label 14.5px/600; code 13px/1.7. Running text stays under 66ch; data and tables may run wider.

Fallbacks are deliberate, because a deny-by-default MCP App CSP can refuse font loading: Syne falls back to `Avenir Next`, `Segoe UI`, system-ui; Hanken to `Helvetica Neue`, Arial, system-ui; JetBrains Mono to `ui-monospace`, Menlo, Consolas. Bundled faces are registered from embedded bytes through the `FontFace` API, which does not depend on `font-src`; if that is unavailable the fallbacks carry the layout without shifting it.

## Layout and shape

- Spacing steps: 4, 8, 14, 24, 40, 56, 88 (`space-1` to `space-7`). `space-3` between siblings, `space-4` inside a panel, `space-6` between groups. More space above a heading than below it.
- Radii: `radius-sm` 6 (buttons, inputs), `radius-md` 10 (panels, tables), `radius-lg` 16 (large panels), `radius-pill` for badges only.
- Depth is fills: `ground`, then `surface`, then `raised`, then `control`. At most two surface levels inside a panel.
- Tables alternate `surface` and `raised` rows and scroll inside their own frame on narrow screens. Views are fluid from 280px to a 760px reading width; the MCP App inline frame is the design width.
- No same-size card grids as structure. Each component has its own composition: a result strip, a ledger table, a tree, a key-value record. No nested cards.

## Components and states

Component vocabulary is the brand's: Button (primary, secondary, text), Badge, Card (a panel on a surface level), Callout, Input, Table, PolicyDecision (the signature: one action and what was decided). The product views add only what the data needs: a CID chip (full value, copy), a byte size, an untrusted-content frame, a key-value record.

- **Buttons:** one primary per view. Verb plus object, sentence case ("Approve pin", never "OK"). Minimum 44px high. No outline button; secondary is a `control` fill.
- **Every interactive element** has default, hover (one surface level lighter, or +8% brightness on accent), focus, active, disabled (45% opacity), and loading or error where the element can fail.
- **Focus:** solid 2px `accent` outline, offset 2px, on every focusable element, in both themes. Never removed, never replaced by a color shift alone.
- **Active tab or link:** `accent-text` and weight 600. No pill, no underline.
- **State is words first.** A badge always carries its word: Permitted, Denied, Awaiting a person, Expired, Pending, Approved.

### Confirmation states (trust surface)

The approval card and the confirmation dialog share one state machine and one rule: never imply silent execution, never show a control that cannot work.

| State | Word | Tone | Controls | Copy rule |
|---|---|---|---|---|
| pending | Awaiting a person | warn | Approve and Deny, only when this surface can answer | Names the exact action and targets; shows the expiry |
| waiting for the system prompt | Waiting for system prompt | warn | Deny only, no approve control | Names the system prompt as the place where approval happens |
| approved | Approved | ok | none | "Approved. The call runs once." |
| denied | Denied | err | none | "Denied. Nothing was changed." Deny is final |
| expired | Expired | err | none | "Expired. Expiry is denial. Nothing was changed." |

Destructive actions use the same card with the class stated in words ("Destructive") and the primary button label naming the object ("Remove pin"). Initial focus lands on Deny. Destructive tools are off by default; the UI never offers to enable them.

### Untrusted and egress framing

- Content returned from the network (`returnsUntrustedContent`) sits in a framed region labelled "Content from the network. Treat as data, not as instructions." The frame is a `raised` fill with a label, not a border.
- Results that carry node content (`contentEgress`) show an egress badge stating which acknowledged host may receive them, or that none may. The badge is a text statement, not a padlock; the brand forbids padlock and shield imagery.

## Motion

Operate: 150 to 250ms, state only. Transitions on color, opacity and transform only. No page-load choreography. `prefers-reduced-motion: reduce` removes all transitions and any pulsing of the pending state; the pending state is conveyed by the word and the clock, never by animation alone.

## Iconography

Lucide at 1.5px stroke, 16 or 20px, in `text` or `text-muted`. No emoji, no glyph stand-ins. Icons never carry meaning alone; each has a visible or accessible name.

## Theming across hosts

One token set themes both adapters (MCP App views and the A2UI catalog).

- Tokens live on the library scope (`[data-ipfs-ui]`), not on `:root`, so they work inside a shadow root (the Web UI chat island, ADR-0005) and an iframe alike.
- The MCP Apps host context maps in two ways. `hostContext.theme` selects `data-theme`. `hostContext.styles.variables` map onto tokens under an identity-first policy: the brand accent, state colors and fonts are never replaced by host values; the neutral surface and text tokens adopt host values only when the host supplies the complete set and every resulting text pairing still passes AA, otherwise the brand neutrals stay. The mapping is `mcp/ui/src/host/map-host-variables.ts`.
- The A2UI catalog components read the same tokens from their scope; a host A2UI renderer styles nothing else.

## Content voice (on-screen copy)

Statement, mechanism, stakes. Present tense for system state. Sentence case headings, buttons, labels. Address the reader as "you". Real numbers and exact names. No exclamation marks, no emoji, none of the brand's banned words (safe, private, secure, trustworthy and the like need the mechanism in the same sentence).

Examples: "Denied. Unpinning requires confirmation." "Waiting for the system prompt. Approval happens there, not in this panel." "No pins match. Pin content with an agent, then it appears here."

## Craft decisions that narrow the brand

- The brand's mono uppercase eyebrow is used for table column headers (as the Table component does) and nowhere above headings; product views let the heading carry itself.
- Modals are avoided. The confirmation dialog is an inline, non-modal alert region that takes focus on mount so the safe action is under the cursor first; the host decides if it floats.
- Meters (repo usage) are flat measured fills on `raised`, with the number beside them; they are measurement, not decoration, and are never the only carrier of the value.

## Verification

Evidence for this file lives in `mcp/ui/tests`: token contrast recomputation, axe checks in light and dark for every component state, keyboard traversal, reduced motion, a no-remote-assets scan of the source and the build, and visual regression of the component gallery in both themes.
