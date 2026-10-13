# Operator answers — ipfs-mcp-server

Recorded 2026-10-09 from the operator, in response to the assessment's open questions. These are binding inputs for `/kbd-plan`.

1. **Upstream intent:** fork-only. Talk to upstream after a working result.
2. **Root dependencies:** go-sdk and its modules in the root `go.mod` are acceptable.
3. **Stock Kubo:** operator asked for a recommendation (see below; the plan must state it and its consequences).
4. **Remote HTTP auth:** static bearer token for now. Streamable HTTP must be able to do everything the stdio server can. Design authentication so it can move to **Ory Kratos** (identity) with **Ory Hydra** (OAuth2) later, without reworking tool handlers.
5. **Writes and destructive tools:** wanted, over every transport, each **requiring confirmation**.
6. **Distribution:** release binaries. Plugin marketplaces for **Claude Code, Codex and OpenCode** (Kimi plugin packaging not required this phase).
7. **WebMCP hosts:** immediate support for **Codex desktop and Kimi desktop**.
8. **Housekeeping:** the orchestrator adds the dependency pins to `versions.toml` (blocked by a project permission rule at the time of recording; see the session). **Commit `AGENTS.md` together with all other changes**, not separately (fork-only, so the upstream separate-PR rule does not apply).

## Orchestrator recommendation for question 3 (stock Kubo)

Do not build a separate binary for stock Kubo. Because the decided `ipfs mcp` command never opens the repo and talks only to a daemon over `/api/v0` via `client/rpc`, the fork's `ipfs` binary can serve MCP against any compatible running Kubo daemon, stock or forked (including the daemon IPFS Desktop runs). The plan should: (a) define the minimum daemon version the tools need and detect it at startup via `/api/v0/version`, failing with a clear message below it; (b) degrade per tool when an RPC endpoint is missing rather than failing the whole server; (c) document "use the fork's `ipfs` binary as an MCP client to your existing node" in the release notes. This serves stock-Kubo users without a second artifact and keeps the decision (F4) intact.

## Consequences the plan must carry

- Destructive tools are available on all transports but gated by confirmation. Clients without MCP elicitation support need a defined fallback (for example a two-step confirm token issued by the server), never silent execution.
- "HTTP can do everything" raises the remote-exposure risk the assessment flagged; loopback default, token required for non-loopback, and least-privilege `API.Authorizations` stay mandatory.
- The auth layer is an interface (static bearer verifier now; OAuth2 resource-server verifier with Hydra token introspection and RFC 9728 protected-resource metadata later).
- WebMCP host support for Codex desktop and Kimi desktop is unverified; the plan's first G8 task is a host-support spike against those two hosts, and G8 implementation is gated on it.

## Answers to the G9–G12 addendum (2026-10-09)

Operator answers (binding):

- **Agent architecture (addendum Q1, Q2): Both.** AG-UI and A2A endpoints are served by `ipfs mcp` now (option (a)), reusing its AuthVerifier, scope map and confirmation gate in-process; when an LLM is needed `ipfs mcp` proxies to an operator-configured UAR instance or an OpenAI-compatible server with a key from the environment (L3; no default, off unless configured). UAR-hosted AG-UI/A2A calling `ipfs mcp` as a tool server is a second supported topology once UAR's conformance fixes ship in a published release.
- **Human gate (Q4): native OS prompt.** Out-of-band confirmation for agents that cannot elicit appears as a native dialog (IPFS Desktop window, or a macOS/Linux system prompt) that a background process cannot click. CLI `ipfs mcp confirm` exists only for headless servers behind an explicit opt-in. Shell-capable agent tools are refused for principals with write scope.
- **Chat UI (Q6): Web UI island, Promethity.** React 19 + shadcn chat island in the GQAdonis/ipfs-webui fork (reaching Desktop through its bundled Web UI), Promethity identity, talking to `ipfs mcp`, never directly to UAR.
- **Desktop (Q7): in scope this phase, owned by ui-ux-engineer.** CSP added, `webSecurity` re-enabled, `safeStorage` key bridge; security-reviewer reviews.

Orchestrator defaults for the remaining questions (the operator may overrule):

- **Q3 delivery gate:** the UAR-hosted topology is deferred and reported unmet if UAR's conformance items are not in a published, installable release by the end of Round 4; G9 is still delivered through `ipfs mcp`. No non-conformant release.
- **Q5 egress:** per-principal `egress_ack` naming model and embedder hosts; content-returning tools refused for non-elicitation principals without it.
- **Q8 embedding scope:** opt-in N9 embedding hook (off by default) so content added via stdio hosts, the MCP App, WebMCP and chat can be indexed; pin-aware deletion plus operator-triggered reconciliation.
- **Q9 SurrealDB license:** accepted for self-hosted use (BSL 1.1 restricts offering SurrealDB as a database service); flagged in docs. Operator to confirm if a hosted offering is planned.
- **Q10 remote modes:** G12 requires both embedded and remote SurrealDB (operator's original goal); a remote embedder is supported by configuration, off by default, behind `egress_ack`.
- **Q11 setup threshold:** 30 minutes or 15 steps from published artifacts, as written.


## Round 0 answers (team-ownership-grant 1.1, 2026-10-09)

Binding operator answers:

- **Q1 (F4) and R1 (E6): approved.** MCP ships as `ipfs mcp` in the fork's binary (RPC-only to a running daemon; stdio plus Streamable HTTP, AG-UI and A2A on `127.0.0.1:5050`). `ipfs mcp` performs embeddings and stores them in SurrealDB: embedded mode supervises the operator-supplied `surreal` binary (3.3.0 installed) on loopback; remote mode uses an operator-configured URL.
- **R2 (SurrealDB client): standard-library HTTP client.** No `surrealdb.go` and no new Go modules for SurrealDB in the root `go.mod`.
- **R3 (human gate): fail-closed accepted.** macOS LocalAuthentication (Touch ID/password); Linux requires the operator-installed polkit action, otherwise non-eliciting writes are refused; Windows refuses unless paired with IPFS Desktop. No CLI fallback on Linux beyond the headless `--confirm=cli` opt-in already in the decision.
- **R4 (Desktop repository): confirmed by default.** `tribe-health/ipfs-desktop` at `/Users/gqadonis/Projects/references/ipfs-desktop`, branch `main` (HEAD 9bd9904).
- **R5 (local models): pull `qwen3:8b` for tool-calling chat; `bge-m3` (1024 dimensions) for embeddings.** Real-service tests use these local Ollama models so no content leaves the machine.
- **Q2:** already answered earlier (confirmation required on every transport). **Q3 (Web UI delivery):** decided after the WebMCP host spike.

- **Destructive tool default (2026-10-09, during Execute R1): off by default, enabled explicitly.** `pin_rm`, `mfs_rm`, `key_rm` and `repo_gc` (and any later `destructive`-class tool) are available on every transport but excluded from the default tool set; the operator enables them by naming the destructive group (stdio `--tools` or the `ipfs mcp` config file; per principal for HTTP, AG-UI and A2A). Once enabled, every destructive call still requires confirmation. This confirms ADR-0003 section 2 and refines answer 5 (capabilities wanted, confirmation required) with a default.

## Q3 Web UI delivery (webui-cid-delivery 1.1, 2026-10-12)

- **Q3: D2 chosen by the operator on 2026-10-12, after the WebMCP host probe (Codex GO, Kimi GO with a caveat; `mcp/webmcp/spike/evidence/`).** The fork bumps `WebUIPath`/`WebUIPaths` in `core/corehttp/webui.go` to the CID of its WebMCP-enabled Web UI build, so every fork user's `/webui` changes. The choice is also the approval the proposal requires for the bump. Sequencing: the bump (1.2) and the changelog entry (1.3) wait for that build, which comes from `webmcp-tool-package` and `webui-webmcp-integration`; making the build CID retrievable on the network (pinning or providing it) is an operator action.
