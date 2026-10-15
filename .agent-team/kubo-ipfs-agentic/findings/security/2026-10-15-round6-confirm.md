# Security review: agent-backend-settings Round 6, confirmation pass

Reviewer: `security-reviewer` (Claude Opus, independent context, read-only). Date: 2026-10-15. Checks the batched correction cycle against `2026-10-15-round6.md` under the operator ruling "Let it follow, document it", and the new `embeddings.insecure_http_hosts`. Static reading; test results quoted are the lead's. Written to disk by the lead.

**Verdict: PASS WITH FINDINGS.** No CRITICAL, HIGH or MEDIUM. H1 mostly fixed under the ruling (Web UI copy residual N2); L1-L4 and I1-I4 resolved; new N1, N2, N3 LOW; I5, I6 INFO; `insecure_http_hosts` passes every criterion.

## Earlier findings

| ID | Status | Evidence |
|---|---|---|
| H1 (LOW under the ruling) | PARTIALLY | Server withholds only the API-stored key (`settings.go:527-546`); `referencedKey` (`:661-666`) names the configured key in the D4 text (`:580-585`), never echoing it (`settings_keymove_test.go:137-172`); ruling pinned for `accept_secrets:false` (`:123-132`) and the probe after a move (`:100-121`); ADR-0012 consistent (`:5`, `:28`, `:53`, `:69-79`, `:85`); docs `agentic.md:97`, `chat-setup.md:44`; Desktop `serverKeyFollows` (`settings-service.js:410-414`), `en.json:379-380`, `:398`, `native-dialog.js:34`; Web UI ConfirmHost correct (`ConfirmHost.js:49-64`, `agentSettings.json:190`, `:193`). Open: Test connection copy contradicts (N2). On a move to a host already acknowledged, only the renderer copy discloses — acceptable (admin bearer is root of trust). |
| L1 | RESOLVED | one snapshot (`settings_probe.go:145-156`, `keyAndCurrent` `:224-228`); write holds `st.mu` (`settings.go:400-401`); `TestProbeKeyReadsKeyAndBackendTogether`; move detection `backendMoved`; race detector at the gate |
| L2 | RESOLVED (static) | `host-confirm.js:22-34`, `:69`; `search/service.js:174-177`; `native-dialog.js:22-30`, `:48-52`; `en.json:456-464`; `webui/index.js:328`; `SEARCH.md:57-62`. Not run without Touch ID |
| L3 | RESOLVED | `FeaturesSection.js:6`, `:78-86`, `:200`; `chat-setup.md:90`, `config-reference.md:24`, `defaults-table.json:59` |
| L4 | RESOLVED (INFO) | `isTrustedProgram` (`mcp-sidecar-search.js:185-197`), `binary_unsafe` (`:237`), `SEARCH.md:19-22`. INFO: a user-owned symlink to a binary in a writable folder passes; also check `dirname(realpathSync(path))` |
| I1 | RESOLVED | per-window decline scopes (`host-confirm.js:45-47`, `:57-60`, `:85-88`; `webui/index.js:345`, `:319`, `:418-419`; `en.json:387`) |
| I2 | RESOLVED, introduces N1 | key held and restored on refusal (`settings-service.js:422-430`, `:468-486`) |
| I3 | RESOLVED | `mcp-sidecar.js:164-173` |
| I4 | RESOLVED | `plain-text.js:10`, `:22` |

## `embeddings.insecure_http_hosts`: PASS

Exact, case-insensitive `==` match on host or host:port, no suffix/glob/regex (`embeddings/settings.go:250-267`; tests `embeddings_insecure_http_test.go:78-86`); trailing dot, scheme-prefixed or bracketed IPv6 entries never match (fail closed). Bare host any port; host:port that port; no port means 80 (`:256-263`, tests `:70-76`). Only `http` relaxed; userinfo/query/fragment still refused (`:204-225`). `egress_ack` still enforced on all three stages with no request leaving (`:99-123`). Default `[]` refuses with a hint (`:51-60`; schema `:1106-1108`); config-file only (`principals.go:126-129`, `embeddings.go:45`, `:66`, `:76`), no settings-API path. Docs `embeddings.md:202-214` state the cost, ClusterIP without Ingress, ingress/egress NetworkPolicy, and that xberg has no auth so the policy is its only access control.

## New defects from the fixes

- **N1 LOW: Desktop key can follow a move after a failed apply.** The server saves the overlay (new `base_url`, `egress_ack`) before `ApplyAgentSettings` and on failure restores only the in-memory revision, not the overlay file (`settings.go:597-611` vs `:619-622`), contrary to its comment (`:593-596`). Desktop restores the key and restarts the sidecar (`settings-service.js:422-430`, `:481-482`), which loads the persisted overlay with the key by env (`mcp-sidecar.js:369`, `:376`), so the Desktop key reaches the new host — the case ADR-0012 B closes on a laptop (`0012-model-key-scope.md:78`). The host was confirmed by Touch ID or dialog and an apply failure is required. `TestSettingsFailedApplyKeepsStoredKey` checks in-memory state only. Fix: restore the previous overlay file on a failed apply (fixes both clients), or restore the Desktop key only on 4xx refusals.
- **N2 LOW (H1 residual): Web UI Test connection copy contradicts the ruling for a server-held key.** `OpenAiBackendForm.js:62`, `:66-69` use `agentSettings.json:136`, `:140`, `:142` ("the key does not follow a move") regardless of `meta.keyServerManaged`. Fix: server-managed variants.
- **N3 LOW: the remote store credential and vectors (and a keyless embedder's chunk text) go over plain http to any host with no opt-in.** `embeddings.md:211`, schema text; example sends a SurrealDB `user:password` Basic credential to `http://ipfs-surrealdb.ipfs.svc:8000` (`vector-search-in-cluster.json:6-11`; `embeddings.md:198`); "The cost" (`:213`) omits it; keyless embedder (`:220`). The opt-in covers three of five plain-http paths. Fix: require a listing for a store with a credential (and add the SurrealDB host to the example), or name the store credential and vectors in the cost text and schema.

## INFO

- **I5:** entries are not validated against the schema pattern on the server (`principals.go:129`, `embeddings.go:45`); malformed entries silently never match; a bare IPv6 entry like `fd00::1:8000` would match `http://[fd00::1]:8000` via `host+":"+port` (`settings.go:262`). Validate at startup.
- **I6:** `embeddings.md:214` calls the example "complete" though it has no NetworkPolicy or principal `egress_ack`; reword or link the cluster manifest.

## AGENTS.md

No CID injection; no tracking; no new hardcoded endpoint (`insecure_http_hosts` default `[]`, off switch the empty list); Web UI Search off switch findable; `/api/v0/`, gateway, CID recipe untouched.

## MCP checklist delta

MCP-01, MCP-04 PASS; RCE/SSRF safe (probe limited to the current backend; plain-http scope config-only and exact); OWASP MCP01 PASS with N1/N3 LOW residuals; MCP06, MCP07 PASS.

## Manual follow-ups

1. Mac without Touch ID: Search dialog wording and parent (L2).
2. Race detector on `settings` and `settings_probe` at the final gate.
3. Desktop: forced 500 on a confirmed move, then check what the restarted sidecar sends (N1).
4. Web UI browser check of Test connection copy for a server-held key (N2).
5. LKE: confirm the NetworkPolicy isolates xberg, the embedder and SurrealDB before enabling `insecure_http_hosts`.
