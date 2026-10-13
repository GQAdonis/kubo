# R13 review: webmcp-tool-package and webui-webmcp-integration

Date: 2026-10-12. Reviewer: reviewer (independent context, read-only tools; nothing was run). Recorded by the lead from the reviewer's hand-back.

Verdict: request changes. Counts: CRITICAL 0, HIGH 3, MEDIUM 6, LOW 8.

Kubo compatibility (by reading): no Go files changed; nothing touches `/api/v0`, the gateway, `core/corehttp/webui.go`, CID defaults or wire behavior; no React dependency in `mcp/webmcp`; pins match `versions.toml`.

## HIGH

- **H1.** `public/locales/en/webmcp.json` uses i18next-native `{{x}}` and `_one`/`_other`, but the Web UI loads `i18next-icu` (`src/i18n.js:16`), so production renders literal `Confirm: {{title}}`, `dialog.expiry`, `dialog.queued`, `status.registered`. Tests use plain i18next. Fix: ICU syntax; tests `.use(ICU)`.
- **H2.** The off switches rely on the host honoring the registration AbortSignal (`WebMcpHost.js:59-64`, `register.ts:41`); no host was shown to. If ignored, tools stay callable after "off", writes still prompt and run. Fix: live gate inside each `execute` (enabled, writes, registration generation) returning `declined` with no RPC; Jest case with a host that ignores the signal; add an off-then-call step to task 1.4.
- **H3.** `car_import` says "never announces" but `kubo-rpc-client` 6.1.0 `dag.import` forwards only `pin-roots`, and Kubo announces CAR roots by default (`core/commands/dag/dag.go:203-211`). Fix: exclude `car_import` in `NO_BROWSER_IMPLEMENTATION` with the reason, or fix the client first.

## MEDIUM

- **M1.** `routing_provide` matches `/disabled/i`; Kubo says `Provide.Enabled is set to 'false'` (`core/commands/routing.go:190`), so it returns `rpc_error` instead of `providing_disabled`. Fix and test.
- **M2.** `ipfs_add` with `provide:false` still announces the root: Kubo `add` defaults `fast-provide-root` on (`core/commands/add.go:107`); affects the Go server (`tools_content.go:228`) and `src/executors/content.ts:26`. Route to the architect; WebMCP can pass `'fast-provide-root': false`.
- **M3.** `car_import` confirmation names no CIDs (`src/confirm.ts:86-90`); a non-CAR file is rejected only after approval. Moot if H3 excludes it.
- **M4.** Modal: defensible (DESIGN.md scopes to `mcp/ui`/`mcp/app`), focus handling sound, contrast 4.58:1 checks out; but the Settings off switch is unreachable behind the backdrop and the queue is unbounded (`confirmation-queue.js:57`). Fix: "Deny all and turn off agent tools" action; cap the queue.
- **M5.** `tsconfig.json:54-105` lists files explicitly and omits `src/webmcp/**`, so task 1.3's typecheck never checks the new code.
- **M6.** `vendor/MANIFEST.json` records no kubo commit, `CONTRACT_VERSION` or generated-file hash; stale vendoring is undetectable. Fix: record provenance; release-checklist item.

## LOW

- **L1.** `tests/integration/daemon.ts:16-17` prefers `cmd/ipfs/ipfs` over `PATH`; align task text.
- **L2.** `src/dagjson.ts:15,35` assigns `out[k]`; a `__proto__` key changes the prototype, so `dag_put` may store other than what was confirmed. Use `Object.fromEntries`/`defineProperty`.
- **L3.** `webmcp` namespace not in `.tx/config` (same as `agentChat`; fork-only).
- **L4.** `Permissions-Policy: tools=()` as an off switch is untested in either host.
- **L5.** `dag.import` has no timeout when the host passes no signal (Kimi).
- **L6.** Preview shows 160 characters; base64 shows "(base64 data)". Add size plus hash or "show full".
- **L7.** Buttons say Approve/Deny without the object (DESIGN.md verb plus object; not binding here).
- **L8.** `mcp/webmcp/` is untracked; commit before archiving.

## OpenSpec

- `webmcp-tool-package` 1.1-1.3: Verify clauses met by the code and tests present (drift test, destructive exclusion, hook-declines-no-RPC per write tool, output-schema integration checks; `pin_verify`/`dag_stat` absence in the 6.1.0 client confirmed).
- `webui-webmcp-integration` 1.1-1.2 met by Jest; 1.3 unconfirmed (M5, H1); 1.4 operator task, open.
