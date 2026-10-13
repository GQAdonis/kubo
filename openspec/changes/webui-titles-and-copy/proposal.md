# Proposal: webui-titles-and-copy

Phase `agent-backend-settings`, change 14 of 28 in plan revision 3, Round 4. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (UI-31).

## Why

Page titles, locale strings and e2e assertions carry the IPFS product name; the protocol keeps its name while the product chrome says Promethity, with attribution kept.

## What Changes

- 1.1: Title helper from `brand.js` in the five `| IPFS` sites plus `src/files/FilesPage.js:327`.
- 1.2: `en` locale rewrites for product-name uses and banned words (`explore.json:48`, `settings.json:57`).
- 1.3: e2e title assertions updated.
- 1.4: Attribution in About and footer (D8); `LICENSE` kept, fork notice beside it.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui or tribe-health/ipfs-desktop fork, outside this OpenSpec root; acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope: Helmet sites, `public/locales/en/*.json`, `test/e2e/navigation.test.js`, About/footer
- Depends on: promethity-design-scope, webui-promethity-foundation
- Size: M; goals: 5
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
