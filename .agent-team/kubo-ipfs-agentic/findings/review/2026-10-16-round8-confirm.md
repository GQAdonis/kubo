# Round 8 confirmation passes: Go (`2cddaae4d`) and UI (`cab1b30f`)

Reviewers: `reviewer` (Claude Opus), read-only, separate contexts; Read, Grep, Glob only, no tests run. Lead-written summary keeping IDs and file:line.

## Go and contract: confirmed, no open HIGH

All ten R8 dispositions hold:

- R8-01: `DiscoveringDaemon` (`cmd/ipfs/kubo/mcp.go:235-236`, `mcp/server/daemon.go:200-225`); the api file is written by temp file plus rename (`repo/fsrepo/fsrepo.go:396-411`).
- R8-02: `WaitDuration` (`config.go:119-125`); first retry at Warn (`probe.go:164-166`).
- R8-03: `settle` (`uar_proxy.go:529-538,618,544-563`); generation plus sequence (`uar_backend.go:81,163`, `uar_proxy.go:223`); tests `uar_proxy_hardening_test.go:385,423,442,455`.
- R8-04: test rename (`admin_test.go:111`), `config-reference.md:82`.
- R8-05: amendment (`openspec/changes/kubo-candidate-image/tasks.md:29-33`).
- R8-06: ADR-0010 Status (`:5`).
- R8-07: `ownedSkills` (`uar_provision.go:379-391`).
- R8-08: `lazystore.go:77-80`, `TestClosedLazyStoreDoesNotServe`.
- R8-09: `agent_settings.go:131-141`.
- R8-10: parameter removed.

Compatibility:

- `llm.kind` empty still means openai.
- The new requirements apply only to `uar`, which has not shipped.
- Stock stdio is unchanged.
- `DiscoveringDaemon` locking is sound; it implements `rawDaemon` and `uploadDaemon` (`daemon.go:285-288`).

New findings:

- **N1 MEDIUM:** after a failed resume on a multi-interrupt segment, `reopen` withdraws all answers and sets `shown` to #1 while the island last displayed #2. Resuming #2 gets 422 and a new message gets 409 without the open interrupt, so the thread is stuck until `sessionIdleTTL` (30 min, `agent_sessions.go:19`) (`uar_proxy.go:544-552`). Only the one-interrupt case is tested. Fix: keep earlier answers, withdraw only the shown interrupt's answer (and denials it filled in), keep `shown`; add a two-interrupt 502 test.
- **N2 LOW:** a skill removed from `skills_dir` stays bound after a restart (`ownedSkills` per backend instance; `uar_backend.go:57`, `uar_provision.go:379-391`). Document it, or mark kubo's bindings.
- **N3 LOW:** `config-reference.md:147` shows the `daemon.wait` default as `"60s"` without the stdio qualifier; the schema `"default"` (`:623`) is transport-dependent.
- **nit-1:** `MCPCredential == nil` branches in `runGrant` unreachable (`uar_proxy.go:419-421`).
- **nit-2:** every settings apply rotates every UAR thread id; write it down.

ADR-0010 against the code:

- **A1:** interrupts are also forgotten when UAR refuses the thread, or when a hop fails where nobody needs to answer.
- **A2:** after eviction or abandon, the id changes because of the per-thread sequence, not the generation.
- **A3:** the A2A wording is wrong:
  - a non-POST gets 405 and a non-a2a principal 403 first;
  - with UAR at startup, `/a2a` and the card answer 404;
  - the 503 applies only after a runtime switch, and the card keeps answering 200 then.

  `docs/mcp/agentic.md:104` repeats the claim.

Phase gate:

1. `make -O test_go_lint`.
2. `git diff 8441e9fb4..2cddaae4d --stat -- core/ client/ config/ repo/ plugin/` is empty.
3. The race detector on the lazy store, discovery, startup, daemon wait, UAR and A2A tests.
4. The stability CLI pins.
5. The N1 test and `config_reference.py --check` after the doc fixes.
6. `agent-backend-uar` 1.5, or carry it open.

## UI: BLOCK

Earlier findings:

- H1, H2, M2, M3, L1, L2 and the nit: resolved.
- M1: partial, now LOW, because "credential" remains at `agentSettings.json:165,172,174`.
- U1 and U13: met.
- U5: not met (see A).

New findings:

- **A HIGH:** `uar.confirm.help` (`agentSettings.json:192`, `ConfirmHost.js:53`) says "The stored API key ... go to this host" on first run (no key stored), on a kind change (the server drops every key, `settings.go:637`), and beside `confirm.helpKey` on a host move (Round 6 N1 class). It also drops the node-content sentence; the tests at `:678-679` pin the false text. The grant goes only when `llm.uar.mcp_principal` is set (`settings.go:706`).
- **B HIGH:** in the browser, an OpenAI-compatible endpoint that needs a key cannot be set up on first run or when switching from a UAR node. With no standalone Save, the 401 test result is `ok:false` (`settings_probe.go:161-162,176-186`), the model field stays disabled (`OpenAiBackendForm.js:23-24`), and Apply is blocked. `key.help` (`:85`) still says to save first.
- **C MEDIUM:** `uar.key.saved.text` (`:179`) promises an Apply that does nothing.
- **D MEDIUM (a11y):** `key.withApply` (`ApiKeyField.js:95`) is not tied to the input with `aria-describedby`.
- **E LOW:** an Apply failing with `secrets_not_accepted` does not switch the field to the server-managed statement (`ApiKeyField.js:44`).
- **F LOW:** "private file" in `uar.key.help` and `helpServer` (`:176-177`), with `DESIGN.md:128`.

Acceptance: `agent-backend-uar` 1.6 "no mocked UAR state" is met; 1.6 as a whole is not accepted until A and B are fixed and tested.

## Lead disposition

The operator chose "One extra targeted fix" for the UI: A-F, the M1 residual and security N1, in the Web UI only, followed by a UI-only re-confirmation. The architect corrects A1-A3, N3 and the N2 note. Go N1 and nit-1 are carried as `agent-backend-uar` 1.7.
