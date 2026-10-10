# Tasks: mcp-embeddings-surrealdb

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md` (plan revision 2), section TASK MODEL ASSIGNMENTS, phase path `ipfs-mcp-server`, change ID `mcp-embeddings-surrealdb`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5. Route, model and prerequisites (every task requires PR-1) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `mcp-agent-principal-enforcement`, `mcp-llm-proxy-loop`, `mcp-tool-contract`. Scheduled in Round 7.

## 1. SurrealDB store and embedder

- [x] 1.1 Verify the SurrealDB 3.x APIs, HNSW DDL, KNN syntax and `surreal start` flags from primary docs, and build-probe `surrealdb.go` v1.7.0 with `CGO_ENABLED=0 make build` and the binary delta. Verify: `mcp/server/embeddings/SOURCES.md` cites every query; the probe result and the client decision are recorded (G12-1)
- [x] 1.2 Implement the store interface, schema bootstrap with the HNSW index, and remote mode. Verify: unit tests against recorded responses; `go list -m all` shows no new module unless the operator approved `surrealdb.go`
- [x] 1.3 Implement embedded mode: absolute binary path, version check, `surrealkv` dir, loopback ephemeral port, 0600 credentials, stop on exit, crash cleanup. Verify: unit tests refuse a relative path and an old version
- [x] 1.4 Implement the OpenAI-compatible `/embeddings` client with config, `egress_ack` check and no default. Verify: unit tests against recorded fixtures and refusal for an unacknowledged host
- [x] 1.5 Add test/cli per-mode acceptance tests with the real `surreal` 3.3.0 binary on free ports: disabled (no process, no connection), embedded (lifecycle, kill cleanup, loopback bind, insert and KNN), remote (separately started server, credentials from env); and write the embeddings modes section of `docs/mcp`. Verify: `make build && go test ./test/cli/... -run TestMCPEmbeddingsStore -v` passes
