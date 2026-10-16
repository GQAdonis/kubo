# Security read: kubo-candidate-image 1.5 (kubo `3d8a128c5`) and know-me-cluster PR #35

Reviewer: `security-reviewer` (Claude Opus), read-only, in a context separate from the builders. Date: 2026-10-16. The lead wrote this file to disk. Method limit: the reviewer had no shell, so it read the HEAD tree of `feat/ipfs-mcp-server` and the checked-out `chore/ipfs-kubo-3d8a128c5` branch with Read and Grep; it ran no tests and could not run `git show` or `gh pr diff`. Paths are under `mcp/server/` unless stated otherwise.

**Verdict: PASS WITH FINDINGS.** No CRITICAL, no HIGH. Part 1: 1 MEDIUM, 4 LOW, 2 INFO. Part 2 (PR #35): safe to merge.

Timing: PR #35 was merged by the operator before this read returned. The read found nothing that blocks it (see Part 2).

## Round 7 follow-ups (`ff1d66e0f`): all seven hold

| ID | Evidence |
|---|---|
| settings N1 | `downgraded` forces confirmation whatever the port (`settings.go:638-655`) |
| settings N2 | `Broker.Cancel` ends pending or approved records and clears `active` (`broker.go:396-413`); only caller is `denyRevoked` for the session's own interrupt (`agent.go:489-501`, `server.go:409`) |
| settings N3 | Retry floor (`settings_limits.go:91`); Retry-After at least 1 s (`:108`) |
| vector N1 | `files/stat` and `ls` in `rpcOptional` for pin_rm, pin_update and reconcile (`catalog.json:383-387`, `:419-423`, `:1349-1351`); description corrected (`:1336`) |
| vector N2 | Per the 1.4 disposition; `afterUnpinWithin` read, not re-traced |
| vector N3 | Separate `dropTimeout` (`index_lifecycle.go:27`, `:180`) |
| vector N4 | Per the 1.4 disposition; clamp not re-read |

## Findings, store-startup fix (`3d8a128c5`)

**S1 MEDIUM (availability, fails closed): the caller's context decides whether the store opens.** `embeddings/lazystore.go:62-67`, `embeddings/service.go:72`, `:136`, `embeddings/supervisor.go:255-257`, `index.go:144-146`. `get` runs `open` under the first caller's context; a cancelled or expired context counts as a store failure and doubles the backoff up to 60 s. (1) Status uses a 10 s deadline while the embedded start allows 30 s for its health wait, so a slow embedded start polled by the UI is killed at 10 s every time and never completes. (2) Any read principal, including anonymous loopback, can call status and cancel at each retry window, keeping the store unavailable; search, indexing and unpin clean-up stop (clean-up keeps entries and warns, so nothing is lost). Fix: run `open` under `context.WithoutCancel(ctx)` with its own deadline (`startupStoreTimeout` remote, the start budget embedded); do not count the caller's `ctx.Err()` as a failure.

**S2 LOW: waiters ignore their own context while an open is in progress.** `lazystore.go:51-62`. `l.mu` is held across `open`, which can run up to 60 s per request remotely (`surreal.go:22`) or about 30 s per try embedded; every store call blocks past its deadline and goroutines pile up. Fix: singleflight with a done channel; waiters `select` on their own `ctx.Done()`.

**S3 LOW (fails closed): a store that dies after opening is never reopened, and status reports it available.** `lazystore.go:53-55`, `:69`, `surreal.go:136-140`, `stages.go:87`. Once `inner` is set, `Ready` returns nil; an exited embedded store returns `ErrStoreExited` until restart while `stages.store` says available. The guard that stops text reaching the embedder (`service.go:399`, `:480`) only covers the never-opened case (text still goes only to egress-acknowledged hosts). The comment at `lazystore.go:11-13` is wrong. Fix: on `ErrStoreExited`, reset `inner` so `openEmbeddedStore` replaces the supervisor (`service.go:126-133`); otherwise correct the comment and make `Ready` check `gone`.

**S4 LOW: `stages.*.reason` shows internal details to every read principal.** `stages.go:87`, `:95`, `:100-104`; `index.go:146-147`. Can contain the resolved dial address of the remote store, absolute paths of `surreal_binary` and `data_dir` (`supervisor.go:167`, `:195`), and up to 300 runes of surreal or xberg stderr (`supervisor.go:351`, `surreal.go:208-215`). No credential leak (`hostOnly` drops userinfo, `surreal.go:200-206`; keys only in headers; the probe sends none, `stages.go:34-46`). Before this change status logged the cause at Debug only (`index.go:151`). Fix: a fixed reason code for non-operator principals; detail in the server log.

**S5 LOW: a config naming principals whose secrets all fail to resolve starts in anonymous mode.** `config.go:264-276`, `cmd/ipfs/kubo/mcp_http.go:225-230`. Bounded: a non-loopback listener without credentials is refused (`mcp_http.go:226-228`, `http.go:149`); forwarded requests are refused (`http.go:427-431`); a stderr warning is printed (`mcp_http.go:294-296`). Anonymous reaches every read-class tool in every group (`access.go:120-121`), is exempt from the `egress_ack` gate (`egress.go:160-162`), and reaches `/index/status` and `/index/search` (`index_routes.go:69`) with search, indexing and probes refused (empty covers, `index.go:88-91`, `embeddings/embedder.go:131`); not `/agui`, `/a2a` or settings (`http.go:247-256`, `settings.go:173`); no writes. In a pod, loopback is every container, which can already reach kubo RPC on `127.0.0.1:5001`; the real risk is silently losing the intended authentication. Fix: fail startup when `cfg.Principals` is non-empty and no credential resolved (or a deny-all verifier); doc note for sidecars.

**S6 INFO:** the comment at `index.go:113-114` says the contract does not list `stages`; it does (`ipfs_index_status.output.json:53-104`).

**S7 INFO (accepted): stage probes are not an SSRF or amplification path.** Targets come only from operator config and only for hosts the caller's ack covers (`stages.go:88-93`); one bare GET, no credential, 3 s timeout (`:21`), redirects off (`embedder.go:89`, `extractor.go:242`), no proxy (`transport.go:13`); at most 2 GETs per call. The MCP tool path has no per-identity rate limit; note at MCP-03.

## Startup and embedded-mode checks (pass)

Plain-http credential refusal stays fatal for store, embedder, extractor and describer (`service.go:58`; `settings.go:200-201`, `:219-220`; `extractor.go:171-173`; `describer.go:129-131`). `ErrAuth` and dimension mismatch are fatal when the store is reached at startup (`service.go:96-98`). Supervisor replacement under `s.mu`, exited supervisor closed first (`service.go:124-143`); `launch` uses `exec.Command`, so a request ending cannot kill a running store; minimal env plus `SURREAL_*` (`supervisor.go:306`, `:327-335`); `Close` waits for an open in progress (`lazystore.go:80-84`, `service.go:174-183`). Embedded xberg failure degrades with `extractorErr` (`service.go:102-108`), no egress widening. Every lazy method fails closed with `ErrStoreUnavailable` (`lazystore.go:89-194`); search never unscoped; unpin, GC and reconcile keep entries with warnings. Schema defined exactly once under `l.mu`. Backoff bounded 2 s to 60 s (`lazystore.go:66`).

## AGENTS.md

No tracking (the probe carries no content or identity); no new endpoints; no CID injection; `/api/v0` and gateway untouched; the output schema gains only the optional `stages`.

## Cluster PR #35

Image-only: all three images (`manifests.yaml:62`, `:136`, `:200`) point at `@sha256:38d453a5…40cca`; no old digest left in `namespaces/ipfs/`. Runbook: the postStart hook detaches its I/O (`:72`), so the 2026-10-13 gotcha does not trigger; never force-delete a stuck pod. Rollback is safe only if both images share `RepoVersion` because `--migrate=true` is set (`:55`).

Effect on the sidecar as configured (`:137-149`, ConfigMap `:746-769`): `--http=0.0.0.0:5050` with `--auth-token-env=IPFS_MCP_TOKEN`, Secret reference required, so the anonymous fallback cannot apply (a non-loopback listener without credentials refuses to start). `agent.agui` and `agent.a2a` default off (`principals.go:37`, `:42`); no LLM backend, so no agent loop (`mcp_agent.go:155-167`); no admin principal, so no settings route (`mcp_agent.go:209-211`); no embeddings section, so no store, extractor or index routes (`mcp_embeddings.go:37-38`). Net effect: the same `/mcp` surface under the same operator token. Unchanged and outside this PR: the HTTPRoute exposes `/mcp` publicly (`:815-840`), and `confirmation.modes: []` goes with a token that can write in every group.

## Acceptance

No open CRITICAL. Suggested owners: S1-S4 and S6 mcp-server-engineer; S5 ipfs-agent-architect with the server engineer. Manual follow-ups: (1) check the PR #35 diff with `gh`; (2) compare `RepoVersion` of the old and new digests; (3) race detector on `embeddings/unavailable_test.go` at the gate.

## Lead disposition

- Follow-up 1 done by the lead: `gh pr diff 35 --name-only` lists only `namespaces/ipfs/manifests.yaml`, and the changed lines are the three `image:` lines.
- Follow-up 2 done by the lead: `version.go` has `RepoVersion = 18` at both `0f15c2407` (the build behind the old digest) and `3d8a128c5`; no migration added between them. Rollback is safe.
- Follow-up 3 carried to `phase-integration-gate`.
- S1-S6 carried as `kubo-candidate-image` 1.6.
