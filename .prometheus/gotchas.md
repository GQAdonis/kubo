# gotchas

Append-only. Dated entries. Mark superseded entries; do not delete them.

## 2026-10-09
- Initialized by prometheus-context-bootstrap.

- 2026-10-09: A subagent copied `pkill -f "ipfs daemon"` from AGENTS.md during test/cli runs. That kills every daemon by name, including the operator's IPFS Desktop daemon on default ports. Never use it here; stop only harness daemons by PID. Afterwards no daemon was listening on 5001/4001.
- 2026-10-10: `kbd-apply list` reports task state from the tasks.md checkboxes, not the runtime. When an agent ticks `[x]` itself, a sweep that skips "done" rows never fires begin-task/end-task, and the runtime task stays pending (mcp-release-binaries). Close by running begin-task/end-task for every task the runtime still shows pending (`prometheus kbd status --json`), not by checkbox state.
- 2026-10-11: Shells hung for ~45 h with no child process. `sample` showed bash 5.3 (`/usr/local/bin/bash`) blocked in `write()` inside `do_redirections`: it writes a here-document or here-string into a pipe whose both ends it holds, assuming the pipe can grow to 64 KB; that macOS pipe stayed at 16 KB, so the write never finished. Sporadic (pipes grow to 64 KB again now). Rules: pass large content through a file, not `python3 - <<EOF` or `<<<"$big"`; use the Edit/Write tools for file edits; run every `kbd-apply` and `prometheus kbd` call under `timeout` with `</dev/null`.
- 2026-10-11: A close that dies after the task mutation but before the after-boundary leaves a complete change or task with an open "Starting" obligation (`boundaryObligations` in `prometheus kbd status --json`; session start lists them as "Outstanding boundaries"). Close it with `prometheus kbd guard evaluate --boundary <change|task> --edge after --subject <change|change/id> --json --repair-projections`. Never re-run a queued close script without first checking the tasks are really done; the hung charcoal-agent queue would have closed unticked tasks.

- 2026-10-13: A Kubernetes postStart exec hook that backgrounds work (`cmd &`) blocks until that work exits unless its I/O is detached (`</dev/null >/dev/null 2>&1 &`); the pod stays PodInitializing and cannot even terminate. Never force-delete a pod stuck that way: the kubelet keeps the old process running, and for kubo it holds `repo.lock` so the replacement crash-loops. LKE nodes have no `crictl`; stop the orphan with a node debug pod (`chroot /host pkill -TERM -f "ipfs daemon"`).
