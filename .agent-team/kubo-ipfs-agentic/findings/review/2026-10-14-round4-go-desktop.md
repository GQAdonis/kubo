# Review findings: agent-backend-settings Round 4 (mcp-content-extractor, desktop-mcp-sidecar)

Reviewer: `reviewer` (Claude Opus, separate context). Date: 2026-10-14. Method: source reading only (no shell); lint, tests, help-width and the `pickLoopbackPort` HEAD comparison unverified by the reviewer. Written to disk by the lead from the returned report.

**Verdict: FAIL** (B1). A alone: PASS WITH FINDINGS. B: FAIL.

## A. mcp-content-extractor (kubo)

Compatibility holds by inspection: no `/api/v0`, gateway or CID recipe change; fixtures set the CID recipe explicitly (`test/cli/mcp_extractor_test.go:89-91`); no `go.mod` change; schema change additive (`store.go:144-146`, `DEFINE FIELD IF NOT EXISTS ... option<string>`, `omitempty`, empty source counts as native); G12 text path unchanged (`index.go:329-344`, `policy.go:89`, `index_test.go` unmodified); `pickLoopbackPort` refactor looks like a pure extraction (`supervisor.go:270-288`); new help lines at most 77 characters.

- **A1 MEDIUM: contract and server disagree on the skip reason** for "extractor configured, type not allowlisted". Server `index.go:356-365`, `policy.go:75-77` return `kind_not_enabled`; contract `ipfs_index_cid.output.json:31` defines it only as "every stage of the content's kind is off". Fix: amend the contract description (and the `gating` note in `content-kinds.json`) or switch the server to `mime_not_allowed`; pick one.
- **A2 (design opinion): UTF-8 JSON/CSV with no binary signature indexing natively is correct**; rejecting it would de-index content when an extractor is added. Reword task 1.6 and the spec scenario so `kind_not_enabled` applies to binary-signature or non-UTF-8 content; document that an already-indexed JSON CID stays `text`/`text` until `force` or reconcile after `application/json` is allowlisted; record that XML/HTML skip as `mime_not_allowed` while PDF skips as `kind_not_enabled`.
- **A3 MEDIUM: wide CSV/TSV misclassified as text.** `index_kinds.go:221`, `:238-241`, `delimited` `:248-264`: the 4096-byte window cuts mid-record, `ErrFieldCount`, false. Fix: cut back to the last `\n`; test with rows of at least 300 bytes.
- **A4 MEDIUM: name-based detection unreachable in production.** `index.go:352` passes `""`; `mimeByName` (`index_kinds.go:91-102`) is test-only. Code without a shebang, YAML, TOML, Markdown, iWork cannot be detected from a CID. Fix: record that names arrive with the Round 5 bulk tools (directory walk) and add a task there, or accept an optional `path`; note it in the design/README.
- **A5 LOW-MEDIUM: an extractor raises the read size for every file.** `index.go:282-285`, `:294`, `:307`: a 40 MiB log is read fully before `too_large`. Fix: read a small prefix first; use `pol.MaxBytes` for text-like content and `ext.MaxBytes()` only for binary signatures.
- **A6 MEDIUM: a second `ipfs mcp` sharing `cache_dir` kills the first one's xberg.** `extractor_supervisor.go:285-308` reaps by command line without checking the parent; no restart logic. Fix: parent PID in the state file and reap only when the parent is dead, or an exclusive lock file with a clear error.
- **A7 LOW: wrong remedy in the egress error** when the extractor host is missing (`index.go:421-422`). Fix: mention `embeddings.extractor.remote.base_url`.
- **A8 LOW (maintainability): extractor supervisor duplicates the store supervisor** (`extractor_supervisor.go:246-350`; reuses `state.KV`, `processIsStore`). Follow-up, not a blocker.
- **A9 LOW: test fakes written without their mutex** (`extractor_test.go:233`, `:250`; `index_extractor_test.go:278`, `:409-418`); may trip `-race`. Fix: setters that take the mutex.
- **A10 LOW: weak tests.** `TestExtractorSupervisorAvoidsListedPorts` (`extractor_supervisor_test.go:144-158`) is vacuous; the watchdog (SIGKILL) path is only covered behind `IPFS_MCP_TEST_XBERG`. Fix: deterministic avoid-port test; a `test/cli` test with a fake xberg script that SIGKILLs `ipfs mcp` and asserts the child is gone.
- **A11 LOW: pre-change rows emit no `kind`/`source`** (`index.go:210-213`). Fix: emit `text`/`text` when both are empty. Note downgrade behaviour in the changelog.
- **A12 LOW: help text overstates the extractor** (`cmd/ipfs/kubo/mcp.go:147-149`, `:156`). Fix: "types you add to embeddings.policy.mime_allowlist ... are turned into Markdown by xberg; text stays on the native path"; re-run `TestCommandDocsWidth`.
- **A13 NIT:** `min` shadows the builtin (`extractor_supervisor.go:131`); range messages say 1..N but 0 means default (`extractor.go:114-118`); `sniffISOBMFF` maps `M4A ` to `video/mp4` (`index_kinds.go:182`); `assert.Contains` (`test/cli/mcp_extractor_test.go:158`); capture `s.exited` locally (`extractor_supervisor.go:226-229`); possible ST1022 on grouped doc comments (`extractor.go:49-57`, `policy.go:75-81`).

Task acceptance: 1.1 met; 1.2 mostly met (A10); 1.3 met with A3/A4 caveats; 1.4 met (A7); 1.5 met; 1.6 met (A1/A2 open); 1.7 met.

## B. desktop-mcp-sidecar (ipfs-desktop)

- **B1 CRITICAL: tokens and LLM key never reach the child** (`src/daemon/mcp-sidecar.js:320`; `secretEnv` built at `:300-308`, unused). `ipfs mcp` refuses to start (`kubo/mcp/server/auth.go:96-97`) and restarts forever. The spec (`test/unit/mcp-sidecar.spec.js:317-321`, `:349-354`, `:365`) would catch it; the RSS evidence set the tokens itself. Fix: `{ ...process.env, ...extraEnv, ...secretEnv, IPFS_PATH: repoPath }`; run the spec.
- **B2 MEDIUM: `stop()` during port discovery is ignored** (`:436-451`); a child starts after the off switch. Fix: `if (!active) return` after the await; spec for it.
- **B3 MEDIUM: enabling with the daemon stopped starts `ipfs mcp` with no daemon** (`:481`, `index.js:25-28`, `:119-121`). Fix: clear `repoPath` on a daemon-driven stop, or defer `setEnabled(true)` until the daemon runs.
- **B4 MEDIUM: quit does not wait for the sidecar** (`index.js:161-163`, async `before-quit`); the child can survive and hold 5050. Fix: `preventDefault` once, await stop, then `app.quit()`; or synchronous SIGKILL of the tracked PID in `will-quit`.
- **B5 LOW: unbounded stderr buffer** (`:333-341`). Fix: cap at 64 KiB.
- **B6 LOW: spec gaps**: `onDidChange` wiring untested (`:465`), no B2/B3 tests, tight timing margins (`:163-185`).
- **B7 FYI: unbounded restarts**; consider a cap or a tray-visible failing state.

Checked and fine: `settingsStore` declaration order (`:249` before `:250-254`); stop signals only the tracked child; no restart after `stop()`; backoff reset; no secret in store, config or argv; fails closed without safeStorage; fallback port reported; off switch works at start.

Task acceptance: 1.1 met; 1.2 met; **1.3 not met (B1)**; 1.4 met at start (B2/B3 at runtime); 1.5 met.

## Verdict

FAIL until B1 is fixed. One batched correction round: B1, B2, A1/A2 wording, A3, A6, A12; then lint, targeted `go test` and the Desktop spec at the gate.
