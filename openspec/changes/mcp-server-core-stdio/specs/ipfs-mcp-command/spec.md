# Spec Delta: ipfs-mcp-command

## Purpose

Specifies the `ipfs mcp` command: an MCP server over stdio that acts as a client of a running Kubo daemon's RPC API, without opening the repo.

## ADDED Requirements

### Requirement: Daemon client only
`ipfs mcp` SHALL connect to a running Kubo daemon through `/api/v0` only. It SHALL NOT open or lock the repo.

#### Scenario: Daemon not running
- **WHEN** `ipfs mcp` starts and no daemon API is reachable
- **THEN** it exits non-zero with an error naming the API address it tried, and `repo.lock` is not held

#### Scenario: Daemon starts while ipfs mcp waits
- **WHEN** `ipfs mcp` is running
- **THEN** `ipfs daemon` can start on the same repo

### Requirement: Clean stdio channel
In stdio mode, the command MUST write only MCP JSON-RPC messages to stdout. Logs SHALL go to stderr.

#### Scenario: Initialize over stdio
- **WHEN** a client sends `initialize` followed by `tools/list` over stdin
- **THEN** every byte on stdout parses as a JSON-RPC message

### Requirement: Version gate and per-tool degradation
The server SHALL refuse to start against a daemon older than the contract's global minimum version, with a clear message. It SHALL omit individual tools whose required RPC commands the daemon does not provide.

#### Scenario: Missing RPC command
- **WHEN** the daemon's command list lacks a tool's required command
- **THEN** that tool is absent from `tools/list`, it is reported as unavailable by `ipfs_node_status`, and the other tools work

### Requirement: Capability switches
The command SHALL support disabling tool groups (`--tools`) and a read-only mode (`--read-only`) that exposes only tools with confirmation class `none`.

#### Scenario: Read-only mode
- **WHEN** `ipfs mcp --read-only` is started
- **THEN** no tool with a confirmation class of `write` or `destructive` is listed

### Requirement: Build-time removal
A build with the `nomcp` tag SHALL NOT contain the MCP server. `ipfs mcp` SHALL then report that MCP support was compiled out.

#### Scenario: nomcp build
- **WHEN** Kubo is built with `make build GOTAGS=nomcp` and `ipfs mcp` is run
- **THEN** it exits non-zero with a message that MCP support is not included in this build
