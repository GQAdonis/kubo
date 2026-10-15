# Security confirmation: agent-backend-settings Round 5 Go correction cycle

Reviewer: `security-reviewer` (Claude Opus, separate context). Date: 2026-10-14. Read-only static reading; no tests run by the reviewer (lead: `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/` ok; builder CLI 22 pass / 2 skip). Written to disk by the lead.

**Verdict: PASS WITH FINDINGS.** H1 fixed with a real-daemon test; M1, M2 fixed; L1, L2, L4, L5, L6 fixed; L3 partial; no new HIGH or MEDIUM. New: 2 LOW, 3 INFO.

## Status of earlier findings

| ID | Status | Evidence |
|---|---|---|
| H1 | RESOLVED | `index_bulk.go:438-461` (`files/stat /` then `/ipfs/<root>/<rel>`; `/ipfs/...` resolves through offline-aware `GetApi`, `core/commands/files.go:251-284`, `:614-626`); offline `ls` without type/size (`:298`); offline `files/stat` (`:352-356`); `not_local` (`:506-512`, `:531-537`); `test/cli/mcp_bulk_test.go:174-212` (lazy copy, source stopped, <60 s, `not_local` 3, `refs local` unchanged) and `:221-246` (pins Kubo's `files ls --offline` fetching); `catalog.json:1373-1378` without `files/ls`; only `tools_mfs.go:107` still calls it. |
| M1 | RESOLVED | `index_bulk_tools.go:340-373`, preflight `:262-271`, summary `:473-480`; test `index_round5_test.go:293` |
| M2 | RESOLVED | `index.go:153-154`, `:198`; `service.go:338-395` (legacy rows as text/text `:340-346`); test `index_round5_test.go:18` |
| L1 | RESOLVED (route) | `index_limit.go:13-73`; `index_routes.go:155-163`; `routes/index.json:104-106`; tests `index_round5_test.go:93`, `:154`. MCP path see N4. |
| L2 | RESOLVED | `index_bulk.go:38-41`, `:239-245`, `:35`, `:399`, `:515-517`; `index_file.go:150`, `:201`; test `index_round5_test.go:395` |
| L3 | PARTIALLY RESOLVED | extractor memo `index_file.go:365-374`, `index_bulk.go:580`; test `index_round5_test.go:416`; interactive budget reservation deferred (optional) |
| L4 | RESOLVED | `snapshotRun` `index_bulk_tools.go:115-123`; `pauseStaleRun` under `bulk.mu` (`:127-139`, `:294`, `:433`, `:147-149`); status and route read-only (`:390-405`, `index_routes.go:231-243`); test `index_round5_test.go:463`; race detector at the gate |
| L5 | RESOLVED | `index_file.go:295-316` (`stage:` + fixed code), log-only endpoint text (`:330`, `:333`, `:359`); `contract.go:38-43`; bulk logs only (`index_bulk.go:608-613`); test `index_round5_test.go:453` |
| L6 | RESOLVED | `cmd/ipfs/kubo/mcp_http.go:140-149` |
| I1 | OPEN (accepted) | `describer.go:462-477`; plain-text rule `routes/index.json:47`, `embeddings.md:422` |
| I2 | PARTIALLY RESOLVED | `directTransport()` `transport.go:11-15` for embedder (`embedder.go:84`), extractor (`extractor.go:233`), describer (`describer.go:227`); store client not (N1) |
| I3 | DEFERRED | not documented under `docs/mcp/` |
| I4 | RESOLVED / accepted | `index_file.go:352-353`, `:356-360`; `embeddings.md:290`, `:365` |

## Server half of the Web UI/Desktop H1 (`settings_probe.go`)

`probeKey` (`:144-158`) attaches the stored key only when the probed host:port equals the current backend (`sameHostPort`, `egress.go:44-48`; explicit vs implicit port fails safe) or `AckCovers(effectiveAck(caller))` (`settings.go:409-411`; `egress.go:91-111`: `local` covers every loopback host and port, bare name every port, `name:port` that port, no wildcards). Key never from the request (`:226-243`); candidate https or loopback http, no userinfo/query/fragment (`:199-219`); no redirects (`:260-262`); echoed key scrubbed (`:296-302`). No `Authorization` reaches an unconfirmed host on test-connection (`:94-95`) or model listing (`:124-125`); header only when the key is non-empty (`:257-259`); tests `settings_probe_test.go:245-276`, `:280-293`; `key_sent` always serialised (`:56`), in the contract (`test-connection.schema.json:34`).

A principal can extend its own ack at the server: any admin bearer holder that sends `egress_confirmation` naming exactly the new backend host with `If-Match` (`settings.go:272-279`, `:524-547`); no server-side human gate; the human step is in the clients (Desktop Touch ID or native dialog; Web UI typed host). The extension happens only with a write that actually moves the backend (`:530-534`, `:526-529`), so the probe gate equals the write gate and the admin bearer is the root of trust.

## New findings

- **N1 LOW: store client honours the proxy env, contrary to the docs.** `embeddings.md:458` says no proxy for the model, store and extractor clients; `newSurrealClient` uses the default transport (`embeddings/surreal.go:57-60`). A remote SurrealDB receives derived text and the store credential. Fix: `directTransport()` in `newSurrealClient` (and the test default), or correct the doc.
- **N2 LOW: a confirmed backend host move also sends the stored key, and the ack outlives the move.** `settings.go:444-477` keeps the key on a `base_url` change; D4 text mentions only node content (`:540-542`); the host stays in `overlay.EgressAck` (`:410`) and later probes keep sending the key (`settings_probe.go:152-155`). Intentional; admin bearer is root of trust. Fix: withhold the stored key on a host change unless the write carries `api_key`, or disclose the key in the confirmation copy (server and both clients).
- **N3 INFO:** ignoring the proxy env is undocumented in `docs/environment-variables.md:239`; the LLM client (`llm_client.go:137`), settings probe (`settings_probe.go:260`) and store client still honour it. Add the exception and make the clients consistent or explain.
- **N4 INFO:** limiter covers only `/index/search`; `ipfs_index_search` over `/mcp` is unthrottled; buckets keyed by configured principal ID (`index_routes.go:155`, `access.go:83-98`), bounded.
- **N5 INFO:** filtered-search cost bounded (one embed, at most four queries of 40/160/640/1000 rows, `service.go:353`, `:380-393`); document that a filtered search can return fewer than `top_k` when matches rank beyond the nearest 1000.

## Other checks (no finding)

Per-call timeouts derive from the run context; a stalled `ls` is logged and skipped (`index_bulk.go:531-543`); the bulk retry loop cannot spin (`describer.go:264-273`); `files/stat --hash /` reads the local MFS root (local cause, no CID injection); no add/pin/write/provide in the bulk RPC set (`catalog.json:1373-1378`; `index_bulk_gate_test.go:180-204`); resume refusals precede confirmation.

## MCP checklist delta

MCP-01 PASS; MCP-02 unchanged; MCP-03 PASS for `/index/search` (N4 INFO for `/mcp`); MCP-04 PASS; MCP-05 PASS. RCE vectors safe; SSRF unchanged with proxy caveats N1, N3. OWASP MCP01 PASS (N2 residual), MCP02 PASS, MCP06 PASS (I1 accepted), MCP07 PASS, MCP10 INFO (I3).

## Manual follow-ups

1. Race detector on bulk and route tests at the phase gate.
2. Decide N2 with the Web UI and Desktop owners (same question as Web UI/Desktop confirm N3 and R1).
3. Fix N1; add the N3 note to `docs/environment-variables.md`.
4. Document I3.
5. Not re-read line by line: `policy.go`, `settings.go` outside the write and ack paths, `store.go` beyond filtered-query callers.
