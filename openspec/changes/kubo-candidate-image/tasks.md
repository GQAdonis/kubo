# Tasks: kubo-candidate-image

Plan reference: `.kbd-orchestrator/phases/agent-backend-settings/plan.md` (plan revision 3), section 10 TASK MODEL ASSIGNMENTS, phase path `agent-backend-settings`, change ID `kubo-candidate-image`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites come from those rows only.
Change-level prerequisites (plan revision 3): every task in this change also requires completion of: mcp-agent-hot-reload, mcp-agui-a2ui-registry, mcp-agent-settings-api, agent-features-default-on, settings-security-review. Scheduled in Round 8.

## 1. Tasks

- [x] 1.1 Pre-image verification on the branch head (exception 5): `make build`, `go test ./mcp/...`, `TestDefaultMatchesExpectedProfile`, `test/cli/gateway_test.go`, `TestTelemetry`. Acceptance: outputs saved in a new dated evidence folder; all pass.
- [x] 1.2 Build a linux/amd64 image from the branch head with the repository `Dockerfile` (`docker buildx build --platform linux/amd64 --load`), tagged with the short commit in the registry named at OG-1. Acceptance: `docker image inspect` shows `linux/amd64` and the tag; build log saved.
- [x] 1.3 Container smoke with its own `IPFS_PATH` on non-default ports. Acceptance: `ipfs version --commit` equals the branch commit; `ipfs mcp --help` lists `--settings-dir`; with the principal tokens and the LLM key file absent, `ipfs mcp --http --agui` starts, `/agui` answers 503 `backend_not_configured` and the settings route answers 404 (CRITICAL-2 disabled state); image digest recorded.
- [x] 1.4 Image-only know-me-cluster PR (the `image:` line set to the digest from 1.3; no flag or config change) and the OG-12 runbook (planned single-replica restart under the 2026-10-13 gotcha; rollback image 0.44.0-dev-0f15c24). Acceptance: `git diff` touches only the `image:` line(s); runbook in the PR body.
- [x] 1.5 Security-reviewer read of the image PR before OG-12. Acceptance: findings file; no open CRITICAL.

## Plan revision 4 amendment (OG-16, 2026-10-14)

- Task 1.3 adds two container smoke checks: with no `embeddings` section, `POST /index/search` answers 404 and no `ipfs_index_bulk_*` tool is listed; with an `embeddings` section whose store, extractor and embedder are unreachable, the container still starts and `GET /index/status` reports those stages unavailable (ADR-0009 absent-dependency rule). Change-level dependencies do not grow (O-10).

## Amendment (2026-10-16, lead ruling): task 1.3 disabled-state codes

The 1.3 acceptance text above is wrong for the no-principal case; the server behavior is correct (an absent secret disables the principal and fails closed; `mcp/server/agui.go:145-149` requires an authenticated `ui` principal before the backend check at `agui.go:150-153`). The corrected expectations, which supersede the 503 wording in 1.3:

- With the principal tokens absent (or a `ui` principal configured but its credential env var unset), `/agui` refuses every request with 403.
- With a throwaway `ui` token, `/agui` answers 503 `backend_not_configured`.
- The settings route answers 404 (it is not mounted without `EnableAgentSettings`; `mcp/server/settings.go:165-168`).

Second note (2026-10-16, lead ruling): the unreachable-dependencies smoke uses a `ui` principal with `egress_ack: ["local"]`. The store, embedder and extractor stages then all report unavailable in `GET /index/status`, and `POST /index/search` answers 503 `index_unavailable`. `ipfs mcp` still starts, because startup no longer fails on unreachable search services.
- [x] 1.6 Follow-ups from the 1.5 security read (`kubo-candidate-image-security-read.md`) and the OG-12 rollout: S1 store open must not depend on the caller's context (own deadline, caller cancellation not counted as a failure); S2 waiters honor their own context during an open; S3 an exited store is reopened and status does not report it available; S4 `stages.*.reason` gives non-operator principals a fixed reason code; S5 startup fails when principals are configured but none resolved a credential; S6 stale comment in `index.go`; and the `ipfs-mcp` sidecar exits while the daemon API is not yet up (two restarts at every pod start on LKE) — wait for the API with a bounded retry instead. Acceptance: each fixed with a pinning test, or rebutted in writing.

## Amendment (2026-10-16, Round 8 review R8-05): the S5 startup rule and the 1.3 evidence

With the S5 startup rule of 1.6, `ipfs mcp --http` with configured principals whose credentials all fail to resolve now exits with an error naming the missing environment variables or files, instead of serving 403 on `/agui`. The 1.3 expectations above (and the amendment of 2026-10-16 that fixed the no-principal codes) describe a start that no longer happens for that case; a partial resolution still starts with only the principals that resolved, and no principals on loopback stays anonymous and read-only.

- Before the next image-only cluster PR, run a read-only cluster check (`kubectl --context know-me`, per the cluster-PR rule) that every configured principal's environment variable or mounted file is present in the pod spec and its Secret. A configured principal whose secret is not mounted will crash-loop the new image at start.
- The LKE sidecar runs with `--auth-token-env` and no principals, so it is unaffected today. The check matters the day principals are added to its configuration.

