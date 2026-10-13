# IPFS plugin for Claude Code, Codex and OpenCode

One plugin directory, `mcp/plugin/ipfs/`, carries the eight IPFS skills from `mcp/skills` and registers the fork's `ipfs mcp` server (stdio) in three harnesses. Kimi Code is out of scope for plugin packaging; it reads `mcp/skills/<name>/SKILL.md` directly.

```
mcp/plugin/ipfs/
  .claude-plugin/plugin.json   Claude Code manifest
  .mcp.json                    Claude Code MCP server: ipfs mcp
  plugin.json, mcp.json        Codex (Agent Plugins) manifest and MCP server
  opencode/                    OpenCode plugin module, package.json, config snippet
  skills/                      copies of mcp/skills/* (see "Skills")
  alternatives/                optional Streamable HTTP entries, not loaded by default
.claude-plugin/marketplace.json   Claude Code marketplace (repo root)
.agents/plugins/marketplace.json  Codex marketplace (repo root)
```

OpenCode has no marketplace file format; see `SOURCES.md`.

## What the plugin does and does not do

- Launches `ipfs mcp` from `PATH`. It names no URL, port, token or third-party service, and sends nothing anywhere itself. The server finds your node by its normal API discovery (`--api`, then `$IPFS_PATH/api`, then `~/.ipfs/api`).
- Needs a running Kubo daemon. With none reachable, `ipfs mcp` exits at start, and the harness shows the server as failed (observed in all three harnesses). Start `ipfs daemon` first.
- Adds no tools beyond what the server offers by default. Tools that change anything are held for a human decision in your harness or in the server's native or desktop prompt. Destructive tools (`pin_rm`, `mfs_rm`, `key_rm`, `repo_gc`, `pin_update`) are off until you start the server with `--tools destructive`; the plugin never does that. The skills explain these prompts and never route around them.

## Install the binary

Local builds: `mcp/release/build.sh` writes `mcp/release/dist/kubo-mcp_<version>_<goos>-<goarch>.tar.gz`. Extract it and put the `ipfs` binary on `PATH`. This must be the fork's `ipfs`; a stock Kubo has no `mcp` command. Check with `ipfs mcp --help`.

## Claude Code

From a checkout of this repository (or the repository where you host these files; replace `<owner>/<repo>`):

```
claude plugin marketplace add /path/to/checkout       # or: claude plugin marketplace add <owner>/<repo>
claude plugin install ipfs@kubo-ipfs
claude plugin details ipfs                            # lists 8 skills and 1 MCP server
claude mcp list                                       # plugin:ipfs:ipfs ... Connected
```

For a one-off session without installing: `claude --plugin-dir /path/to/checkout/mcp/plugin/ipfs`. Skills appear as `/ipfs:<skill>`.

## Codex

```
codex plugin marketplace add /path/to/checkout        # or: codex plugin marketplace add <owner>/<repo>
codex plugin add ipfs@kubo-ipfs
codex plugin list                                     # ipfs@kubo-ipfs installed, enabled
codex mcp list
```

Codex starts stdio servers with an allowlisted environment. An `IPFS_PATH` exported in your shell did not reach the server in testing (Codex 0.160.1), while the default repo at `~/.ipfs` was found. If your node uses another repo location, use the HTTP alternative below.

## OpenCode

Add the plugin by file URL in your `opencode.json` (use an absolute path):

```json
{ "$schema": "https://opencode.ai/config.json",
  "plugin": ["file:///ABSOLUTE/PATH/TO/mcp/plugin/ipfs/opencode/ipfs-mcp.js"] }
```

The module registers the `ipfs` MCP server and the skills folder, and does nothing if you already defined `mcp.ipfs`. Without the plugin, copy `opencode/opencode.json` into your config and replace the skills path. Then `opencode mcp list` shows `ipfs` connected and `opencode debug skill` lists the eight skills. Publishing `opencode/` as an npm package is an operator action (`package.json` is `"private": true`).

## Streamable HTTP instead of stdio

Run the server beside your daemon: `ipfs mcp --http` (default `127.0.0.1:5050`, path `/mcp`). A loopback listener with no credential serves a read-only anonymous principal. For writes, start it with `--auth-token-env IPFS_MCP_TOKEN` (the flag takes the variable's name; the token never goes in a file here) and give your harness the same variable:

- Claude Code: `alternatives/claude-code-http.mcp.json`. Disable or uninstall the plugin's stdio server first, or the two collide on the name `ipfs`.
- Codex: `alternatives/codex-http.config.toml`.
- OpenCode: `alternatives/opencode-http.json`.

These files are examples; the address is the server's own default and is yours to change.

## Skills

`mcp/skills` is the source of truth. Plugin hosts refuse component paths outside the plugin root, so `mcp/plugin/ipfs/skills` holds real copies. After editing a skill:

```
mcp/plugin/sync-skills.sh          # refresh the copies
mcp/plugin/sync-skills.sh --check  # exit 1 if they differ
```

The OpenSpec delta says "the six IPFS skills"; `mcp/skills` now has eight, and all eight are packaged.

## Verify without touching your own configuration

```
mcp/plugin/verify.sh --ipfs-bin /path/to/ipfs [claude] [codex] [opencode]
```

It runs each harness with `HOME`, `CLAUDE_CONFIG_DIR`, `CODEX_HOME` and the XDG directories pointed at a temporary directory, starts a private offline daemon on ports 4301/5301/8381, stops only that daemon, and writes `evidence/<harness>-isolated-load.txt`. No harness login is used, so Claude Code cannot run a model session here: its check ends at validate, install, details and the MCP health check.
