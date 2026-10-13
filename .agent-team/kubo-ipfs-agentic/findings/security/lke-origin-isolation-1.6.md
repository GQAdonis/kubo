# Security review: lke-origin-isolation 1.6 (know-me-cluster `feat/ipfs-console-origin-isolation`, before OG-6)

- Reviewer: `security-reviewer` (Claude Code subagent, claude-opus-5-5, fresh context, read-only)
- Date: 2026-10-13
- Artifact: know-me-cluster branch `feat/ipfs-console-origin-isolation`, commits `e5f82d8`..`0922620` on `origin/main` (`7e517e5`); diff exported by the lead
- Saved by the lead (the reviewer has no write tool); text as returned, abbreviations `M` = `namespaces/ipfs/manifests.yaml`, `PR` = `namespaces/ipfs/PR_BODY.md`, `OI` = `namespaces/ipfs/lke-checks/origin-isolation.sh`, `WA` = `namespaces/ipfs/lke-checks/webui-auth-targeting.sh`

**Verdict: BLOCK — one CRITICAL open (CRITICAL-1).** It is in the rollback path, not the forward path, and is a two-line fix. The forward change is sound and closes the shared-origin RPC vector at kubo after the planned restart. Fix CRITICAL-1 and HIGH-1/2 and this should go to PASS WITH FINDINGS.

## Findings

### CRITICAL-1 — The documented rollback strips basic auth from the full RPC API on the public gateway host

- Evidence: `apps/ipfs.yaml:15-18` automated sync with `prune: true`, `selfHeal: true`; `M:691-719` now tracks the SecurityPolicy in git (tracking-id annotation `M:695`); `PR:110-130` rollback is a plain `git revert`, step 3 says "re-apply it immediately by hand" if pruned; `PR:53` "Any failure means immediate revert".
- Scenario: any OG-6 check fails (plausible: console DNS not yet pointed, `console-tls` not yet issued, wrong Secret name, password quoting per LOW-2). The operator reverts. Auto-sync prunes `ipfs-webui-auth` (tracked, now absent from git). `ipfs-webui` reverts to `ipfs.prometheusags.ai` with `/webui` and `/api/v0` to 5001 and no auth: internet-wide config replace, key export, pin removal, shutdown. Worse than today. The manual fix likely fails: re-applying the PR's manifest carries the tracking-id annotation, so Argo treats it as an app orphan and prunes it again on the next auto-sync.
- Fix (before merge): add `argocd.argoproj.io/sync-options: Prune=false` to the SecurityPolicy metadata (`M:693-700`); Argo honors it on the live object, so a later revert leaves the policy in place, still targeting `ipfs-webui` wherever it lives. Rewrite Rollback step 1 as a forward-fix commit that restores the old routes and CORS but keeps the SecurityPolicy in git (targetRefs `ipfs-webui`), not a plain revert. In step 3, state that any hand re-apply must strip `tracking-id` (or be done with Prune=false set).

### HIGH-1 — Post-merge checks never verify auth on the legacy host

- Evidence: `OI:45-48` `BASE_URL`/`API_URL` are console-only; `OI:77-88` unauthenticated check runs only against the console.
- Scenario: after adoption the live policy fails to cover `ipfs-webui-legacy` (EG resolving a live/git spec mismatch differently, a leftover singular `targetRef`, a sectioned target). Gateway-host `/api/v0` is unauthenticated on the public internet and every OI check is green.
- Fix: add unauthenticated `POST https://ipfs.prometheusags.ai/api/v0/version` and `GET https://ipfs.prometheusags.ai/webui/` expecting 401. Add console `/` and console `/ipfs/<non-WebUI CID>` expecting 404 (unauth 401; credentialed 404), which proves no catch-all and no 8080 reach. Add a credentialed POST with `Origin: https://webui.ipfs.io` expecting 403 (see LOW-6).

### HIGH-2 — Pre-merge preconditions are only a YAML comment, not in the OG-6 runbook

- Evidence: `M:704-709` the Secret name `ipfs-webui-auth` is a guess flagged "OPERATOR: confirm"; `PR` never mentions DNS, Certificate readiness or the Secret name (grep `DNS|Secret|OG-0` hits only `PR:18`); the plan says the live spec and Secret name "come from the operator" and adoption is unverified (`plan.md:663`).
- Scenario: wrong Secret name → EG marks the policy invalid. I believe EG v1.9.1 then fails closed with 500 (unverified); either way OG-6 fails → revert → CRITICAL-1. If EG instead drops the filter, both hosts serve RPC with no auth.
- Fix: add a "Before merge" checklist to `PR` (none of these print secret data): live SecurityPolicy name and spec equal git (`kubectl -n ipfs get securitypolicy -o yaml`); Secret exists and has key `.htpasswd` (`kubectl -n ipfs get secret <name> -o jsonpath='{.data}' | jq 'keys'`); console DNS resolves to the `ipfs-gateway-gw` LoadBalancer; `argocd app diff ipfs` shows only the intended changes. After sync and before the restart: Certificate `console-tls` Ready, and SecurityPolicy `Accepted=True` for both routes.

### MEDIUM-1 — `WA` tests path strings, not the security property

- Evidence: `WA:16-22` flags only rules whose `path.value` matches `^/api/v0(/|$)` or `^/webui(/|$)`; `WA:25-31` counts any targetRef by name as full coverage; an empty `PROTECTED` set passes.
- Scenario: a later PR adds `PathPrefix /` or `/api` to `ipfs-api:5001` on an unpolicied route, or a RegularExpression match, a no-path match (defaults to `/`), or a policy targetRef scoped by `sectionName`. `WA` passes and the full RPC is exposed. `lke-agent-settings` 1.2 reuses this check.
- Fix: assert that every HTTPRoute rule with a backendRef to Service `ipfs-api` (any port), on any match, belongs to a route targeted without `sectionName` by a SecurityPolicy with `basicAuth`; fail when no `ipfs-api` rule exists. Also assert the StatefulSet args (`M:53-56`) do not contain `--unrestricted-api`, which would make console `/ipfs` serve arbitrary CIDs (`cmd/ipfs/kubo/daemon.go:874-878`).

### MEDIUM-2 — Console `/ipfs` serves about 70 historic Web UI builds on a public, credentialed origin

- Evidence: `M:637-649` sends all console `/ipfs` to 5001; 5001 serves every entry in `core/corehttp/webui.go:24-97`, back to 2016-era `Qm...` builds (`daemon.go:875`).
- Scenario: any historic build with a DOM-XSS flaw becomes RPC takeover. The attacker gets the operator to open `https://console.prometheusags.ai/ipfs/<old-cid>/#<payload>`; the browser's cached basic-auth credentials and the allowed console origin do the rest. Upstream accepts this for `localhost:5001`; here it is an internet hostname. Other ways onto the console origin are closed: `/api/v0` GET/HEAD is 405 (go-ipfs-cmds v0.16.1 `http/handler.go:124-133`), cross-site POST is 403, 8080 is never routed.
- Fix: narrow the console rule to `PathPrefix /ipfs/bafybeiblix77cfttmrmz5h54qacay5f4z5bgajc736ufiyoy5cus54stki`, the current `WebUIPath` (`webui.go:21`); the cost is that the OG-9 image roll must update it. Otherwise record it in `PR` as accepted residual risk.

### MEDIUM-3 — `PR` does not keep D0/OG-0 hygiene in force

- Evidence: ADR-0009 (`docs/mcp/0009-runtime-settings-and-hot-reload.md:42-44`) lifts hygiene only after all three OG-6 checks pass and 1.8 records them; `PR:104-108` does not say hygiene continues. `PR:14-15` and `PR:35-47` honestly state that the legacy route keeps `/webui` and `/api/v0` on the gateway host until 1.9 and that the old-host Web UI breaks.
- Missing: until the restart, the old CORS list (including the gateway origin) is live and the original exposure is unchanged; after the restart, the gateway host still accepts credentialed RPC from non-browser clients.
- Fix: state both, plus "OG-0 hygiene remains in force until 1.8 records all checks passing".

### MEDIUM-4 (pre-existing, outside the diff, not blocking) — RPC reachable unauthenticated from any pod

- Evidence: `M:273-274` says "secured by network policy", but the ipfs namespace has no NetworkPolicy (the repo's only one is in `namespaces/onyx/helm-onyx.yaml`); `ipfs-api` (`M:323-345`) and `ipfs-headless` (`M:411-414`) expose 5001 in-cluster.
- Scenario: any compromised pod in the cluster gets the full Kubo RPC without basic auth.
- Fix: a separate change adding a NetworkPolicy that allows 5001 only from the Envoy data-plane namespace and the pod itself.

### LOW-1 — The rollback command fails on a merge commit

- `PR:118` `git revert <merge-sha>` errors on a true merge commit without `-m 1`, mid-incident. State `git revert -m 1 <merge-sha>` (or revert the squash commit). Moot once CRITICAL-1 replaces the revert with a forward fix.

### LOW-2 — Password quoting in the credentialed check can cause a false failure

- `OI:73` `printf 'user = "%s:%s"\n'`: a `"` or `\` in the password breaks curl's config parsing; checks 2 and 3 return 401 and report FAIL, which triggers a revert and CRITICAL-1. Escape `\` and `"` first. Also `CONSOLE_BASE_URL` (`OI:45`) can send credentials to any host or plain HTTP: require `https://` and the console host, or drop the override. Fine as is: credentials only on stdin, `printf` is a builtin, no `-L`, `set +x`, no echo.

### LOW-3 — Check 2 proves the status code, not who refused

- `PR:101` claims the body proves kubo refused; `OI:103-110` checks only the status. Also require the body to contain `403 - Forbidden` (kubo's message, `handler.go:137`).

### LOW-4 — "Exactly one origin" is inaccurate

- `PR:22-27` and the render text say exactly one origin. Kubo always appends six localhost/127.0.0.1/[::1] origins on port 5001 and two IPFS Companion extension origins (`core/corehttp/commands.go:39-51`, `:90-93`), all with credentialed CORS. Low risk (they need a local node on 5001 or the Companion extension). Reword to "one configured origin" and name the built-in additions.

### LOW-5 — Stale gateway config from the shared-origin era

- `M:282-287` `Gateway.PublicGateways` still lists `/api` and `/webui`, and the `ExposeRoutingAPI` comment says "required for webui". Harmless: 8080 has no RPC or Web UI handler (`daemon.go:1112-1126`), so the catch-all `/` (`M:587-596`) and any encoded or case-variant path that slips past Envoy's prefix match reach only the gateway. Clean up in the redirect PR (1.9).

### LOW-6 — Possible live/git drift

- ADR-0009:38 records the live CORS list as `webui.ipfs.io` and `localhost:3000`; the git pre-image (hunk at `M:275`) had `ipfs.prometheusags.ai` and `127.0.0.1:5001`. The init container runs only on pod creation, so hand edits survive container restarts. `kubectl rollout restart` recreates the pod and `ipfs config --json` replaces the whole key, so the stray origins go. Run `argocd app diff ipfs` before merge so the sync carries no other drift (for example a StatefulSet template change would restart the pod at merge time instead of in the planned window). Add the `webui.ipfs.io` 403 check from HIGH-1.

## Answers to the brief

1. No unauthenticated `/api/v0` or `/webui` in the forward state: both live only on `ipfs-webui` and `ipfs-webui-legacy`, both targeted; rules match all methods; Gateway API prefix matching is per segment, so `/api/v0x` and `/API/v0` fall to `/` → 8080, which has no RPC; the console has no catch-all; both listeners are 443 with exact hostnames, and there is no wildcard or HTTP listener in the repo. The exposure is in the rollback path (CRITICAL-1).
2. Console `/ipfs` goes to 5001 only (`M:640-649`); no console rule reaches 8080. Residual risk: MEDIUM-2.
3. The legacy route keeps the original routing and `PR` says so. After the restart kubo's Origin check closes the browser vector (same-origin POSTs carry `Origin`; `Origin: null` and an Origin-less POST from a browser user agent are refused, `config.go:96-193`; GET is 405). The D0 statement is missing (MEDIUM-3).
4. CORS: the configured origin is correct, credentials kept, no wildcard, effective only at pod start (`PR` says so). Built-in extra origins: LOW-4.
5. Secrets: names only in the diff; no secret values or private keys. Certificate `console-tls` uses `letsencrypt-prod` (DNS-01 via Cloudflare, `cluster/manifests.yaml:59-74`), matching the `argocd-config` Certificates.
6. Argo adoption: same name, applied as a patch with no `Replace=true`, so the forward sync does not delete the live policy; sync-wave -1 means a failed policy apply stops the route changes (fails closed). Prune on revert: CRITICAL-1.
7. Restart and runbook: no `lifecycle:` key added; the existing postStart detaches its I/O (`M:64-72`); the runbook forbids force-deleting a pod stuck in PodInitializing. Revert command: LOW-1.
8. Check scripts: read-only, fail closed (`set -euo pipefail`, curl errors are failures), no credentials printed. Gaps: HIGH-1, MEDIUM-1, LOW-2, LOW-3.
9. Other: MEDIUM-4 (pre-existing, no NetworkPolicy).

MCP baseline controls and RCE vectors do not apply: the artifact is deployment configuration; the `/mcp` route and the `ipfs-mcp` container are unchanged.

## Not verified (no cluster access)

The live SecurityPolicy name and spec, and the htpasswd Secret's name and key; Argo's tracking mode and adoption by name; EG v1.9.1 behavior on an invalid SecurityPolicy (fail closed with 500 or drop the filter), its path normalization and escaped-slash settings; console DNS and certificate issuance; whether the datreeio CRD schemas match EG v1.9.1; whether the deployed image uses go-ipfs-cmds v0.16.1 (taken from the fork's `go.mod:37`); browser `Origin` behavior (from the Fetch spec, not tested live).

## Status

- Open CRITICAL: 1 (CRITICAL-1). Task 1.6 stays open until the batched correction and the reviewer's confirmation.
