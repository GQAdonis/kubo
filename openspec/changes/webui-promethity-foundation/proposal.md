# Proposal: webui-promethity-foundation

Phase `agent-backend-settings`, change 8 of 28 in plan revision 3, Round 3. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-10).

## Why

The Web UI has no token system, no dark theme and hard-coded IPFS colours. An override layer of tokens, fonts and class remapping recolours about 163 files with zero edits and keeps the fork mergeable with upstream.

## What Changes

- 1.1: `tokens.css` light and dark; a contrast unit test recomputing pairings from shipped CSS.
- 1.2: Fonts: woff2 only (Hanken 400-700 from `mcp/ui/fonts`, Syne, JetBrains Mono, Newsreader), OFL texts in `public/fonts/LICENSES/`, `font-display: swap`, `unicode-range` system fallback; drop ipfs-css Inter/Montserrat.
- 1.3: `theme-overrides.css` remapping ipfs-css colour classes onto tokens, imported last.
- 1.4: `brand.js`, theme provider and toggle (D16), persisted as a versioned non-secret `localStorage` key.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: `../ipfs-webui/src/promethity/**`, `src/index.js`, `public/fonts/**`
- Depends on: promethity-design-scope
- Size: L; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
