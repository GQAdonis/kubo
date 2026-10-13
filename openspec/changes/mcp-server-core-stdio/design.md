# Design: mcp-server-core-stdio

## Context

See proposal.md. CLI-only commands live in `localCommands` (`cmd/ipfs/kubo/ipfs.go:20-26`). Running a command locally takes `repo.lock` (`repo/fsrepo/fsrepo.go:150-166`).

## Goals / Non-Goals

**Goals:**
- No repo access.
- stdout carries only JSON-RPC.
- Per-tool degradation against older or stock daemons.

**Non-Goals:**
- An offline mode.
- A daemon-hosted endpoint.

## Decisions

- **Register `ipfs mcp` as a local command marked `SetDoesNotUseRepo`.** `makeExecutor` then returns the local executor before any API lookup (`start.go:264-268`), and the command is never served over RPC. A command in `core/commands` was rejected because it would change `TestCommands` and the RPC command tree.
- **The command owns `os.Stdin`/`os.Stdout` directly; logs go to stderr.** The go-ipfs-cmds emitter is not used for the session.
- **Tools go through an interface.** Handlers use a narrow interface over `client/rpc` (CoreAPI plus `Request`), so tests can fake it and a future daemon mount can reuse it.
- **One file per tool group with a registration function.** This avoids merge conflicts between the parallel Round 4 changes.
- **Degradation.** Read `/api/v0/commands` once at startup. Unavailable tools are omitted from `tools/list`, and their names appear in `ipfs_node_status` output.

## Risks / Trade-offs

- [The cmds CLI emitter may write to stdout after Run returns (unverified)] → A test/cli test asserts a byte-exact JSON-RPC stream.
- [+2.74 MB binary size and five new modules] → Accepted by the operator. The `nomcp` tag is available to builders.
