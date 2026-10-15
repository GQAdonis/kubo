# Security review: vector-search-security-review 1.1 (phase `agent-backend-settings`, Round 7)

Reviewer: `security-reviewer` (Claude Opus), working read-only in its own context, separate from the builders. Date: 2026-10-15. Scope: kubo `master` plus the uncommitted tree (`mcp/server/embeddings/**`, `mcp/server/index*.go`, contract, docs), ipfs-webui `src/search/**`, and ipfs-desktop Search (`src/agent/search/**`, `src/daemon/mcp-sidecar-search.js`, `src/agent/host-confirm.js`, `src/agent/confirm/native-dialog.js`). This was a static reading only. The reviewer ran no tests, binaries, browser, Electron or Touch ID, and relied on the xberg evidence READMEs as written. Builds on Round 4-6 findings and does not repeat them. Accepted residuals: the xberg loopback port, and a referenced key following a host move. Saved by the lead from the reviewer's returned report; the raw text is beside this file as `vector-search-security-review.raw.md`.

**Verdict: PASS WITH FINDINGS.** No CRITICAL or HIGH. One MEDIUM (M1), five LOW (L1-L5), eight INFO (I1-I8). `embeddings.insecure_http_hosts` passes; Round 6 confirm N3 and I5 are resolved.

## (a) Egress: `egress_ack` is checked before every request is built

Verified:
- Embedder: `Embed` refuses before marshalling when `covers(e.host)` is false (`embeddings/embedder.go:131-133`).
- Remote extractor: the check runs after the size check and before `build` (`extractor.go:355-360`).
- Embedded extractor: a child process on loopback reached through the proxy, no ack check; nothing reaches it unless `CheckEgress` or `CheckStageEgress` passed first (`index_file.go:108`, `index_bulk.go:576`).
- Describer: kind, size and MIME checks, then egress (`describer.go:338-340`), then the hourly reservation (`:341`), then `build` (`:346`).
- Store writes and searches: `Service.Index` and `SearchFiltered` call `CheckEgress` on the embedder and the remote store (`service.go:287`, `:364`; `Hosts` `:181-187`).
- `ipfs_index_cid`: refuses before reading anything (`index_file.go:106-110`); a stage refused for egress becomes a warning when another stage produced text, a hard error naming the host otherwise (`:416-427`).
- Bulk runs: preflight before the confirmation (`index_bulk_tools.go:259`), at start and resume (`:285-287`), per file (`index_bulk.go:574-578`); `StageHosts` includes the remote extractor and describer (`service.go:193-202`); a paused run never auto-resumes (`recoverBulkRun` `:143-156`; `projectRun` `:102-109`); `covers` built from the identity that starts or resumes the run (`:285`, `:309`).
- `autoIndex`: native text only, with `CheckEgress` first (`index_lifecycle.go:48-51`).

**L5 LOW: store metadata goes to a remote store with no egress check.** `ipfs_index_status` sends counts and the caller's `cid` (`index.go:124-148`); unpin, GC and reconcile send `DeleteCID` and `ListCIDs` (`index_lifecycle.go:90-107`, `:147`, `:224-246`); bulk run state via `SaveRun` (`embeddings/runs.go:99-107`) carries MFS path and entry names in `cursor.after`/`cursor.inside` plus `started_by` (`index_bulk.go:381-383`, `:467-469`). `service.go:149-151` says these "do not send content anywhere", but file names are content metadata. Fix: say so in `embeddings.md` "Data that leaves the machine", or check the store host for the status `cid` lookup and reconcile.

## (b) The `/index/*` routes

Verified: mount gating (`index_routes.go:62-76`; otherwise 404); only `ui` and `human-mcp` kinds (`:69`), `admin`/`a2a`/`uar` get 403 (`http.go:408-417`), admin in no default list (`http.go:243-257`); `id.Allows(t)` and `egressRefusal` (`index_routes.go:121-131`); strict CORS from `agent.origin_allowlist`, origin echoed, never `*`, no credentials header (`http.go:290-334`, `:384-393`); Host check (`:339-344`); fetch-metadata rejection (`:350-356`); body limit and read deadline (`:328-331`); schema validation (query 1-2000, `top_k` 1-50, `additionalProperties:false`) (`index_routes.go:169`; `ipfs_index_search.input.json:5-58`), status `cid` validated (`:203-210`); per-identity limiter (burst 10, 1/s, 2 in flight per identity, 8 overall) before any embed or store call, 429 with `Retry-After` (`index_routes.go:155-163`; `index_limit.go:13-73`); route errors strip confirmation data and send `Cache-Control: no-store` (`:288-298`); `bulk_run` only when the caller may read `ipfs_index_bulk_status` (`:231-243`); anonymous loopback principal is `human-mcp` read-all (`access.go:120-122`), can read status counts, search refused for lack of ack, matching `routes/index.json:17`.

INFO I6: the `setStrictCORS` comment says no exposed headers are sent, but the code sends `Expose` (`http.go:382-392`); code matches the contract, comment drifted.

## (c) SSRF

Verified: extractor and describer URLs only from operator config (`extractor.go:200-208`, `describer.go:186-196`); redirects disabled on every client (`extractor.go:242`, `describer.go:236`, `embedder.go:89`, `surreal.go:64`, `extractor_proxy.go:85`); `Proxy=nil` everywhere (`transport.go:11-13`); media as base64 `data:` URLs (`describer.go:385-397`) with MIME constrained by `^[a-z]+/[a-z0-9.+-]{1,100}$` before splicing into hand-built JSON (`:316`, `:334-337`); extractor form sends only `output_format`, the files part and a fixed OCR `config` literal (`extractor.go:402-427`), file name sanitised (`:432-444`); authenticated proxy: 32-byte random token compared in constant time (`extractor_proxy.go:70-73`, `:117-122`), exact `Host`, any `Origin` refused (`:113-116`), only `POST /extract` without a query (`:123-131`), multipart allowlist `files`, `output_format=markdown`, `config`, duplicates refused, `urls`/`inputs`/others refused (`:200-250`), body and response bounded (`:83`, `:137`, `:184`); minimal xberg environment so `XBERG_API_ALLOW_LOCAL_URI_INPUTS` and `XBERG_LLM_*` are never inherited (`extractor_supervisor.go:243-273`), so xberg's `inputs` and `file://` handling stay disabled (`task-1.1/README.md:117-119`). Through `ipfs mcp` the xberg endpoints cannot be driven to read server-side paths or fetch URLs; the raw xberg port is the accepted residual under (d).

## (d) The supervised `xberg` process

Verified: absolute path, no `PATH` lookup, regular executable, minimum version (`extractor.go:146-148`; `extractor_supervisor.go:176-212`); argv `serve --host 127.0.0.1 --port <p> --config <file>` in its own process group (`:277-282`); `/health` version must match, stranger detected (`:311-345`); minimal environment with `XBERG_CORS_ORIGINS` set to its own origin, `HF_HUB_OFFLINE=1` unless downloads allowed, `DO_NOT_TRACK=1` (`:247-273`); `cache_dir` owned by the user and not group/other-writable (`extractor_cachedir_unix.go:22-37`), `flock` (`:41-57`), state and config 0600; watchdog guarded by the command line (`supervisor_unix.go:34-57`) and `reapStale` (`extractor_supervisor.go:375-399`); Desktop binaries and directory owned by the user or root, not group/other-writable, checked on the realpath too (`mcp-sidecar-search.js:186-206`), directories 0700 (`:163-173`).

**L1 LOW: the residual is stated honestly in one place but understated or missing where Desktop users read.** `embeddings.md:292` and `extractor_proxy.go:31-37` are honest (local processes, web pages via simple POST, `urls` fetches of public URLs, `DELETE /cache/clear`, DNS rebinding with readable responses, operator acceptance). `embeddings.md:473` (Limitations) and ADR-0011 `:201` say only "any local process can submit files … spend CPU". `ipfs-desktop/docs/agent/SEARCH.md` and the Search window copy (`assets/locales/en.json:406`) do not mention it; Desktop is where it applies. ADR-0011 `:200` is stale (I8). Fix: link or summarise `embeddings.md:292` from `:473`, ADR-0011 `:201`, `SEARCH.md` and the Search window intro.

**L2 LOW: nothing watches xberg after startup, so the proxy keeps forwarding after xberg dies.** Exit observed only during startup and `Close` (`extractor_supervisor.go:286-292`, `:311-345`, `:417-449`); the proxy forwards to the fixed port unconditionally (`extractor_proxy.go:156-163`); the embedded SurrealDB supervisor has the same pattern (`supervisor.go:126-127`; the client keeps sending its root credential). Scenario: a crafted document crashes xberg; another local user's process binds the freed port (logged at startup, `extractor_supervisor.go:239`, `service.go:169`) and receives later documents, returning injected "extracted" text. Mainly multi-user hosts. Fix: on `Done()` close the proxy and fail extraction closed (`extractor_unavailable`); same for the store.

## (e) Untrusted content: Web UI XSS and prompt injection

Verified, Web UI: hits type-checked; CID `^[A-Za-z0-9]{10,200}$` before links; `kind`/`source` enum-checked; strings through `plainText` (`client.js:111-126`), which strips bidi, zero-width, tag and C0/C1 controls (`plain-text.js:6-24`); text and paths as React text only (`Snippet.js:23`, `ResultItem.js:27-29`); links from the CID only (`:42-49`); no `innerHTML` in `src/search/**`; descriptions labelled "AI description by <model>" (`ProvenanceLabel.js:18-20`); token only in `Authorization` with `credentials:'omit'`, `redirect:'error'`, `no-referrer` (`client.js:207-221`). Desktop Search window: CSP `default-src 'none'; connect-src 'none'` (`ui/search.html:5`), no `innerHTML`. Agent loop: `ipfs_index_search` `returnsUntrustedContent: true` (`catalog.json:1250`), marked in `_meta` (`registry.go:191-193`), framed before the model (`agent.go:828-833`); fixed describer instruction not to follow instructions in media (`describer.go:70-74`); stage warnings are fixed codes (`index_file.go:307-316`); responses bounded, UTF-8, no NUL (`extractor.go:447-495`, `describer.go:434-481`).

INFO I4: `index_unavailable` errors carry a clipped excerpt of embedder or store error text to the model, unframed (`index_file.go:535`, `embedder.go:162-166`); operator-chosen endpoints.

## (f) No CID injection; bulk reads offline

Verified: offline `files/stat` (`index_bulk.go:357-361`), `ls` with `resolve-type:false`/`size:false` (`:303`), `cat` and the sniff prefix through `indexTarget.Offline` (`:592`; `index_file.go:88-93`, `:152`, `:203`); MFS walked once through its root CID (`index_bulk.go:443-477`); search re-reads and presence check offline (`index.go:292`, `:312`); nothing added, pinned, written or announced; extracted images never read (`extractor.go:329-343`); `mfs_root` cannot escape (`schemas/common.json:24` `MfsPath`; `withinRoot` `index_bulk_tools.go:209-214`, `:230-234`); entry names sanitised (`index_bulk.go:422-432`).

INFO I2: `ipfs_index_cid` reads online (`index_file.go:111-116`, `:152`) — a confirmed operator request, within AGENTS.md; with `Provide.Strategy` "all" fetched blocks are announced, and a file whose stage host is unacknowledged is fetched before the refusal.

## (g) Resource exhaustion

Verified: caps only lowered per run (`index_bulk_tools.go:218-226`, `clampCaps` `:376-381`); bytes cap checked before the read, a file larger than the whole cap skipped (`index_bulk.go:563-573`); pacing (`:638-647`); one run at a time (`index_bulk_tools.go:289-293`, `:320-325`); shared hourly describer limit reserved before sending (`describer.go:288-298`, `:341`), bulk waits on `RetryAfter` and reuses extractor output (`index_bulk.go:606-613`; memo `index_file.go:368-377`); oversize refused before send (`extractor.go:355-357`; `describer.go:331-333`); read capped from stat plus sniff (`index_file.go:130-138`, `:195-216`); 2000-chunk cap (`:433-438`, `:491-496`) and `max_text_bytes` (`extractor.go:487-488`); responses bounded (64 MiB, 4 MiB, 32 KiB); filtered-search widening capped at 1000 rows with one embed (`service.go:351-394`).

**L4 LOW: `ipfs_index_search` memory per call is high and only the route is capped.** Native hits read each distinct source up to `policy.max_bytes`, re-chunked through `[]rune`, all held until return (`index.go:211-252`, `:308-322`; `policy.go:135-153`). About 50 × 1 MiB at `top_k` 50 by default, up to 50 × 100 MiB if `max_bytes` is raised (`policy.go:19`) — estimates, not measured. `/index/search` caps 8 in flight (`index_limit.go:20`); `/mcp` and the agent loop have no cap. Fix: bound total bytes read per search, or cut only the needed chunk.

INFO I5: the bulk `seen` map and the pin list grow with the node (`index_bulk.go:130`, `:400-416`).

## (h) Stored derived text and its deletion

Verified: derived text stored only for non-`text` sources (`service.go:299-306`); deletion after a confirmed `pin_rm`/`pin_update` and after GC by multihash (`index_lifecycle.go:113-184`; `tools_pin.go:79`, `:112`; `tools_diag.go:179`, `:213`); a failed pin check never deletes (`:97-100`); reconcile paged and confirmed; derived hits only after an offline one-byte presence check (`index.go:214-233`, `:289-302`); Desktop store under 0700 userData.

**M1 MEDIUM: unpinning a directory through `ipfs mcp` leaves the derived text of every file indexed under it.** `afterUnpin` checks only the CIDs `pin/rm` returned (the roots) (`tools_pin.go:79`; `index_lifecycle.go:113-127`); bulk runs index each file under its own CID (`index_bulk.go:592`), so a directory root has no rows (`dropIfUnpinned` returns at `:94-96`). Children's vectors and stored OCR, extracted text and descriptions stay in SurrealDB, and until GC the presence check passes so search keeps returning that text (`index.go:289-302`). The docs say "Deletion still follows pins" and that only out-of-band unpins wait for reconcile (`embeddings.md:471`, `:478`; ADR-0011 `:202`) — not true for a pinned folder. Related: MFS removals never trigger deletion; reconcile deletes entries for files still in MFS because `pinCovers` ignores MFS (`index_lifecycle.go:71-84`, `:234-252`). Fix: after an unpin, drop indexed CIDs no longer covered (walk the unpinned DAG offline, or run the reconcile loop over `ListCIDs`); treat MFS reachability as coverage or document it; test: bulk-index a pinned directory, `pin_rm` the root, assert no derived hit returns.

INFO I1: offline reads use the stored `Path` or `CID` unvalidated (`index.go:219`, `:235`); a compromised remote store could steer them to arbitrary `/ipfs` or `/ipns` paths. Fix: build the path from a parsed CID.

## (i) Desktop egress confirmation and Touch ID for Search

Verified: unknown fields refused, absolute paths without NUL, https or loopback http without credentials (`service.js:53-111`; `mcp-sidecar-search.js:37`, `:101-111`); IDN to punycode via `new URL`; only new, used, non-loopback hosts asked (`service.js:167-185`); Touch ID naming the host and what goes there, else a native dialog with Search copy (`en.json:458-464`), Cancel default, parented only to the visible Search window (`native-dialog.js:22-56`), one confirmation at a time, declines per scope (`host-confirm.js:43-89`); nothing persisted after a refusal (`service.js:182`); `egress_ack` is `local` plus confirmed, still-configured hosts (`mcp-sidecar-search.js:137-144`); unconfirmed hosts keep Search unavailable (`:249-252`); window sandboxed, own partition, deny-all permissions, request filter, IPC sender checks, content protection (`search/window.js:54-165`).

**L3 LOW: two confirmation texts understate where data goes.** `ipfs_index_bulk_start` says "Contacts only this machine." when all stage hosts are loopback (`index_bulk_tools.go:492-496`) though with `allow_model_downloads: true` xberg may reach the Hugging Face Hub (`extractor_supervisor.go:78-83`, `:257-259`); `ipfs_index_cid` says only "Index {cid} with the configured embedder" (`catalog.json:1298`) though a file may go to a remote extractor or hosted describer. Fix: append the stage hosts (and the Hugging Face Hub when downloads are on) to both summaries.

## `embeddings.insecure_http_hosts` and `by_kind`

- `insecure_http_hosts`: PASS. N3 resolved: a remote store with a credential over plain http to a non-loopback host must be listed (`settings.go:200-202`); the example lists SurrealDB (`vector-search-in-cluster.json:27-31`); schema and docs name the store credential (`schema:1107-1108`; `embeddings.md:210-213`). I5 resolved: entries validated at startup against the pattern, ports 1-65535, bare IPv6 rejected (`settings.go:143-167`, `:132`). Matching exact and case-insensitive (`:280-297`); `egress_ack` still applies.
- `by_kind`: aggregate counts per kind (`store.go:381-406`; `index.go:136-140`); no content or egress; visible to `index`-group readers and the anonymous loopback principal. INFO I3: the Web UI does not read it (`client.js:92-108`) though `routes/index.json:65` says the panel shows it.

## Other INFO

- I7: `ORT_DYLIB_PATH` and `TESSDATA_PREFIX` pass through to xberg (`extractor_supervisor.go:267-271`; Desktop `mcp-sidecar.js:181`); library paths not checked by `isTrustedProgram`; same user's environment.
- I8: ADR-0011 `:200` is stale; the offline switch is verified (`task-1.1/README.md:85-105`).

## AGENTS.md rules

No tracking (queries not logged; `DO_NOT_TRACK=1` for xberg). Endpoints configurable with off switches (`embeddings`, `extractor`, `describer` default null; `allow_model_downloads` false; `bulk.enabled`, `search_route.enabled`, `insecure_http_hosts:[]`; Desktop Search off by default). No silent CID injection (bulk and search offline; I2 is a confirmed operator read). `/api/v0/`, gateway, CID recipe untouched. Docs gaps: M1, L1.

## Verified controls (summary)

Egress before build on all four clients and all bulk paths; route kind, group and egress authorization; strict CORS; Host and fetch-metadata checks; body limit; per-identity limiter; schema validation; operator-only URLs with no redirects and no proxy; `data:` media; proxy field allowlist and token; minimal xberg environment and loopback bind; binary, `cache_dir` and Desktop binary-trust checks; watchdog and reaping; offline bulk and search; caps, pacing, one run at a time; hourly describer limit; oversize refused before send; widening cap; untrusted framing and plain-text rendering; Desktop Touch ID and dialog gate.

## MCP checklist

Streamable HTTP, network-exposed on LKE behind a bearer.

| Control | Status | Justification |
|---|---|---|
| MCP-01 | PASS | Bearer on every route (`http.go:405-436`); route kinds (`index_routes.go:69`); per-tool grant (`:121-131`) |
| MCP-02 | NEEDS INVESTIGATION | Streamable HTTP sessions SDK-managed (`http.go:196-200`); unchanged |
| MCP-03 | PASS (route) / note | `index_limit.go:13-73`; `/mcp` search uncapped (L4); writes confirmed |
| MCP-04 | PASS | `validateArgs` (`index_routes.go:169`); schemas bounded |
| MCP-05 | PASS | `modelcontextprotocol/go-sdk` |

| RCE vector | Status | Justification |
|---|---|---|
| Command injection | SAFE | argv exec (`extractor_supervisor.go:277`); watchdog arguments positional (`supervisor_unix.go:51`) |
| Dynamic code evaluation | N/A | None |
| Unsafe deserialization | SAFE | JSON only, bounded |
| Path traversal | SAFE | `MfsPath` + `withinRoot`; `safeEntryName`; `safeFilename`; owned `cache_dir` |
| SSTI | N/A | i18n interpolation only |
| Dependency hijacking | SAFE | No xberg code linked; operator binaries checked |
| SSRF | SAFE | Operator URLs only, no redirects, `data:` media, proxy refuses `urls`/`inputs`; raw port residual L1 |

| OWASP | Status | Justification |
|---|---|---|
| MCP01 | PASS | `Secret` redaction (`settings.go:33-57`); credentials by reference |
| MCP02 | PASS | Write and destructive classes; Desktop `ui` has no `index` write grant |
| MCP03 | PASS | Static catalog; fixed describer instruction |
| MCP04 | NEEDS INVESTIGATION | xberg image or binary digest pinning on LKE |
| MCP05 | PASS | No shell built from input |
| MCP06 | PASS | `agent.go:828-833`; `registry.go:191`; plain-text Web UI |
| MCP07 | PASS | Server-side authorization per route and tool |
| MCP08 | PASS (bounded) | Run and stage logs; no query logging |
| MCP09 | PASS with residual | xberg port documented (L1 wording) |
| MCP10 | PASS | Hits carry chunk text only; status is aggregate |

## Manual follow-ups

1. xberg 1.3.10 raw port: can an `/extract` `config` value make xberg contact a URL (LLM or VLM OCR backends, remote models) and bypass the `urls` SSRF policy? Does `/cache/warm` honour `HF_HUB_OFFLINE`? Do HTML, DOCX or SVG files with external references cause fetches during extraction? Run with a packet capture.
2. A DNS-rebinding proof of concept against the Desktop xberg port (sizes L1).
3. M1 regression test: bulk-index a pinned directory, `pin_rm` the root, confirm no derived hits.
4. Measure peak memory of `ipfs_index_search` at `top_k` 50, default and raised `max_bytes` (L4).
5. Crash xberg mid-run; check the proxy and the bulk run (L2).
6. Race detector on `index_bulk*` at the final gate.
7. Not re-checked: Round 6 confirm N1 and N2 (settings); the LKE NetworkPolicy isolating xberg, the embedder and SurrealDB.
8. Mac without Touch ID: the Search dialog path.

## Severity count

CRITICAL 0, HIGH 0, MEDIUM 1 (M1), LOW 5 (L1-L5), INFO 8 (I1-I8). Under 1.2 only CRITICAL and HIGH are batched, so none is required. Recommended for the owning roles: mcp-server-engineer for M1, L2, L4; ipfs-agent-architect for L1, L3 and docs; ui-ux-engineer and the Desktop owner for `SEARCH.md` and the Search window copy.


> Raw copy: the reviewer's returned report, saved verbatim by the lead (the reviewer had read-only tools). The findings file beside it is the same text.
