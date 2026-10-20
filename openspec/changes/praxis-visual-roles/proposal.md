# Proposal: praxis-visual-roles

Phase `agent-backend-settings › praxis-agentic-ux`, change 2 of 8, child plan r1, Round 1. See `.kbd-orchestrator/phases/agent-backend-settings/children/praxis-agentic-ux/plan.md`.

## Why

KnowMe Praxis ed. 1.2 UI-07 defines token roles (canvas, chrome, surface, raised, hover, muted, text tiers, action, AI/streaming, ok/warn/err) and ships them with KnowMe Flat 2.0 values (Space Grotesk, Inter, Cyan for AI). The assessment lists this as conflict 1: Praxis mandates Flat 2.0 values while the Web UI, Desktop and `mcp/ui` already ship Promethity (`promethity-delivery`). The Web UI has Promethity tokens in `src/promethity/` but no named roles, so new Praxis surfaces (assistant pane, canvas, inbox, lifecycle strip) have nothing to bind to except raw brand tokens, and there is no AI/streaming role at all.

Decision D6 (decision-log PX-02, PX-08): adopt the Praxis roles, keep Promethity values and fonts (Syne, Hanken Grotesk, JetBrains Mono). The AI/streaming role maps to Promethity `info` (#7FB2D9 dark, #1F5E8C light). No borders and no layout shadows; both systems agree.

PX-02 is an operator assumption, revisitable before this change executes. If it is overturned, only this change is affected.

## What Changes

- Add a role layer of CSS custom properties (`--role-canvas`, `--role-chrome`, `--role-surface`, `--role-raised`, `--role-hover`, `--role-muted`, `--role-text`, `--role-text-muted`, `--role-text-subtle`, `--role-action`, `--role-ai`, `--role-ok`, `--role-warn`, `--role-err`) in the Web UI, each aliasing an existing Promethity token in both themes.
- Mirror the same role names on the `mcp/ui` library scope (`[data-ipfs-ui]`) as aliases of its existing tokens, so the chat island, MCP App views and the Web UI resolve the same role to the same value.
- AI/streaming role (`--role-ai`) is used only for agent-originated surfaces: streaming text, lifecycle strip, context strip, AI-drafted suggestions. It is never used for a control.
- A role-coverage test: new Praxis surfaces may reference roles, not raw hex or raw brand tokens.
- Record the role map and the Flat 2.0 deviation in `DESIGN.md` and in the Web UI `src/promethity/` README comment.

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: behavior changes live in the GQAdonis/ipfs-webui fork, outside this OpenSpec root; the `mcp/ui` part is a pure alias layer with no behavior change. Acceptance is in tasks.md.

### Modified Capabilities

None.

## Impact

- Owner: ui-ux-engineer
- Scope:
  - `../ipfs-webui/src/promethity/theme.js`, `src/promethity/theme-overrides.css`, `src/promethity/shell.css`, `src/promethity/components.css`, new `src/promethity/roles.css`
  - `mcp/ui/src/styles/tokens.css` (role aliases on `[data-ipfs-ui]`), `mcp/chat/src/styles/**` only if a role is consumed there
  - `DESIGN.md` (role table)
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. No visual change on existing screens: every role resolves to the value already shown.
- Size: S-M.

## Dependencies

None. Round 1. Change 3 (`praxis-shell-modes`) depends on this one.
