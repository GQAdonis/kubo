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

`SHA256SUMS` is signed with an SSH key (`SHA256SUMS.sig`, namespace `kubo-mcp-release`). The binaries themselves are not notarized; macOS may quarantine a downloaded binary.

## Verifying a release

You need only `ssh-keygen` (OpenSSH 8.1 or newer). Download the archive, `SHA256SUMS` and `SHA256SUMS.sig` into one directory. Get the release public key from a source independent of this download (for example the repository at a commit you trust); a key delivered with the archives proves nothing. Save it as `allowed_signers`, one line:

    <identity> namespaces="kubo-mcp-release" <keytype> <base64>

Then run both commands; both must succeed:

    ssh-keygen -Y verify -f allowed_signers -I <identity> -n kubo-mcp-release -s SHA256SUMS.sig < SHA256SUMS
    shasum -a 256 -c SHA256SUMS

From a source checkout, `mcp/release/verify.sh [--allowed-signers FILE] [DIST_DIR]` does both. A release built with `KUBO_MCP_UNSIGNED=1` has no `.sig` and is unsigned; `verify.sh --allow-unsigned` checks its checksums only.

## Not included in this release

- `webui-cid-delivery`: Kubo does not pin or deliver Web UI content by CID.
- The native confirmation prompt has not been run live (human gate HG-1 is pending).
- Desktop chat pairing (desktop-agent-chat 1.4 to 1.6) is not delivered.
- The Web UI chat and the MCP App views have not had a live operator run.

This is a local build. Nothing here is tagged, published or uploaded.
