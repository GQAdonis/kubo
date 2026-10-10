# Proposal: mcp-tool-contract

Phase `ipfs-mcp-server`, change 1 in plan revision 1 order (plan revision 2 lists 40 changes). See `.kbd-orchestrator/phases/ipfs-mcp-server/plan.md`.

## Why

The MCP server, MCP App, A2UI catalog, skills and WebMCP package must all describe the same tools. Without one machine-readable source they drift, and annotations such as `destructiveHint` become inconsistent. The contract is the foundation every other change builds on.

## What Changes

- Add `mcp/contract/catalog.json`. It lists every in-scope tool with its group, title, description, MCP annotations, confirmation class (`none`, `write` or `destructive`), `returnsUntrustedContent`, required `/api/v0` command paths and `minKuboVersion`.
- Add a JSON Schema 2020-12 input schema and output schema per tool, with a shared error model and cursor pagination for list tools.
- Add the resource URI conventions (`ipfs://`, `ipns://`, `ui://`) and the confirmation protocol schema.
- Add derivation rules for WebMCP annotations (`readOnlyHint`, `consequentialHint`, `untrustedContentHint`, `debugging`) and A2UI component bindings.
- Add a Go validator test in the root module that fails on any inconsistent contract.
- Add `mcp/README.md`.

## Capabilities

### New Capabilities

- `mcp-tool-contract`: single machine-readable definition of every IPFS MCP tool, its schemas, annotations, confirmation class and derived WebMCP/A2UI metadata

### Modified Capabilities

None.

## Impact

New `mcp/contract/**` and `mcp/README.md`. Root `go.mod`/`go.sum` gain `github.com/google/jsonschema-go` via `go get` + `make mod_tidy` (an operator-accepted dependency, answer 2). No change to `/api/v0`.

## Plan revision 2

Tasks 1.1-1.6 are registered and unchanged. Group 2 (tasks 2.1-2.3) adds what G9-G12 need: `contentEgress`, A2A skill metadata, A2UI binding hints, AG-UI/A2A fixtures, the `ipfs mcp` config schema (principals, LLM provider, agent endpoints, confirmation resolvers, embeddings) and the `ipfs_index_*` tools. The contract stays the single emitter; the validator test covers the new files.
