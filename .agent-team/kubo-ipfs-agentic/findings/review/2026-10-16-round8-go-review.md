# Review: Round 8, Go and contract (`8441e9fb4..b9fce98c2`)

Reviewer: `reviewer` (Claude Opus), independent, read-only. No shell: no `git diff`, lint or tests; read HEAD. Lead-written summary keeping IDs, severities and file:line. Lead-verified before the review: unit suites, the `noagent` build tag, the full `TestMCP` CLI family, gofmt, `config_reference.py --check`, `openspec validate --all`, `go.mod` unchanged.

**Verdict: changes requested.** 1 HIGH, 4 MEDIUM, 5 LOW, 2 nits. Nothing breaks `/api/v0`, the gateway or the CID recipe.

## Holds

Lazy store singleflight, `WithoutCancel` plus its own deadline, waiter cancellation not counted, `dropLocked` scope, `synctest` tests, `Service.Close` waits for an open in flight. Daemon wait: injected clock, only `ErrDaemonUnreachable` retried. UAR proxy: resume chaining, bounded hops, deny-all, auto answers never approved, per-principal threads. Provisioning serialized, failures not cached, a 404 re-provisions. `llm.kind` empty means openai (`agent_settings.go:134-141`).

## Findings

- **R8-01 HIGH:** `daemon.wait` never covers a missing api file (`cmd/ipfs/kubo/mcp.go:228-234`, `mcp/server/daemon.go:146-150`). Without `--api`, `Discover` reads `$IPFS_PATH/api` once and returns `ErrDaemonUnreachable` before the wait. `server.go:52-55`, `probe.go:136-137` and `chat-setup.md:46` claim otherwise. The CLI tests all pass `--api` (`test/cli/mcp_daemon_wait_test.go:28,47`). A daemon restarted on another port is never re-read. Fix: discovery inside the wait; a CLI test with no `--api` and no api file. Check whether the LKE sidecar passes `--api`.
- **R8-02 MEDIUM:** the 60 s default changes stock `ipfs mcp` (`config.go:111`, `probe.go:161-163`). Stdio users with `--api` or a stale api file now wait 60 s silently (Info under the default Warn level), which can exceed an MCP host's startup timeout. Fix: the first retry at Warn, or the default only with `--http`; update the changelog.
- **R8-03 MEDIUM:** open UAR interrupts are forgotten on failures that do not settle them, and the UAR thread id is deterministic (`uar_proxy.go:430-434,494`, `uar_backend.go:121-124`, `agent.go:323-325`). A transport error, 401 or 5xx on resume, idle eviction, or a settings apply clears `th.pending` while UAR still holds the interrupt; the next message hits the same `kubo-<sha(principal,thread)>` thread. The comment at `:414-415` is wrong for 401 and 5xx. Fix: clear only after a 200 stream, or add a per-backend generation to `uarThreadID`; test: interrupt, resume answered 502, new message.
- **R8-04 MEDIUM:** stale docs and test for the principal-credential rule (`docs/mcp/config-reference.md:82`, `config-reference.tests.json:26`, `mcp/server/admin_test.go:107-110`; comment `cmd/ipfs/kubo/mcp_http.go:104-105`). Fix: rename the test to `TestAllCredentialsAbsentVerifierAuthenticatesNothing`; reword row 82; add `cmd/ipfs/kubo/mcp_http_principals_test.go:41,54,65` and `test/cli/mcp_principal_unresolved_test.go:15` to rows 79 and 82 and the `.tests.json`.
- **R8-05 MEDIUM:** S5 makes the 1.3 amendment and the candidate-image evidence stale (`openspec/changes/kubo-candidate-image/tasks.md:20-24`, `cmd/ipfs/kubo/mcp_http.go:216-220`). A deployed `--http` config whose principal secrets are not all mounted will crash-loop on the next image. Fix: amend 1.3; before the next image-only PR, a read-only cluster check that every configured principal's env var or file is present.
- **R8-06 LOW:** the ADR-0010 Status still says "Proposed" (`docs/mcp/0010-uar-agent-behind-agui.md:5`, `:43`) against its amendment (`:99-101`). Fix: "Accepted (date), OG-4 met; real-service verification (1.5) open"; point section 3 at the existing disabled tests.
- **R8-07 LOW:** provisioning takes over remote state on a shared UAR (`uar_provision.go:203-230,234-274`). `bindSkills` unbinds every skill kubo did not upload, even with `skills_dir` null; `putAgent` overwrites a same-id agent; the default `kubo-ipfs` collides across nodes; `config-reference.md:101,108` is silent. Fix: document ownership and a per-node `agent_id`, or unbind only ids kubo bound.
- **R8-08 LOW:** a closed lazy store can still serve (`lazystore.go:76-87`: `get` returns `l.inner` before checking `l.closed`); no test of `close()` racing an open. Fix: check `closed` first; a `synctest` case.
- **R8-09 LOW:** `llm.kind` is ignored when `llm.base_url` and `llm.model` are empty (`agent_settings.go:128-141`); `{"llm":{"kind":"uar"}}` or an invalid kind silently disables the agent. Fix: validate `kind` first.
- **R8-10 LOW:** the `withEmbeddings` parameter of `startIndexMCP` is unused (`test/cli/mcp_embedding_index_test.go:104`). Fix: assert the store is available before returning, or remove the parameter.
- **nit-1:** `uar_proxy.go:552-553` relays UAR's `RUN_ERROR` message, contradicting the comment at `:49-51`.
- **nit-2:** `test/cli/mcp_daemon_wait_test.go:30` has a real 2.5 s sleep (harmless).

## Phase gate must run

1. `make -O test_go_lint`.
2. `git diff 8441e9fb4..HEAD --stat -- core/ client/ config/ cmd/ipfs/kubo/daemon.go repo/ plugin/` empty; `go.mod` unchanged in all three modules.
3. The race detector on `./mcp/server/embeddings/...` and `./mcp/server/` for `Lazy|Open|Waiters|Exited|Cancelled|Stage|UAR|DaemonWait|Startup`.
4. `go test ./cmd/ipfs/kubo -run TestHTTPStartup`.
5. `make build`, then the CLI tests `TestDefaultMatchesExpectedProfile|TestGateway|TestTelemetry|TestProvider|TestMCPDaemonWait|TestMCPHTTPStartupFails|TestMCPUARBackendFake|TestCommandDocsWidth`.
6. The R8-01 no-api-file test and the R8-03 and R8-08 tests.
7. `config_reference.py --check` after R8-04.
8. `agent-backend-uar` 1.5 with the opt-in env, or record 1.2, 1.5 and 1.6 open in the gate report.
