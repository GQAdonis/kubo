# OG-11: apply the `team.json` patch and re-export native definitions

Task 1.2 artifact. Nothing here is applied by the architect: `team.json`, the
native exports (`.claude/agents/`, `.codex/agents/`, `.kimi-code/agents/`,
`.opencode/agents/`) and `.agent-team/kubo-ipfs-agentic/state.json` are unowned
(W5), so the operator runs these commands at OG-11 after reviewing
`team-json.patch`.

## What the patch changes

`team-json.patch` (unified diff against `.agent-team/kubo-ipfs-agentic/team.json`):

- `ipfs-agent-architect` gains: `docs/config.md`, `docs/changelogs/v0.44.md`,
  `../../travis/personal/know-me-cluster/namespaces/ipfs/**`.
- `mcp-server-engineer` gains: `../ipfs-desktop/src/daemon/mcp-sidecar*.js`,
  `../ipfs-desktop/src/daemon/index.js`, `../ipfs-desktop/test/**/mcp-sidecar*`.
- `ui-ux-engineer` gains: `../ipfs-webui/**` (whole fork; replaces the five
  narrower `../ipfs-webui/...` entries it subsumes), `../ipfs-desktop/src/tray.js`,
  `../ipfs-desktop/assets/icons/**`, `../ipfs-desktop/assets/build/**`,
  `PRODUCT.md`.

Constraints that stay prose-level (they are not expressible in `owns` globs):

- `../ipfs-desktop/src/daemon/index.js`: only the sidecar wiring lines belong to
  `mcp-server-engineer`, not the whole file's future history.
- `docs/config.md`: pointer entries only (D12); full `ipfs mcp` key entries live
  in `docs/mcp/config-reference.md`.
- `docs/changelogs/v0.44.md`: TOC and Highlights only; Changelog and
  Contributors stay auto-generated (AGENTS.md).
- Writes under `../ipfs-webui/src/webmcp/**` still require `security-reviewer`
  review before merge (proposal, "Reviewer of `src/webmcp`").
- `../ipfs-desktop/test/**/mcp-sidecar*` overlaps `ui-ux-engineer`'s
  `../ipfs-desktop/test/**`. The most specific pattern wins, so the sidecar
  tests belong only to `mcp-server-engineer`. This is the established carve-out
  convention in the same file (`mcp/uar/**` vs `mcp/uar/spike/**`); no other
  path is owned by two builder roles.

## Commands (operator, OG-11)

```bash
cd /Users/gqadonis/Projects/references/kubo

# 1. Review and apply the team.json patch.
git apply --check openspec/changes/team-ownership-agent-backend/team-json.patch
git apply openspec/changes/team-ownership-agent-backend/team-json.patch

# 2. Re-export the native definitions from the patched manifest into a fresh
#    staging directory (the exporter never overwrites an existing out dir).
SKILL="$HOME/.claude/skills/agent-team-creator/scripts/cli.mjs"
STAGE=.agent-team/kubo-ipfs-agentic/exports/reexport-2026-10-13
for t in claude codex kimi opencode; do
  jq -n --argjson team "$(cat .agent-team/kubo-ipfs-agentic/team.json)" \
        --arg target "$t" --arg out "$STAGE/$t" \
        '{team: $team, target: $target, out: $out}' > "/tmp/export-request-$t.json"
  node "$SKILL" export --input "/tmp/export-request-$t.json"
done

# 3. Deliberate merge: copy the six re-rendered role definitions over the
#    installed native files. Verified against the 2026-10-13 manifest: the only
#    change in each rendered file is the role's `Owns:` line.
cp "$STAGE"/claude/.claude/agents/*.md     .claude/agents/
cp "$STAGE"/codex/.codex/agents/*.toml     .codex/agents/
cp "$STAGE"/kimi/.kimi-code/agents/*.md    .kimi-code/agents/
cp "$STAGE"/opencode/.opencode/agents/*.md .opencode/agents/

# 4. Recommended: bring the team copy embedded in state.json up to the patched
#    manifest, so later exports that omit an inline `team` do not render stale
#    ownership. Reads the live revision first; refuses a stale expectedRevision.
REV=$(jq '.revision' .agent-team/kubo-ipfs-agentic/state.json)
jq -n --argjson team "$(cat .agent-team/kubo-ipfs-agentic/team.json)" \
      --argjson rev "$REV" \
      '{state: ".agent-team/kubo-ipfs-agentic/state.json", expectedRevision: $rev, team: $team}' \
  > /tmp/team-update-request.json
node "$SKILL" team-update --input /tmp/team-update-request.json
```

Notes:

- The exporter is the `agent-team-creator` skill CLI (`node
  ~/.claude/skills/agent-team-creator/scripts/cli.mjs export`), the same adapter
  that produced the current native definitions and the staged MiniMax export.
  Requires Node 22+; verified on Node v24.11.1.
- The `claude` and `kimi` exports also stage `plugins/kubo-ipfs-agentic/agents/`
  copies; the project has no `.claude/plugins` or `.kimi-code/plugins`
  installed, so those stay in the staging directory untouched.
- `.claude/agents/` also holds unrelated non-team agents (`artifact-critic`,
  `impeccable-*`); step 3 copies only the six team role files the export
  renders and does not touch them.
- `install-project` was considered and rejected for this refresh: its contract
  retains existing native file contents exactly and only reports differences,
  so it would not update the `Owns:` lines.

## Verification done at draft time (2026-10-13, against the current `team.json`)

- `git apply --check` of `team-json.patch` against the current
  `.agent-team/kubo-ipfs-agentic/team.json`: OK.
- Applying the patch to a scratch copy reproduces the intended manifest byte
  for byte.
- Each of the 11 granted path strings appears exactly once across all roles'
  `owns` lists.
- No `owns` string is shared by two builder roles (`mcp-server-engineer`,
  `skills-plugin-specialist`, `ui-ux-engineer`).
- Steps 2 and 4 were dry-run against scratch copies (`/tmp`): the export
  renders the updated `Owns:` lines for all four targets, and `team-update`
  moves `state.json` from revision 0 to 1 with the patched manifest.

After OG-11, task 1.3 verifies the applied grants (`jq '.roles[] | {id, owns}'`
plus a grep of the four native agent directories).
