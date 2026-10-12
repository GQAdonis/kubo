# decisions

Append-only. Dated entries. Mark superseded entries; do not delete them.

## 2026-10-09
- Initialized by prometheus-context-bootstrap.
- All work defaults to the `kubo-ipfs-agentic` agent team (6 roles; `.agent-team/kubo-ipfs-agentic/team.json`). Rationale: three parallel streams (Go MCP server, agent skills and plugins, A2UI/MCP App UI) with disjoint ownership, joined by one tool contract in `mcp/contract/`.
- OpenSpec is the change workflow (`openspec/`), initialized for Codex, Claude Code, Kimi Code, OpenCode, MiniMax Code and Zed.
- Kubo rules in `AGENTS.md` win over the managed `prometheus-base` region: pins live in the three `go.mod` files (`make mod_tidy`), and the final gate is the "Before Submitting" sequence plus targeted `test/cli` tests. `versions.toml` covers decisions and the new `mcp/` TypeScript pins only.
- Profile `mixed`: Kimi and MiniMax are in the fleet and unmeasured (see `model-fleet.md`).
- Proposed (not yet decided): MCP server as a separate Go module in `mcp/server` that reaches Kubo only via `/api/v0` through `client/rpc`; destructive tools opt-in. Owner: `ipfs-agent-architect`, via the first OpenSpec change.

## 2026-10-09 (ipfs-mcp-server assessment answers)
- Fork-only; upstream later. go-sdk in root go.mod accepted.
- MCP placement F4 (`ipfs mcp`, RPC-only, stdio + Streamable HTTP) from the adversarial-reviewed assessment; stock-Kubo users served by using the fork's binary as an MCP client to any compatible daemon (no separate binary).
- HTTP does everything incl. destructive tools; every write/destructive call requires confirmation. Static bearer now; auth designed for later Ory Kratos + Hydra (OAuth2).
- Distribution: release binaries; marketplaces for Claude Code, Codex, OpenCode.
- WebMCP: Codex desktop and Kimi desktop first, gated on a host-support spike.
- AGENTS.md is committed together with the implementation (fork-only).
- Pins for versions.toml verified 2026-10-09 (go-sdk 1.8.0, ext-apps 2.0.3, server 2.3.1, @a2ui/react + web_core 0.12.0, react 19.3.0, zod 4.6.5, typescript 7.0.2, vite 8.3.4); written to versions.toml on 2026-10-09 after the operator removed the Edit(versions.toml) deny rule.

## 2026-10-09 Round 7, architect: A2A endpoint reconciliation (mcp-a2a-endpoint)
- Accepted: destructive calls from `a2a` principals need a presence resolver (`cli` alone fails closed). Rationale: ADR-0004 "A2A writes go to the native prompt"; `cli` has no presence factor. ADR-0003/0004 Round 7 amendments.
- Accepted: A2A `error.data` is the 9.5 array (ErrorInfo + `ipfs.mcp.ToolError`); busy/interrupt/full/cap = -32603, terminal follow-up = -32004, cancel finished = -32002.
- Fixture `stream-read.json` corrected to spec 3.1.2 order (Task first); schema enforces it.
- Accepted: cancel-as-deny, retention until context eviction, no text deltas, Host-derived card URL (no `public_url` key until a reverse-proxy deployment is supported).
- Changed: an A2A context whose loop session was evicted is rejected with -32001 instead of silently restarting (A2A 3.4); needs loop change L5.

## 2026-10-10 Operator decisions recorded by the architect
- OQ-EGRESS-ENUM closed: `swarm_peers`, `routing_*` and `key_list` stay `contentEgress: false`; no catalog change. The operator gave no separate rationale; the recorded arguments for no were: they describe the node, not its stored content, and flagging them would refuse the node-status view to every principal without `egress_ack`. Consequence: a principal with read grants on those groups can send peer lists, routing answers and key names to the model backend or an `a2a` peer without `egress_ack`; operators withhold those groups to prevent it. Recorded in `mcp-tool-contract`, `mcp-agui-endpoint`, `mcp-a2a-endpoint` design.md and ADR-0004 (2026-10-10 amendment).
- Web UI chat token stored in browser storage (`localStorage`), replacing memory-only (`webui-agent-chat` 1.4). Operator choice; effect: the token survives reloads instead of being re-entered. Trade-off: any script in the Web UI origin can read it. Mitigations: chat off clears it, a "Forget token" control, Settings copy, and the server-side scope (token authenticates only the `ui` principal; writes still need confirmation). Recorded in `openspec/changes/webui-agent-chat/design.md`.

## 2026-10-11 Operator decisions on the open R11 items, recorded by the architect
- 1. Same-user forgery of the Desktop pairing verifier: accepted as residual risk (decider: operator). Bounds: write-class confirmations only, since `desktop` is not a presence resolver and destructive `ui`/`a2a` calls need one; same OS user only; same-user code can already call the RPC API with the operator's credentials. Off switch: leave `desktop` out of the modes or out of `confirmation.resolvers`. Revisit when the Desktop confirmation window ships. ADR-0003 amendment of 2026-10-11, item 1.
- 2. Web UI chat token stays in `localStorage` (R11 security M6): accepted, no code change. Rationale: one-time entry per browser profile; the token authenticates only the `ui` principal and approves nothing by itself. Risk: any script in the Web UI origin, including other content on a shared gateway origin, can read it. ADR-0005 amendment of 2026-10-11, item 1.
- 3. `confirmation.resolvers`, `confirmation.cli`, `confirmation.ttl`: implement (not remove). Semantics: `resolvers` narrows after mode resolution from any source and can only make the gate stricter, `[]` runs no desktop/native resolver, startup warning when nothing requested is left; `cli: true` enables the `cli` mode unless `--confirm`/`IPFS_MCP_CONFIRM` is given, `cli: false` with `cli` in modes is a validation error; `ttl` default 120s, > 0 and <= 10m, governs pending confirmations, tokens and the confirm channel/CLI, expiry is denial. Work: `native-confirmation-prompt` 1.6 (`mcp-server-engineer`). ADR-0003 amendment of 2026-10-11, item 2.
- 4. Signing `SHA256SUMS` (L5): not decided. Recommendation given: SSH signing (`ssh-keygen -Y sign -n file`, verify with `allowed_signers`), GitHub artifact attestations later if releases move to CI. Awaiting the operator's answer and public key.
- 5. CI under `.github/` (R11-Q05): yes, one new workflow only, upstream workflows unchanged. `.github/workflows/mcp.yml`, OpenSpec `mcp-ci-workflow`; jobs guarded to `GQAdonis/kubo`, `contents: read`, actions pinned by SHA. First run pending.
- 6. Desktop `openExternal` scheme allowlist (L6): no restriction; upstream Desktop code; L6 closed won't-fix by operator decision. ADR-0005 amendment of 2026-10-11, item 2.

## 2026-10-11 SHA256SUMS signing decided (supersedes item 4 of the entry above)
- Operator decision: sign `SHA256SUMS` with SSH (`ssh-keygen -Y sign`, namespace `kubo-mcp-release`) using a new dedicated ed25519 release key, not the operator's login key. Rationale (the architect's reading; the operator gave none): the release key can be rotated or revoked without touching the operator's access keys, and verification needs only OpenSSH 8.1+. Implemented by `skills-plugin-specialist` in `a36a99219`: `mcp/release/signing.sh` (key path from `KUBO_MCP_SIGNING_KEY`; `KUBO_MCP_UNSIGNED=1` builds without a signature), `verify.sh` (`--allowed-signers`, `--allow-unsigned`, `DIST_DIR`), `mcp/release/allowed_signers` (placeholder). Verification steps documented in `docs/mcp/user-guide.md` ("Verifying a release"). Security L5 closed in code. Pending: the operator generates the key; the lead commits its public line. GitHub artifact attestations remain a later option if releases move to CI.
- CI first run 38182586841 (`eae28f59a`): all jobs ok except 32 `electron44` screenshots in Playwright `chat` (classic scroll bars on the runner, 753 px vs 768 px). Remedy chosen by the lead: set overlay scroll bars on the chat job (`a36a99219`) instead of changing baselines or thresholds, consistent with "CI never regenerates the operator's baselines". Re-run 38200585039 pending.

## 2026-10-12 ADR-0002 written as Deferred (operator decision)
- The operator decided to record ADR-0002 (WebMCP tools in the IPFS Web UI) now with status Deferred instead of waiting, because the WebMCP host probe in Codex desktop and Kimi desktop has not run (`webmcp-host-spike` 1.2, 1.3 open; `evidence/codex/` and `evidence/kimi/` empty; no `mcp/webmcp/spike/REPORT.md`). Moves to Accepted per host on a GO result against the four runbook criteria (`document.modelContext` present, `echo` registers, the host agent invokes it, the transcript contains the nonce); Rejected for this phase if both hosts are NO-GO. MCP or MCP App support (`FALLBACK.md`) is not GO. Parked: `webmcp-tool-package`, `webui-webmcp-integration`, `webui-cid-delivery`, and `webui-agent-chat`'s change-level prerequisite on `webui-webmcp-integration`. Consequence: no in-browser agent tools until a host is proven; G8 not delivered this phase. `mcp-architecture-adrs` 1.2 ticked.
