# Evidence: lke-origin-isolation 1.8 (OG-6), 2026-10-14

Production checks after the console origin isolation went live on the `know-me` LKE cluster: know-me-cluster #24 (console host, basic-auth policy in git, API CORS reduced to the console origin), the fixes #25 (htpasswd Secret name), #27, #29 and #31 (console TLS on the wildcard `ipfs-tls`), and the planned OG-6 restart that armed the new CORS list.

| Check | Expected | Result | File |
|---|---|---|---|
| Console `POST /api/v0/version`, no credentials | 401 from the basic-auth policy | PASS | `unauth.txt` |
| Legacy host `ipfs.prometheusags.ai` `POST /api/v0/version`, no credentials | 401 | PASS | `unauth.txt` |
| Legacy host `GET /webui/`, no credentials | 401 | PASS | `unauth.txt` |
| Credentialed `POST`, `Origin: https://ipfs.prometheusags.ai` | 403 from kubo, body `403 - Forbidden` | PASS | `credentialed.txt` |
| Credentialed `POST`, `Origin: https://webui.ipfs.io` | 403 from kubo (stray pre-merge origin gone) | PASS | `credentialed.txt` |
| Credentialed `POST`, `Origin: https://console.prometheusags.ai` | 200 with the kubo version JSON | PASS | `credentialed.txt` |
| Credentialed `GET` console `/` | 404 (no catch-all) | PASS | `credentialed.txt` |
| Credentialed `GET` console `/ipfs/<non-WebUI CID>` | 404 (no reach to the 8080 gateway) | PASS | `credentialed.txt` |
| Live verify (`lke-gitops-change` `verify.sh`): listeners, routes, policy, workloads, external fetches | 0 failing checks | PASS | `verify-20261014T104435Z.txt` |
| Planned restart | bounded, no hang | 10:41:44Z-10:42:17Z (33 s) | `og6-pre-restart-*.txt`, `og6-restart-*.txt` |

All checks pass. Per ADR-0009 (D0) and task 1.8, **OG-0 hygiene is lifted** as of this record: the gateway origin can no longer drive the RPC API with a browser's cached basic-auth credentials.

Notes:

- The restart overlapped the Argo sync of #29 by seconds (see the phase `execution.md`); the results above were taken after both completed.
- The credentialed part was run by the operator in their own terminal; the credentials never reached the lead session or these files.
- Remaining work on the legacy host: task 1.9 (redirect PR) and 1.10-1.11.
