# Review: Round 6, Go and Desktop, confirmation pass (`agent-backend-settings`)

Reviewer: `reviewer` (Claude Opus, independent, read-only). Date 2026-10-15. Confirms `2026-10-15-round6-go-desktop.md` after the operator ruling on M1 ("Let it follow, document it") and the batched fixes; also reviews `embeddings.insecure_http_hosts` (`vector-search-contract` 1.10, `mcp-content-extractor` 1.8). Not run by the reviewer: lint, `go test`, race, `git diff`, Desktop specs, Playwright. Lead-verified: `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/` ok; `config_reference.py --check` OK; Desktop specs pass under the shim. Written to disk by the lead.

**Verdict: PASS WITH FINDINGS.** No compatibility break. One new MEDIUM (M3) from the M2 fix plus the server's failed-apply path — fix before merge. Rest LOW or nits.

## Earlier findings

| ID | Status | Evidence |
|---|---|---|
| M1 | RESOLVED (by decision) | ADR `0012-model-key-scope.md:5`, `:28`, `:53`, `:69-79`; `settings.go:527-546`, `referencedKey` `:661-666`, D4 text `:580-585`; tests `settings_keymove_test.go:106`, `:126`, `:137`; docs `agentic.md:97`, `chat-setup.md:44`; Desktop `settings-service.js:448-455`, `en.json:379-380,398` |
| M2 | RESOLVED in Desktop; see M3 | `settings-service.js:468-486`, `restoreKey` `:422-430`; specs `agent-settings-service.spec.js:528`, `:549`, `:561`, `:571` |
| L1 | RESOLVED | `settings_probe.go:55-58`, `:64` |
| L2 | RESOLVED | `settings_probe.go:152`; tests `settings_probe_test.go:395-409`, `settings_keymove_test.go:284-285` |
| L3 | RESOLVED | `host-confirm.js:22-34,69`; `native-dialog.js:22-35`; `search/service.js:174-175`; `en.json:458-461` |
| L4 | RESOLVED | `v0.44.md:33`, `:49` (nit: identical paragraphs) |
| L5 | RESOLVED | `mcp-sidecar.js:164-173` (nit: omits the vector store) |
| L6 | PARTIALLY | in memory fixed (`settings.go:624`, `:637`, `:646-659`); on disk the overlay with the new `base_url` and `api_key: null` is saved before apply (`:597-611`) and not rolled back on failure (`:619-623`), contrary to `:438-440` and `:594-595`; same fix as M3 |
| L7 | OPEN | `agent-search.spec.js:185-187` asserts the 404 leg only in a comment; `desktop-search-sidecar/tasks.md:13` still says "asserts all three"; kubo coverage exists (`index_routes_test.go:147`, `test/cli/mcp_bulk_test.go:318`). Amend the Acceptance line or add a check |
| Nits (defaults test, Trim, uarcheck) | RESOLVED | `mcp_defaults_test.go:93`; `index_bulk.go:453-456`; `uarcheck.go:192` |
| Nit (Desktop `embeddings` block vs schema) | OPEN (nit) | no schema validation spec seen by the reviewer (the Desktop builder reported one in `agent-search.spec.js` via ajv) |

## New findings

- **M3 MEDIUM (kubo plus Desktop): after a failed apply, Desktop's key goes to the new host.** The server persists the overlay with the new `base_url` before apply (`settings.go:597-611`) with no rollback on failure (`:619-623`); Desktop treats the 500 as a refusal (`settings-service.js:465`; spec `:549-558`), restores the key and restarts the sidecar, which loads the new host from the overlay and the restored key by env reference (`:461-462`). Breaks ADR-0012 B as amended (`0012-model-key-scope.md:78`). Narrow trigger. Fix (kubo, primary): on a failed apply restore the previous overlay (`st.overlay.save(st.dir)`) and the previous key file if overwritten; extend `TestSettingsFailedApplyKeepsStoredKey` to assert `LoadSettingsOverlay(dir)` keeps the old `base_url` and no `api_key: null`. Defense in depth (Desktop): after the restore restart, keep the key only if the backend origin equals the pre-move origin, or restore only on 4xx.
- **L8 LOW (OpenSpec):** no requirement or scenario for ADR-0012 behaviour in `openspec/changes/mcp-agent-settings-api/specs/mcp-agent-settings-api/spec.md`; task 1.8 says "stays as configured", not "follows". Add a requirement with three scenarios and amend 1.8's text.
- **L9 LOW (docs):** `embeddings.insecure_http_hosts` not in the v0.44 Highlights; add one sentence (default `[]` is the off switch; listed hosts still need `egress_ack`).
- **Nits:** `0012-model-key-scope.md:83` "Open points … before this is accepted" under an Accepted ADR; `embeddings_insecure_http_test.go:51-52` subtest name vs content, `:102` `context.Background()` (use `t.Context()`), no malformed-entry case; `TestProbeKeyReadsKeyAndBackendTogether` (`settings_keymove_test.go:205-242`) uses a 50 ms `time.After` (fails safe; `testing/synctest` preferred).
- **FYI:** moves to a loopback port covered by `egress_ack: ["local"]` need no confirmation, so a referenced key follows to any loopback listener without a prompt (within the accepted residual).

## `insecure_http_hosts`

Contract `ipfs-mcp-config.schema.json:1096-1108` (pattern, `uniqueItems`, default `[]`, off switch) enforced by `ParseConfig` (`config.go:157-165`); example `examples/vector-search-in-cluster.json:27-30`. Server `insecureHTTPListed` (`embeddings/settings.go:245-267`) applied to keyed embedder (`:189`), extractor (`extractor.go:171`), describer (`describer.go:129`); wired `embeddings.go:45,66,76`, `principals.go:126-129`; proxy path unchanged. Tests `embeddings_insecure_http_test.go:37-130`. Acceptance: `vector-search-contract` 1.10 PASS; `mcp-content-extractor` 1.8 PASS; 1.9 correctly unbuilt; `lke-vector-search` 1.4 consistent.

## Changelog, compatibility, style

`v0.44.md` content only in TOC and Highlights; no `/api/v0`, gateway, CID recipe, `core/`, `client/rpc/` or wire change seen (confirm with `git diff --stat`); contract additive; Go style followed.

## `TestSettingsFailedApplyKeepsStoredKey`

White-box failure forcing via `agentExt = nil` and corrupted `rawIn` (`settings_keymove_test.go:184-189`): acceptable (isolated, fails loudly) but brittle; an injectable apply seam would be cleaner. It checks only in-memory state, never the persisted overlay — exactly where M3 and L6 live.

## Phase gate must run

1. `make -O test_go_lint`. 2. `make test_go_fmt`. 3. `make build`, then `go test ./test/cli/... -run 'TestMCPStockDefaults|TestMCPBulk|TestMCPIndexSearchRouteDisabled|TestDefaultMatchesExpectedProfile' -v`. 4. `go test ./mcp/server/... ./mcp/contract/... ./cmd/ipfs/kubo/` after the M3/L6 fix. 5. `go test -tags noagent ./mcp/server/...`. 6. `go test -race ./mcp/server/...` (tier 3). 7. Desktop specs under the shim plus `agent-confirm.spec.js` under Electron/Playwright (tier 3). 8. `config_reference.py --check`. 9. `prometheus-ui-review` of the Desktop Search window and dialog copy. 10. Gateway-conformance CI and `git diff --stat`.
