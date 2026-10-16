# lke-origin-isolation 1.11: post-sync redirect check (2026-10-16)

Operator merged Prometheus-AGS/know-me-cluster#36 ("PR #36 is merged", merge `f3af91a`) before the 1.10 security read returned. Argo app `ipfs` synced to `f3af91a`, Synced/Healthy; `HTTPRoute/ipfs-webui-legacy` NotFound.

Read-only checks from this machine (`verify.txt`, `lke-gitops-change` `verify.sh`): PASS, 0 failing.

| Check | Expected | Result |
|---|---|---|
| gateway `/ipfs/bafkqaaa` | 200 | pass |
| gateway `/webui/` | 302 to console | pass (`Location: https://console.prometheusags.ai/webui/`) |
| gateway `/api/v0/version` | 302 to console | pass |
| console `/webui/` without credentials | 401 | pass |
| console `/api/v0/version` without credentials | 401 | pass |
| listeners, routes, SecurityPolicy `ipfs-webui-auth` Accepted, StatefulSet ready, PVC Bound, DNS | pass | pass |
