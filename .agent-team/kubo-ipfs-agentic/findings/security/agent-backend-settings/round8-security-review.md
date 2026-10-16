# Security read: Round 8 (UAR backend, 1.6 fixes, Web UI UAR form)

Reviewer: `security-reviewer` (Claude Opus), read-only, separate context. No shell; read the HEAD tree of `feat/ipfs-mcp-server` (`b9fce98c2`) and the Web UI working tree file by file; no tests run. Lead-written summary of the reviewer's report; IDs, severities and file:line kept. Paths under `mcp/server/` unless stated.

**Verdict: FAIL until U1 and U2 are fixed (1 HIGH).** Kubo side alone: PASS WITH FINDINGS. Totals: 1 HIGH, 5 MEDIUM, 6 LOW, 2 INFO.

## Findings

- **U1 HIGH:** Web UI "Save key" on the UAR form switches the running backend to `uar` at the current OpenAI host with no typed confirmation and sends the new credential there. `core/http-client.js:141-146` sends `{backend:{api_key, kind}}`; `AgentSettings.js:124` passes `kind:'uar'`; the field is open because `serverBackendOfDraft` is undefined across kinds (`core/form.js:205`, `ApiKeyField.js:41-42,67,83`); the client confirmation guards Apply only (`form.js:259-276,316-323`). Server: the write sets `next.kind` (`settings.go:536-540`); D4 compares host and scheme only (`:654-677`); the next chat sends `GET /.well-known/agent-card.json` with `Authorization: Bearer <UAR key>` (`uar_client.go:93-95,157-158`). Scenario: the UAR JWT goes to a third-party model provider and the stored OpenAI key file is overwritten. Fix: Save key never sends `kind`; kind changes only through Apply; server side U2.
- **U2 MEDIUM:** D4 and the ADR-0012 key-scope rule do not treat a kind change as a move (`settings.go:619-638`, `backendMoved` `:848-855`, `:654-677`). `{"backend":{"kind":"uar"}}` sends the API-stored key and the `llm.uar.mcp_principal` bearer to a host confirmed only as a model host. Fix: kind changed = moved (require `egress_confirmation`); do not carry a stored key across kinds.
- **U3 MEDIUM:** the proxy relays every AG-UI event type (`uar_proxy.go:561-563`), including CUSTOM `a2ui` (the island renders it, `mcp/ui/src/chat/agui.ts:237-249`, `mcp/chat/src/island/ToolCallSurface.tsx:56,75-77`) and `MESSAGES_SNAPSHOT` (`agui.ts:234-235`); contradicts `docs/mcp/agentic.md:104`; `threadId`/`runId` not rewritten (contradicts `README.md:133`). Fix: allowlist `TEXT_MESSAGE_*`, `TOOL_CALL_START/ARGS/END/RESULT`, `RUN_STARTED`; drop CUSTOM, STATE_*, MESSAGES_SNAPSHOT; set the island's thread and run ids.
- **U4 MEDIUM:** the "grant" is the uar principal's static bearer (`uar_proxy.go:360-378`); `expires_at_unix` is advisory (`config.go:301-312`); `config-reference.md:107` claims a short `mcp_grant_ttl` limits a leaked grant. Fix: mint a short-lived token verified by `ipfs mcp`, or correct the docs and `uar_settings.go:16-18`.
- **U5 MEDIUM:** the grant can travel over plain http: the refusal at `agent_settings.go:160-162` covers only `APIKey`. D4 copy names only the model key (`settings.go:689-694`, `referencedKey` `:840-842`); Web `agentSettings.json:173` ("stays on the node") and `confirm.help` (`:179`). Fix: extend the plain-http refusal to `MCPCredential`; name the kubo MCP grant in D4 and `confirm.help` for `uar`; rewrite `uar.agent.text`.
- **U6 MEDIUM (availability):** A2A tasks can dereference a nil `Agent` after a switch to UAR (`a2a.go:146` checks at request time; `a2a_run.go:24`, `:184` call `h.srv.Agent().Run` later; `Agent()` is nil while `uarExt` is set, `agent.go:301-302`; no recover in `start`, `a2a.go:104-107`); the process exits. Fix: capture `*Agent` at request time, or nil-check and reject.
- **U7 LOW:** a pre-existing same-name skill on UAR is bound in place of kubo's (`uar_provision.go:167-181`; a missing `artifact_digest` is accepted). Fix: fail closed on a missing or different digest, or node-scoped names.
- **U8 LOW:** relayed `RUN_ERROR` and text are not scrubbed of the UAR key or the grant (`uar_proxy.go:551-553,561-563`; `probeDetail` does scrub, `settings_probe.go:327-333`). Fix: replace both secrets in every relayed string.
- **U9 LOW:** a UAR approval card looks like a kubo confirmation (`cf_` id, `class: write`, peer-chosen `serverName`; `uar_proxy.go:624-630,696-726`). Fix: a distinct label or reason; drop the `cf_` id.
- **U10 LOW:** a `SKILL.md` symlink is followed (`uar_provision.go:71`). Fix: `Lstat` and refuse non-regular files, or keep `EvalSymlinks` under `skills_dir`.
- **U11 LOW:** the stream bounds are looser than they look: a fresh `WallClock` per hop (`uar_proxy.go:428-429,478`, up to 6x); no total byte cap; `provMu` held across all provisioning calls (`uar_backend.go:93-105`). Fix: one deadline per island request, a byte budget, non-blocking provisioning.
- **U12 LOW:** `surreal_binary` has no group- or world-writable check (`embeddings/supervisor.go:160-171`; compare `factor_polkit.go:67`, `extractor_cachedir_unix.go:33`). Fix: refuse `mode & 0o022`.
- **U13 LOW (Web UI copy):** the `uar.key.*` strings were missing (`ApiKeyField.js:40,57,72,103,113`). Lead: the UI correction added them after this read.
- **I1 INFO:** the `daemon.wait` Info line logs `d.Endpoint()` on each attempt; an `--api` URL with userinfo is kept verbatim (`probe.go:162`, `daemon.go:124`).
- **I2 INFO:** a 4xx with no Content-Type is treated as unreachable and retried within the wait (`client/rpc/response.go:105-107`); bounded.

## Holds

- Confirmation gate: `mayElicit` human-mcp only (`gate.go:189-191`); `uar` never accepts tokens (`principal.go:38-40`); the AG-UI resolver fits `ui` only (`gate.go:115-119`), so a chat approval only answers UAR and cannot approve a kubo write.
- Interrupt translation: auto-answered interrupts never approved (`uar_proxy.go:632-646`); deny-all on deny (`:293-302`); no replay (`:430-434`); a resume must match (`:279-286`); duplicates refused (`:585-587`); 16 interrupts, 6 hops; per-owner threads (`uar_backend.go:121-124`); SSE re-marshalled (`agui_stream.go:62-66`).
- Client: no redirects or proxy, status-only errors, the grant only to the configured base.
- Settings: UAR test-connection sends no credential; `validateCandidateURL` applies; the stored key is cleared on a host move; the overlay persists `kind`.
- A2A answers 503 while UAR is the backend (race: U6). The 1.6 fixes all hold.
- Web UI: the key draft lives in React state only; admin token handling unchanged; host confirmation shown (gap: U1).
- AGENTS.md: no tracking, no default UAR endpoint, `llm.kind` defaults to openai, no CID injection.

## Not verified

Real UAR wire shapes; the Desktop bridge's `setApiKey` handling of `kind` (applies to U1); `git diff --stat` for `/api/v0` and the gateway; no tests run; the real-service test has not run.
