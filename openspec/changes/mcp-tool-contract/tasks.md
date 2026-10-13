# Tasks: mcp-tool-contract

Plan reference: `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`, section TASK MODEL ASSIGNMENTS (plan revision 2), phase path `ipfs-mcp-server`, change ID `mcp-tool-contract`, backend task IDs 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 2.1, 2.2, 2.3. Route, model and prerequisites (every task requires PR-1 unless this is `team-ownership-grant`) come from those rows only.
Change-level prerequisites (plan revision 2): every task in this change also requires completion of: `team-ownership-grant`. Scheduled in Round 1.

## 1. Shared tool contract

- [x] 1.1 Write `mcp/contract/catalog.json` covering every tool in the plan's CHANGE LIST items 4-12, with annotations, confirmation class, `returnsUntrustedContent`, required command paths and `minKuboVersion`. Verify: the validator in 1.5 loads it without errors
- [x] 1.2 Write the input and output schemas (2020-12) per tool, with the shared error model and cursor pagination. Verify: the 1.5 validator checks every schema against the metaschema
- [x] 1.3 Write the resource URI conventions (`ipfs://`, `ipns://`, `ui://`) and the confirmation protocol schema (elicitation request, `confirmation_required` result, confirm token fields). Verify: the schemas validate, and an example of each is included and validates
- [x] 1.4 Write the WebMCP annotation derivation rules and the A2UI component binding references. Verify: a validator check derives the annotations for every tool and matches the expected table
- [x] 1.5 Add the Go contract validator test in the root module (`go get github.com/google/jsonschema-go` + `make mod_tidy`). Verify: `go test ./mcp/contract/...` passes, and fails when an entry is made destructive and read-only
- [x] 1.6 Write `mcp/README.md` describing the contract layout and its consumers. Verify: every file it links exists

## 2. Agentic and embedding contract (plan revision 2)

- [x] 2.1 Add agentic metadata per tool (`contentEgress`, A2A skill metadata, A2UI binding hints for the chat components) and recorded AG-UI and A2A event fixtures. Verify: the validator checks that every tool with `returnsUntrustedContent` also has `contentEgress`, and that every fixture validates
- [x] 2.2 Add the `ipfs mcp` config schema in `mcp/contract/config/`: principals (kind `human-mcp`, `ui`, `a2a` or `uar`; groups; `egress_ack`; `shell_capable`; `check`), `llm` (base URL, model, key env or file, no defaults), `agent` (`agui`, `a2a`, loop limits, origin allowlist), `confirmation` (resolvers, `--confirm=cli`, TTL), `embeddings` (mode embedded or remote, absolute `surreal` binary path, data dir, endpoint, embedder, policy), and the HTTP listener default `127.0.0.1:5050`; every key carries `default`, `offSwitch` and `ownerChange` annotations used to generate the config reference. Verify: example configs validate; a literal secret, a relative binary path, a reserved default port and a key without `default` or `offSwitch` each fail validation
- [x] 2.3 Add contract entries for `ipfs_index_search`, `ipfs_index_cid`, `ipfs_index_status` and `ipfs_index_reconcile` with schemas and confirmation classes. Verify: the validator passes and the derived WebMCP annotations are computed for them
