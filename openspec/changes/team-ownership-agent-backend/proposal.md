# Proposal: team-ownership-agent-backend

Phase `agent-backend-settings`, change 1 of 28 in plan revision 3, Round 1. See `.kbd-orchestrator/phases/agent-backend-settings/plan.md` section 4. Notes: (D10, W5).

## Why

This phase edits paths no role owns: most of the Web UI fork, Desktop tray, sidecar and branding files, kubo `PRODUCT.md`, `docs/config.md`, `docs/changelogs/v0.44.md` and the know-me-cluster manifests. AGENTS.md routes such paths through the architect and an OpenSpec change before anyone edits them, and `team.json` itself is unowned, so the operator applies the patch.

## What Changes

- 1.1: Record the ownership extension in the proposal: `ui-ux-engineer` gains the whole `../ipfs-webui/**` fork (with `src/webmcp/**` writes still requiring security review), `../ipfs-desktop/src/tray.js`, Desktop branding assets (`assets/icons/**`, `assets/build/**`) and kubo `PRODUCT.md`; `mcp-server-engineer` gains `../ipfs-desktop/src/daemon/mcp-sidecar*.js`, the wiring lines in `src/daemon/index.js`, and `../ipfs-desktop/test/**/mcp-sidecar*` (W5); the architect gains `docs/config.md` (pointer entries only), `docs/changelogs/v0.44.md` (TOC and Highlights only; W5) and a know-me-cluster feature branch under `namespaces/ipfs/**`; the `WebUIPath` edit stays with the architect under D17.
- 1.2: Write the exact `team.json` patch and the native re-export command into the change folder, not applied (W5: `team.json` and the exports are unowned).
- 1.3: After OG-11, verify the applied grants.

Full task text and acceptance checks are in `tasks.md`; code-running checks run once in `phase-integration-gate` unless the plan names an exception.

## Ownership extension (task 1.1 record)

Every path from the assessment "Ownership" section (revision 2), plus round-2 S3 and plan-review W5. Sibling repositories are written relative to the kubo repo root (`../`).

### `ui-ux-engineer` gains

| Path | Source and constraint |
|---|---|
| `../ipfs-webui/**` (the whole fork) | D10; subsumes the unowned paths listed in assessment "Ownership": `src/navigation/**`, `src/status/**`, `src/files/**`, `src/peers/**`, the other route folders, `src/bundles/routes.js`, `public/index.html`, `public/manifest.json`, and `public/locales/en/*.json` beyond `agentChat` |
| `../ipfs-webui/src/webmcp/**` | Included in the whole-fork grant, but every write still requires security review (see "Reviewer of `src/webmcp`" below) |
| `../ipfs-desktop/src/tray.js` | assessment "Ownership" |
| `../ipfs-desktop/assets/icons/**`, `../ipfs-desktop/assets/build/**` | Desktop branding assets, assessment "Ownership" |
| `PRODUCT.md` (kubo) | round-2 S3 |

### `mcp-server-engineer` gains

| Path | Source and constraint |
|---|---|
| `../ipfs-desktop/src/daemon/mcp-sidecar*.js` | Sidecar lifecycle additions under `src/daemon/**` (assessment "Ownership"); the files are created by `desktop-mcp-sidecar`, they do not exist yet |
| The sidecar wiring lines in `../ipfs-desktop/src/daemon/index.js` | assessment "Ownership" (`src/daemon/**` additions); only the wiring lines, not the whole file |
| `../ipfs-desktop/test/**/mcp-sidecar*` | plan-review W5 |

### `ipfs-agent-architect` gains

| Path | Source and constraint |
|---|---|
| `docs/config.md` | round-2 S3; pointer entries only (D12 keeps the full `ipfs mcp` key entries in `docs/mcp/config-reference.md`) |
| `docs/changelogs/v0.44.md` | plan-review W5; TOC and Highlights only — the Changelog and Contributors sections stay auto-generated (AGENTS.md) |
| A know-me-cluster feature branch under `namespaces/ipfs/**` | D10; the `lke-origin-isolation` and `lke-agent-settings` work |

### Stays with `ipfs-agent-architect`

- The `WebUIPath` edit in `core/corehttp/webui.go` stays with the architect under D17 (the single operator-approved end-of-phase bump, OG-7).

### Reviewer of `src/webmcp`

Writes to `../ipfs-webui/src/webmcp/**` require review by the `security-reviewer` role (Claude Code, separate context per W3) before merge.

### Application

`team.json` and the native exports are unowned (W5): task 1.2 writes the exact `team.json` patch and re-export command into this change folder, and the operator applies them at OG-11. Task 1.3 verifies the applied grants.

Task 1.2 artifacts: `team-json.patch` (the exact patch, `git apply --check`-verified against the current `team.json`) and `og11-apply-and-reexport.md` (the OG-11 apply, re-export and merge commands, dry-run verified).

## Capabilities

### New Capabilities

None. `skip_specs: true` is set: team governance only; no system behavior changes.

### Modified Capabilities

None.

## Impact

- Owner: ipfs-agent-architect drafts; operator applies (OG-11)
- Scope: `openspec/changes/team-ownership-agent-backend/`
- Depends on: none
- Size: S; goals: all goals
- Stability: no `/api/v0`, HTTP Gateway, default CID recipe or wire change. Every new feature keeps an off switch (AGENTS.md).
