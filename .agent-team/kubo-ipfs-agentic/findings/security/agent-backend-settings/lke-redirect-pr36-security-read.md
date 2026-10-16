# Security read: know-me-cluster PR #36 (gateway-host redirect), `lke-origin-isolation` 1.10

Reviewer: `security-reviewer` (Claude Opus), read-only, separate context; no shell (read the branch files and kubo sources). Lead-written summary keeping IDs and file:line (paths in the know-me-cluster repo unless stated). The operator merged the PR ("PR #36 is merged", `f3af91a`) before this read returned.

**Verdict: PASS WITH FINDINGS; fix F1 before merge.** In steady state no path on either host reaches `ipfs-api:5001` without basic auth; the gap is the sync order.

## Holds

- Exposure: no gateway-host rule backs onto `ipfs-api` (`manifests.yaml:556-613`, `811-820`); port 8080 has no RPC handler (kubo `cmd/ipfs/kubo/daemon.go:1113-1129` vs `:885`), so case, encoding and double-slash variants land on 8080 (404) or the redirect. Console host: every rule is on `ipfs-webui`, targeted whole by the SecurityPolicy (`:697-700`). Envoy Gateway defaults (normalize, merge slashes, `UnescapeAndRedirect`); no `ClientTrafficPolicy` in the repo.
- Redirect: fixed scheme and hostname (`:587-589`, `:597-599`), no open redirect, no credential in `Location`; 302 is right (not cached, no body replay).
- Rollback: a plain `git revert` restores the targetRef in sync-wave -1 before the route in wave 0; `Prune=false` (`:683`).
- AGENTS.md: ingress only; kubo `/api/v0`, gateway and CID defaults untouched.

## Findings

- **F1 HIGH: unauthenticated RPC window during sync** (`manifests.yaml:688` wave -1, `:697-700`). Argo applies the narrowed SecurityPolicy in wave -1 while the live `ipfs-webui-legacy` route (no wave annotation) survives until the prune, so `https://ipfs.prometheusags.ai/api/v0/*` could reach 5001 without auth; indefinitely if wave 0 fails. Fix: keep the legacy targetRef in this PR and drop it in a later cleanup (confirm Envoy Gateway v1.9.1 ignores a targetRef to a missing route).
- **F2 MEDIUM: stale rollback instructions** (`PR_BODY.md:214-265`, `origin-isolation.sh:234`): they describe the earlier PR's rollback; a partial restore of the legacy route without its targetRef would serve 5001 without auth. Fix: a #36-specific rollback that restores route and targetRef together; update the FAIL message.
- **F3 LOW: check script tests only canonical paths** (`origin-isolation.sh:162-165`): add `/%61pi/v0/version`, `//api/v0/version`, `/webui` without slash, `/API/v0/version` on the gateway host and an encoded variant on the console host, plus a query-string case.
- **F4 LOW: LOW-5 not done** (`manifests.yaml:284-286`): stale `/api` and `/webui` under `Gateway.PublicGateways` and the "required for webui" comment, promised in `PR_BODY.md:139-143`.
- **F5 INFO:** the redirect copies the query into `Location`; `?arg=` can carry secrets (`pin/remote/service/add`, `config`); the follow-up GET gets 405. Gateway API cannot strip the query; a direct 410 for `/api/v0` would avoid it.
- **F6 INFO:** `offline-render.txt:52,79-82,120-123` still lists the legacy route; the 1.9 acceptance re-run of the `yq` check is not in the changed files.
- Carried, unchanged: MEDIUM-4 (no NetworkPolicy; any pod reaches 5001 via `ipfs-api` or `ipfs-headless`, `manifests.yaml:323-345,392-423`); `/mcp` on the gateway host gated only by `IPFS_MCP_TOKEN` with `read_only:false` and `confirmation.modes:[]` (`:733-748`).

## Lead disposition (after the merge)

- F1: the window occurred. Envoy access logs on `envoy-ipfs-ipfs-gateway-gw-a8eb8933-5cd9c69455-6q5qj` from 09:38Z: every gateway-host `/api` or `/webui` request before the sync got 401 via `ipfs-webui-legacy` (last at 09:45:05.215Z); Argo sync 09:45:05-09:45:09Z; next request 09:45:26.111Z got 302. No gateway-host request reached 5001 without auth (no 200 on `/api` or `/webui`, no request in the gap). Recorded as a gotcha for future auth-narrowing cluster PRs.
- F2, F3, F4, F6: carried as `lke-origin-isolation` 1.12. F5: accepted; the client-migration note covers moving to the console host.
- The 1.9 `yq` re-run (`webui-auth-targeting.sh`) was run by the architect before the PR and passed (only `ipfs-webui` covered); it is not committed in the repo (F6).
