# R12 review: confirmation.resolvers / cli / ttl and the MCP CI workflow

Date: 2026-10-11. Reviewer: reviewer (independent context, read-only tools; nothing was run). Recorded by the lead from the reviewer's hand-back, because the reviewer session had no write tool.

Counts: CRITICAL 0, HIGH 0, MEDIUM 5, LOW 8, NIT 4. Verdict: PASS-WITH-FINDINGS.

Compatibility: no `/api/v0`, gateway, CID-recipe or wire impact; with the three keys absent, behavior equals the old code.

## MEDIUM

- **M1.** `docs/mcp/progress-report.md:258` still says the keys are unimplemented and the TTL fixed at 120 s.
- **M2.** CI premise is false: Playwright baselines are not darwin-suffixed (`snapshotPathTemplate` has no `{platform}`; files live under `__screenshots__/chromium/` and `electron44/`). `mcp.yml:191`, `openspec/changes/mcp-ci-workflow/design.md:10` and Decision 6, `tasks.md` 1.3. Regenerating on CI would overwrite the operator's baselines. Fix the rationale (macOS is the closest renderer) or add `{platform}`.
- **M3.** Playwright jobs will likely fail on the first run (different macOS and fonts; `maxDiffPixelRatio: 0.005`, `retries: 0`). Pin the runner image instead of `macos-latest`; decide the baseline source. `check:dist` on Linux against a macOS-built dist carries the same risk.
- **M4.** No end-to-end test for `confirmation.resolvers` or the startup warning: `TestMCPConfirmationConfigKeys` only checks schema rejection of `["cli"]`; the spec's "Empty resolver list" scenario needs `confirmation_unavailable` plus the warning on stderr.
- **M5.** Spec `native-confirmation-prompt/specs/mcp-out-of-band-confirmation/spec.md:31` and the `ipfs mcp confirm` helptext (`cmd/ipfs/kubo/mcp_confirm.go:51`) say only `--confirm=cli` enables cli; `IPFS_MCP_CONFIRM`, `confirmation.modes` and `confirmation.cli` also do.

## LOW

- **L1.** Config file parsed up to three times per start (`mcp_confirm.go:125-157`, `mcp_http.go:113-117`); TOCTOU and waste. Load once.
- **L2.** A very short ttl (`1ms`) validates; `sweep` uses `now - ttl` as grace, so a retry sees `confirmation_required` instead of `confirmation_expired`. Consider a 1 s minimum or a fixed grace.
- **L3.** `cli: true` with `modes` absent adds cli to the default modes; the schema, ADR-0003 and config reference do not say so.
- **L4.** Stale comments: `mcp/server/config.go:20-22,27`, `mcp/server/confirm/config.go:13-14`, `cmd/ipfs/kubo/mcp_confirm.go:25`.
- **L5.** `TestConfirmationTTLReachesTheServerOptions` checks the loaded config, not `mcpserver.Options.ConfirmTTL`.
- **L6.** CI path triggers miss `test/cli/harness/**`, `Makefile`, `mk/**`.
- **L7.** Unfiltered `push` plus `pull_request` runs the workflow twice per change.
- **L8.** pnpm 11.25.0, Node and action SHAs unverified offline; `pnpm/action-setup` runs before `setup-node`.

## NIT

- **N1.** `server.go:149` error says "greater than 0" though 0 means default.
- **N2.** `TTLDuration` silently falls back to 120 s on a parse error.
- **N3.** `mcp.yml:213-215` lists `mcp/ui/pnpm-lock.yaml` twice in `cache-dependency-path`.
- **N4.** `TestMCPConfirmationConfigKeys` is not cited in `docs/mcp/config-reference.tests.json`.
