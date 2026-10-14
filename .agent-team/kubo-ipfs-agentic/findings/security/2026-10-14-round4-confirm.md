# Security findings: agent-backend-settings Round 4, confirmation pass

Reviewer: `security-reviewer` (Claude Opus, separate context). Date: 2026-10-14. Method: static reading only; no tests, binaries or network captures run. Written to disk by the lead from the returned report.

**Verdict: PASS WITH FINDINGS.** B1 fixed; nothing at HIGH or CRITICAL remains. One MEDIUM (C1: the documentation of the accepted A1 residual understates it). Other new items LOW or INFO.

## Status of earlier findings

| ID | Earlier | Status | Evidence |
|---|---|---|---|
| A1 | MEDIUM | Proxy sound; residual accepted by the operator ("accept the xberg residual risk, keep going", 2026-10-14) but under-documented (C1) | `mcp/server/embeddings/extractor_proxy.go:69-138`, `extractor.go:258-263`, `docs/mcp/embeddings.md:275-277` |
| A2 | MEDIUM | RESOLVED | `mcp/server/index.go:200-213`, `:265-278`; test `index_extractor_test.go:400` |
| A3 | MEDIUM | RESOLVED | `extractor.go:37`, `:131-133`, `:484-485`; `index.go:41`, `:470-471`, `:488-493`; `io.Pipe` `extractor.go:392-397`; schema `ipfs-mcp-config.schema.json:833` |
| A4 | LOW | RESOLVED in code; network capture outstanding | `extractor_supervisor.go:262-265`; `embeddings.md:249`, `:263` |
| A5 | LOW | PARTIALLY RESOLVED | `extractor_supervisor.go:331-341`; proxy binds port 0; xberg pick-close-bind remains (`:227-231`); no post-start watch (C3) |
| A6 | LOW | RESOLVED on Unix; no-op on Windows (C4) | `extractor_cachedir_unix.go:22-56`; `extractor_supervisor.go:87-99` |
| A7 | LOW | RESOLVED | `extractor.go:166-171`; `settings.go:219-233` |
| A8 | LOW | RESOLVED (docs) | `embeddings.md:271-273` |
| A9 | LOW | RESOLVED | `catalog.json:1272` |
| A10 | INFO | OPEN (optional, accepted) | lower-bound version check, no hash pin |
| B1 | HIGH | RESOLVED | `ipfs-desktop/src/daemon/mcp-sidecar.js:416`, `:165-169` |
| B2 | MEDIUM | PARTIALLY RESOLVED (latent: no `getPort()` consumer yet) | `mcp-sidecar.js:382-391`, `:394-407`, `:470`, `:483-487`; see C2 |
| B3 | LOW | RESOLVED | `mcp-sidecar.js:151-175`; gaps in C5 |
| B4 | LOW | RESOLVED | `mcp-sidecar.js:24-25`, `:394-396`, `:440-445` |

## Addendum: operator decision on A1 (2026-10-14)

- A1 moves to **ACCEPTED RESIDUAL** by operator decision ("accept the xberg residual risk, keep going", recorded in the phase `execution.md`), conditional on the documentation stating the risk honestly. The open item is only the wording in `docs/mcp/embeddings.md` (C1).
- The proxy stays sound; C3 (LOW) and C6 (INFO) still apply.
- C1 stays MEDIUM as a documentation finding: a page that finds xberg's port can make xberg fetch URLs (`urls`; RFC1918, link-local and `169.254.169.254` unverified), wipe the pre-warmed model cache (`DELETE /cache/clear`), and read every response after DNS rebinding (no `Host` check). Once that text lands, C1 is resolved and A1 is fully accepted.

## A1 proxy soundness: yes

32-byte `crypto/rand` token, `subtle.ConstantTimeCompare` on the full `Bearer <token>` (`extractor_proxy.go:70-81`, `:117-118`); `Host` must equal `127.0.0.1:<port>` and any `Origin` is refused before auth (`:113-116`); only `POST /extract` without a query (`:123-131`); only `files`, `output_format=markdown`, `config` (≤4 KiB) forwarded, duplicates and other fields refused, refused parts never written upstream, filename re-sanitised (`:200-249`); `MaxBytesReader` at `max_bytes + 1 MiB`, streamed through `io.Pipe`, no goroutine leak, capped relay, only `Content-Type` copied; no token forwarded (`:156-162`); redirects not followed; `OpenExtractor` sends embedded traffic only to the proxy. Tests in `extractor_proxy_test.go`.

## New findings

- **C1 MEDIUM: residual-risk text understates the open xberg port.** `docs/mcp/embeddings.md:277` says a page "cannot read the answer" and the worst case is CPU. The raw port also accepts `urls` (server-side fetch; RFC1918, link-local and `169.254.169.254` blocking unverified), `DELETE /cache/clear` (wipes the pre-warmed model cache), and `/extract-async`, `/v1/convert/file`, `PUT /process`. xberg checks no `Host`, so DNS rebinding makes a page same-origin and able to read responses: a readable SSRF to whatever xberg's policy does not block. Fix: state all of this; verify the SSRF policy against RFC1918, link-local and the metadata address and record the result; optionally mirror in `ipfs mcp --help` (`cmd/ipfs/kubo/mcp.go:146-162`).
- **C2 LOW (latent): Desktop bound-address parser unanchored** (`mcp-sidecar.js:26`, `:400-406`); external text echoed into stderr could move `getPort()`. Fix: anchor to `^ipfs mcp: serving MCP over HTTP on http://127\.0\.0\.1:(\d+)/mcp$`, first match per child only, consumers read only in `running`.
- **C3 LOW: nothing watches xberg after start** (`ExtractorSupervisor.Done()` unused, `extractor_supervisor.go:136`; proxy forwards to fixed upstream `extractor_proxy.go:156`). After an xberg crash a local process can take the port and poison the index. Fix: refuse with 502 `extractor_unavailable` once `Done()` is closed; optionally restart or log.
- **C4 LOW: docs mismatch.** `embeddings.md:245` says over-cap text is truncated; code discards it as `too_large`. `embeddings.md:238` omits that the `cache_dir` checks and lock are no-ops on Windows.
- **C5 INFO: Desktop env allowlist drops** `DO_NOT_TRACK`, `IPFS_TELEMETRY`, `HF_ENDPOINT`, `TESSDATA_PREFIX`, `ORT_DYLIB_PATH` (`mcp-sidecar.js:151-156`). No telemetry impact today; the HF mirror and OCR runtime paths silently stop working under Desktop. Fix: add them.
- **C6 INFO: proxy robustness.** No `IdleTimeout` (`extractor_proxy.go:88`); `drainErr` non-blocking (`:167`, `:187-195`) can classify a malformed request as 502.
- **C7 INFO: presence check reads 1 byte** (`index.go:268`); a shared first block surviving GC keeps derived text searchable. Optional stronger check or document the edge case.

## Checks on the fixes (no further defects)

flock taken before `reapStale`, `O_CLOEXEC`, released on all failure paths and in `Close`; no embeddings hot reload so no reload conflict; Desktop sidecar and a stdio `ipfs mcp` can no longer share a `cache_dir` (error and doc say so). `cache_dir` stat follows symlinks, uid checked on target. Presence check is read-only and offline (no CID injection). Desktop quit deferred once, `will-quit` SIGKILLs tracked PIDs only. Restart cap ends the B1/B2 loop.

## MCP checklist delta

MCP-01..05 unchanged. SSRF safe from `ipfs mcp`, at risk through xberg's raw port (accepted residual, C1). MCP01 PASS for kubo and Desktop; MCP10 PASS; MCP06 PASS.

## Manual follow-ups

1. Check xberg 1.3.10's SSRF policy against `10.0.0.0/8`, `192.168.0.0/16`, `169.254.169.254` and a rebound hostname; record in `embeddings.md`.
2. Network capture of an embedded xberg run with downloads off.
3. Run the sidecar spec and the proxy/extractor unit tests (the lead ran the sidecar spec under the shim, 38/38, and the embeddings unit tests, ok).
4. A `test/cli` case for content removed with `ipfs pin rm` + `ipfs repo gc` outside `ipfs mcp`, then search.
5. Fix C2 and decide how the Web UI learns the sidecar port and token before any consumer uses `getPort()`.
