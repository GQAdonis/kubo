# Proposal: native-confirmation-prompt

Phase `ipfs-mcp-server`, plan revision 2 (new change). N14: native confirmation prompt with a presence factor (human gate). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

Operator answer (Q4): agents that cannot elicit get their writes confirmed by a native dialog that a background process cannot click (an IPFS Desktop window, or a macOS or Linux system prompt). A CLI confirm exists only for headless servers behind an explicit opt-in. The adjudicated finding showed that a 0600 socket or a loopback page alone admits any same-user process.

## What Changes

- Clients that can elicit keep elicitation. For every principal that cannot elicit, writes and destructive calls wait in a broker (pending IDs, summaries naming tool, CID, path or name and effect; expiry is denial; exactly-once execution).
- Resolvers with a presence factor a same-user background process cannot simulate: the IPFS Desktop confirmation window when Desktop is running and paired over a local authenticated channel (Desktop side in N6); otherwise a macOS LocalAuthentication prompt (Touch ID or password) or a Linux polkit authentication-agent prompt raised by `ipfs mcp`. Mechanisms are verified on real machines first and recorded in `mcp/server/confirm/MECHANISMS.md`.
- `ipfs mcp confirm <id>` exists only for headless servers behind `--confirm=cli`; it requires a TTY and prints, at startup and per request, that same-user processes can reach it.
- Where no resolver is available, writes from non-eliciting principals are refused (`confirmation_unavailable`).
- Falsifier HG-1: an agent with shell access and a plain same-user script try to approve through AppleScript/System Events, `xdotool`, the control socket and the Desktop channel; any success blocks that mechanism.

## Capabilities

### New Capabilities

- `mcp-out-of-band-confirmation`: N14: native confirmation prompt with a presence factor (human gate)

### Modified Capabilities

None.

## Impact

`mcp/server/**`, `cmd/ipfs/kubo/mcp*.go` (`ipfs mcp confirm`/`deny`), `test/cli/mcp_*_test.go`. No daemon change. Linux polkit needs a one-time operator-installed action file (documented, never installed by Kubo).
