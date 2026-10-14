# Tasks: desktop-mcp-sidecar

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `desktop-mcp-sidecar`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: team-ownership-agent-backend, mcp-agent-settings-api. Scheduled in Round 4.

## 1. Tasks

- [x] 1.1 Supervise `ipfs mcp --http=127.0.0.1:<port> --agui --confirm=native,desktop --settings-dir=<userData>/mcp`, tied to the daemon lifecycle, restart with backoff, stderr to the log; stop by tracked PID only. Acceptance: grep finds no `pkill` or name-based kill.
- [x] 1.2 Port discovery (R4): prefer 5050, else a free loopback port, never a silent fallback. Acceptance: a test with 5050 occupied picks another port and reports it.
- [x] 1.3 Generate `ui` and `admin` tokens, store them in `safeStorage`, pass them and the LLM key by environment names. Acceptance: a test asserts no token in the electron-store plaintext.
- [x] 1.4 Off switch `mcpSidecar.enabled` (store key, tray toggle). Acceptance: a disabled-state test asserts no child and no bound port.
- [x] 1.5 Idle RSS capture of the child into a new dated evidence folder. Acceptance: README records binary, macOS version, method and RSS.
