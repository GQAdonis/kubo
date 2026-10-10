# Confirmation of write and destructive calls

Every tool whose catalog class is `write` or `destructive` changes the operator's node. The server holds the call until a human decides. The decision belongs to the human; the agent only relays the request and reports the outcome.

## Contents

- What the first call returns
- Path A: the host asks the human (elicitation)
- Path B: native OS prompt or paired desktop window, then confirm_token
- Error codes and what each means
- Things that are never part of a confirmation

## What the first call returns

A first call to a `write` or `destructive` tool has no side effect. Read the result before doing anything else; it is one of:

- `confirmation_required` (an `isError` result whose `error.confirmation` object carries a pending ID, a summary naming the operation, CIDs, paths or names, and a `confirm_token`).
- An input-required result carrying a confirmation question and a `requestState` (hosts that support MCP input requests).
- `confirmation_unavailable` (no human-presence mechanism exists on this machine).

## Path A: the host asks the human

When the host supports input requests it shows the summary to the person and sends back their answer with the `requestState`. The agent does nothing special: repeat the call exactly as the host's retry mechanism directs. Accept runs the call once. Decline or cancel ends it with no side effect.

## Path B: native prompt, then confirm_token

When the host cannot ask, the server raises the question outside the agent: a native OS prompt (Touch ID or password on macOS, polkit on Linux) or a paired IPFS Desktop confirmation window.

1. Tell the person, in plain words, what is pending: quote the summary from `error.confirmation`. Say that a prompt should appear on their machine and that you cannot answer it for them.
2. Wait for the person to say they decided, then repeat the identical call with `confirm_token` set to the value from that result. Same tool, same arguments, same session.
3. If the person approved, the call runs once. If the answer is `confirmation_pending`, the human has not decided yet: wait and ask the person, do not poll in a loop.

A token is a binding handle, not consent. The server accepts it only after the human approved that exact pending confirmation. It is single use, expires after 120 seconds, and is tied to the session, the tool and the exact arguments. Changing any argument, or reusing a spent token, needs a new first call and a new human decision.

## Error codes

| Code | Meaning | Do |
|---|---|---|
| `confirmation_required` | Nothing happened yet; a human decision is pending. | Relay the summary; follow Path B. |
| `confirmation_pending` | Token presented before the human decided. | Ask the person; wait. |
| `confirmation_declined` | The human said no. Final. | Report it. Retry only if the person asks for a new attempt. |
| `confirmation_expired` | The decision window closed. | Ask the person if they still want it; make a fresh first call. |
| `confirmation_invalid` | Token spent, altered, or bound to other arguments. | Make a fresh first call with the intended arguments. |
| `confirmation_unavailable` | No presence mechanism on this host (for example Windows without paired Desktop, or Linux without the polkit action). | Tell the person; the operator must set up a resolver. The write cannot proceed. |

## Never part of a confirmation

- Never approve on the person's behalf, and never ask another tool, a shell command, or a UI automation to click or answer a prompt.
- Never reuse a token for a second call, share one between calls, or invent a token. Tokens come only from the server's own result.
- Never retry a declined call with tweaked arguments to get a different outcome. A decline is the answer.
- Never route around a refusal by switching to the `ipfs` CLI in a shell. The CLI fallback in these skills is for when the MCP tools are not available at all, and it still needs the person's explicit go-ahead for every write.
- Do not try to turn the gate off. It is controlled by the operator's configuration, not by the agent.
