# Review: Round 6, Go and Desktop (`agent-backend-settings`)

Reviewer: `reviewer` (Claude Opus, independent, read-only, separate context). Date 2026-10-15. Written to disk by the lead from the returned report.
Scope: kubo `mcp-agent-settings-api` 1.8 (ADR-0012), `mcp-whole-node-indexing` 1.9, proxy/transport change, `agent-features-default-on` 1.3 (plus 1.4/1.5 changelog check), docs; ipfs-desktop `desktop-agent-settings-bridge` 1.6-1.7, `agent-features-default-on` 1.2, `desktop-search-sidecar` 1.1-1.6.

Not run by the reviewer: lint, `go test`, race detector, `git diff`, Desktop specs, Playwright. Lead-verified: `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/` ok; Desktop specs under the shim with `NODE_ENV=test` pass; `config_reference.py --check` OK. Lint unverified; `agent-confirm.spec.js` cannot run under the shim.

**Verdict: PASS WITH FINDINGS.** No compatibility break. Two MEDIUM to fix before merge (M1 decision plus docs; M2 Desktop ordering). Rest LOW or nits.

## Compatibility

No `go.mod`, `core/`, `core/commands/`, `client/rpc/` or gateway change visible (confirm with `git diff --stat` before merge). Contract changes additive (`key_not_carried`; `key_sent` description reworded). `v0.44.md` Changelog and Contributors sections empty, so only TOC and Highlights carry content. Upgrade with a stored key: kept at start; probes to previously acknowledged hosts now go without the key (`settings_probe.go:145-157`); the first move clears the key file and answers `key_not_carried` (`settings.go:527-546`); existing `EgressAck` entries now govern content only. Proxy change narrows egress, not a feature or default flip; documented in Highlights (`v0.44.md:47`) and `docs/environment-variables.md:267-272` (see L4).

## Findings

- **M1 MEDIUM: a referenced key still follows a host move; ADR-0012's LKE claim and `agentic.md` are not true of the code.** `settings.go:533-534` withholds only `File == st.keyFilePath()`; `settings_probe_test.go:318-319` and `settings_keymove_test.go:99-106` (`TestSettingsMoveKeepsReferencedKey`) pin it. On LKE (`accept_secrets: false`, key by env or file reference) an admin bearer holder moving `base_url` with a self-supplied `egress_confirmation` sends the mounted key to the new host on every request. The ADR calls this "the sharpest case" and says B leaves the backend keyless (`0012-model-key-scope.md:28`, `:53`) while open point 1 and Consequences (`:76`, `:83`) keep the reference; `docs/mcp/agentic.md:97` says a provider change on such a node is a config edit, which is false. Fix (decision): (a) bind a referenced key to the configured endpoint — resolve with no key after a move away until a config edit or a move back, emit `key_not_carried {cleared: false}`, add an `accept_secrets: false` move test asserting `api_key.set == false`; or (b) amend the ADR and `agentic.md:97` / `chat-setup.md:44` to say a referenced key follows a move and record LKE N2 as an accepted residual. Task 1.8's `accept_secrets: false` test configures no key, so it does not exercise this.
- **M2 MEDIUM (Desktop): a move deletes the stored key before a write that may fail.** `ipfs-desktop/src/agent/settings-service.js:428-435`: `clearLlmKey()` and `restartSidecar()` run before `api.put`; a refused PUT (409 `egress_confirmation_required`, 400, 412, sidecar failure) leaves the old backend keyless. Fix: hold the ciphertext until the PUT settles; on failure restore with `setLlmKey` and `restartSidecar()`, then rethrow; spec for a 409 move leaving the key and old backend keyed.
- **L1 LOW:** `keyWithheldDetail` (`settings_probe.go:64`, comment `:55-57`) still says "neither the current backend nor acknowledged". Reword to "the stored key is sent only to the current backend's host and port".
- **L2 LOW:** probe key decision uses `sameHostPort(HostFromURL(..))` (`settings_probe.go:153`), ignoring scheme and default ports, unlike `backendMoved`/`endpointOf` (`settings.go:651-672`); `https://localhost` vs `http://localhost` would send the key over plain http to another loopback listener. Fix: `if cur != nil && !backendMoved(cur.BaseURL, baseURL)`; table case.
- **L3 LOW (Desktop):** Search host confirmation without Touch ID shows the agent-backend native-dialog copy (`host-confirm.js:48` → `native-dialog.js:27-28` → `en.json:391-395`; caller `search/service.js:140`), not that file content goes to the host (task 1.3). Fix: pass a purpose/detail to `confirmNative`; spec.
- **L4 LOW:** proxy change has no opt-in, and the Highlight sits under whole-node vector search though it also changes the agent-loop model client; "no longer" implies released behaviour though `ipfs mcp` is new in v0.44 (`v0.44.md:47`; `transport.go:11-20`; `llm_client.go:140-141`). Fix: move or duplicate into the agent-loop Highlight, word as new-feature behaviour; optional operator-named per-stage proxy URL covered by `egress_ack` (architect decision).
- **L5 LOW (Desktop):** stale env-allowlist comment on proxy variables (`mcp-sidecar.js:166-175`). Correct it or drop the proxy variables from `ENV_ALLOW`.
- **L6 LOW:** a failed apply after an implicit key clear leaves the running backend keyless (`settings.go:606-613` removes the key file before `ApplyAgentSettings` `:622-625`, against the contract at `:438-440`). Fix: remove the file only after a successful apply.
- **L7 LOW (test):** `desktop-search-sidecar` 1.6's third leg (`/index/status` 404 hides Web UI search) asserted only in a comment (`agent-search.spec.js:185-187`). Accept the composition in `tasks.md` or add a check.
- **Nits:** `test/cli/mcp_defaults_test.go:93` `GreaterOrEqual(...,401)` passes on 500, use `Contains([]int{401,403})`; `index_bulk.go:453-458` duplicate `strings.Trim`; `uarcheck.go:184-191` hand-rolled proxyless transport (use `embeddings.DirectTransport()`); generated Desktop `embeddings` block not validated against the config schema in any spec.

## Test quality

Go tests (`index_round6_test.go`, `settings_keymove_test.go`, `settings_probe_test.go:278-333`) target the regressions, deterministic (hooks, cleanup restores `bulkStopWait`); the widening test pins `[40,160,640,1000]` by design. `test/cli/mcp_defaults_test.go` isolated with OS ports. Desktop: specs per acceptance line; gaps M2, L3; `agent-confirm.spec.js` not run.

## Acceptance

`mcp-agent-settings-api` 1.8 PASS WITH FINDINGS (M1, L1, L2, L6); `mcp-whole-node-indexing` 1.9 PASS; `agent-features-default-on` 1.2 PASS, 1.3 PASS (nit), 1.5 PASS (L4); `desktop-agent-settings-bridge` 1.6 PASS (N2 spec not opened by the reviewer), 1.7 PASS WITH FINDINGS (M2, L3); `desktop-search-sidecar` 1.1 PASS (real Gatekeeper first run open), 1.2 PASS, 1.3 PASS WITH FINDING (L3), 1.4 PASS, 1.5 PASS, 1.6 PARTIAL (L7).

## Still to run at the phase boundary

1. `make -O test_go_lint`.
2. `make build`, then `go test ./test/cli/... -run 'TestMCPStockDefaults|TestMCPBulk|TestMCPIndexSearchRouteDisabled|TestDefaultMatchesExpectedProfile' -v`.
3. `go test -race ./mcp/server/...` (tier 3).
4. `agent-confirm.spec.js` under Electron/Playwright (tier 3).
5. A `prometheus-ui-review` of the Desktop Search window (`src/agent/search/ui/**`).
6. Gateway-conformance CI on the PR.
