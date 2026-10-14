# Confirmation review: agent-backend-settings Round 4 (mcp-content-extractor, desktop-mcp-sidecar)

Reviewer: `reviewer` (Claude Opus, separate context). Date: 2026-10-14. One confirmation pass after the batched correction cycle; earlier findings in `2026-10-14-round4-go-desktop.md`. Method: source reading only. Written to disk by the lead from the returned report.

Lead-verified facts accepted: sidecar spec 38/38 under the plain-node shim; embeddings and contract unit tests pass; `./mcp/server` fails only `TestEveryCatalogToolHasAnImplementation`; gofmt clean; `go.mod` unchanged. Not verified by anyone: `make -O test_go_lint`, `TestCommandDocsWidth`, the race detector, `TestMCPExtractorFakeXbergIsReapedAfterSIGKILL`, a real Electron quit, Playwright.

**Verdict: PASS WITH FINDINGS.** No CRITICAL or HIGH. One MEDIUM wording fix required before archive (N1).

Compatibility holds by inspection: nothing under `core/`, `client/rpc/`, the gateway or `config/`; only help text in `cmd/ipfs/kubo/mcp.go:146-162`; store DDL additive (`store.go:144-146`); config additive (`max_text_bytes` 0 = default, `principals.go:141`).

## A. mcp-content-extractor

| ID | Status | Evidence |
|---|---|---|
| A1 | RESOLVED | `ipfs_index_cid.output.json:31`, `content-kinds.json:118`, `design.md:7-8`, `spec.md:35-43`, `embeddings.md:265-269`; server `index.go:440-448` (see N3) |
| A2 | RESOLVED | `tasks.md:13`, `index.go:441-447`, `content-kinds.json:118`, `design.md:8`; tests `TestPlainTextOutsideTheRefinedAllowlistEntryStaysNative`, `TestExtractorWithAnUnlistedTypeIsKindNotEnabled` |
| A3 | RESOLVED | `index_kinds.go:251-263`; tests `index_kinds_test.go:51-53` |
| A4 | DEFERRED | `mcp-whole-node-indexing` task 1.8, `design.md:13`, `content-kinds.json:118` |
| A5 | RESOLVED | `index.go:392-409`; `TestTextKeepsPolicyMaxBytesEvenWithAnExtractor` |
| A6 | PARTIALLY RESOLVED | Unix flock (`extractor_cachedir_unix.go:41-57`, `extractor_supervisor.go:90`); Windows no-op (`extractor_cachedir_windows.go:12`), undocumented (N5) |
| A7 | RESOLVED | `extractor.go:356`, `index.go:503-508`, test `index_extractor_test.go:580` |
| A8 | OPEN (accepted follow-up) | duplication and `state.KV`/`processIsStore` reuse (`extractor_supervisor.go:301-307`, `:389`); needs a recorded follow-up |
| A9 | RESOLVED | `extractor_test.go:84-99`, `index_extractor_test.go:26-181`, `:386`, `:624` |
| A10 | RESOLVED | `TestPickPortSkipsListedPorts` (`extractor_supervisor_test.go:167-182`); `test/cli/mcp_extractor_fake_test.go:86-142` (see N6) |
| A11 | PARTIALLY RESOLVED | `index.go:243-255`, `TestRowsWithoutKindOrSourceAreReportedAsText`; downgrade behaviour undocumented |
| A12 | RESOLVED | `cmd/ipfs/kubo/mcp.go:146-162`; widest lines land exactly at 80 by hand count (not run) |
| A13 | PARTIALLY RESOLVED | fixed `minVer`, range messages, M4A, local `exited`; unchanged `assert.Contains` at `test/cli/mcp_extractor_test.go:158` and grouped `ErrX:` docs (lint not run) |

## B. desktop-mcp-sidecar

| ID | Status | Evidence |
|---|---|---|
| B1 | RESOLVED | `mcp-sidecar.js:416`; spec `:488`, `:523` |
| B2 | RESOLVED | `:578-588`, `:476-478`; spec `:447`, `:462` |
| B3 | RESOLVED | `:596-599`, `:616`; spec `:671` |
| B4 | RESOLVED | `index.js:166-184`, `mcp-sidecar.js:623-637`; spec `:778-831`; real Electron quit not exercised |
| B5 | RESOLVED | `:24-25`, `:394-396`, `:440-445`; spec `:198` |
| B6 | RESOLVED | `:841`, `:856`, `:249`, `:296` |
| B7 | PARTIALLY RESOLVED / DEFERRED | restart cap `:22`, `:483-487`; `failing` state `:642-646`; tray status deferred to `desktop-agent-settings-bridge` 1.3 |

## Task acceptance re-checked

`mcp-content-extractor` 1.1-1.7 met (1.2's SIGKILL path via the unrun CLI test; 1.5 runs at the gate). `desktop-mcp-sidecar` 1.1-1.3 met; 1.4 acceptance met, tray toggle deferred to `desktop-agent-settings-bridge` 1.3 (until then the off switch is flipped by editing the store); 1.5 unchanged.

## New findings

- **N1 MEDIUM (required): `max_text_bytes` described as truncation.** Schema `ipfs-mcp-config.schema.json:838` and `docs/mcp/embeddings.md:245` say "truncated at a UTF-8 boundary"; the server discards and skips `too_large` (`extractor.go:63-65`, `:484-485`, `index.go:470-471`). Fix the schema description and the docs row.
- **N2 LOW:** `too_large` description (`ipfs_index_cid.output.json:31`, `embeddings.md:267`) omits `extractor.max_text_bytes` and the 2000-chunk cap.
- **N3 LOW:** "(or is not UTF-8)" cannot trigger; unknown binary goes native and skips `mime_not_allowed` (`index.go:436-438`, `index_kinds_test.go:56`). Reword to "content of a listed kind with a binary signature" in `ipfs_index_cid.output.json:31`, `content-kinds.json:118`, `spec.md:35`, `tasks.md:13`, `embeddings.md:268`.
- **N4 LOW:** proxy refusal status timing-dependent (`extractor_proxy.go:166-167`, `:188-195`); block on the channel after `hc.Do` fails.
- **N5 LOW:** Windows has no `cache_dir` lock and still reaps; document Unix-only at `embeddings.md:238` or skip reaping when the recorded parent is alive.
- **N6 LOW:** fake-xberg CLI test does not clean up its xberg or surreal children on failure; add `t.Cleanup` SIGKILL of tracked PIDs.
- **N7 LOW:** Desktop `ENV_ALLOW` (`mcp-sidecar.js:151-156`) drops `DO_NOT_TRACK` and `IPFS_TELEMETRY`; allowlist both.
- **N8 NIT:** `extraEnv` can override `IPFS_PATH` (`mcp-sidecar.js:416`).
- **N9 NIT:** misplaced doc comment (`index_kinds_test.go:68-86`).
- **N10 NIT:** `cache_dir/huggingface` created before the ownership check (`extractor_supervisor.go:84-89`).
- **N11 NIT:** comment at `mcp-sidecar.js:18-21` overstates retry after `failing`.

Checked and fine: proxy token, Host and Origin checks, field filtering, no redirects; lock fd close-on-exec; `waitHealthy` refuses a foreign responder; https required for non-loopback remote; `BackendHosts` adds only a remote extractor; presence check offline and read-only; store and config backward compatible.

## Verdict

PASS WITH FINDINGS. Fix N1 before archive; record A8. N2-N7 can ride the next change. The phase gate runs, against a `make build` binary: `make -O test_go_lint`; `go test ./test/cli/... -run 'TestCommandDocsWidth|TestMCPExtractorDisabled|TestMCPExtractorFakeXbergIsReapedAfterSIGKILL'`; the race detector on `./mcp/server/...`.
