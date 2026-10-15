# Review confirmation: Round 5 Go correction (`mcp-media-describer`, `mcp-whole-node-indexing`, settings probe server half)

Reviewer: `reviewer` (Claude Opus, independent, read-only). Date 2026-10-14. One confirmation pass after the batched correction cycle; baseline `2026-10-14-round5-go.md`. Not run by the reviewer: lint, `go test`, race detector, `git diff`. Lead-verified: `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/` ok, gofmt clean, `go.mod` unchanged. Builder CLI: 22 pass, 2 skip. Written to disk by the lead.

**Verdict: PASS WITH FINDINGS.** HIGH-1 and MEDIUM-1-3 resolved; LOW-1-11 resolved or accepted; new LOW items and test gaps, none blocking.

Compatibility holds: only `cmd/ipfs/kubo/mcp_http.go` and new `test/cli` tests outside `mcp/`; bulk RPC set `cat`, `files/stat`, `ls`, `pin/ls` (`catalog.json:1373-1378`); `files stat` on `/ipfs/` goes through `GetApi` (`core/commands/files.go:256`, `:614-618`); cursor additions `pin`/`inside` additive with `omitempty` (`embeddings/runs.go:51-56`), old cursors keep meaning (`index_bulk.go:369-371`, `:455-457`); `key_sent` additive.

## Earlier findings

HIGH-1 RESOLVED (`index_bulk.go:438-461`, `:298`, `:352-356`, `:239-245`; tests `index_round5_test.go:181-221`, `:395-412`, `test/cli/mcp_bulk_test.go:174-212`, `:221-246`, `index_bulk_gate_test.go:180-187`). MEDIUM-1 RESOLVED (`index_file.go:253-254`, `:455-468`; contract and docs agree). MEDIUM-2 RESOLVED (`index_bulk_tools.go:102-139`, `:143-156`; test `index_round5_test.go:463-478`). MEDIUM-3 RESOLVED (`runs.go:44-56`; `index_bulk.go:363-391`, `:312-323`, `:339-350`; tests `index_round5_test.go:237-289`). LOW-1 RESOLVED (`index.go:151-154`, `:198`; `service.go:327-395`; test gap N3). LOW-2 RESOLVED (`server.go:353-357`; `mcp_http.go:140-149`; `test/cli/mcp_bulk_test.go:273`). LOW-3 accepted (`proposal.md:41`). LOW-4 RESOLVED (`contract.go:38-43`; `index_file.go:307-316`; `index_bulk.go:608-613`). LOW-5, LOW-6, LOW-9, LOW-10, LOW-11, Nit-1 RESOLVED. LOW-7 and LOW-8 RESOLVED but untested (fake `ls` ignores options, `bulk_helpers_test.go:173-195`; no listing-failure test). FYI-1, FYI-2 unchanged.

## New findings (none blocking)

- **N1 LOW:** a pause during a file's read or embed counts the file twice (`index_bulk.go:578-579` before `indexResolved`; cursor advances at `:322`). Undo the count and `seen` entry on a context-caused exit; test pausing inside a slow embed.
- **N2 LOW:** a missing narrowed `mfs_root` becomes one `extraction_failed` file and a `completed` run (`index_bulk_tools.go:208-214`; `index_bulk.go:274-276`, `:519-523`). Treat a failed stat of the run root as a run-level outcome.
- **N3 LOW (test):** `TestSearchFilteredWidensUntilTopKPassTheFilter` (`index_round5_test.go:51-89`) never calls `SearchFiltered`; the widening loop (`service.go:380-394`) is untested.
- **N4 LOW (contract):** `TestConnectionResult.detail` described as present "when ok is false" (`settings/test-connection.schema.json:38-42`) but set with `ok: true` on a withheld key (`settings_probe.go:171-174`). Architect amends.
- **N5 LOW (spec):** describer task 1.4 text says "the file result names the host" (`mcp-media-describer/tasks.md:11`); warnings are now fixed codes; the host is named only in the `egress_not_acknowledged` error. Amend the acceptance text.
- **N6 LOW (docs):** embedder, extractor, describer ignore `HTTP(S)_PROXY`/`NO_PROXY` (`embeddings/transport.go:11-15`); proxy-only deployments of the G12 embedder break with no override; `docs/environment-variables.md:237-278` still says Kubo clients honour them. Add an `ipfs mcp` exception note and a changelog Highlights line.
- **N7 LOW:** cancel reports "another bulk run started…" when the run merely did not stop within `bulkStopWait` (`index_bulk.go:156-163`; `index_bulk_tools.go:430-432`).
- **N8 LOW (optional):** `snapshotRun` holds `bulk.mu` across the store read (`index_bulk_tools.go:115-123`).
- **Nits:** `mfsDone` dead in production (`index_bulk.go:463-471`); `embeddings.md:424` error list omits 429; contract `describer_not_configured` omits `max_requests_per_hour: 0`; no spec scenario for the `/index/search` limiter.
- **FYI:** `TestKuboFilesLsOfflineStillFetchesLazyMFSChildren` pins upstream behaviour deliberately; the lazy-copy CLI test relies on harness cleanup after an early `require` failure.

Go style per AGENTS.md followed; ordering deterministic; `bulkCallTimeout` package variable changed only in a non-parallel test.

## Acceptance

`mcp-media-describer`: 1.1-1.3, 1.6, 1.7 PASS; 1.4 PASS with text amendment (N5); 1.5 PARTIAL (gate evidence naming the host). `mcp-whole-node-indexing`: 1.1-1.8 PASS (N1 counting edge). Contract, server, spec and docs agree except N4, N5 and the text nits.

## Phase gate must still run

1. `make -O test_go_lint` (not run this cycle).
2. `make build`, then `go test ./test/cli/... -run 'TestMCPBulk|TestKuboFilesLsOfflineStillFetchesLazyMFSChildren|TestMCPIndexSearchRouteDisabled|TestMCPDescriber|TestMCPExtractor' -v` and `TestDefaultMatchesExpectedProfile`.
3. `go test -race ./mcp/server/...` (tier 3, operator-held).
4. `TestMCPDescriberRealService` against an operator-configured describer (OG-17) with evidence.
5. Gateway-conformance CI on the PR.
