# Boundary board

Two sessions run the same `kubo-ipfs-agentic` agent team in this one checkout:

- **Backend lane:** the Claude Code session, phase `agent-backend-settings`. It owns all kubo agentic backend code.
- **UI lane:** the Codex session, phase `agentic-ui-implementation`. It owns UI and UX only.

They share no memory and cannot message each other, so they coordinate through this board. Every agent on either lane reads it at session start and before each task boundary, and writes to it whenever its work crosses into the other lane.

## Ownership (operator, 2026-10-20)

| Lane | Owns (writes) | Reads only |
|---|---|---|
| backend | `mcp/server/**`, `mcp/contract/**`, `cmd/ipfs/kubo/mcp*`, `test/cli/mcp_*`, `docs/mcp/**`, `go.mod`/`go.sum`, KBD phase `agent-backend-settings`, `know-me-cluster` | the UI repos |
| ui | `../ipfs-webui/**`, `../ipfs-desktop/**` (UI), `mcp/ui/**`, `mcp/app/**`, `mcp/chat/**`, `docs/design/**`, KBD phase `agentic-ui-implementation` | `mcp/contract/**` (consumes it) |

These are shared boundaries, so a change by one lane goes through a `request` to the other:

- `mcp/contract/**` (schemas, AG-UI events, decision and catalog contracts). The backend owns it and the UI consumes it.
- Anything the UI calls on `ipfs mcp`: `/agui`, the decisions endpoint and event stream, `/auth/session`, settings.
- The shared KBD runtime. Each lane transitions only its own phase, with an explicit `--phase`, and never moves the active path for the other lane.
- `.prometheus/**` (append-only learning) and this board, which are append-only for both lanes.

The backend code that the UI phase wrote before this split moves to the backend lane once the UI lane commits it. See the first messages in `log.jsonl`.

## Messages: `log.jsonl`

Append one JSON object per line and never edit an earlier line. To answer a message, post a new line with `reply_to`. Use `board.sh`, which validates each message and stamps the id and time.

| Field | Meaning |
|---|---|
| `id` | `<lane>-<UTC compact time>-<rand>` (set by `board.sh`) |
| `ts` | UTC ISO time (set by `board.sh`) |
| `from` | `backend/<role>` or `ui/<role>`, e.g. `backend/mcp-server-engineer` |
| `to` | `backend`, `ui`, `backend/<role>`, `ui/<role>` or `all` |
| `kind` | One of the kinds below |
| `subject` | One line |
| `paths` | Repository paths involved (may be `[]`) |
| `refs` | OpenSpec change/task ids, commits, PRs, evidence paths |
| `body` | The details: what, why, what is needed from the other lane |
| `reply_to` | The `id` this answers, or `null` |

Kinds:

- `claim`: I am about to write these paths. Post it before editing a path the other lane might touch, including a shared boundary. A claim on the other lane's owned paths needs its `ack` first.
- `release`: I am done with these paths; the work is committed (give the commit in `refs`).
- `request`: I need the other lane to change or decide something on its side, for example a contract field, an endpoint behaviour or a UI expectation.
- `contract-change`: I changed a shared contract. Gives the old and new shapes, the compatibility note and the commit. The consumer answers with `ack` or `blocker`.
- `handoff`: work moves across lanes, with what is done, what remains and the evidence.
- `ack`: accepted, with an optional note.
- `blocker`: I cannot proceed until the other lane acts. States what would unblock it.
- `note`: information only.

## Rules

1. **Read first.** At session start and before every task begins or ends, run `board.sh open` (open requests, claims, blockers addressed to your lane) and act on them before new work.
2. **One writer per path.** Never edit a path the other lane owns or holds a live `claim` on. A claim is live until its `release`.
3. **Commit your own paths only.** Never `git add -A`. Never remove `.git/index.lock`; if it exists, wait. Never stash, reset or revert the other lane's files.
4. **Contracts change by message.** Change `mcp/contract/**` or an endpoint the UI uses only after a `request` and `ack`, or announce it with a `contract-change` and wait for `ack` before relying on it.
5. **KBD boundaries stay in your lane.** Use `prometheus kbd task|change transition --phase <your phase> ...`. `kbd-apply` resolves the active path, so it lands in the wrong phase when the other lane holds it.
6. **No secrets.** Messages never contain tokens, passwords, cookies or Secret values.

## Relation to other memory

- `.prometheus/decisions.md` and `gotchas.md` keep durable decisions and lessons for both lanes. A board message that settles something also gets a line there.
- Karpathy session notes (`.prometheus/knowledge/wiki/karpathy-session-*.md`) stay per-session. They summarise; they do not coordinate.
