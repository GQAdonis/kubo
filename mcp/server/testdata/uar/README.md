# UAR settings fixtures

Synthetic fixtures for the `check-uar` unit tests. They are NOT recordings of a
published UAR release: no published release exposes the effective-settings
endpoint yet (`mcp/uar/requests/CONFORMANCE.md` item 8). The field names are the
shape `mcp/server/uarcheck.go` reads, an assumption until the UAR team documents
them. When a release publishes the endpoint, replace these with recordings and
keep the tests.

- `defaults.json`: UAR's own defaults as listed in the conformance request item 7
  (model `openai/gpt-4o`, OpenAI embedders, governance not enforced).
- `profile.json`: the shape of the planned shipped profile (local model and
  embedders, governance on, no `terminal_exec`, only `ipfs mcp` as MCP server).
