# R11 security review: `ipfs-mcp-server`

- Date: 2026-10-11
- Reviewer: `security-reviewer` subagent, in a separate context, with read-only tools
- Scope: everything since `833e04a24`, plus the trust boundaries of the Web UI and Desktop forks

**Verdict: PASS-WITH-FINDINGS.** The reviewer found no CRITICAL issue. Four HIGH findings each undo a guarantee that ADR-0003 or ADR-0004 claims.

## What holds

- **HTTP listener:** loopback by default, a Host check against DNS rebinding, and an Origin and Sec-Fetch-Site policy. It refuses to bind a non-loopback address without a token.
- **Bearer tokens:** compared as hashes in constant time, and read only from an env var or a file.
- **Confirmation tokens:** confirm tokens and request states use separate keys, and each nonce is single-use.
- **Outbound clients** (LLM, embedder, SurrealDB, UAR) do not follow redirects.
- **Embedded SurrealDB credentials** are kept in the environment and a 0600 file.
- **MCP App views** are read-only, render no HTML, and leave the host's CSP at deny-by-default.
- **Desktop Web UI window:** sandboxed with a strict CSP. The token bridge answers only the main frame, and the token is stored only through safeStorage.
- **AGENTS.md rules:** no tracking, no hardcoded endpoints, no change under `core/`, and no CID injection.

## MCP baseline

| Control | Status | Basis |
|---|---|---|
| MCP-01 per-request auth | PASS | `http.go:390-410` |
| MCP-02 sessions | NEEDS INVESTIGATION | Session binding happens inside the go-sdk, which was not read |
| MCP-03 rate limits | FAIL | No limits exist (M3, M4) |
| MCP-04 schema validation | PASS | `registry.go:92-104`; input schemas use `additionalProperties: false` |
| MCP-05 official SDK | PASS | Built on `modelcontextprotocol/go-sdk` |

## Findings

| id | severity | file:line | finding | failure scenario | suggested fix |
|---|---|---|---|---|---|
| H1 | HIGH | `mcp/server/gate.go:224-226`, `elicit.go:23`, `http.go:233`, `principal.go:30-36` | The gate uses elicitation for any principal kind, and an elicitation approval is always opened with `nativePrompt=false`. | A `ui` principal with `destructive_groups` declares `elicitation:{}`, calls `pin_rm`, and accepts its own prompt. The pin is removed with no native prompt. | Allow elicitation only for `human-mcp` and stdio, and check the kind again in `elicitAnswer`. Send `ui` destructive calls to the native prompt. Add tests. |
| H2 | HIGH | `mcp/server/tools_content.go:542-625`, `access.go:141-172` | `resources/read` on `ipfs://` and `ipns://` bypasses group grants and `egress_ack`. | A principal with only `groups:["node"]` and no `egress_ack` reads up to 16 MiB of content. | In `installAccess`, treat `resources/read` like `ipfs_cat`: require a `content` read grant and apply `egressRefusal`. Add a negative test. |
| H3 | HIGH | `mcp/server/confirm/factor_polkit.go:34,45,62`, `platform_linux.go:9` | `pkcheck` and `pkaction` are looked up on `$PATH`. | A fake `~/.local/bin/pkcheck` that exits 0 approves destructive calls. | Use absolute paths owned by root and not writable by others, and run them with a minimal environment. |
| H4 | HIGH | `confirm/pairing.go:32-50`, `cmd/ipfs/kubo/mcp_confirm.go:79-98`, `confirm/channel.go:143-149`, `broker.go:44-49` | `ipfs mcp pair` needs no TTY or presence check and silently replaces the key. `desktop` counts as a presence resolver, but no Desktop window consumes the key yet. | An agent runs `pair`, keeps the key, and approves its own `pin_rm` over the socket. | Stop counting `desktop` as presence until the Desktop window exists. `pair` should require a TTY and the native factor, and refuse while a server is running. |
| M1 | MEDIUM | `confirm/factor_macos.go:40-43,92` | `/usr/bin/swift` runs with the full inherited environment. `DEVELOPER_DIR`, `TOOLCHAINS` and `SDKROOT` can redirect it. | Setting `DEVELOPER_DIR` in an MCP config's env points it at a fake `swift` that approves everything. | Build `cmd.Env` from an allowlist and strip `DEVELOPER_DIR`, `TOOLCHAINS`, `SDKROOT` and `DYLD_*`. |
| M2 | MEDIUM | `cmd/ipfs/kubo/mcp.go:48-50,152`, `docs/mcp/user-guide.md:74,186` | The docs tell users to pass the RPC secret on argv (`--api-auth bearer:<secret>`). | Another process reads the secret from the process list and calls `/api/v0/pin/rm` directly. | Add `--api-auth-env` and `--api-auth-file`, update the docs, and warn when the secret is on argv. |
| M3 | MEDIUM | `broker.go:225-249`, `confirm/presence.go:57-67`, `gate.go:269-288` | There is no cap on pending confirmations. | A prompt-injected loop floods the user with Touch ID or polkit prompts. | Cap open confirmations per principal and per session, and refuse extras with `confirmation_pending`. |
| M4 | MEDIUM | `agent.go:30,386-416`, `http.go:273-316` | There are no per-principal session quotas or rate limits. Session eviction works across principals. | One `a2a` peer evicts the chat's sessions or spends the operator's LLM key without limit. | Add per-principal quotas and a run rate (429 with Retry-After), and evict only within the same principal. |
| M5 | MEDIUM | `docs/mcp/user-guide.md:131`, `http.go:321-326`, `mcp_http.go:201-206` | The reverse-proxy advice plus anonymous mode exposes unauthenticated reads. | A user follows the TLS proxy docs without setting a token, and anyone who can reach the proxy reads MFS. | Refuse anonymous requests that carry forwarding headers, and change the docs to require a credential behind a proxy. |
| M6 | MEDIUM | `ipfs-webui/src/agent-chat/settings-store.js:2-10,24-34` | The `ui` token is persisted in `localStorage` at the Web UI origin. | Another CID served on the same gateway origin reads the token. | Persist only by opt-in, refuse to persist on `/ipfs/` and `/ipns/` paths, and re-confirm the 2026-10-10 operator decision. |
| L1 | LOW | `a2a_card.go:184-188,203` | The agent card URL comes from the Host header and is publicly cacheable. | Cache poisoning sends A2A clients to the wrong host. | Take the URL from config, or add `Vary: Host` and `private`. |
| L2 | LOW | `agent.go:824` | Neutralizing the `</untrusted-content` tag is case-sensitive. | An upper-case closing tag ends the frame early. | Neutralize case-insensitively, or use a random tag. |
| L3 | LOW | `agent_settings.go:183-196` | `llm.base_url` may be plain `http://` to a remote host while an API key is set. | The API key and node content go over cleartext. | Require https when a key is set and the host is not loopback. |
| L4 | LOW | `confirm/config.go:97`, `channel.go:106-118` | The owner of `/tmp/ipfs-mcp-<uid>` is not checked. | Another user pre-creates the directory and blocks startup. | Check the owner uid. |
| L5 | LOW | `mcp/release/build.sh:73`, `verify.sh:7` | `SHA256SUMS` is unsigned. | Whoever can replace the archives can also replace the checksums. | Sign the checksum file. |
| L6 | LOW | `ipfs-desktop/src/webui/open-external.js:9` (upstream code) | `openExternal` has no scheme allowlist. | A crafted URL scheme reaches an OS protocol handler. | Allowlist `http`, `https` and `mailto`. |

## Not verified

- **Nothing was executed**, and the diff was not computed.
- **M1** is based on known xcrun behavior, not a run on this machine.
- **go-sdk session binding** and the multi-round-trip path for older protocol versions were not read.
- **WebMCP:** no registration code exists in `ipfs-webui/src`.
- **Desktop:** the confirmation window and pairing consumer do not exist yet.
- **Not reviewed:** the chat island bundle, `la_prompt.swift`, the polkit policy file and the A2UI renderer.
- **HG-1** needs a person at the machine.
- **No live UAR instance** was available.
