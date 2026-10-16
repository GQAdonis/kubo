# Confirmation: vector-search-security-review 1.3 (phase `agent-backend-settings`)

Reviewer: `security-reviewer` (Claude Opus), read-only, in a context separate from the builders. This was a static reading of the current kubo tree and `ipfs-desktop`. I ran no tests. The lead's runs stand as reported: `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/` ok, `-tags noagent` ok, Desktop `agent-search` 66/66. Accepted residual, not re-raised: xberg's loopback port is unauthenticated. The lead wrote this file to disk.

**Verdict: PASS WITH FINDINGS.** No CRITICAL, no HIGH, so 1.3 is accepted with nothing to name for the phase gate. Every disposition holds. The fixes introduced two LOW and three INFO issues (N1-N5). Every one of them fails closed or needs a compromised store.

| ID | State | Evidence |
|---|---|---|
| M1 | Fixed, holds | `afterUnpin` walks each unpinned root offline (`index_lifecycle.go:136-168`). The walk is bounded at 10000 nodes and depth 64, dedups through `seen`, and a missing block marks it incomplete instead of fetching (`index_coverage.go:17-26`, `:62-113`). `keepEntries` checks pins first, then the MFS; an unknown answer keeps the entries with a warning (`index_lifecycle.go:109-128`). MFS coverage is one offline walk from the root CID, compared by multihash, and an incomplete walk counts as unknown (`index_coverage.go:128-163`). A CID reachable from both a pin and the MFS is kept on the pin check alone. GC passes no MFS check, which is correct because GC never removes MFS blocks (`index_lifecycle.go:253-263`). Pinned by `index_round7_test.go:63-151`. Docs: `embeddings.md:428`, `:482`. Gaps: N1, N2, N3 |
| L1 | Fixed, holds | `embeddings.md:293` section; ADR-0011 `:186`; Desktop `docs/agent/SEARCH.md:31-37`; Search window intro `assets/locales/en.json:406`, shown at `src/agent/search/ui/search.html:18` |
| L2 | Fixed, holds | The proxy closes when xberg exits (`extractor_supervisor.go:170-176`). `Extract` checks `Done()` before building a request (`extractor.go:358-364`). The store client checks `gone` before every request (`surreal.go:55-63`, `:136-140`; set at `service.go:64`). Pinned by `extractor_exit_test.go:19-88`. Only a narrow window remains between process death and `Wait` returning |
| L3 | Fixed, holds | `contactsText` (`index_bulk_tools.go:504-518`); `index_round7_test.go:154-207` |
| L4 | Fixed, holds | 8 MiB budget per call (`index.go:47-49`, `:229`). Prefix read, refused before any `cat` once the budget is spent (`:335-358`). Pinned by `index_round7_test.go:210-264`. Gap: N4 |
| L5 | Documented, holds | `embeddings.md:291` |
| L6 (settings review) | Fixed, holds | Minimal environment on every child: surreal `version` (`supervisor.go:184-185`), surreal `start` (`:306`, `minimalEnv` `:327-335`), watchdog `sh` and `ps` (`supervisor_unix.go:51-52`, `:76-77`), xberg `--version` (`extractor_supervisor.go:203-204`), xberg `serve` (`:253-279`). Pinned by `supervisor_unix_test.go:60-80` and `extractor_supervisor_test.go:81-112`. The xberg `--version` call has no test of its own |
| I1 | Fixed, holds | `safeIndexPath` (`index.go:364-389`) is used for both reads and the presence check (`:236-239`, `:252-256`). Pinned by `index_round7_test.go:267-303`. Residual (N5): a store-chosen CID can still name any block held locally |
| I2 | Rebuttal holds | Confirmed operator request, allowed by AGENTS.md |
| I3 | Rebuttal holds | Not a security issue |
| I4 | Rebuttal holds | Operator-chosen endpoints, clipped excerpt |
| I5 | Rebuttal holds, reasoning inaccurate | The run caps limit files read, not `seen`: directories and skipped files enter `seen` without counting against `MaxFiles` (`index_bulk.go:289`, `:495`, `:571`). About 60 bytes per entry, and no problem has been observed, so it stays INFO |
| I6 | Fixed, holds | `http.go:381-384` |
| I7 | Rebuttal holds | Same user's environment (`extractor_supervisor.go:273-277`) |
| I8 | Fixed, holds | ADR-0011 `:211` cites `HF_HUB_OFFLINE=1` |

## New findings

**N1 LOW: with a scoped `API.Authorizations`, the M1 walk and the MFS check are refused, and reconcile stops deleting anything.** The new code calls `files/stat` and `ls` (`index_coverage.go:33`, `:43`, `:134`). The catalog does not list either for `pin_rm` and `pin_update` (`catalog.json:383-385`, `:417-419`) or for `ipfs_index_reconcile` (`:1342-1344`), so an allowlist printed by `--print-rpc-allowed-paths` omits them. They only get in when `ipfs_index_bulk_start` (`:1373-1378`) is also selected. When they are refused, everything fails closed: entries are kept with warnings. But `keepEntries` then reports the MFS as unknown for every unpinned CID, so reconcile keeps every unpinned CID and derived text is never deleted. The reconcile description still says "no longer have any pin" (`:1332`). Fix: add `files/stat` and `ls` to those three entries and update the description.

**N2 LOW: CIDs that `ipfs_add` auto-indexes skip the walk, and MFS coverage ignores directories.** An `ipfs_add` with a filename wraps the file in a directory and indexes the wrapper's CID (`tools_content.go:242-244`, `:277`; `index_lifecycle.go:56-62`). `afterUnpin` skips the walk for any root that has entries of its own (`index_lifecycle.go:152-154`). Files under that wrapper that a bulk run indexed therefore keep their entries until reconcile. In the other direction, both walks visit files only (`index_coverage.go:92-93`, `:141-145`). So reconcile deletes the wrapper's entries even when the MFS holds that directory. That deletion is repairable by re-indexing. Fix: walk every root, and record directory hashes in the MFS coverage set.

**N3 INFO: one 60 s budget covers the whole clean-up.** The unpinned walk, the MFS walk and a `pin ls --type=all` per file all share `lifecycleTimeout` (`index_lifecycle.go:141`), with up to 20 s per call (`index_coverage.go:25`). On large trees the deadline expires and everything stays with warnings, which is fail-closed. Consider giving the drop phase its own deadline.

**N4 INFO: a `chunk_index` from the store is not clamped.** In `readChunks`, `(upto+2)*ChunkChars*4` can overflow `int64` (`index.go:339`). That makes `want` negative and raises the budget (`:346`). The per-source `MaxBytes+1` cap still applies, so the worst case falls back to the pre-fix top_k × max_bytes. Only a compromised remote store can trigger it. Fix: clamp `upto` to `MaxBytes/ChunkChars`.

**N5 INFO (residual of I1):** a store-chosen CID still selects any block held locally for an offline read. This is what the I1 fix intended, and it is noted only so the residual is on record.

## AGENTS.md

No tracking was added. No new endpoints were added. No CID injection: the walk, the MFS check and search are all offline, and none of them adds, pins or announces anything. `/api/v0/`, the gateway and the CID recipe are untouched.

## Severity count

CRITICAL 0, HIGH 0, MEDIUM 0 open, LOW 2 new (N1, N2), INFO 3 new (N3-N5). Suggested owners: mcp-server-engineer for N2 and N4; ipfs-agent-architect for the N1 catalog change, together with the server engineer.

## Lead disposition

1.3 accepted (no open CRITICAL or HIGH). The Round 7 correction cycle is spent, so N1-N4 are carried as follow-up task 1.4 of `vector-search-security-review` for Round 8; N5 is recorded only.

## Disposition (1.4)

| Finding | Status | Fix | Test |
|---|---|---|---|
| N1 | fixed | catalog `rpcOptional` (`files/stat`, `ls`) | `go test ./mcp/contract/...` |
| N2 | fixed | `index_lifecycle.go` and `index_coverage.go` `walkNodes` | `TestPinRmWalksARootThatHasEntriesOfItsOwn`, `TestReconcileKeepsADirectoryTheMFSHolds` |
| N3 | fixed | `dropTimeout` | `TestAfterUnpinDropPhaseHasItsOwnDeadline` |
| N4 | fixed | clamp in `index.go` | `TestReadChunksClampsAHugeChunkIndex` |
| N5 | recorded only | none | none |
