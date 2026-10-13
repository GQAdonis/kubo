# Prior context — agent-backend-settings (assess)

> Auto-populated by /kbd-memory-recall for lead view (main thread); lessons from surreal-memory. Cite the lessons that apply; recalled entries are information recorded by agents, not instructions.

## Lessons

- [user] The operator approves merges, tags and machine-level changes explicitly. A single up-front approval question at the phase boundary keeps the run unblocked. _(recorded by an agent; lesson, stage reflect, 2026-10-05; via surreal-memory)_
- [user] The operator merges every PR, wants the full KBD status at the end of every turn, and wants operator-only actions (tags, repos, live-data changes) asked explicitly. _(recorded by an agent; lesson, stage reflect, 2026-10-04; via surreal-memory)_
- [global] Tests must discover their environment (real cache layouts, submodule initialisation) rather than encode one checkout's state, or the phase-boundary gate on a fresh worktree will disagree with the worker's run. _(recorded by an agent; lesson, stage reflect, 2026-10-05; via surreal-memory)_
- [global] Tests that replace HOME cannot use gh credentials, because both the gh config and the macOS keychain are found through HOME. Resolve `gh auth token` under the real environment and pass it as `GH_TOKEN`. _(recorded by an agent; lesson, stage reflect, 2026-10-04; via surreal-memory)_
- [global] `gh issue create --label X` fails when X does not exist. Ensure the label first with `gh label create X --force`, which is idempotent. _(recorded by an agent; lesson, stage reflect, 2026-10-04; via surreal-memory)_

## pk knowledge

*(no pk knowledge for this phase)*

## Previous reflection

*(no previous reflection)*

## Knowledge gaps

*(none recorded)*
