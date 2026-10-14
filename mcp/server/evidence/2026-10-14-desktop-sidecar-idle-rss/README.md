# Desktop sidecar idle RSS (desktop-mcp-sidecar 1.5)

One idle-RSS capture of the `ipfs mcp` child as IPFS Desktop launches it.

## Result

Idle resident set size of the `ipfs mcp` child: **42336 KB (about 41.3 MiB)**, steady across five samples taken 6 s apart after the listener was up (first sample 42320 KB).

## Environment

- Binary: `/Users/gqadonis/Projects/references/kubo/cmd/ipfs/ipfs`, `ipfs version 0.44.0-dev`, built 2026-10-14 08:05 local from the `feat/ipfs-mcp-server` working tree at or near `4e71cce72` (not rebuilt for this capture because another builder was editing `mcp/server`). SHA-256 `b211d998d4b61b3a6497500ccef1f3ddaba4a663d52ef86c7ec82adaf2999254`. `ipfs mcp --help` lists `--settings-dir`.
- OS: macOS 26.7.2 (Darwin 25.6.0), Apple Silicon.

## Method

`measure-rss.sh` in this folder:

1. Fresh `IPFS_PATH` in a `mktemp -d` directory, `ipfs init --profile=test`; API, gateway and swarm addresses set to free ports chosen by binding port 0 (this run: API 51600, gateway 51602, swarm 51603). Nothing used 4001, 5001, 8080 or 5050.
2. `timeout 120 ipfs daemon --offline` in the background.
3. `timeout 90 ipfs mcp --http=127.0.0.1:<free port> --agui --confirm=native,desktop --settings-dir=<dir> --mcp-config=<dir>/sidecar-config.json`, the argument shape `src/daemon/mcp-sidecar.js` uses, with a `ui` and an `admin` principal whose tokens come from environment names (random tokens generated per run). No LLM backend configured, so `/agui` answers 503 as in a fresh Desktop install.
4. The child PID was taken as the child of the `timeout` wrapper (`pgrep -P`). An unauthenticated GET of `/mcp` returned 401, confirming the listener was up. RSS read with `ps -o rss=` five times, 6 s apart, starting about 20 s after launch.
5. Both processes were signalled by tracked PID only and the temp directory removed.

## Not measured

The idle state excludes any tool call, chat run, or configured LLM backend; RSS under load is not covered. One run on one machine; no variance estimate. The Electron main-process cost of supervising the child is not included.
