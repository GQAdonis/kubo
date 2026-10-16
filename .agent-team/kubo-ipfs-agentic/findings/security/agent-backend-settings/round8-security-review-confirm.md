# Security confirmation: Round 8 corrections (kubo `2cddaae4d`, Web UI `cab1b30f`, Desktop `e75e05d`)

Reviewer: `security-reviewer` (Claude Opus), read-only, separate context. No shell: read the trees at the named heads; no tests, no `git show`. Lead-written summary keeping IDs, states and file:line. Paths under `mcp/server/` unless stated.

**Verdict: PASS WITH FINDINGS. No CRITICAL or HIGH open.** U1 and U2 closed; the Round 8 FAIL is lifted. All 15 dispositions hold (U4 and U9 as the lead's recorded residuals). New: 4 LOW, 2 INFO.

| ID | State | Evidence |
|---|---|---|
| U1 HIGH | Fixed | Web UI Save/Clear return early when the node's kind differs (`AgentSettings.js:123,141`); no `kind` passed (`:126`); key saves alone only when kinds match (`core/form.js:252-255`, `ApiKeyField.js:41,83`). Desktop `setApiKey`/`clearApiKey` never forward `kind` (`settings-service.js:640-673`) |
| U2 MED | Fixed | `kindChanged` is `moved` (`settings.go:633,678`), always needs `egress_confirmation` (`:689`), no key across kinds (`:637-649`); Web UI sends the confirmation (`form.js:288`); Desktop `changesKind` is a move with Touch ID or the native dialog, drops the bridge key, rollback restores (`settings-service.js:406-409,520-561,475-488`) |
| U3 MED | Fixed | Allowlisted types and fields only (`uar_proxy.go:63-71,698-713`); CUSTOM, STATE_*, MESSAGES_SNAPSHOT, RAW dropped (`:673-677`); RUN_* written by kubo with the island's ids (`:647-672`) |
| U4 MED | Residual (lead) | Docs say the TTL is advisory and a leaked grant works until rotation (`docs/mcp/config-reference.md:107`, `uar_settings.go:16-21`, `uar_proxy.go:409-416`) |
| U5 MED | Fixed | Plain http refused for the grant unless loopback or `insecure_http_hosts` (`agent_settings.go:199-207`; settings writes `settings.go:197-199,610`); D4 names the grant (`:706-709`); Web strings (`agentSettings.json:172,192`) |
| U6 MED | Fixed | `runAgent` looks up the loop each step and fails the task when gone (`a2a_run.go:42-49`, `:25,215`); `recover` in the task goroutine (`:18,53-61`) |
| U7 LOW | Fixed | Missing or different digest stops provisioning (`uar_provision.go:309-316`); 201 needs `skill_id` (`:293-298`) |
| U8 LOW | Fixed, residual | Scrubber on RUN_ERROR, relayed strings and card text (`uar_proxy.go:622,670,706,782-783`); split or encoded secrets not caught (`:715-718`) |
| U9 LOW | Residual (lead) | Prefix, `uar:` label, `cf_uar_` ids, `node_confirmation:false` (`uar_proxy.go:55,887-902,918-952`) |
| U10 LOW | Fixed | `Lstat`, regular-file check, bounded read (`uar_provision.go:101-144`); Lstat-to-Open swap only by a writer of the operator's directory |
| U11 LOW | Fixed | One deadline per island request (`uar_proxy.go:276-279`); events and bytes budget across hops (`:487-488,631-636`, see N4); provisioning semaphore, 5 s wait, retryable (`uar_backend.go:119-150`) |
| U12 LOW | Fixed | Group- or world-writable binary refused (`embeddings/supervisor.go:173-174`) |
| U13 LOW | Fixed | `uar.key.*` strings exist (`agentSettings.json:175-187`) |
| I1 INFO | Fixed | `redactEndpoint` drops userinfo (`daemon.go:127,168-178`) |
| I2 INFO | Accepted | Bounded by `daemon.wait` |

Checked and passing: interrupt state after a failed resume (only a 200 stream settles; 400/404/409/410/422 abandons to a fresh id; other errors withdraw answers, never replay an approval; kubo's own answers never approvals; `lastRun` advances only on an accepted request; `uar_proxy.go:529-564,618-621,830-845`, `uar_backend.go:163-173`); `mcp_principal` required for `uar` (`uar_settings.go:77-83`); tool allowlist is kubo's tools only (`uar_provision.go:211-213,231-241`); provider required (`agent_settings.go:192-194`); `X-UAR-Admin-Key` only to the settings URL, no redirects, no header in errors (`uarcheck.go:410-428`).

## New findings

- **N1 LOW:** Save key with the same kind and a host move drafted stores the key for the current host (`form.js:252-255`, `AgentSettings.js:118-127`); B starts with no key after Apply. Predates Round 8 in the OpenAI flow. Fix: `keySavesAlone` false when `hostChanged`.
- **N2 LOW:** the grant follows a uar-to-uar host move, or a first UAR backend, onto a host the admin's `egress_ack` covers, with no typed confirmation (`settings.go:689`, grant notice inside that branch `:706-709`). Fix: require confirmation whenever a `uar` backend with a grant gets a new host.
- **N3 LOW:** a kind change drops a config-referenced key silently: `dropAny` clears it but `wasSet` looks only at the stored key, so no `key_not_carried` (`settings.go:637-649`); Web server-managed copy says the server key follows (`agentSettings.json:189`).
- **N4 LOW:** the relay budget counts only `data:` lines (`uar_proxy.go:625-633`); a peer can stream non-data lines until the wall clock ends. Fix: count every line.
- **N5 INFO:** `DiscoveringDaemon` re-reads the api file after each unreachable error and sends the RPC `Authorization` to the new address (`daemon.go:162-164,200-225`). Option: with `--api-auth`, refuse a re-read non-loopback address.
- **N6 INFO:** after a reopen, the island gets RUN_ERROR but no interrupt to answer; new messages refused until answered or idle TTL (`uar_proxy.go:535-537,568-575,305-307`). Fails closed.

Not verified: tests; `git diff` for `/api/v0` and the gateway; real UAR wire shapes; how the Desktop sidecar receives the bridge key.

## Lead disposition

N1 is fixed in the operator-approved targeted Web UI correction. N2-N6 carried as `agent-backend-uar` 1.7.

Update (agent-backend-uar 1.7): N2, N3, N4 and N5 are fixed in 1.7, as are Go review N1 (a failed resume withdraws only the answer to the interrupt shown at the failure, plus the denials it filled in; earlier answers stay and the shown interrupt stays answerable) and nit-1 (no UAR RUN_ERROR message is relayed unscrubbed). N6 is rebutted: after a failure once the stream has started, the interrupts are settled; a failure before the stream starts is an HTTP 502, and Go N1 keeps the shown interrupt answerable. No RUN_ERROR field was added.

