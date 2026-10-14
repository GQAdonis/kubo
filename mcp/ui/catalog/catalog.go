// Package catalog embeds the generated A2UI v0.9 catalog (ipfs-mcp.catalog.json) so the Go
// server can serve it at the contract route without a JavaScript toolchain in `make build`.
//
// ipfs-mcp.catalog.json is committed; `pnpm gen:catalog` (scripts/gen-catalog.mjs) regenerates
// it from the component Zod schemas and `pnpm gen:check` proves it matches the TypeScript
// sources. This package registers nothing: route registration and the run-stream advertisement
// live in mcp/server per the contract.
package catalog

import _ "embed"

// catalogJSON is the generated catalog document, embedded unchanged. It is the only copy the
// Go build sees: no duplication of the JSON anywhere else.
//
//go:embed ipfs-mcp.catalog.json
var catalogJSON []byte

// Bytes returns the embedded catalog document.
func Bytes() []byte { return catalogJSON }
