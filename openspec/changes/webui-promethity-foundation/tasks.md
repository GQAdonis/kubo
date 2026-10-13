# Tasks: webui-promethity-foundation

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `webui-promethity-foundation`, backend task IDs 1.1, 1.2, 1.3, 1.4. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: promethity-design-scope. Scheduled in Round 3.

## 1. Tasks

- [ ] 1.1 `tokens.css` light and dark; a contrast unit test recomputing pairings from shipped CSS. Acceptance: the test covers every DESIGN.md pairing including the form edge.
- [ ] 1.2 Fonts: woff2 only (Hanken 400-700 from `mcp/ui/fonts`, Syne, JetBrains Mono, Newsreader), OFL texts in `public/fonts/LICENSES/`, `font-display: swap`, `unicode-range` system fallback; drop ipfs-css Inter/Montserrat. Acceptance: no Inter/Montserrat file in `build/`; four OFL files present.
- [ ] 1.3 `theme-overrides.css` remapping ipfs-css colour classes onto tokens, imported last. Acceptance: the remap covers `.bg-navy`, `.aqua`, `.teal`, `.bg-snow`, `.charcoal`, `.b--*`.
- [ ] 1.4 `brand.js`, theme provider and toggle (D16), persisted as a versioned non-secret `localStorage` key. Acceptance: grep finds one constant for the product name.
