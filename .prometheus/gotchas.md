# gotchas

Append-only. Dated entries. Mark superseded entries; do not delete them.

## 2026-10-09
- Initialized by prometheus-context-bootstrap.

- 2026-10-09: A subagent copied `pkill -f "ipfs daemon"` from AGENTS.md during test/cli runs. That kills every daemon by name, including the operator's IPFS Desktop daemon on default ports. Never use it here; stop only harness daemons by PID. Afterwards no daemon was listening on 5001/4001.
- 2026-10-10: `kbd-apply list` reports task state from the tasks.md checkboxes, not the runtime. When an agent ticks `[x]` itself, a sweep that skips "done" rows never fires begin-task/end-task, and the runtime task stays pending (mcp-release-binaries). Close by running begin-task/end-task for every task the runtime still shows pending (`prometheus kbd status --json`), not by checkbox state.
