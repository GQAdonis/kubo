# Kubo MCP fork @VERSION@

Local build of the fork's `ipfs` binary with the `ipfs mcp` command, built from commit `@COMMIT@` (Kubo `@NUMBER@`). Built with `make build`; `/api/v0`, the HTTP gateway, the default CID recipe and wire behavior are unchanged.

## Use it with your existing node

You do not have to replace your Kubo daemon. Keep running stock Kubo and use the fork's `ipfs` binary as an MCP client to your existing node:

    ipfs mcp --api /ip4/127.0.0.1/tcp/5001

`ipfs mcp` never opens your repo or takes `repo.lock`. It calls the daemon's RPC API only. With no reachable daemon it exits with an error that names the address it tried.

- Minimum daemon version: Kubo @MINVERSION@. Below that the server refuses to start.
- Per-tool degradation: when the daemon lacks an RPC command a tool needs, that tool is left out of `tools/list` and `ipfs_node_status` reports it as unavailable. Newer tools may be missing on older daemons; the rest keep working.
- Writes and destructive tools ask a human to confirm every call. Destructive tools are off until you name `destructive` in `--tools`. A client that cannot show the confirmation gets a refusal and nothing changes.
- If your RPC API requires authorization, pass `--api-auth` with a dedicated secret limited to the enabled tools (`--print-rpc-allowed-paths` prints a matching entry).

See `docs/mcp/` in the source tree and `mcp/README.md`.

## Archives

| File | Platform |
|------|----------|
| `kubo-mcp_@VERSION@_darwin-arm64.tar.gz` | macOS, Apple silicon |
| `kubo-mcp_@VERSION@_darwin-amd64.tar.gz` | macOS, Intel |
| `kubo-mcp_@VERSION@_linux-amd64.tar.gz` | Linux x86-64 |
| `kubo-mcp_@VERSION@_linux-arm64.tar.gz` | Linux arm64 |
| `kubo-mcp_@VERSION@_windows-amd64.zip` | Windows x86-64 |

Verify with `shasum -a 256 -c SHA256SUMS` before running. These binaries are not signed or notarized; macOS may quarantine a downloaded binary.

## Not included in this release

- `webui-cid-delivery`: Kubo does not pin or deliver Web UI content by CID.
- The native confirmation prompt has not been run live (human gate HG-1 is pending).
- Desktop chat pairing (desktop-agent-chat 1.4 to 1.6) is not delivered.
- The Web UI chat and the MCP App views have not had a live operator run.

This is a local build. Nothing here is tagged, published or uploaded.
