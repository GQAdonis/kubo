# Model fleet

Profile in AGENTS.md: `mixed` (set `2026-10-09`)

AGENTS.md is per repository, not per model. Every model listed below reads the
same file, so the weakest one governs its content. Record the fleet here — a
profile choice nobody wrote down gets re-argued every time someone new arrives.

## Models that work this repo

| Model | Harness | Scaffold needed | Measured |
|---|---|---|---|
| Claude Opus 5.5 / Sonnet 5.5 (`opus` architect and reviewers, `sonnet` builders) | Claude Code | low | no |
| Codex harness default model | Codex CLI | low | no |
| Kimi harness default model (per-role model frontmatter is ignored) | Kimi Code | yes | no |
| MiniMax harness default model | MiniMax Code (`mcode`) | yes | no |
| OpenCode configured provider model | OpenCode | depends on provider | no |
| Zed agent configured model | Zed | depends on provider | no |

Decision: `mixed`. Kimi and MiniMax read this repo and are unmeasured, so the execution scaffold stays until the procedure below shows no regression.

## Before switching to lean

Do not adopt `lean` because a model is reported to be capable. Adopt it when
this repo's task set says so.

1. Fix ~10 representative tasks for this repo.
2. Run them under `mixed`, per model. Record pass rate and token cost.
3. Run them under `lean`, per model. Record the same.
4. Adopt `lean` only if no model regressed.

Pass rate is the gate. Token cost is the tiebreaker. A configuration that costs
less and passes less is a regression carrying an efficiency argument.

## Results

<!-- date | model | profile | pass rate | tokens | decision -->
